package job

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/ha"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

const (
	Thirsty = 0.25
	Soaked  = 0.60
)

const SettleWindow = 45 * time.Minute
const MaxWateringReadingAge = 45 * time.Minute

type Water struct {
	Store         *store.Store
	HA            *ha.Client
	Log           *slog.Logger
	Notifications Notifier

	PumpSwitch string
	PumpSensor string
	RunFor     time.Duration
}

func (w Water) Run(ctx context.Context) error {
	onLine, err := w.Store.ListPlants(ctx, store.PlantFilter{
		Status:         plant.StatusAlive,
		WateringMethod: plant.WateringLetPot,
	})
	if err != nil {
		return err
	}

	if len(onLine) == 0 {
		return nil
	}
	if w.PumpSwitch == "" {
		return errors.New("plants are on the LetPot line but no pump switch is configured")
	}

	thirsty, soaked, blind := w.survey(ctx, onLine)
	if len(blind) > 0 {
		w.Log.Info("not watering: uncalibrated plants on the line", "plants", blind)
		return nil
	}
	if len(thirsty) == 0 {
		w.Log.Info("not watering: nothing on the line is dry")
		return nil
	}
	if len(soaked) > 0 {
		return w.reportConflict(ctx, thirsty, soaked)
	}
	return w.runLine(ctx, thirsty)
}

func moisture(ctx context.Context, s *store.Store, p plant.Plant) (float64, bool) {
	links, err := s.SensorLinks(ctx, &p.ID)
	if err != nil {
		return 0, false
	}

	driest, heard := 1.0, false
	for _, link := range links {
		if link.Role != plant.RoleSoilMoisture || !link.Calibrated() {
			continue
		}
		latest, err := s.LatestReading(ctx, link.ID)
		if err != nil {
			continue
		}
		if !freshForWatering(latest, time.Now().UTC()) {
			continue
		}
		fraction, err := link.Fraction(latest.Value)
		if err != nil {
			continue
		}
		if !heard || fraction < driest {
			driest = fraction
		}
		heard = true
	}
	return driest, heard
}

func freshForWatering(reading plant.Reading, now time.Time) bool {
	age := now.Sub(reading.TakenAt)
	return age >= 0 && age <= MaxWateringReadingAge
}

func (w Water) survey(ctx context.Context, onLine []plant.Plant) (thirsty, soaked, blind []string) {
	for _, p := range onLine {
		fraction, heard := moisture(ctx, w.Store, p)
		switch {
		case !heard:
			blind = append(blind, p.CommonName)
		case fraction <= Thirsty:
			thirsty = append(thirsty, p.CommonName)
		case fraction >= Soaked:
			soaked = append(soaked, p.CommonName)
		}
	}
	return thirsty, soaked, blind
}

func (w Water) reportConflict(ctx context.Context, thirsty, soaked []string) error {
	message := fmt.Sprintf(
		"%s needs water, but %s on the same line is already wet.\n\n"+
			"One pump waters everything, so running it would drown the wet one. "+
			"Water the dry one by hand, and move it off the line or off the "+
			"schedule the wet one is on.",
		strings.Join(thirsty, " and "), strings.Join(soaked, " and "))

	w.Log.Warn("watering conflict on the line", "thirsty", thirsty, "soaked", soaked)
	return notify(ctx, w.Notifications, "The LetPot line is mismatched", message, nil)
}

func (w Water) runLine(ctx context.Context, thirsty []string) error {
	if w.RunFor <= 0 {
		return errors.New("pump run duration must be positive")
	}

	started := time.Now().UTC()
	if err := w.HA.CallService(ctx, "switch", "turn_on",
		map[string]any{"entity_id": w.PumpSwitch}); err != nil {
		return fmt.Errorf("start pump: %w", err)
	}
	w.Log.Info("pump on", "for", w.RunFor, "thirsty", thirsty)

	defer func() {
		stop, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		if err := w.HA.CallService(stop, "switch", "turn_off",
			map[string]any{"entity_id": w.PumpSwitch}); err != nil {
			w.Log.Error("PUMP DID NOT STOP", "entity", w.PumpSwitch, "error", err)
		}
	}()

	select {
	case <-time.After(w.RunFor):
	case <-ctx.Done():
		return ctx.Err()
	}

	return w.verify(ctx, started)
}

func (w Water) verify(ctx context.Context, started time.Time) error {
	onLine, err := w.Store.ListPlants(ctx, store.PlantFilter{
		Status:         plant.StatusAlive,
		WateringMethod: plant.WateringLetPot,
	})
	if err != nil {
		return err
	}

	ingest := Ingest{Store: w.Store, HA: w.HA, Log: w.Log}

	var clogged []string
	for _, p := range onLine {
		if _, err := w.Store.AddObservation(ctx, plant.Observation{
			PlantID:    p.ID,
			Kind:       plant.ObservedWatered,
			Body:       "LetPot line ran",
			OccurredAt: started,
			Source:     plant.SourceAutomation,
			Actor:      "planty",
		}); err != nil {
			w.Log.Error("could not record watering", "plant", p.Slug, "error", err)
		}

		rose, err := ingest.VerifyWatering(ctx, p, started)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			w.Log.Error("could not verify watering", "plant", p.Slug, "error", err)
			continue
		}
		if !rose {
			clogged = append(clogged, p.CommonName)
		}
	}

	if len(clogged) == 0 {
		return nil
	}
	message := fmt.Sprintf(
		"The pump ran, but the soil never got wetter for %s.\n\n"+
			"That usually means a blocked dripper or a line that popped off. "+
			"It can also mean bone dry soil shedding water down the side of the "+
			"pot without wetting the roots.",
		strings.Join(clogged, " and "))

	w.Log.Warn("pump ran but soil did not change", "plants", clogged)
	return notify(ctx, w.Notifications, "Water is not reaching the soil", message, nil)
}
