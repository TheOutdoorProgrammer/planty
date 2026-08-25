package store

import (
	"errors"
	"testing"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/google/uuid"
)

func TestActuatorRegistryLeaseAndAuditAreDurableAndIdempotent(t *testing.T) {
	s, ctx := testStore(t)
	grown, err := s.CreatePlant(ctx, plant.Plant{
		CommonName: "Actuator store test", Domain: plant.DomainHouseplant,
		Steward: plant.StewardSelf, Status: plant.StatusAlive, Location: "test",
		Accessibility: plant.AccessEasy, WateringMethod: plant.WateringHand,
	})
	if err != nil {
		t.Fatal(err)
	}
	actuator, err := s.RegisterActuator(ctx, plant.Actuator{
		EntityID: "fan.test_cabinet", Name: "Test cabinet", Kind: plant.ActuatorFan,
		PlantIDs: []uuid.UUID{grown.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(actuator.PlantIDs) != 1 || actuator.PlantIDs[0] != grown.ID {
		t.Fatalf("plant assignments = %#v", actuator.PlantIDs)
	}
	key := uuid.New()
	_, lease, created, err := s.BeginActuatorLease(ctx, plant.ActuatorLease{
		ActuatorID: actuator.ID, RequestedSeconds: 30, Actor: "tester", Source: plant.SourceApp, IdempotencyKey: key,
	})
	if err != nil || !created || lease.Deadline.IsZero() {
		t.Fatalf("begin = %#v created=%v err=%v", lease, created, err)
	}
	listed, err := s.Actuators(ctx)
	if err != nil || len(listed) != 1 || listed[0].ActiveLease == nil || listed[0].ActiveLease.ID != lease.ID {
		t.Fatalf("active lease was not restored: %#v err=%v", listed, err)
	}
	_, replay, created, err := s.BeginActuatorLease(ctx, plant.ActuatorLease{
		ActuatorID: actuator.ID, RequestedSeconds: 30, Actor: "tester", Source: plant.SourceApp, IdempotencyKey: key,
	})
	if err != nil || created || replay.ID != lease.ID {
		t.Fatalf("replay = %#v created=%v err=%v", replay, created, err)
	}
	if err := s.DeleteActuator(ctx, actuator.ID); !errors.Is(err, plant.ErrInvalid) {
		t.Fatalf("deleted active actuator: %v", err)
	}
	overdue, err := s.OverdueActuatorLeases(ctx, lease.Deadline.Add(time.Second))
	if err != nil || len(overdue) != 1 || overdue[0].ID != lease.ID {
		t.Fatalf("overdue = %#v err=%v", overdue, err)
	}
	stopped, err := s.FinishActuatorLease(ctx, lease, "tester", plant.SourceApp, "test", nil, nil)
	if err != nil || !stopped {
		t.Fatalf("finish stopped=%v err=%v", stopped, err)
	}
	stopped, err = s.FinishActuatorLease(ctx, lease, "tester", plant.SourceApp, "repeat", nil, nil)
	if err != nil || stopped {
		t.Fatalf("repeated finish stopped=%v err=%v", stopped, err)
	}
	events, err := s.ActuatorEvents(ctx, actuator.ID, 20)
	if err != nil || len(events) != 2 || events[0].Action != "stopped" || events[1].Action != "start_requested" {
		t.Fatalf("events = %#v err=%v", events, err)
	}
}
