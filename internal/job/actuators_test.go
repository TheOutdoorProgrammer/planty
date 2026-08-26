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
	grown, err := db.CreatePlant(ctx, plant.Plant{
		CommonName: "Actuator job test", Domain: plant.DomainHouseplant,
		Steward: plant.StewardSelf, Status: plant.StatusAlive, Location: "test",
		Accessibility: plant.AccessEasy, WateringMethod: plant.WateringHand,
	})
	if err != nil {
		t.Fatal(err)
	}
	neighbor, err := db.CreatePlant(ctx, plant.Plant{
		CommonName: "Actuator job neighbor", Domain: plant.DomainHouseplant,
		Steward: plant.StewardSelf, Status: plant.StatusAlive, Location: "test",
		Accessibility: plant.AccessEasy, WateringMethod: plant.WateringHand,
	})
	if err != nil {
		t.Fatal(err)
	}
	actuator, err := db.RegisterActuator(ctx, plant.Actuator{
		EntityID: "fan.job_test", Name: "Job test", Kind: plant.ActuatorFan,
		PlantIDs: []uuid.UUID{grown.ID, neighbor.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	ha := &actuatorHA{}
	control := ActuatorControl{Store: db, HA: ha, Log: quietLog()}
	key := uuid.New()
	lease, created, err := control.Start(ctx, actuator.ID, 1, "tester", plant.SourceApp, key)
	if err != nil || !created {
		t.Fatalf("start created=%v err=%v", created, err)
	}
	if _, created, err := control.Start(ctx, actuator.ID, 1, "tester", plant.SourceApp, key); err != nil || created {
		t.Fatalf("replayed start created=%v err=%v", created, err)
	}
	for _, assigned := range []plant.Plant{grown, neighbor} {
		observations, err := db.Observations(ctx, assigned.ID, 10)
		if err != nil || len(observations) != 1 {
			t.Fatalf("%s airflow observations = %#v err=%v", assigned.CommonName, observations, err)
		}
		if observations[0].Kind != plant.ObservedAirflow || observations[0].Body != "Job test started for up to 1 second." {
			t.Errorf("%s airflow observation = %#v", assigned.CommonName, observations[0])
		}
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

func TestActuatorControlRejectsAPlantOutsideTheAssignment(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, pgtest.DSN(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	assigned := createActuatorTestPlant(t, ctx, db, "Assigned plant")
	other := createActuatorTestPlant(t, ctx, db, "Other plant")
	actuator, err := db.RegisterActuator(ctx, plant.Actuator{
		EntityID: "fan.assigned", Name: "Assigned fan", Kind: plant.ActuatorFan,
		PlantIDs: []uuid.UUID{assigned.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	ha := &actuatorHA{}
	control := ActuatorControl{Store: db, HA: ha, Log: quietLog()}
	if _, _, err := control.StartForPlant(ctx, actuator.ID, other.ID, 60, "agent", plant.SourceAgent, uuid.New()); err == nil {
		t.Fatal("started an actuator for an unassigned plant")
	}
	if len(ha.calls) != 0 {
		t.Fatalf("Home Assistant calls = %#v", ha.calls)
	}
}

func createActuatorTestPlant(t *testing.T, ctx context.Context, db *store.Store, name string) plant.Plant {
	t.Helper()
	p, err := db.CreatePlant(ctx, plant.Plant{
		CommonName: name, Domain: plant.DomainHouseplant, Steward: plant.StewardSelf,
		Status: plant.StatusAlive, Location: "test", Accessibility: plant.AccessEasy,
		WateringMethod: plant.WateringHand,
	})
	if err != nil {
		t.Fatal(err)
	}
	return p
}
