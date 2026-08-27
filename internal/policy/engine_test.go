package policy

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestExampleCompilesAndReturnsTypedDecision(t *testing.T) {
	fraction := 0.1
	decision, _, err := (Engine{}).Evaluate(context.Background(), ExampleSource, Input{
		Version: InputVersion,
		Plant:   PlantFacts{CommonName: "Fern"},
		Sensors: SensorFacts{SoilMoisture: &SensorFact{Calibrated: true, Fraction: &fraction}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(decision.Signals) != 1 || decision.Signals[0].Kind != SignalNeedsWatered {
		t.Fatalf("decision = %#v", decision)
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

func TestEngineRejectsUnknownOutputAndUnsafeBuiltins(t *testing.T) {
	if err := (Engine{}).Compile(context.Background(), `package planty`); err == nil {
		t.Fatal("policy without the decision entrypoint compiled")
	}

	unknown := `package planty
decision := {"summary":"bad", "signals":[], "notifications":[], "fan_runs":[], "agent":{"facts":[],"guidance":[],"deny_actions":[]}, "execute_shell":true}`
	if _, _, err := (Engine{}).Evaluate(context.Background(), unknown, Input{}); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown output returned %v", err)
	}

	unsafe := `package planty
decision := {"summary": sprintf("%d", [time.now_ns()]), "signals":[], "notifications":[], "fan_runs":[], "agent":{"facts":[],"guidance":[],"deny_actions":[]}}`
	if err := (Engine{}).Compile(context.Background(), unsafe); err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("unsafe builtin returned %v", err)
	}
}

func TestDecisionRejectsUnboundedFanAndHealthWrites(t *testing.T) {
	decision := Decision{Summary: "bad", Health: &HealthAdjustment{Delta: 21, Reason: "too much"}}
	if err := decision.Valid(); err == nil {
		t.Fatal("unbounded health delta was accepted")
	}
	decision.Health = nil
	decision.FanRuns = []FanRun{{DurationSeconds: 3601, Reason: "too long"}}
	if err := decision.Valid(); err == nil {
		t.Fatal("unbounded fan run was accepted")
	}
	decision = Decision{
		Summary: "too many facts",
		Agent:   AgentGuidance{Facts: make([]string, MaxDecisionItems+1)},
	}
	if err := decision.Valid(); err == nil {
		t.Fatal("unbounded agent context was accepted")
	}
}
