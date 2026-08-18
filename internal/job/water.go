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

// Fractions of each probe's own range. Between them, doing nothing is correct.
const (
	Thirsty = 0.25
	Soaked  = 0.60
)

// SettleWindow is how long to wait before checking whether water arrived.
const SettleWindow = 45 * time.Minute

// Water runs the LetPot line and checks that it worked. One pump, one line, no
// zones, so every decision is about the line as a whole.
type Water struct {
	Store    *store.Store
	HA       *ha.Client
	Log      *slog.Logger
	Notifier string

	PumpSwitch string
	PumpSensor string
	RunFor     time.Duration
}

// Run decides whether to water the line, does it, and verifies the result.
func (w Water) Run(ctx context.Context) error {
	onLine, err := w.Store.ListPlants(ctx, store.PlantFilter{
		Status:         plant.StatusAlive,
		WateringMethod: plant.WateringLetPot,
	})
	if err != nil {
		return err
	}

	// Nothing on the line is a garden that waters by hand, not a fault. Only a
	// line with plants on it and no pump behind it is broken, and that has to
	// stay loud: it is watering silently not happening.
	if len(onLine) == 0 {
		return nil
	}
	if w.PumpSwitch == "" {
		return errors.New("plants are on the LetPot line but no pump switch is configured")
	}

	thirsty, soaked, blind := w.survey(ctx, onLine)

	// A plant with no calibrated probe is not evidence either way, and running
	// a pump over an unknown is exactly the mistake this project exists to stop.
	if len(blind) > 0 {
		w.Log.Info("not watering: uncalibrated plants on the line", "plants", blind)
		return nil
	}
	if len(thirsty) == 0 {
		w.Log.Info("not watering: nothing on the line is dry")
		return nil
	}

	// One pump waters everything, so a single soaked plant vetoes the run. This
	// is the group-by-thirst rule enforced rather than merely written down.
	if len(soaked) > 0 {
		return w.reportConflict(ctx, thirsty, soaked)
	}
	return w.runLine(ctx, thirsty)
}

func (w Water) survey(ctx context.Context, onLine []plant.Plant) (thirsty, soaked, blind []string) {
	for _, p := range onLine {
		links, err := w.Store.SensorLinks(ctx, &p.ID)
		if err != nil {
			blind = append(blind, p.CommonName)
			continue
		}

		var seen bool
		for _, link := range links {
			if link.Role != plant.RoleSoilMoisture || !link.Calibrated() {
				continue
			}
			latest, err := w.Store.LatestReading(ctx, link.ID)
			if err != nil {
				continue
			}
			fraction, err := link.Fraction(latest.Value)
			if err != nil {
				continue
			}

			seen = true
			switch {
			case fraction <= Thirsty:
				thirsty = append(thirsty, p.CommonName)
			case fraction >= Soaked:
				soaked = append(soaked, p.CommonName)
			}
		}
		if !seen {
			blind = append(blind, p.CommonName)
		}
	}
	return thirsty, soaked, blind
}

// reportConflict is what a mixed line looks like: the pump cannot help one
// plant without hurting another, and only moving a dripper fixes it.
func (w Water) reportConflict(ctx context.Context, thirsty, soaked []string) error {
	message := fmt.Sprintf(
		"%s needs water, but %s on the same line is already wet.\n\n"+
			"One pump waters everything, so running it would drown the wet one. "+
			"Water the dry one by hand, and move it off the line or off the "+
			"schedule the wet one is on.",
		strings.Join(thirsty, " and "), strings.Join(soaked, " and "))

	w.Log.Warn("watering conflict on the line", "thirsty", thirsty, "soaked", soaked)
	return w.HA.Notify(ctx, w.Notifier, "The LetPot line is mismatched", message, nil)
}

func (w Water) runLine(ctx context.Context, thirsty []string) error {
	runFor := w.RunFor
	if runFor <= 0 {
		runFor = 2 * time.Minute
	}

	started := time.Now().UTC()
	if err := w.HA.CallService(ctx, "switch", "turn_on",
		map[string]any{"entity_id": w.PumpSwitch}); err != nil {
		return fmt.Errorf("start pump: %w", err)
	}
	w.Log.Info("pump on", "for", runFor, "thirsty", thirsty)

	// The stop is deferred so a cancelled context or a panic still closes the
	// valve. A duration this code is holding cannot be reset by a restart the
	// way a Home Assistant `for:` trigger can.
	defer func() {
		stop, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		if err := w.HA.CallService(stop, "switch", "turn_off",
			map[string]any{"entity_id": w.PumpSwitch}); err != nil {
			w.Log.Error("PUMP DID NOT STOP", "entity", w.PumpSwitch, "error", err)
		}
	}()

	select {
	case <-time.After(runFor):
	case <-ctx.Done():
		return ctx.Err()
	}

	return w.verify(ctx, started)
}

// verify checks that water reached the soil. When the pump ran and the soil did
// not change, that dripper is clogged, and nothing else would ever catch it.
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
	return w.HA.Notify(ctx, w.Notifier, "Water is not reaching the soil", message, nil)
}
