package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/TheOutdoorProgrammer/planty/internal/job"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/google/uuid"
)

type agentActuatorHA struct{ calls []string }

func (h *agentActuatorHA) CallService(_ context.Context, domain, service string, data map[string]any) error {
	h.calls = append(h.calls, domain+"/"+service+":"+data["entity_id"].(string))
	return nil
}

func TestAgentActuatorVerbsUsePlantyIDAndIdempotency(t *testing.T) {
	deps, _, ctx := toxicityDeps(t)
	actuator, err := deps.Store.RegisterActuator(ctx, plant.Actuator{EntityID: "switch.agent_fan", Name: "Agent fan", Kind: plant.ActuatorSwitch})
	if err != nil {
		t.Fatal(err)
	}
	ha := &agentActuatorHA{}
	control := &job.ActuatorControl{Store: deps.Store, HA: ha}
	deps.Actuators = control
	key := uuid.NewString()
	if _, err := runVerbCtx(t, ctx, deps, "actuatorstart", "--id", actuator.ID.String(), "--seconds", "60", "--key", key); err != nil {
		t.Fatal(err)
	}
	replay, err := runVerbCtx(t, ctx, deps, "actuatorstart", "--id", actuator.ID.String(), "--seconds", "60", "--key", key)
	if err != nil || !strings.Contains(replay, "already accepted") || len(ha.calls) != 1 {
		t.Fatalf("replay=%q calls=%#v err=%v", replay, ha.calls, err)
	}
	stopKey := uuid.NewString()
	if _, err := runVerbCtx(t, ctx, deps, "actuatorstop", "--id", actuator.ID.String(), "--key", stopKey); err != nil {
		t.Fatal(err)
	}
	if _, err := runVerbCtx(t, ctx, deps, "actuatorstop", "--id", actuator.ID.String(), "--key", stopKey); err != nil {
		t.Fatal(err)
	}
	if len(ha.calls) != 3 || ha.calls[1] != "switch/turn_off:switch.agent_fan" ||
		ha.calls[2] != "switch/turn_off:switch.agent_fan" {
		t.Fatalf("calls = %#v", ha.calls)
	}
}
