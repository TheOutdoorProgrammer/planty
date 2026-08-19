package job

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/ha"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

// Ingest pulls the current value of every linked Home Assistant entity.
type Ingest struct {
	Store *store.Store
	HA    *ha.Client
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

	states, err := i.HA.States(ctx)
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
