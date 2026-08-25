package job

import (
	"context"
	"testing"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/pgtest"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
	"github.com/google/uuid"
)

type actuatorHA struct{ calls []string }

func (h *actuatorHA) CallService(_ context.Context, domain, service string, data map[string]any) error {
	h.calls = append(h.calls, domain+"/"+service+":"+data["entity_id"].(string))
	return nil
}

func TestActuatorControlUsesRegisteredEntityAndReconcilesDeadline(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, pgtest.DSN(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	actuator, err := db.RegisterActuator(ctx, plant.Actuator{EntityID: "fan.job_test", Name: "Job test", Kind: plant.ActuatorFan})
	if err != nil {
		t.Fatal(err)
	}
	ha := &actuatorHA{}
	control := ActuatorControl{Store: db, HA: ha, Log: quietLog()}
	lease, created, err := control.Start(ctx, actuator.ID, 1, "tester", plant.SourceApp, uuid.New())
	if err != nil || !created {
		t.Fatalf("start created=%v err=%v", created, err)
	}
	stopped, err := control.Reconcile(ctx, lease.Deadline.Add(time.Second))
	if err != nil || stopped != 1 {
		t.Fatalf("reconcile stopped=%d err=%v", stopped, err)
	}
	want := []string{"fan/turn_on:fan.job_test", "fan/turn_off:fan.job_test"}
	if len(ha.calls) != len(want) || ha.calls[0] != want[0] || ha.calls[1] != want[1] {
		t.Fatalf("HA calls = %#v", ha.calls)
	}
}
