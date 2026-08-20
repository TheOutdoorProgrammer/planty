package api

import (
	"testing"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/ha"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

func TestManagedChoicesMergeEquivalentPlacesAndHomeAssistantAreas(t *testing.T) {
	older := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	newer := older.Add(24 * time.Hour)
	catalog := mergeManagedChoices([]store.ChoiceCandidate{
		{Kind: store.ChoicePlace, Value: "living-room", Source: "home_assistant_area", UsedAt: older},
		{Kind: store.ChoicePlace, Value: "Living Room", Source: "plant_location", UsedAt: newer},
		{Kind: store.ChoiceOwner, Value: "Maya", Source: "plant_owner", UsedAt: newer},
		{Kind: store.ChoicePotMaterial, Value: "Terracotta", Source: "plant_pot_material", UsedAt: older},
	}, []ha.Entity{
		{EntityID: "sensor.living_room_temp", Area: "Living_Room"},
		{EntityID: "sensor.porch_temp", Area: "Back Porch"},
	})

	if len(catalog.Places.All) != 2 {
		t.Fatalf("places = %#v, want one Living Room plus Back Porch", catalog.Places.All)
	}
	if catalog.Places.All[1].Value != "Living Room" {
		t.Fatalf("newest user spelling was not kept: %#v", catalog.Places.All)
	}
	if len(catalog.Places.Recent) != 1 || catalog.Places.Recent[0].Value != "Living Room" {
		t.Fatalf("recent places = %#v", catalog.Places.Recent)
	}
	if len(catalog.Owners.All) != 2 { // Maya plus the built-in self choice.
		t.Fatalf("owners = %#v", catalog.Owners.All)
	}
	if len(catalog.PotMaterials.All) != 1 || catalog.PotMaterials.All[0].Value != "Terracotta" {
		t.Fatalf("materials = %#v", catalog.PotMaterials.All)
	}
}

func TestManagedPlaceKeyDoesNotInventSynonyms(t *testing.T) {
	if managedPlaceKey("Living-room") != managedPlaceKey("living_room") {
		t.Fatal("punctuation variants should collapse")
	}
	if managedPlaceKey("Living Room") == managedPlaceKey("Lounge") {
		t.Fatal("different names must not be guessed to mean the same place")
	}
	if textChoiceKey("Mary-Jane") == textChoiceKey("Mary Jane") {
		t.Fatal("punctuation in owner names must remain meaningful")
	}
}
