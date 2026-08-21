package store

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

func TestReliableDigestRefusesPartialRunAllClear(t *testing.T) {
	s, ctx := testStore(t)

	run, err := s.StartJudgmentRun(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordJudgmentResult(ctx, run.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordJudgmentResult(ctx, run.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteJudgmentRun(ctx, run.ID); err != nil {
		t.Fatal(err)
	}

	digest, err := s.ReliableDigest(ctx, plant.StaleAfter)
	if err != nil {
		t.Fatal(err)
	}
	if digest.Checked != 1 || digest.Expected != 2 || digest.Failed != 1 || !digest.RunComplete {
		t.Fatalf("unexpected run truth: %+v", digest)
	}
	if digest.AllClear() {
		t.Fatal("a partial judgment run must never be all clear")
	}
}

func TestLatestUnfinishedJudgmentRunOutranksOlderCompleteRun(t *testing.T) {
	s, ctx := testStore(t)

	old, err := s.StartJudgmentRun(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordJudgmentResult(ctx, old.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteJudgmentRun(ctx, old.ID); err != nil {
		t.Fatal(err)
	}

	latest, err := s.StartJudgmentRun(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordJudgmentResult(ctx, latest.ID, true); err != nil {
		t.Fatal(err)
	}

	digest, err := s.ReliableDigest(ctx, plant.StaleAfter)
	if err != nil {
		t.Fatal(err)
	}
	if digest.RunComplete || digest.Expected != 3 || digest.Checked != 1 {
		t.Fatalf("interrupted latest run was hidden by old success: %+v", digest)
	}
}

func TestCompleteVerdictIsAtomicAndIdempotent(t *testing.T) {
	s, ctx := testStore(t)
	p := newPlant(t, s, ctx, "Atomic completion")
	v, err := s.SaveVerdict(ctx, plant.Verdict{
		PlantID:   p.ID,
		ForDate:   time.Now().UTC(),
		Action:    plant.ActionWater,
		Reasoning: "dry",
	})
	if err != nil {
		t.Fatal(err)
	}

	key := uuid.New()
	completion := VerdictCompletion{
		IdempotencyKey: key,
		VerdictID:      v.ID,
		Kind:           plant.ObservedWatered,
		Body:           "done",
	}
	first, err := s.CompleteVerdict(ctx, completion)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.CompleteVerdict(ctx, completion)
	if err != nil {
		t.Fatalf("retry should replay the committed completion: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("retry duplicated the observation: %s != %s", first.ID, second.ID)
	}

	var observations int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM observations WHERE plant_id = $1 AND kind = 'watered'`, p.ID).
		Scan(&observations); err != nil {
		t.Fatal(err)
	}
	if observations != 1 {
		t.Fatalf("completion wrote %d observations, want 1", observations)
	}

	var acknowledged *time.Time
	if err := s.pool.QueryRow(ctx,
		`SELECT acknowledged_at FROM verdicts WHERE id = $1`, v.ID).Scan(&acknowledged); err != nil {
		t.Fatal(err)
	}
	if acknowledged == nil {
		t.Fatal("care observation committed without acknowledging its verdict")
	}
}

func TestObservationPagesReachOlderHistoryWithoutOverlap(t *testing.T) {
	s, ctx := testStore(t)
	p := newPlant(t, s, ctx, "Paged observations")
	base := time.Now().UTC().Add(-time.Hour)

	for i := 0; i < 3; i++ {
		if _, err := s.AddObservation(ctx, plant.Observation{
			PlantID:    p.ID,
			Kind:       plant.ObservedNote,
			Body:       "page",
			OccurredAt: base.Add(time.Duration(i) * time.Minute),
			Source:     plant.SourceApp,
		}); err != nil {
			t.Fatal(err)
		}
	}

	first, next, err := s.ObservationsPage(ctx, p.ID, nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 || next == nil {
		t.Fatalf("first page len=%d next=%v, want 2 and cursor", len(first), next)
	}
	second, done, err := s.ObservationsPage(ctx, p.ID, next, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 || done != nil {
		t.Fatalf("second page len=%d next=%v, want 1 and no cursor", len(second), done)
	}
	if first[0].ID == second[0].ID || first[1].ID == second[0].ID {
		t.Fatal("page cursor repeated an observation")
	}
}

func TestPhotoPagesReachOlderHistoryWithoutOverlap(t *testing.T) {
	s, ctx := testStore(t)
	p := newPlant(t, s, ctx, "Paged photos")
	base := time.Now().UTC().Add(-time.Hour)

	for i := 0; i < 3; i++ {
		if _, err := s.SavePhoto(ctx, plant.Photo{
			PlantID:     p.ID,
			StorageKey:  uuid.NewString() + ".jpg",
			TakenAt:     base.Add(time.Duration(i) * time.Minute),
			ContentHash: uuid.NewString(),
		}); err != nil {
			t.Fatal(err)
		}
	}

	first, next, err := s.PhotosPage(ctx, p.ID, nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 || next == nil {
		t.Fatalf("first page len=%d next=%v, want 2 and cursor", len(first), next)
	}
	second, done, err := s.PhotosPage(ctx, p.ID, next, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 || done != nil {
		t.Fatalf("second page len=%d next=%v, want 1 and no cursor", len(second), done)
	}
	seen := map[uuid.UUID]bool{first[0].ID: true, first[1].ID: true}
	if seen[second[0].ID] {
		t.Fatal("page cursor repeated a photo")
	}
}
