package job

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"syscall"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/ha"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

type ingestStore interface {
	SensorLinks(context.Context, *uuid.UUID) ([]plant.SensorLink, error)
	RecordReading(context.Context, plant.Reading) error
	MoistureRoseAfter(context.Context, uuid.UUID, time.Time, time.Duration) (bool, error)
}

type ingestHomeAssistant interface {
	States(context.Context) ([]ha.State, error)
}

// Ingest pulls the current value of every linked Home Assistant entity.
type Ingest struct {
	Store ingestStore
	HA    ingestHomeAssistant
	Log   *slog.Logger
}

// Run records one sample per linked sensor.
func (i Ingest) Run(ctx context.Context) error {
	links, err := i.Store.SensorLinks(ctx, nil)
	if err != nil {
		return fmt.Errorf("sensor links: %w", err)
	}
	if len(links) == 0 {
		return nil
	}

	states, err := i.readStates(ctx)
	if err != nil {
		return fmt.Errorf("ha states: %w", err)
	}
	byID := make(map[string]ha.State, len(states))
	for _, s := range states {
		byID[s.EntityID] = s
	}

	var stored, skipped int
	for _, link := range links {
		state, ok := byID[link.HAEntityID]
		if !ok {
			i.Log.Warn("linked entity missing from home assistant", "entity", link.HAEntityID)
			skipped++
			continue
		}

		value, err := state.Float()
		if err != nil {
			// unavailable and unknown are normal for battery sensors; not an error.
			skipped++
			continue
		}

		if err := i.Store.RecordReading(ctx, plant.Reading{
			SensorLinkID: link.ID,
			Value:        value,
			Unit:         state.Unit(),
			TakenAt:      time.Now().UTC(),
		}); err != nil {
			return fmt.Errorf("record %s: %w", link.HAEntityID, err)
		}
		stored++
	}

	i.Log.Info("ingest complete", "stored", stored, "skipped", skipped)
	return nil
}

func (i Ingest) readStates(ctx context.Context) (states []ha.State, err error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	ctx, span := otel.Tracer("planty/jobs").Start(ctx, "planty.ingest.states")
	attempts := 0
	defer func() {
		span.SetAttributes(attribute.Int("ha.attempts", attempts))
		if err != nil {
			span.SetStatus(codes.Error, "read states failed")
			span.SetAttributes(attribute.String("error.type", fmt.Sprintf("%T", err)))
		}
		span.End()
	}()

	delay := 5 * time.Second
	var lastErr error
	for {
		if err := ctx.Err(); err != nil {
			return nil, errors.Join(err, lastErr)
		}
		attempts++
		states, err = i.HA.States(ctx)
		if err == nil {
			return states, nil
		}
		if ctx.Err() != nil {
			return nil, errors.Join(ctx.Err(), err, lastErr)
		}
		// A restarting listener can outlast Kubernetes' short job retry window.
		// Retry only this read, before any readings are written.
		if !errors.Is(err, syscall.ECONNREFUSED) {
			return nil, err
		}
		lastErr = err
		span.AddEvent("connection refused; retrying states")
		i.Log.WarnContext(ctx, "home assistant connection refused; retrying states", "attempt", attempts, "retry_in", delay)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, errors.Join(ctx.Err(), lastErr)
		case <-timer.C:
		}
		delay = min(delay*2, 20*time.Second)
	}
}

// WateringWindow is how long water has to show up in the soil before the claim
// is treated as unverified.
const WateringWindow = 3 * time.Hour

// VerifyWatering reports whether moisture rose after a claimed watering.
func (i Ingest) VerifyWatering(ctx context.Context, p plant.Plant, claimedAt time.Time) (bool, error) {
	links, err := i.Store.SensorLinks(ctx, &p.ID)
	if err != nil {
		return false, err
	}
	measured := false
	for _, link := range links {
		if link.Role != plant.RoleSoilMoisture || !link.Calibrated() {
			continue
		}
		rose, err := i.Store.MoistureRoseAfter(ctx, link.ID, claimedAt, WateringWindow)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return false, err
		}
		measured = true
		if rose {
			return true, nil
		}
	}
	if measured {
		return false, nil
	}
	// ErrNotFound: cannot-tell must not read as did-not-work.
	return false, store.ErrNotFound
}
