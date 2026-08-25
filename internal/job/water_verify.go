package job

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

type VerifyWater struct {
	Store         *store.Store
	Log           *slog.Logger
	Notifications Notifier
	SettleAfter   time.Duration
	Now           func() time.Time
}

func (v VerifyWater) Run(ctx context.Context) error {
	settleAfter := v.SettleAfter
	if settleAfter <= 0 {
		settleAfter = SettleWindow
	}
	now := time.Now().UTC()
	if v.Now != nil {
		now = v.Now().UTC()
	}
	attempts, err := v.Store.WateringAttemptsReadyForEvidence(ctx, now.Add(-settleAfter))
	if err != nil {
		return fmt.Errorf("list watering attempts: %w", err)
	}
	for _, attempt := range attempts {
		results, outcome, err := v.verifyAttempt(ctx, attempt, now)
		if err != nil {
			return fmt.Errorf("verify watering attempt %s: %w", attempt.ID, err)
		}
		if err := v.Store.FinalizeWateringAttempt(ctx, attempt.ID, outcome, results); err != nil {
			return fmt.Errorf("finalize watering attempt %s: %w", attempt.ID, err)
		}
	}
	return v.sendPendingAlerts(ctx)
}

func (v VerifyWater) verifyAttempt(ctx context.Context, attempt store.WateringAttempt, checkedAt time.Time) ([]store.WateringPlantEvidence, store.WateringAttemptOutcome, error) {
	results := make([]store.WateringPlantEvidence, 0, len(attempt.Plants))
	counts := make(map[store.WateringPlantOutcome]int)
	for _, p := range attempt.Plants {
		result := store.WateringPlantEvidence{
			PlantID: p.ID, PlantName: p.CommonName, PlantSlug: p.Slug,
			Details: map[string]any{"checked_at": checkedAt, "window_seconds": int(WateringWindow.Seconds())},
		}
		if attempt.PumpActivity == store.PumpActivityInactive {
			result.Outcome = store.WateringPlantPumpFailed
			result.Details["pump_activity"] = attempt.PumpActivity
		} else {
			outcome, evidence, err := v.verifyPlant(ctx, p, *attempt.PumpStartedAt)
			if err != nil {
				return nil, "", err
			}
			result.Outcome = outcome
			result.Details["probes"] = evidence
		}
		counts[result.Outcome]++
		results = append(results, result)
	}
	return results, wateringOutcome(counts, len(results)), nil
}

func (v VerifyWater) verifyPlant(ctx context.Context, p plant.Plant, started time.Time) (store.WateringPlantOutcome, []map[string]any, error) {
	links, err := v.Store.SensorLinks(ctx, &p.ID)
	if err != nil {
		return "", nil, err
	}
	var evidence []map[string]any
	measured, rose := false, false
	for _, link := range links {
		if link.Role != plant.RoleSoilMoisture || !link.Calibrated() {
			continue
		}
		change, err := v.Store.MoistureChangeAfter(ctx, link.ID, started, WateringWindow)
		if errors.Is(err, store.ErrNotFound) {
			evidence = append(evidence, map[string]any{"entity_id": link.HAEntityID, "status": "missing_readings"})
			continue
		}
		if err != nil {
			return "", nil, err
		}
		measured = true
		increased := change.After > change.Before
		rose = rose || increased
		evidence = append(evidence, map[string]any{
			"entity_id": link.HAEntityID,
			"before":    change.Before,
			"after":     change.After,
			"rose":      increased,
		})
	}
	switch {
	case rose:
		return store.WateringPlantVerified, evidence, nil
	case measured:
		return store.WateringPlantClogged, evidence, nil
	default:
		return store.WateringPlantSensorUnknown, evidence, nil
	}
}

func wateringOutcome(counts map[store.WateringPlantOutcome]int, total int) store.WateringAttemptOutcome {
	if total == 0 {
		return store.WateringSensorUnknown
	}
	if counts[store.WateringPlantVerified] == total {
		return store.WateringVerified
	}
	if counts[store.WateringPlantClogged] == total {
		return store.WateringClogged
	}
	if counts[store.WateringPlantSensorUnknown] == total {
		return store.WateringSensorUnknown
	}
	if counts[store.WateringPlantPumpFailed] == total {
		return store.WateringPumpFailed
	}
	return store.WateringMixed
}

func (v VerifyWater) sendPendingAlerts(ctx context.Context) error {
	attempts, err := v.Store.WateringAlertsPending(ctx)
	if err != nil {
		return fmt.Errorf("list watering alerts: %w", err)
	}
	for _, attempt := range attempts {
		title, body := wateringAlert(attempt)
		sendErr := notify(ctx, v.Notifications, title, body, map[string]any{"screen": "today"})
		if markErr := v.Store.MarkWateringAlert(ctx, attempt.ID, sendErr == nil, sendErr); markErr != nil {
			return errors.Join(sendErr, fmt.Errorf("record watering alert: %w", markErr))
		}
		if sendErr != nil {
			return sendErr
		}
	}
	return nil
}

func wateringAlert(attempt store.WateringAttempt) (string, string) {
	groups := make(map[store.WateringPlantOutcome][]string)
	for _, result := range attempt.Results {
		groups[result.Outcome] = append(groups[result.Outcome], result.PlantName)
	}
	if attempt.Outcome == store.WateringPumpFailed {
		return "The LetPot pump did not run safely", "Planty could not confirm a safe pump cycle. Check the pump, reservoir, and independent cutoff before trying again."
	}
	if attempt.Outcome == store.WateringClogged {
		return "Water is not reaching the soil", cloggedMessage(groups[store.WateringPlantClogged])
	}
	if attempt.Outcome == store.WateringSensorUnknown {
		return "Planty cannot verify the watering", unknownMessage(groups[store.WateringPlantSensorUnknown])
	}
	var sections []string
	if names := groups[store.WateringPlantClogged]; len(names) > 0 {
		sections = append(sections, cloggedMessage(names))
	}
	if names := groups[store.WateringPlantSensorUnknown]; len(names) > 0 {
		sections = append(sections, unknownMessage(names))
	}
	if names := groups[store.WateringPlantPumpFailed]; len(names) > 0 {
		sections = append(sections, "Pump activity was not detected for "+strings.Join(names, " and ")+".")
	}
	return "The LetPot watering needs attention", strings.Join(sections, "\n\n")
}

func cloggedMessage(names []string) string {
	return "The pump ran, but the soil never got wetter for " + strings.Join(names, " and ") + ". Check for a blocked dripper, a disconnected line, or water running around dry soil."
}

func unknownMessage(names []string) string {
	return "Planty did not receive enough post-watering sensor readings for " + strings.Join(names, " and ") + ". Check the probes and do not assume water reached the roots."
}
