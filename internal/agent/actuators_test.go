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
	deps, grown, ctx := toxicityDeps(t)
	actuator, err := deps.Store.RegisterActuator(ctx, plant.Actuator{
		EntityID: "switch.agent_fan", Name: "Agent fan", Kind: plant.ActuatorFan,
		PlantIDs: []uuid.UUID{grown.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	ha := &agentActuatorHA{}
	control := &job.ActuatorControl{Store: deps.Store, HA: ha}
	deps.Actuators = control
	listed, err := runVerbCtx(t, ctx, deps, "actuators", "--plant", grown.Slug)
	if err != nil || !strings.Contains(listed, "plants="+grown.Slug) {
		t.Fatalf("plant-scoped actuators = %q err=%v", listed, err)
	}
	key := uuid.NewString()
	if _, err := runVerbCtx(t, ctx, deps, "actuatorstart", "--plant", grown.Slug, "--id", actuator.ID.String(), "--seconds", "60", "--key", key); err != nil {
		t.Fatal(err)
	}
	replay, err := runVerbCtx(t, ctx, deps, "actuatorstart", "--plant", grown.Slug, "--id", actuator.ID.String(), "--seconds", "60", "--key", key)
	if err != nil || !strings.Contains(replay, "already accepted") || len(ha.calls) != 1 {
		t.Fatalf("replay=%q calls=%#v err=%v", replay, ha.calls, err)
	}
	stopKey := uuid.NewString()
	if _, err := runVerbCtx(t, ctx, deps, "actuatorstop", "--plant", grown.Slug, "--id", actuator.ID.String(), "--key", stopKey); err != nil {
		t.Fatal(err)
	}
	if _, err := runVerbCtx(t, ctx, deps, "actuatorstop", "--plant", grown.Slug, "--id", actuator.ID.String(), "--key", stopKey); err != nil {
		t.Fatal(err)
	}
	if len(ha.calls) != 3 || ha.calls[1] != "switch/turn_off:switch.agent_fan" ||
		ha.calls[2] != "switch/turn_off:switch.agent_fan" {
		t.Fatalf("calls = %#v", ha.calls)
	}
	observations, err := deps.Store.Observations(ctx, grown.ID, 10)
	if err != nil || len(observations) != 1 || observations[0].Kind != plant.ObservedAirflow {
		t.Fatalf("airflow observations = %#v err=%v", observations, err)
	}
}

func TestAgentLightScheduleAcceptsRepeatedWindows(t *testing.T) {
	deps, grown, ctx := toxicityDeps(t)
	actuator, err := deps.Store.RegisterActuator(ctx, plant.Actuator{
		EntityID: "switch.agent_light", Name: "Agent light", Kind: plant.ActuatorLight,
		PlantIDs: []uuid.UUID{grown.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	output, err := runVerbCtx(t, ctx, deps, "lightschedule",
		"--plant", grown.Slug, "--id", actuator.ID.String(),
		"--window", "07:00-08:00", "--window", "12:00-13:00", "--timezone", "UTC")
	if err != nil || !strings.Contains(output, "07:00-08:00,12:00-13:00") {
		t.Fatalf("lightschedule = %q err=%v", output, err)
	}
	schedule, err := deps.Store.LightSchedule(ctx, actuator.ID)
	if err != nil || len(schedule.Windows) != 2 {
		t.Fatalf("stored schedule = %#v err=%v", schedule, err)
	}
	if _, err := runVerbCtx(t, ctx, deps, "lightschedule",
		"--plant", grown.Slug, "--id", actuator.ID.String(),
		"--window", "07:00-09:00", "--window", "08:00-10:00", "--timezone", "UTC"); err == nil {
		t.Fatal("accepted overlapping windows")
	}
}
