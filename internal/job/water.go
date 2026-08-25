package job

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
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
		w.Log.Info("not watering: uncalibrated plants on the line", "plants", plantNames(blind))
		return nil
	}
	if len(thirsty) == 0 {
		w.Log.Info("not watering: nothing on the line is dry")
		return nil
	}
	if len(soaked) > 0 {
		return w.reportConflict(ctx, plantNames(thirsty), plantNames(soaked))
	}
	return w.runLine(ctx, onLine)
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

func (w Water) survey(ctx context.Context, onLine []plant.Plant) (thirsty, soaked, blind []plant.Plant) {
	for _, p := range onLine {
		fraction, heard := moisture(ctx, w.Store, p)
		switch {
		case !heard:
			blind = append(blind, p)
		case fraction <= Thirsty:
			thirsty = append(thirsty, p)
		case fraction >= Soaked:
			soaked = append(soaked, p)
		}
	}
	return thirsty, soaked, blind
}

func plantNames(plants []plant.Plant) []string {
	names := make([]string, 0, len(plants))
	for _, p := range plants {
		names = append(names, p.CommonName)
	}
	return names
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

func (w Water) runLine(ctx context.Context, onLine []plant.Plant) error {
	if w.RunFor <= 0 {
		return errors.New("pump run duration must be positive")
	}

	attempt, err := w.Store.CreateWateringAttempt(ctx, w.PumpSwitch, w.PumpSensor, w.RunFor, onLine)
	if err != nil {
		return fmt.Errorf("record watering attempt: %w", err)
	}

	if err := w.HA.CallService(ctx, "switch", "turn_on",
		map[string]any{"entity_id": w.PumpSwitch}); err != nil {
		if recordErr := w.Store.FailWateringStart(ctx, attempt.ID, err); recordErr != nil {
			return errors.Join(fmt.Errorf("start pump: %w", err), fmt.Errorf("record pump failure: %w", recordErr))
		}
		alertErr := notify(ctx, w.Notifications, "The LetPot pump did not start",
			"Planty could not start the pump. Check Home Assistant, the smart switch, and the reservoir before trying again.", map[string]any{"screen": "today"})
		if markErr := w.Store.MarkWateringAlert(ctx, attempt.ID, alertErr == nil, alertErr); markErr != nil {
			alertErr = errors.Join(alertErr, markErr)
		}
		return errors.Join(fmt.Errorf("start pump: %w", err), alertErr)
	}
	started := time.Now().UTC()
	activity := w.pumpActivity(ctx)
	if err := w.Store.MarkWateringStarted(ctx, attempt.ID, started, activity); err != nil {
		stopErr := w.stopPump(ctx)
		return errors.Join(fmt.Errorf("record pump start: %w", err), stopErr)
	}
	w.Log.Info("pump on", "for", w.RunFor, "plants", plantNames(onLine), "activity", activity)

	var runErr error
	if activity == store.PumpActivityInactive {
		runErr = errors.New("pump activity sensor stayed inactive")
	} else {
		select {
		case <-time.After(w.RunFor):
		case <-ctx.Done():
			runErr = ctx.Err()
		}
	}

	stopErr := w.stopPump(ctx)
	stopped := time.Now().UTC()
	if stopErr != nil {
		w.Log.Error("PUMP DID NOT STOP", "entity", w.PumpSwitch, "error", stopErr)
		if err := w.Store.FailWateringStop(context.WithoutCancel(ctx), attempt.ID, stopped, stopErr); err != nil {
			stopErr = errors.Join(stopErr, fmt.Errorf("record pump stop failure: %w", err))
		}
		alertErr := notify(context.WithoutCancel(ctx), w.Notifications, "The LetPot pump may still be running",
			"Planty could not confirm the pump stopped. The independent Home Assistant cutoff should stop it, but check the reservoir and line now.", map[string]any{"screen": "today"})
		if markErr := w.Store.MarkWateringAlert(context.WithoutCancel(ctx), attempt.ID, alertErr == nil, alertErr); markErr != nil {
			alertErr = errors.Join(alertErr, markErr)
		}
		return errors.Join(runErr, stopErr, alertErr)
	}
	if err := w.Store.MarkWateringStopped(context.WithoutCancel(ctx), attempt.ID, stopped, nil); err != nil {
		return errors.Join(runErr, fmt.Errorf("record pump stop: %w", err))
	}
	return runErr
}

func (w Water) stopPump(ctx context.Context) error {
	stop, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	return w.HA.CallService(stop, "switch", "turn_off", map[string]any{"entity_id": w.PumpSwitch})
}

func (w Water) pumpActivity(ctx context.Context) store.PumpActivity {
	if w.PumpSensor == "" {
		return store.PumpActivityUnknown
	}
	state, err := w.HA.State(ctx, w.PumpSensor)
	if err != nil {
		w.Log.Warn("could not inspect pump activity", "entity", w.PumpSensor, "error", err)
		return store.PumpActivityUnknown
	}
	normalized := strings.ToLower(strings.TrimSpace(state.State))
	switch normalized {
	case "on", "running", "active":
		return store.PumpActivityConfirmed
	case "off", "idle", "inactive", "unavailable", "unknown":
		return store.PumpActivityInactive
	}
	value, err := strconv.ParseFloat(normalized, 64)
	if err != nil {
		return store.PumpActivityUnknown
	}
	if value > 0 {
		return store.PumpActivityConfirmed
	}
	return store.PumpActivityInactive
}
