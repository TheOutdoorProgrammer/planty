package policy

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestExampleCompilesAndReturnsIndependentRules(t *testing.T) {
	fraction := 0.1
	result, _, err := (Engine{}).Evaluate(context.Background(), ExampleSource, Input{
		Version: InputVersion,
		Plant:   PlantFacts{CommonName: "Fern"},
		Sensors: SensorFacts{SoilMoisture: &SensorFact{Calibrated: true, Fraction: &fraction}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rules) != 5 || result.Rules[4].Name != "notification" {
		t.Fatalf("rules = %#v", result.Rules)
	}
	if len(result.Notifications) != 1 || len(result.Agent.Facts) != 1 || len(result.Agent.Guidance) != 1 {
		t.Fatalf("normalized result = %#v", result)
	}
}

func TestRulePresenceAndBooleanValueDetermineActivity(t *testing.T) {
	source := `package planty.v1
health := {}
incident := null
needs_misted := false
needs_water := true`
	result, _, err := (Engine{}).Evaluate(context.Background(), source, Input{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rules) != 4 {
		t.Fatalf("rules = %#v", result.Rules)
	}
	active := map[string]bool{}
	for _, rule := range result.Rules {
		active[rule.Name] = rule.Active
	}
	if !active["health"] || !active["incident"] || active["needs_misted"] || !active["needs_water"] {
		t.Fatalf("activity = %#v", active)
	}
	if _, exists := active["needs_pruned"]; exists {
		t.Fatal("undefined rule was materialized")
	}
}

func TestMissingRulesAndFalseTypedRulesHaveNoEffect(t *testing.T) {
	result, _, err := (Engine{}).Evaluate(context.Background(), `package planty.v1`, Input{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rules) != 0 {
		t.Fatalf("empty package returned %#v", result.Rules)
	}

	result, _, err = (Engine{}).Evaluate(context.Background(), `package planty.v1
fan_run := false`, Input{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rules) != 1 || result.Rules[0].Active || len(result.FanRuns) != 0 {
		t.Fatalf("false typed rule returned %#v", result)
	}
}

func TestDailyIdempotencyUsesTheUTCDayWithoutChangingTheInput(t *testing.T) {
	now := time.Date(2026, 8, 27, 23, 42, 0, 0, time.FixedZone("local", -4*60*60))
	input := Input{Context: Context{Trigger: TriggerDaily, Now: now}}

	if got := IdempotencyKey(input, "fingerprint"); got != "2026-08-28" {
		t.Fatalf("daily idempotency key = %q", got)
	}
	if !input.Context.Now.Equal(now) {
		t.Fatalf("idempotency changed the input clock to %s", input.Context.Now)
	}
}

func TestEngineRequiresVersionedPackageAndBlocksUnsafeBuiltins(t *testing.T) {
	if err := (Engine{}).Compile(context.Background(), `package planty`); err == nil || !strings.Contains(err.Error(), "planty.v1") {
		t.Fatalf("wrong package returned %v", err)
	}

	unsafe := `package planty.v1
health := time.now_ns()`
	if err := (Engine{}).Compile(context.Background(), unsafe); err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("unsafe builtin returned %v", err)
	}
}

func TestEngineAcceptsNeedsFamilyAndRejectsUnknownRules(t *testing.T) {
	result, _, err := (Engine{}).Evaluate(context.Background(), `package planty.v1
needs_staked := {"urgency": "soon"}`, Input{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rules) != 1 || result.Rules[0].Name != "needs_staked" || !result.Rules[0].Active {
		t.Fatalf("needs family result = %#v", result.Rules)
	}

	unknown := `package planty.v1
watering_now := {"anything": [1, 2, 3]}`
	if _, _, err := (Engine{}).Evaluate(context.Background(), unknown, Input{}); err == nil || !strings.Contains(err.Error(), "watering_now") {
		t.Fatalf("unknown rule returned %v", err)
	}
}

func TestEngineRejectsMalformedTypedRules(t *testing.T) {
	malformed := `package planty.v1
fan_run := {"duration_seconds": 30}`
	if _, _, err := (Engine{}).Evaluate(context.Background(), malformed, Input{}); err == nil || !strings.Contains(err.Error(), "fan_run") {
		t.Fatalf("malformed typed rule returned %v", err)
	}
}

func TestResultRejectsUnboundedFanAndHealthWrites(t *testing.T) {
	result := Result{Rules: []Rule{}, Health: &HealthAdjustment{Delta: 21, Reason: "too much"}}
	if err := result.Valid(); err == nil {
		t.Fatal("unbounded health delta was accepted")
	}
	result.Health = nil
	result.FanRuns = []FanRun{{DurationSeconds: 3601, Reason: "too long"}}
	if err := result.Valid(); err == nil {
		t.Fatal("unbounded fan run was accepted")
	}
	result = Result{
		Rules: []Rule{{Name: "agent_facts", Active: true, Value: json.RawMessage(`[]`)}},
		Agent: AgentGuidance{Facts: make([]string, MaxOutputItems+1)},
	}
	if err := result.Valid(); err == nil {
		t.Fatal("unbounded agent context was accepted")
	}
}
