package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/TheOutdoorProgrammer/planty/internal/pgtest"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

// toxicityDeps gives the verb a real database, because everything worth
// checking here is a rule the store enforces on the way in.
func toxicityDeps(t *testing.T) (Deps, plant.Plant, context.Context) {
	t.Helper()

	ctx := context.Background()
	s, err := store.Open(ctx, pgtest.DSN(t))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(s.Close)
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Named after the test, because these share one database and a fixed name
	// would collide on the slug.
	p, err := s.CreatePlant(ctx, plant.Plant{
		CommonName: t.Name(), Domain: plant.DomainHouseplant,
		Steward: plant.StewardSelf, Status: plant.StatusAlive,
		Location: "sill", Accessibility: plant.AccessEasy,
		WateringMethod: plant.WateringHand,
	})
	if err != nil {
		t.Fatalf("create plant: %v", err)
	}
	return Deps{Store: s}, p, ctx
}

// A fresh plant is unrated, and unrated is not safe.
func TestAPlantStartsUnratedRatherThanCleared(t *testing.T) {
	_, p, _ := toxicityDeps(t)

	if p.Toxicity.Checked() {
		t.Error("a brand new plant claims somebody checked its toxicity")
	}
	if p.Toxicity.Cats != plant.HarmUnknown {
		t.Errorf("an unrated plant reads as %q to a cat", p.Toxicity.Cats)
	}
}

func TestRecordingTheLily(t *testing.T) {
	deps, p, ctx := toxicityDeps(t)

	out, err := runVerbCtx(t, ctx, deps, "toxicity", "--plant", p.Slug,
		"--cats", "severe", "--dogs", "mild", "--people", "mild",
		"--basis", "derived", "--identified-as", "Lilium longiflorum",
		"--principle", "unidentified nephrotoxin",
		"--first-aid", "a cat that groomed pollen goes to a vet before any sign shows",
		"--source", "www.aspca.org")
	if err != nil {
		t.Fatalf("recording the lily failed: %v", err)
	}
	if !strings.Contains(out, "the audiences differ") {
		t.Error("a plant that kills cats and mildly upsets dogs did not report divergence")
	}

	back, err := deps.Store.GetPlant(ctx, p.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if back.Toxicity.Cats != plant.HarmSevere || back.Toxicity.Dogs != plant.HarmMild {
		t.Errorf("ratings did not survive: %+v", back.Toxicity)
	}
	if back.Toxicity.IdentifiedAs != "Lilium longiflorum" {
		t.Error("the botanical name was lost, which is the only thing making this rating mean anything")
	}
	if back.Toxicity.CheckedAt == nil {
		t.Error("nothing recorded when it was checked")
	}
	if !back.Toxicity.Dangerous() {
		t.Error("a lily did not read as dangerous")
	}
}

// The rule that stops the field filling with confident guesses.
func TestASevereRatingIsRefusedWithoutItsMechanism(t *testing.T) {
	deps, p, ctx := toxicityDeps(t)

	_, err := runVerbCtx(t, ctx, deps, "toxicity", "--plant", p.Slug,
		"--cats", "severe", "--basis", "derived")
	if err == nil {
		t.Fatal("a severe rating was accepted with nothing behind it")
	}
	if !strings.Contains(err.Error(), "principle") {
		t.Errorf("the refusal did not say what was missing: %v", err)
	}
}

func TestAnInventedLevelIsRefused(t *testing.T) {
	deps, p, ctx := toxicityDeps(t)

	if _, err := runVerbCtx(t, ctx, deps, "toxicity", "--plant", p.Slug,
		"--cats", "deadly", "--basis", "source"); err == nil {
		t.Fatal("an invented rating was accepted")
	}
}

// Adding a note later must not blank ratings somebody already looked up.
func TestALaterNoteKeepsTheRatings(t *testing.T) {
	deps, p, ctx := toxicityDeps(t)

	if _, err := runVerbCtx(t, ctx, deps, "toxicity", "--plant", p.Slug,
		"--cats", "mild", "--dogs", "mild", "--people", "mild",
		"--basis", "source", "--identified-as", "Epipremnum aureum"); err != nil {
		t.Fatal(err)
	}
	if _, err := runVerbCtx(t, ctx, deps, "toxicity", "--plant", p.Slug,
		"--notes", "the sap is the part that stings"); err != nil {
		t.Fatal(err)
	}

	back, err := deps.Store.GetPlant(ctx, p.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if back.Toxicity.Cats != plant.HarmMild {
		t.Errorf("writing a note wiped the rating, leaving %q", back.Toxicity.Cats)
	}
	if back.Toxicity.IdentifiedAs != "Epipremnum aureum" {
		t.Error("writing a note wiped the identification")
	}
}
