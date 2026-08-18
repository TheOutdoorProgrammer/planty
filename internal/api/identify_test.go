package api

import (
	"net/http/httptest"
	"testing"

	"github.com/TheOutdoorProgrammer/planty/internal/judge"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

func described(t *testing.T, query string, candidates ...judge.Candidate) plant.Plant {
	t.Helper()

	p, err := describedBy(httptest.NewRequest("POST", "/v1/plants/from-photo?"+query, nil), candidates)
	if err != nil {
		t.Fatalf("describedBy: %v", err)
	}
	return p
}

func TestTheTopCandidateNamesThePlant(t *testing.T) {
	p := described(t, "", judge.Candidate{
		CommonName:     "Golden pothos",
		ScientificName: "Epipremnum aureum",
		Confidence:     0.55,
	}, judge.Candidate{CommonName: "Heartleaf philodendron", Confidence: 0.3})

	if p.CommonName != "Golden pothos" {
		t.Errorf("named %q, want the most likely candidate", p.CommonName)
	}
	if p.BotanicalName != "Epipremnum aureum" {
		t.Errorf("botanical name is %q", p.BotanicalName)
	}
}

// Somebody holding the plant knows better than a model holding a picture of it.
func TestAGivenNameBeatsTheModel(t *testing.T) {
	p := described(t, "common_name=Aric%27s+monstera&location=Guest+room",
		judge.Candidate{CommonName: "Golden pothos", ScientificName: "Epipremnum aureum"})

	if p.CommonName != "Aric's monstera" {
		t.Errorf("named %q, want the name that was given", p.CommonName)
	}
	if p.Location != "Guest room" {
		t.Errorf("location is %q", p.Location)
	}
	if p.BotanicalName != "" {
		t.Errorf("kept the model's botanical name %q against a given common name", p.BotanicalName)
	}
}

// An empty candidate list is the model saying it does not know, and inventing
// a name there is exactly what its system prompt forbids.
func TestNothingRecognisedIsNotAPlantNamedEmpty(t *testing.T) {
	if _, err := describedBy(httptest.NewRequest("POST", "/v1/plants/from-photo", nil), nil); err == nil {
		t.Fatal("an unrecognised photograph created a plant with no name")
	}

	p := described(t, "common_name=Some+cactus")
	if p.CommonName != "Some cactus" {
		t.Errorf("named %q despite being told", p.CommonName)
	}
}

func TestAPhotographedPlantStillGetsTheDefaults(t *testing.T) {
	p := described(t, "", judge.Candidate{CommonName: "Golden pothos"})

	if p.Status != plant.StatusAlive {
		t.Errorf("status is %q", p.Status)
	}
	if p.Steward != plant.StewardSelf {
		t.Errorf("steward is %q", p.Steward)
	}
	if p.WateringMethod != plant.WateringHand {
		t.Errorf("watering method is %q", p.WateringMethod)
	}
	if p.Domain != plant.DomainHouseplant {
		t.Errorf("domain is %q", p.Domain)
	}
}

func TestAStewardAndDomainCanBeGiven(t *testing.T) {
	p := described(t, "steward=Aric&domain=mushroom", judge.Candidate{CommonName: "Blue oyster"})

	if p.Steward != "Aric" {
		t.Errorf("steward is %q", p.Steward)
	}
	if p.Domain != plant.Domain("mushroom") {
		t.Errorf("domain is %q", p.Domain)
	}
}
