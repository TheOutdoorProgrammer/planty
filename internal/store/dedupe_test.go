package store

import (
	"testing"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

// Saving a capture and then asking about it are two uploads of one picture.
// This shipped as two rows on the bonsai that nobody could tell apart.
func TestOneCaptureIsOnePhotograph(t *testing.T) {
	s, ctx := testStore(t)
	p, err := s.CreatePlant(ctx, plant.Plant{
		CommonName: t.Name(), Domain: plant.DomainHouseplant,
		Steward: plant.StewardSelf, Status: plant.StatusAlive,
		Location: "sill", Accessibility: plant.AccessEasy,
		WateringMethod: plant.WateringHand,
	})
	if err != nil {
		t.Fatal(err)
	}

	first, err := s.SavePhoto(ctx, plant.Photo{
		PlantID: p.ID, StorageKey: t.Name()+"-a.jpg", ContentHash: t.Name()+"-same",
	})
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.SavePhoto(ctx, plant.Photo{
		PlantID: p.ID, StorageKey: t.Name()+"-b.jpg", ContentHash: t.Name()+"-same",
	})
	if err != nil {
		t.Fatal(err)
	}

	if again.ID != first.ID {
		t.Errorf("the same picture was filed twice: %s then %s", first.ID, again.ID)
	}
	if again.StorageKey != t.Name()+"-a.jpg" {
		t.Errorf("the second upload replaced the first: %s", again.StorageKey)
	}

	shots, err := s.Photos(ctx, p.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(shots) != 1 {
		t.Errorf("the timeline shows %d photographs, want 1", len(shots))
	}
}

// A different picture is still a different picture.
func TestADifferentPictureIsKept(t *testing.T) {
	s, ctx := testStore(t)
	p, err := s.CreatePlant(ctx, plant.Plant{
		CommonName: t.Name(), Domain: plant.DomainHouseplant,
		Steward: plant.StewardSelf, Status: plant.StatusAlive,
		Location: "sill", Accessibility: plant.AccessEasy,
		WateringMethod: plant.WateringHand,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.SavePhoto(ctx, plant.Photo{PlantID: p.ID, StorageKey: t.Name()+"-a.jpg", ContentHash: t.Name()+"-aaa"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SavePhoto(ctx, plant.Photo{PlantID: p.ID, StorageKey: t.Name()+"-b.jpg", ContentHash: t.Name()+"-bbb"}); err != nil {
		t.Fatal(err)
	}

	shots, _ := s.Photos(ctx, p.ID, 10)
	if len(shots) != 2 {
		t.Errorf("two different pictures collapsed to %d", len(shots))
	}
}

// Photographs stored before hashing have no hash and must not collide.
func TestUnhashedPhotographsDoNotCollide(t *testing.T) {
	s, ctx := testStore(t)
	p, err := s.CreatePlant(ctx, plant.Plant{
		CommonName: t.Name(), Domain: plant.DomainHouseplant,
		Steward: plant.StewardSelf, Status: plant.StatusAlive,
		Location: "sill", Accessibility: plant.AccessEasy,
		WateringMethod: plant.WateringHand,
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, key := range []string{t.Name() + "-old-1.jpg", t.Name() + "-old-2.jpg"} {
		if _, err := s.SavePhoto(ctx, plant.Photo{PlantID: p.ID, StorageKey: key}); err != nil {
			t.Fatalf("an unhashed photograph was refused: %v", err)
		}
	}
	shots, _ := s.Photos(ctx, p.ID, 10)
	if len(shots) != 2 {
		t.Errorf("unhashed photographs collided: got %d, want 2", len(shots))
	}
}
