package seed

import (
	"encoding/json"
	"testing"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

func load(t *testing.T) friendsFile {
	t.Helper()
	var file friendsFile
	if err := json.Unmarshal(friendsRaw, &file); err != nil {
		t.Fatalf("seed json does not parse: %v", err)
	}
	return file
}

// A mistyped JSON key decodes silently into a zero value, so every seeded plant
// is validated the same way the store would validate it.
func TestSeededPlantsValidate(t *testing.T) {
	file := load(t)
	if len(file.Plants) == 0 {
		t.Fatal("seed contains no plants")
	}

	for _, p := range file.Plants {
		p.Steward = "friend"
		if err := p.Valid(); err != nil {
			t.Errorf("%s: %v", p.Slug, err)
		}
		if p.Slug == "" {
			t.Error("a seeded plant has no slug")
		}
		if p.CareProfile.OwnerSays == "" {
			t.Errorf("%s: lost the owner's own words, which outrank generic advice", p.Slug)
		}
	}
}

// Every one of these has to come indoors, and the threshold is what the cold
// watch queries. A nil here means the plant is silently never protected.
func TestEverySeededPlantHasAColdThreshold(t *testing.T) {
	for _, p := range load(t).Plants {
		if p.MinTempF == nil {
			t.Errorf("%s: no min_temp_f, so the cold watch will never pick it up", p.Slug)
		}
	}
}

func TestSeededPlantsAreHandWatered(t *testing.T) {
	for _, p := range load(t).Plants {
		if p.WateringMethod != plant.WateringHand {
			t.Errorf("%s: nothing is on the LetPot line yet", p.Slug)
		}
	}
}

func TestOwnerQuestionsAreQueued(t *testing.T) {
	if len(load(t).Questions) == 0 {
		t.Fatal("the owner asked to be asked; the queue should not be empty")
	}
}
