package store

import (
	"errors"
	"testing"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/google/uuid"
)

func TestHealthStartsUnknownAndChangesAppendOnly(t *testing.T) {
	s, ctx := testStore(t)
	p := newPlant(t, s, ctx, "Health ledger subject")
	if _, err := s.LatestHealth(ctx, p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown health returned %v", err)
	}

	minus := -5.0
	if _, _, err := s.RecordHealth(ctx, plant.HealthChange{
		PlantID: p.ID, Delta: &minus, Rationale: "droop", Evidence: plant.HealthEvidence{Summary: "visible droop"}, Source: plant.SourceApp,
	}); !errors.Is(err, plant.ErrInvalid) {
		t.Fatalf("delta before baseline returned %v", err)
	}

	baseline := 90.0
	first, inserted, err := s.RecordHealth(ctx, plant.HealthChange{
		PlantID: p.ID, Baseline: &baseline, Rationale: "healthy baseline",
		Evidence: plant.HealthEvidence{Summary: "upright with active growth"}, Source: plant.SourceApp,
	})
	if err != nil || !inserted || first.Score != 90 || !first.IsBaseline() {
		t.Fatalf("baseline = %#v inserted=%v err=%v", first, inserted, err)
	}
	evidence, err := s.AddObservation(ctx, plant.Observation{
		PlantID: p.ID, Kind: plant.ObservedNote, Body: "new growth after treatment",
		Source: plant.SourceApp,
	})
	if err != nil {
		t.Fatal(err)
	}

	plus := 30.0
	second, inserted, err := s.RecordHealth(ctx, plant.HealthChange{
		PlantID: p.ID, Delta: &plus, Rationale: "recovered",
		Evidence: plant.HealthEvidence{Summary: "new growth after treatment", ObservationIDs: []uuid.UUID{evidence.ID}},
		Source:   plant.SourceAgent, Actor: "test agent",
	})
	if err != nil || !inserted {
		t.Fatalf("adjust: inserted=%v err=%v", inserted, err)
	}
	if second.Score != 100 || second.RequestedDelta == nil || *second.RequestedDelta != 30 || second.AppliedDelta == nil || *second.AppliedDelta != 10 {
		t.Fatalf("clamped event = %#v", second)
	}
	toZero := -200.0
	zero, inserted, err := s.RecordHealth(ctx, plant.HealthChange{
		PlantID: p.ID, Delta: &toZero, Rationale: "confirmed collapse",
		Evidence: plant.HealthEvidence{Summary: "no living tissue visible"}, Source: plant.SourceApp,
	})
	if err != nil || !inserted || zero.Score != 0 {
		t.Fatalf("zero event = %#v inserted=%v err=%v", zero, inserted, err)
	}

	history, err := s.HealthHistory(ctx, p.ID, 10)
	if err != nil || len(history) != 3 || history[0].ID != zero.ID || history[1].ID != second.ID || history[2].ID != first.ID {
		t.Fatalf("history = %#v err=%v", history, err)
	}
	if got, err := s.GetPlant(ctx, p.Slug); err != nil || got.Status != plant.StatusAlive {
		t.Fatalf("health altered lifecycle: status=%q err=%v", got.Status, err)
	}
}

func TestHealthWritesAreIdempotentAndAutomatedChangesNeedNewEvidence(t *testing.T) {
	s, ctx := testStore(t)
	p := newPlant(t, s, ctx, "Health evidence subject")
	baseline, key := 70.0, uuid.New()
	first, inserted, err := s.RecordHealth(ctx, plant.HealthChange{
		PlantID: p.ID, Baseline: &baseline, Rationale: "baseline",
		Evidence: plant.HealthEvidence{Summary: "initial inspection"}, Source: plant.SourceApp, IdempotencyKey: &key,
	})
	if err != nil || !inserted {
		t.Fatalf("baseline: %v", err)
	}
	replayed, inserted, err := s.RecordHealth(ctx, plant.HealthChange{
		PlantID: p.ID, Baseline: &baseline, Rationale: "baseline",
		Evidence: plant.HealthEvidence{Summary: "initial inspection"}, Source: plant.SourceApp, IdempotencyKey: &key,
	})
	if err != nil || inserted || replayed.ID != first.ID {
		t.Fatalf("replay = %#v inserted=%v err=%v", replayed, inserted, err)
	}

	old, err := s.AddObservation(ctx, plant.Observation{
		PlantID: p.ID, Kind: plant.ObservedSymptom, Body: "older evidence",
		Source: plant.SourceApp, OccurredAt: first.CreatedAt.Add(-time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := s.StartJudgmentRun(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	delta := -5.0
	if _, _, err := s.RecordHealth(ctx, plant.HealthChange{
		PlantID: p.ID, Delta: &delta, Rationale: "stale symptom",
		Evidence: plant.HealthEvidence{ObservationIDs: []uuid.UUID{old.ID}},
		Source:   plant.SourceAutomation, Actor: "planty daily", JudgmentRunID: &run.ID,
	}); !errors.Is(err, plant.ErrInvalid) {
		t.Fatalf("stale automated evidence returned %v", err)
	}

	fresh, err := s.AddObservation(ctx, plant.Observation{
		PlantID: p.ID, Kind: plant.ObservedSymptom, Body: "new damage",
		Source: plant.SourceApp,
	})
	if err != nil {
		t.Fatal(err)
	}
	event, inserted, err := s.RecordHealth(ctx, plant.HealthChange{
		PlantID: p.ID, Delta: &delta, Rationale: "new damage",
		Evidence: plant.HealthEvidence{ObservationIDs: []uuid.UUID{fresh.ID}},
		Source:   plant.SourceAutomation, Actor: "planty daily", JudgmentRunID: &run.ID,
	})
	if err != nil || !inserted || event.Score != 65 {
		t.Fatalf("fresh automated change = %#v inserted=%v err=%v", event, inserted, err)
	}
	retry, inserted, err := s.RecordHealth(ctx, plant.HealthChange{
		PlantID: p.ID, Delta: &delta, Rationale: "new damage",
		Evidence: plant.HealthEvidence{ObservationIDs: []uuid.UUID{fresh.ID}},
		Source:   plant.SourceAutomation, Actor: "planty daily", JudgmentRunID: &run.ID,
	})
	if err != nil || inserted || retry.ID != event.ID {
		t.Fatalf("run retry = %#v inserted=%v err=%v", retry, inserted, err)
	}
}

func TestHealthEvidenceCannotBelongToAnotherPlant(t *testing.T) {
	s, ctx := testStore(t)
	first := newPlant(t, s, ctx, "Health evidence owner")
	second := newPlant(t, s, ctx, "Health evidence thief")
	observation, err := s.AddObservation(ctx, plant.Observation{
		PlantID: first.ID, Kind: plant.ObservedSymptom, Source: plant.SourceApp,
	})
	if err != nil {
		t.Fatal(err)
	}
	baseline := 50.0
	_, _, err = s.RecordHealth(ctx, plant.HealthChange{
		PlantID: second.ID, Baseline: &baseline, Rationale: "wrong plant",
		Evidence: plant.HealthEvidence{ObservationIDs: []uuid.UUID{observation.ID}}, Source: plant.SourceApp,
	})
	if !errors.Is(err, plant.ErrInvalid) {
		t.Fatalf("cross-plant evidence returned %v", err)
	}
}
