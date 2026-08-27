package job

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/ha"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/policy"
	"github.com/google/uuid"
)

type policyWeatherFake struct {
	state     ha.State
	forecasts []ha.Forecast
}

func TestSuccessfulPolicyResultsExcludeFailedEvaluations(t *testing.T) {
	advisoryID, enforcedID := uuid.New(), uuid.New()
	got := successfulPolicyResults([]policy.Evaluation{
		{ID: advisoryID, Outcome: "advisory"},
		{ID: uuid.New(), Outcome: "failed"},
		{ID: enforcedID, Outcome: "enforced"},
	})

	if len(got) != 2 || got[0].ID != advisoryID || got[1].ID != enforcedID {
		t.Fatalf("successful policy results = %#v", got)
	}
}

func TestEnforcingPolicyRunsOnlyAnOptedInFanAndDailyRetryIsIdempotent(t *testing.T) {
	db, ctx := testStore(t)
	subject := tender(t, db, ctx, "Policy fan subject", plant.StewardSelf, 50)
	actuator, err := db.RegisterActuator(ctx, plant.Actuator{
		EntityID: "fan.policy_test", Name: "Policy test fan", Kind: plant.ActuatorFan,
		PlantIDs: []uuid.UUID{subject.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	actuator, err = db.UpdateActuator(ctx, actuator.ID, actuator.Name, actuator.PlantIDs, true)
	if err != nil {
		t.Fatal(err)
	}
	source := fmt.Sprintf(`package planty.v1
fan_run := {"actuator_id": %q, "duration_seconds": 60, "reason": "High humidity."}`, actuator.ID.String())
	item, err := db.CreatePolicy(ctx, policy.Policy{
		Name: "Policy fan integration", Source: source, Mode: policy.ModeEnforce, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.ArchivePolicy(ctx, item.ID) })
	ha := &actuatorHA{}
	runner := PolicyRunner{
		Store: db, Engine: policy.Engine{},
		Actuators: ActuatorControl{Store: db, HA: ha, Log: quietLog()}, Log: quietLog(),
	}

	first, err := runner.EvaluateEnabled(ctx, subject, policy.TriggerDaily)
	if err != nil || len(first) != 1 || first[0].Outcome != "enforced" {
		t.Fatalf("first daily evaluation = %#v, %v", first, err)
	}
	second, err := runner.EvaluateEnabled(ctx, subject, policy.TriggerDaily)
	if err != nil || len(second) != 1 || second[0].ID != first[0].ID {
		t.Fatalf("replayed daily evaluation = %#v, %v", second, err)
	}
	if len(ha.calls) != 1 || ha.calls[0] != "fan/turn_on:fan.policy_test" {
		t.Fatalf("Home Assistant calls = %#v", ha.calls)
	}
}

func (f policyWeatherFake) State(context.Context, string) (ha.State, error) {
	return f.state, nil
}

func (f policyWeatherFake) Forecast(context.Context, string) ([]ha.Forecast, error) {
	return f.forecasts, nil
}

func TestPolicyWeatherUsesOnlyTheNearForecast(t *testing.T) {
	now := time.Now()
	farLow := 10.0
	nearLow := 31.0
	runner := PolicyRunner{
		WeatherEntity: "weather.home",
		Weather: policyWeatherFake{
			state: ha.State{Attributes: map[string]any{"temperature": 54.0}},
			forecasts: []ha.Forecast{
				{DateTime: now.Add(12 * time.Hour), Temperature: 60, TemplowRaw: &nearLow},
				{DateTime: now.Add(5 * 24 * time.Hour), Temperature: 50, TemplowRaw: &farLow},
			},
		},
	}

	facts, err := runner.weatherFacts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if facts.CurrentTempF == nil || *facts.CurrentTempF != 54 ||
		facts.ForecastLowF == nil || *facts.ForecastLowF != 31 || !facts.FrostRisk {
		t.Fatalf("weather facts = %#v", facts)
	}
}
