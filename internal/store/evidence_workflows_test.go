package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/google/uuid"
)

func testPhoto(t *testing.T, s *Store, ctx context.Context, plantID uuid.UUID, label string) plant.Photo {
	t.Helper()
	photo, err := s.SavePhoto(ctx, plant.Photo{
		PlantID: plantID, StorageKey: "evidence-test/" + uuid.NewString() + ".jpg",
		TakenAt: time.Now().UTC(), Caption: label,
	})
	if err != nil {
		t.Fatalf("save %s photo: %v", label, err)
	}
	return photo
}

func testRecheckProposal(t *testing.T, p plant.Plant, baseline plant.Photo) plant.EvidenceWindow {
	t.Helper()
	now := time.Now().UTC()
	return plant.EvidenceWindow{
		Kind:             plant.WindowRecheck,
		PlantIDs:         []uuid.UUID{p.ID},
		InterventionKind: plant.ObservedMoved,
		Baseline: []plant.EvidenceRef{{
			PlantID: p.ID, Kind: plant.EvidencePhoto, ID: baseline.ID, Phase: plant.EvidenceBaseline,
		}},
		Expected: []plant.EvidenceExpectation{{
			PlantID: p.ID, Kind: plant.EvidencePhoto, Instruction: "Repeat the same viewpoint.",
		}},
		EarliestReviewAt: now.Add(48 * time.Hour),
		LatestReviewAt:   now.Add(7 * 24 * time.Hour),
		ProposedBy:       plant.SourceAgent,
		ProposedActor:    "daily",
	}
}

func TestEvidenceWindowRunsOneClosedLoopLifecycle(t *testing.T) {
	s, ctx := testStore(t)
	p := newPlant(t, s, ctx, "Evidence lifecycle")
	baseline := testPhoto(t, s, ctx, p.ID, "baseline")

	window, err := s.ProposeEvidenceWindow(ctx, testRecheckProposal(t, p, baseline))
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if window.Status != plant.WindowProposed || window.Guardrail == nil {
		t.Fatalf("proposal did not include code-owned guardrail: %+v", window)
	}

	intervention, err := s.AddObservation(ctx, plant.Observation{
		PlantID: p.ID, Kind: plant.ObservedMoved, Source: plant.SourceApp,
		Actor: "joey", Body: "Moved to the east shelf.",
	})
	if err != nil {
		t.Fatalf("intervention: %v", err)
	}
	window, err = s.StartEvidenceWindow(ctx, window.ID, intervention.ID, plant.SourceApp, "joey")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if window.Status != plant.WindowActive || window.InterventionObservationID == nil {
		t.Fatalf("window did not start: %+v", window)
	}

	_, err = s.pool.Exec(ctx, `UPDATE evidence_windows
		SET earliest_review_at = now() - interval '1 minute', latest_review_at = now() + interval '1 hour'
		WHERE id = $1`, window.ID)
	if err != nil {
		t.Fatalf("advance review clock: %v", err)
	}
	time.Sleep(time.Millisecond)
	review := testPhoto(t, s, ctx, p.ID, "review")
	window, err = s.MarkEvidenceWindowReady(ctx, window.ID, []plant.EvidenceRef{{
		PlantID: p.ID, Kind: plant.EvidencePhoto, ID: review.ID, Phase: plant.EvidenceReview,
	}})
	if err != nil {
		t.Fatalf("ready: %v", err)
	}
	if window.Status != plant.WindowReady || len(window.Review) != 1 {
		t.Fatalf("review evidence was not linked: %+v", window)
	}

	window, err = s.ConcludeEvidenceWindow(ctx, window.ID, plant.OutcomeImproved,
		"The same-viewpoint photograph shows firmer new growth.", plant.SourceAgent, "reviewer")
	if err != nil {
		t.Fatalf("conclude: %v", err)
	}
	if window.Status != plant.WindowCompleted || window.Outcome == nil || *window.Outcome != plant.OutcomeImproved {
		t.Fatalf("window did not complete: %+v", window)
	}
}

func TestEvidenceWindowRejectsAnInterventionBeforeItsBaseline(t *testing.T) {
	s, ctx := testStore(t)
	p := newPlant(t, s, ctx, "Evidence ordering")
	baseline := testPhoto(t, s, ctx, p.ID, "baseline")
	window, err := s.ProposeEvidenceWindow(ctx, testRecheckProposal(t, p, baseline))
	if err != nil {
		t.Fatal(err)
	}
	intervention, err := s.AddObservation(ctx, plant.Observation{
		PlantID: p.ID, Kind: plant.ObservedMoved, Source: plant.SourceApp,
		Actor: "joey", OccurredAt: baseline.TakenAt.Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.StartEvidenceWindow(ctx, window.ID, intervention.ID, plant.SourceApp, "joey"); err == nil {
		t.Fatal("an intervention before the baseline started the evidence window")
	}
}

func TestEvidenceWindowRejectsEvidenceOwnedByAnotherPlant(t *testing.T) {
	s, ctx := testStore(t)
	first := newPlant(t, s, ctx, "Evidence owner one")
	second := newPlant(t, s, ctx, "Evidence owner two")
	wrong := testPhoto(t, s, ctx, second.ID, "wrong owner")
	proposal := testRecheckProposal(t, first, wrong)
	proposal.Baseline[0].PlantID = first.ID

	if _, err := s.ProposeEvidenceWindow(ctx, proposal); err == nil || !strings.Contains(err.Error(), "does not belong") {
		t.Fatalf("foreign evidence returned %v", err)
	}
}

func TestConflictingObservationMarksExperimentConfoundedWithoutBlockingHistory(t *testing.T) {
	s, ctx := testStore(t)
	p := newPlant(t, s, ctx, "Confounded experiment")
	baseline := testPhoto(t, s, ctx, p.ID, "baseline")
	proposal := testRecheckProposal(t, p, baseline)
	proposal.Kind = plant.WindowExperiment
	proposal.Experiment = &plant.Experiment{
		Title: "Shelf trial", Hypothesis: "The east shelf improves growth.",
		VariableKind: "location", VariableValue: "east shelf",
		HoldConstantRules: []string{"Do not move it again."},
		SuccessCriteria:   []string{"New growth remains upright."},
	}
	window, err := s.ProposeEvidenceWindow(ctx, proposal)
	if err != nil {
		t.Fatalf("propose experiment: %v", err)
	}
	origin, err := s.AddObservation(ctx, plant.Observation{
		PlantID: p.ID, Kind: plant.ObservedMoved, Source: plant.SourceApp, Actor: "joey",
	})
	if err != nil {
		t.Fatalf("origin observation: %v", err)
	}
	if _, err := s.StartEvidenceWindow(ctx, window.ID, origin.ID, plant.SourceApp, "joey"); err != nil {
		t.Fatalf("start experiment: %v", err)
	}

	conflict, err := s.AddObservation(ctx, plant.Observation{
		PlantID: p.ID, Kind: plant.ObservedMoved, Source: plant.SourceApp,
		Actor: "joey", Body: "Moved it again because the shelf was too hot.",
	})
	if err != nil || conflict.ID == uuid.Nil {
		t.Fatalf("truthful conflicting observation was blocked: %+v %v", conflict, err)
	}
	window, err = s.EvidenceWindow(ctx, window.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if window.ConfoundedAt == nil || !strings.Contains(window.ConfoundReason, "moved") {
		t.Fatalf("conflict did not mark the shared window: %+v", window)
	}
}

func TestGuardrailOverrideIsAuditedAndConfoundsTheWindow(t *testing.T) {
	s, ctx := testStore(t)
	p := newPlant(t, s, ctx, "Override audit")
	baseline := testPhoto(t, s, ctx, p.ID, "baseline")
	window, err := s.ProposeEvidenceWindow(ctx, testRecheckProposal(t, p, baseline))
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	origin, _ := s.AddObservation(ctx, plant.Observation{
		PlantID: p.ID, Kind: plant.ObservedMoved, Source: plant.SourceApp, Actor: "joey",
	})
	window, err = s.StartEvidenceWindow(ctx, window.ID, origin.ID, plant.SourceApp, "joey")
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	override, err := s.OverrideGuardrail(ctx, plant.GuardrailOverride{
		WindowID: window.ID, PlantID: p.ID, Kind: plant.ObservedMoved,
		Reason: "The shelf is overheating.", Source: plant.SourceApp, Actor: "joey",
	})
	if err != nil || override.ID == uuid.Nil {
		t.Fatalf("override: %+v %v", override, err)
	}
	window, _ = s.EvidenceWindow(ctx, window.ID)
	if window.ConfoundedAt == nil || len(window.Overrides) != 1 {
		t.Fatalf("override was not auditable: %+v", window)
	}
}

func TestScheduledAutomationCannotStartAProposedWindow(t *testing.T) {
	s, ctx := testStore(t)
	p := newPlant(t, s, ctx, "Automation start gate")
	baseline := testPhoto(t, s, ctx, p.ID, "baseline")
	window, err := s.ProposeEvidenceWindow(ctx, testRecheckProposal(t, p, baseline))
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	origin, _ := s.AddObservation(ctx, plant.Observation{
		PlantID: p.ID, Kind: plant.ObservedMoved, Source: plant.SourceAutomation, Actor: "daily",
	})
	if _, err := s.StartEvidenceWindow(ctx, window.ID, origin.ID, plant.SourceAutomation, "daily"); err == nil {
		t.Fatal("scheduled automation started a proposed window")
	}
}
