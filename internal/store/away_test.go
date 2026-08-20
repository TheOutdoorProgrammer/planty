package store

import (
	"errors"
	"testing"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

func awayTestWindow() (time.Time, time.Time) {
	// Far enough out to stay in the normal current/future listing, with a
	// nanosecond-derived offset so repeated local runs do not collide with rows
	// left by an interrupted earlier run.
	start := time.Date(2100, 1, 1, 12, 0, 0, 0, time.UTC).
		Add(time.Duration(time.Now().UnixNano()%1_000_000) * time.Second)
	return start, start.Add(48 * time.Hour)
}

func TestAwayPeriodCanBeListedChangedAndCancelled(t *testing.T) {
	s, ctx := testStore(t)
	starts, ends := awayTestWindow()

	created, err := s.GoAway(ctx, plant.AwayPeriod{
		StartsAt: starts, EndsAt: ends,
		BackupContact: "Sam", Note: "first draft",
	})
	if err != nil {
		t.Fatalf("create away period: %v", err)
	}
	t.Cleanup(func() { _ = s.DeleteAway(ctx, created.ID) })

	periods, err := s.AwayPeriods(ctx, false)
	if err != nil {
		t.Fatalf("list away periods: %v", err)
	}
	var found bool
	for _, period := range periods {
		if period.ID == created.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("new coverage was not visible in the current/future list")
	}

	created.StartsAt = starts.Add(24 * time.Hour)
	created.EndsAt = ends.Add(24 * time.Hour)
	created.BackupContact = "Maya"
	created.Note = "corrected"
	updated, err := s.UpdateAway(ctx, created.ID, created)
	if err != nil {
		t.Fatalf("update away period: %v", err)
	}
	if updated.BackupContact != "Maya" || updated.Note != "corrected" {
		t.Fatalf("updated fields did not survive: %+v", updated)
	}

	if err := s.DeleteAway(ctx, created.ID); err != nil {
		t.Fatalf("cancel away period: %v", err)
	}
	if _, err := s.AwayPeriod(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cancelled period lookup returned %v, want ErrNotFound", err)
	}
}

func TestAwayPeriodsRejectOverlapButAllowTouchingWindows(t *testing.T) {
	s, ctx := testStore(t)
	starts, ends := awayTestWindow()

	first, err := s.GoAway(ctx, plant.AwayPeriod{StartsAt: starts, EndsAt: ends})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.DeleteAway(ctx, first.ID) })

	if _, err := s.GoAway(ctx, plant.AwayPeriod{
		StartsAt: starts.Add(12 * time.Hour),
		EndsAt:   ends.Add(12 * time.Hour),
	}); !errors.Is(err, plant.ErrInvalid) {
		t.Fatalf("overlap returned %v, want plant.ErrInvalid", err)
	}

	adjacent, err := s.GoAway(ctx, plant.AwayPeriod{
		StartsAt: ends,
		EndsAt:   ends.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("touching windows should be allowed: %v", err)
	}
	t.Cleanup(func() { _ = s.DeleteAway(ctx, adjacent.ID) })

	first.StartsAt = adjacent.StartsAt.Add(30 * time.Minute)
	first.EndsAt = adjacent.EndsAt.Add(30 * time.Minute)
	if _, err := s.UpdateAway(ctx, first.ID, first); !errors.Is(err, plant.ErrInvalid) {
		t.Fatalf("overlapping edit returned %v, want plant.ErrInvalid", err)
	}
}
