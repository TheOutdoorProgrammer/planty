package store

import (
	"testing"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

func TestChoiceCandidatesComeFromPlantsAndSensorZones(t *testing.T) {
	s, ctx := testStore(t)
	p, err := s.CreatePlant(ctx, plant.Plant{
		CommonName: "Choice subject", Domain: plant.DomainHouseplant,
		Steward: "Maya", Status: plant.StatusAlive, Location: "Living Room",
		HAArea: "living-room", Accessibility: plant.AccessEasy,
		WateringMethod: plant.WateringHand, PotMaterial: "Terracotta",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.LinkSensor(ctx, plant.SensorLink{
		Zone: "Back Porch", HAEntityID: "sensor.choice_temperature",
		Role: plant.RoleAmbientTemp,
	}); err != nil {
		t.Fatal(err)
	}

	choices, err := s.ChoiceCandidates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := map[ChoiceKind]map[string]bool{
		ChoicePlace:       {"Living Room": true, "living-room": true, "Back Porch": true},
		ChoiceOwner:       {"Maya": true},
		ChoicePotMaterial: {"Terracotta": true},
	}
	for _, choice := range choices {
		if values := want[choice.Kind]; values != nil {
			delete(values, choice.Value)
		}
	}
	for kind, values := range want {
		if len(values) != 0 {
			t.Errorf("missing %s choices: %#v", kind, values)
		}
	}
	_ = p
}
