package job

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/ha"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/policy"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const policySymptomWindow = 7 * 24 * time.Hour

type PolicyRunner struct {
	Store         *store.Store
	Engine        policy.Engine
	Actuators     ActuatorControl
	Notifications Notifier
	Log           *slog.Logger
	Weather       PolicyWeather
	WeatherEntity string
}

type PolicyWeather interface {
	State(context.Context, string) (ha.State, error)
	Forecast(context.Context, string) ([]ha.Forecast, error)
}

func (r PolicyRunner) BuildInput(ctx context.Context, subject plant.Plant, trigger policy.Trigger) (policy.Input, error) {
	now := time.Now().UTC()
	input, err := BuildPolicyInput(ctx, r.Store, subject, trigger, now)
	if err != nil {
		return input, err
	}
	if r.Weather == nil || r.WeatherEntity == "" {
		return input, nil
	}
	weather, err := r.weatherFacts(ctx)
	if err != nil {
		if r.Log != nil {
			r.Log.Warn("policy weather unavailable", "entity", r.WeatherEntity, "error", err)
		}
		return input, nil
	}
	input.Weather = &weather
	return input, nil
}

func (r PolicyRunner) Preview(ctx context.Context, item policy.Policy, input policy.Input) (policy.Result, time.Duration, error) {
	input.Version = policy.InputVersion
	input.Context.Trigger = policy.TriggerPreview
	if input.Context.Now.IsZero() {
		input.Context.Now = time.Now().UTC()
	}
	return r.Engine.Evaluate(ctx, item.Source, input)
}

func (r PolicyRunner) EvaluateEnabled(ctx context.Context, subject plant.Plant, trigger policy.Trigger) ([]policy.Evaluation, error) {
	policies, err := r.Store.Policies(ctx)
	if err != nil {
		return nil, err
	}
	input, err := r.BuildInput(ctx, subject, trigger)
	if err != nil {
		return nil, err
	}
	out := []policy.Evaluation{}
	var failures []error
	for _, item := range policies {
		if !item.Enabled {
			continue
		}
		evaluation, _, err := r.Evaluate(ctx, item, input)
		if evaluation.ID != uuid.Nil {
			out = append(out, evaluation)
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", item.Name, err))
			continue
		}
	}
	return out, errors.Join(failures...)
}

func (r PolicyRunner) weatherFacts(ctx context.Context) (policy.WeatherFacts, error) {
	state, stateErr := r.Weather.State(ctx, r.WeatherEntity)
	periods, forecastErr := r.Weather.Forecast(ctx, r.WeatherEntity)
	if stateErr != nil && forecastErr != nil {
		return policy.WeatherFacts{}, errors.Join(stateErr, forecastErr)
	}
	facts := policy.WeatherFacts{}
	if stateErr == nil {
		if current, ok := state.Attributes["temperature"].(float64); ok {
			facts.CurrentTempF = &current
		}
	}
	if len(periods) > 0 {
		cutoff, earliest := time.Now().Add(36*time.Hour), time.Now().Add(-ha.StaleForecast)
		var low, high float64
		found := false
		for _, period := range periods {
			if period.DateTime.Before(earliest) || period.DateTime.After(cutoff) {
				continue
			}
			if !found {
				low, high, found = period.Low(), period.Temperature, true
				continue
			}
			low = min(low, period.Low())
			high = max(high, period.Temperature)
		}
		if found {
			facts.ForecastLowF, facts.ForecastHighF = &low, &high
			facts.FrostRisk = low <= 32
		}
	}
	return facts, nil
}

func (r PolicyRunner) Evaluate(ctx context.Context, item policy.Policy, input policy.Input) (policy.Evaluation, bool, error) {
	ctx, span := otel.Tracer("planty/policy").Start(ctx, "policy.evaluate")
	span.SetAttributes(
		attribute.String("policy.id", item.ID.String()),
		attribute.Int("policy.version", item.Version),
		attribute.String("policy.mode", string(item.Mode)),
		attribute.String("policy.trigger", string(input.Context.Trigger)),
		attribute.String("plant.id", input.Plant.ID.String()),
	)
	defer span.End()
	if input.Version != policy.InputVersion {
		err := fmt.Errorf("input version must be %q", policy.InputVersion)
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid input version")
		return policy.Evaluation{}, false, err
	}
	if input.Context.Trigger == policy.TriggerPreview {
		return policy.Evaluation{}, false, fmt.Errorf("preview input cannot be persisted or enforced")
	}
	result, duration, evalErr := r.Engine.Evaluate(ctx, item.Source, input)
	span.SetAttributes(attribute.Float64("policy.duration_ms", float64(duration.Microseconds())/1000))
	if evalErr != nil {
		span.RecordError(evalErr)
		span.SetStatus(codes.Error, "evaluation failed")
	}
	fingerprint, err := policy.FingerprintInput(input)
	if err != nil {
		return policy.Evaluation{}, false, err
	}
	evaluation := policy.Evaluation{
		PolicyID: item.ID, PolicyVersion: item.Version, PolicyMode: item.Mode, PlantID: input.Plant.ID,
		Trigger: input.Context.Trigger, InputFingerprint: fingerprint,
		IdempotencyKey:    policy.IdempotencyKey(input, fingerprint),
		PolicyFingerprint: item.Fingerprint(), Input: input, Result: result,
		DurationMS: float64(duration.Microseconds()) / 1000, Outcome: "advisory", Enforced: []string{},
	}
	if evalErr != nil {
		evaluation.Outcome = "failed"
		evaluation.Error = evalErr.Error()
	}
	saved, created, saveErr := r.Store.SavePolicyEvaluation(ctx, evaluation)
	if saveErr != nil {
		return policy.Evaluation{}, false, saveErr
	}
	if !created || evalErr != nil || item.Mode != policy.ModeEnforce {
		span.SetAttributes(attribute.String("policy.outcome", saved.Outcome))
		return saved, created, evalErr
	}

	enforced, enforceErr := r.enforce(ctx, saved)
	outcome, failure := "enforced", ""
	if enforceErr != nil {
		outcome, failure = "failed", enforceErr.Error()
	}
	if err := r.Store.FinishPolicyEvaluation(ctx, saved.ID, outcome, failure, enforced); err != nil {
		return saved, true, errors.Join(enforceErr, err)
	}
	saved.Outcome, saved.Error, saved.Enforced = outcome, failure, enforced
	span.SetAttributes(attribute.String("policy.outcome", outcome), attribute.Int("policy.enforced_count", len(enforced)))
	if enforceErr != nil {
		span.RecordError(enforceErr)
		span.SetStatus(codes.Error, "enforcement failed")
	}
	return saved, true, enforceErr
}

func (r PolicyRunner) enforce(ctx context.Context, evaluation policy.Evaluation) ([]string, error) {
	var applied []string
	var failures []error
	result := evaluation.Result
	if result.Health != nil {
		if !evaluation.Input.Health.Known || !evaluation.Input.Health.EvidenceNew {
			applied = append(applied, "health skipped: requires an existing score and newer evidence")
		} else {
			evidence := policyHealthEvidence(evaluation.Input)
			key := evaluation.ID
			_, inserted, err := r.Store.RecordHealth(ctx, plant.HealthChange{
				PlantID: evaluation.PlantID, Delta: &result.Health.Delta,
				Rationale: result.Health.Reason, Evidence: evidence,
				Source: plant.SourceAutomation, Actor: "OPA policy", IdempotencyKey: &key,
			})
			if err != nil {
				failures = append(failures, fmt.Errorf("health: %w", err))
			} else if inserted {
				applied = append(applied, fmt.Sprintf("health adjusted by %g", result.Health.Delta))
			}
		}
	}

	for _, run := range result.FanRuns {
		actuator, ok := slices.BinarySearchFunc(evaluation.Input.Actuators, run.ActuatorID,
			func(candidate policy.ActuatorFacts, id uuid.UUID) int { return bytes.Compare(candidate.ID[:], id[:]) })
		_ = actuator
		if !ok {
			failures = append(failures, fmt.Errorf("fan %s is not assigned to %s", run.ActuatorID, evaluation.Input.Plant.Slug))
			continue
		}
		facts := evaluation.Input.Actuators[actuator]
		if facts.Kind != string(plant.ActuatorFan) || !facts.PolicyControlEnabled {
			failures = append(failures, fmt.Errorf("fan %s is not opted into policy control", facts.Name))
			continue
		}
		key := uuid.NewSHA1(evaluation.ID, []byte(run.ActuatorID.String()))
		_, created, err := r.Actuators.StartForPlant(ctx, run.ActuatorID, evaluation.PlantID,
			run.DurationSeconds, "OPA policy", plant.SourceAutomation, key)
		if err != nil {
			failures = append(failures, fmt.Errorf("fan %s: %w", facts.Name, err))
		} else if created {
			applied = append(applied, fmt.Sprintf("ran fan %s for %d seconds", facts.Name, run.DurationSeconds))
		}
	}

	for _, notification := range result.Notifications {
		err := notify(ctx, r.Notifications, notification.Title, notification.Body, map[string]any{
			"policy_evaluation_id": evaluation.ID.String(),
			"plant_slug":           evaluation.Input.Plant.Slug,
		})
		if err != nil {
			failures = append(failures, fmt.Errorf("notification %q: %w", notification.Title, err))
		} else {
			applied = append(applied, fmt.Sprintf("sent notification %q", notification.Title))
		}
	}
	return applied, errors.Join(failures...)
}

func BuildPolicyInput(ctx context.Context, db *store.Store, subject plant.Plant, trigger policy.Trigger, now time.Time) (policy.Input, error) {
	input := policy.Input{
		Version: policy.InputVersion, Context: policy.Context{Trigger: trigger, Now: now},
		Plant: policy.PlantFacts{
			ID: subject.ID, Slug: subject.Slug, CommonName: subject.CommonName,
			BotanicalName: subject.BotanicalName, Domain: string(subject.Domain), Status: string(subject.Status),
			Location: subject.Location, AcquiredAt: subject.AcquiredAt,
			MinTempF: subject.MinTempF, WateringMethod: string(subject.WateringMethod),
			WantsLight: string(subject.CareProfile.WantsLight), HumidityPref: subject.CareProfile.HumidityPref,
			FrostSensitive: subject.CareProfile.FrostSensitive, Risk: subject.Risk(),
		},
		Actuators: []policy.ActuatorFacts{},
	}
	if subject.AcquiredAt != nil {
		age := int(now.Sub(*subject.AcquiredAt).Hours() / 24)
		if age < 0 {
			age = 0
		}
		input.Plant.AgeDays = &age
	}

	observations, err := db.Observations(ctx, subject.ID, 100)
	if err != nil {
		return input, err
	}
	for _, observation := range observations {
		fact := eventFact(observation, now)
		switch observation.Kind {
		case plant.ObservedWatered:
			if input.Care.LastWatered == nil {
				input.Care.LastWatered = &fact
			}
		case plant.ObservedMisted:
			if input.Care.LastMisted == nil {
				input.Care.LastMisted = &fact
			}
		case plant.ObservedAirflow:
			if input.Care.LastAirflow == nil {
				input.Care.LastAirflow = &fact
			}
		case plant.ObservedFertilized:
			if input.Care.LastFertilized == nil {
				input.Care.LastFertilized = &fact
			}
		case plant.ObservedMoved:
			if input.Care.LastMoved == nil {
				input.Care.LastMoved = &fact
			}
		case plant.ObservedSymptom:
			if input.Care.LatestSymptom == nil {
				input.Care.LatestSymptom = &fact
			}
		}
	}
	input.Plant.IsSick = subject.Status == plant.StatusStruggling ||
		(input.Care.LatestSymptom != nil && input.Care.LatestSymptom.HoursAgo <= policySymptomWindow.Hours())

	links, err := db.SensorLinks(ctx, &subject.ID)
	if err != nil {
		return input, err
	}
	for _, link := range links {
		reading, err := db.LatestReading(ctx, link.ID)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return input, err
		}
		fact := policy.SensorFact{
			ReadingID: reading.ID, Raw: reading.Value, Unit: reading.Unit, Calibrated: link.Calibrated(),
			TakenAt: reading.TakenAt, AgeMinutes: now.Sub(reading.TakenAt).Minutes(),
			Stale: now.Sub(reading.TakenAt) > plant.StaleAfter,
		}
		if fact.Calibrated {
			if fraction, err := link.Fraction(reading.Value); err == nil {
				fact.Fraction = &fraction
			}
		}
		switch link.Role {
		case plant.RoleSoilMoisture:
			input.Sensors.SoilMoisture = &fact
		case plant.RoleAmbientTemp:
			input.Sensors.AmbientTemp = &fact
		case plant.RoleAmbientHumidity:
			input.Sensors.AmbientHumidity = &fact
		case plant.RoleIlluminance:
			input.Sensors.Illuminance = &fact
		}
	}

	if health, err := db.LatestHealth(ctx, subject.ID); err == nil {
		input.Health.Known = true
		input.Health.Score = &health.Score
		input.Health.AssessedAt = &health.CreatedAt
		for _, observation := range observations {
			if observation.OccurredAt.After(health.CreatedAt) {
				input.Health.EvidenceNew = true
				break
			}
		}
		for _, sensor := range []*policy.SensorFact{input.Sensors.SoilMoisture, input.Sensors.AmbientTemp, input.Sensors.AmbientHumidity, input.Sensors.Illuminance} {
			if sensor != nil && sensor.TakenAt.After(health.CreatedAt) {
				input.Health.EvidenceNew = true
			}
		}
	} else if !errors.Is(err, store.ErrNotFound) {
		return input, err
	}
	if verdict, err := db.LatestVerdict(ctx, subject.ID); err == nil {
		input.Verdict = &policy.VerdictFacts{Action: string(verdict.Action), Reasoning: verdict.Reasoning,
			Confidence: verdict.Confidence, CreatedAt: verdict.CreatedAt}
	} else if !errors.Is(err, store.ErrNotFound) {
		return input, err
	}
	actuators, err := db.Actuators(ctx)
	if err != nil {
		return input, err
	}
	for _, actuator := range actuators {
		if !slices.Contains(actuator.PlantIDs, subject.ID) {
			continue
		}
		facts := policy.ActuatorFacts{ID: actuator.ID, Name: actuator.Name, Kind: string(actuator.Kind),
			EntityID: actuator.EntityID, PolicyControlEnabled: actuator.PolicyControlEnabled}
		if actuator.ActiveLease != nil {
			facts.ActiveUntil = &actuator.ActiveLease.Deadline
		}
		input.Actuators = append(input.Actuators, facts)
	}
	slices.SortFunc(input.Actuators, func(a, b policy.ActuatorFacts) int { return bytes.Compare(a.ID[:], b.ID[:]) })
	return input, nil
}

func eventFact(observation plant.Observation, now time.Time) policy.EventFact {
	hours := now.Sub(observation.OccurredAt).Hours()
	if hours < 0 {
		hours = 0
	}
	return policy.EventFact{At: observation.OccurredAt, HoursAgo: hours, Recent24H: hours <= 24,
		Body: observation.Body, RecordID: observation.ID}
}

func policyHealthEvidence(input policy.Input) plant.HealthEvidence {
	evidence := plant.HealthEvidence{Summary: "OPA policy evaluation"}
	for _, sensor := range []*policy.SensorFact{input.Sensors.SoilMoisture, input.Sensors.AmbientTemp, input.Sensors.AmbientHumidity, input.Sensors.Illuminance} {
		if sensor != nil {
			evidence.ReadingIDs = append(evidence.ReadingIDs, sensor.ReadingID)
		}
	}
	for _, event := range []*policy.EventFact{input.Care.LastWatered, input.Care.LastMisted, input.Care.LastAirflow,
		input.Care.LastFertilized, input.Care.LastMoved, input.Care.LatestSymptom} {
		if event != nil {
			evidence.ObservationIDs = append(evidence.ObservationIDs, event.RecordID)
		}
	}
	return evidence
}
