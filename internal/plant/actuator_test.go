package plant

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestActuatorRequiresMatchingAllowlistedDomainAndBoundedLease(t *testing.T) {
	plantID := uuid.New()
	if err := (Actuator{Name: "Cabinet fan", EntityID: "fan.cabinet", Kind: ActuatorFan, PlantIDs: []uuid.UUID{plantID}}).Valid(); err != nil {
		t.Fatal(err)
	}
	for _, actuator := range []Actuator{
		{Name: "Light", EntityID: "light.grow", Kind: ActuatorSwitch, PlantIDs: []uuid.UUID{plantID}},
		{Name: "Wrong domain", EntityID: "fan.cabinet", Kind: ActuatorSwitch, PlantIDs: []uuid.UUID{plantID}},
		{Name: "Policy switch", EntityID: "switch.circulator", Kind: ActuatorSwitch,
			PlantIDs: []uuid.UUID{plantID}, PolicyControlEnabled: true},
		{Name: "Unassigned", EntityID: "fan.unassigned", Kind: ActuatorFan},
	} {
		if !errors.Is(actuator.Valid(), ErrInvalid) {
			t.Fatalf("accepted invalid actuator %#v", actuator)
		}
	}
	lease := ActuatorLease{
		ActuatorID: uuid.New(), RequestedSeconds: int(MaxActuatorDuration/time.Second) + 1,
		Actor: "test", Source: SourceApp, IdempotencyKey: uuid.New(),
	}
	if !errors.Is(lease.Valid(), ErrInvalid) {
		t.Fatal("accepted a lease beyond the hard maximum")
	}
}
