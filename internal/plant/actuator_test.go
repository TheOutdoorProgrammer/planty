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

func TestLightScheduleUsesItsTimezoneAndSupportsOvernightWindows(t *testing.T) {
	schedule := LightSchedule{
		ActuatorID: uuid.New(), StartMinute: 20 * 60, EndMinute: 6 * 60,
		Timezone: "America/New_York", Enabled: true,
	}
	for _, test := range []struct {
		at   string
		want bool
	}{
		{at: "2026-08-31T01:00:00Z", want: true},
		{at: "2026-08-31T12:00:00Z", want: false},
	} {
		at, err := time.Parse(time.RFC3339, test.at)
		if err != nil {
			t.Fatal(err)
		}
		got, err := schedule.WantsOn(at)
		if err != nil || got != test.want {
			t.Fatalf("WantsOn(%s) = %v, %v; want %v", test.at, got, err, test.want)
		}
	}
	schedule.Enabled = false
	if got, err := schedule.WantsOn(time.Now()); err != nil || got {
		t.Fatalf("disabled WantsOn = %v, %v", got, err)
	}
	schedule.Enabled = true
	schedule.Timezone = "Nowhere/Imaginary"
	if !errors.Is(schedule.Valid(), ErrInvalid) {
		t.Fatal("accepted an unknown timezone")
	}
}
