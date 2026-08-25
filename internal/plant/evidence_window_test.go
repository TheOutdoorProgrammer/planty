package plant

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func validRecheck(now time.Time) EvidenceWindow {
	plantID := uuid.New()
	return EvidenceWindow{
		Kind:             WindowRecheck,
		Status:           WindowProposed,
		PlantIDs:         []uuid.UUID{plantID},
		InterventionKind: ObservedMoved,
		Baseline: []EvidenceRef{{
			PlantID: plantID, Kind: EvidencePhoto, ID: uuid.New(), Phase: EvidenceBaseline,
		}},
		Expected: []EvidenceExpectation{{
			PlantID: plantID, Kind: EvidencePhoto, Instruction: "Repeat the same viewpoint.",
		}},
		EarliestReviewAt: now.Add(48 * time.Hour),
		LatestReviewAt:   now.Add(7 * 24 * time.Hour),
		ProposedBy:       SourceAgent,
		ProposedActor:    "daily",
	}
}

func TestVisualRecheckRequiresPhotographsOnBothSides(t *testing.T) {
	now := time.Now().UTC()
	window := validRecheck(now)
	if err := window.ValidProposal(now); err != nil {
		t.Fatalf("valid recheck: %v", err)
	}

	window.Baseline[0].Kind = EvidenceObservation
	if err := window.ValidProposal(now); err == nil || !strings.Contains(err.Error(), "photograph") {
		t.Fatalf("non-visual baseline returned %v", err)
	}
}

func TestReviewBoundsComeFromTheInterventionTemplate(t *testing.T) {
	now := time.Now().UTC()
	window := validRecheck(now)
	window.EarliestReviewAt = now.Add(12 * time.Hour)
	if err := window.ValidProposal(now); err == nil || !strings.Contains(err.Error(), "review bounds") {
		t.Fatalf("moved plant accepted an early review: %v", err)
	}

	window = validRecheck(now)
	window.LatestReviewAt = now.Add(15 * 24 * time.Hour)
	if err := window.ValidProposal(now); err == nil || !strings.Contains(err.Error(), "review bounds") {
		t.Fatalf("moved plant accepted an unbounded review: %v", err)
	}
}

func TestExperimentRequiresOneNamedVariableAndRules(t *testing.T) {
	now := time.Now().UTC()
	window := validRecheck(now)
	window.Kind = WindowExperiment
	window.Experiment = &Experiment{
		Title: "Shelf move", Hypothesis: "The brighter shelf improves new growth.",
		VariableKind: "location", VariableValue: "east shelf",
		HoldConstantRules: []string{"Keep watering behavior unchanged."},
		SuccessCriteria:   []string{"New growth remains upright."},
	}
	if err := window.ValidProposal(now); err != nil {
		t.Fatalf("valid experiment: %v", err)
	}

	window.Experiment.VariableValue = ""
	if err := window.ValidProposal(now); err == nil || !strings.Contains(err.Error(), "variable_value") {
		t.Fatalf("experiment without one variable value returned %v", err)
	}
}

func TestScheduledAutomationCanProposeButCannotStart(t *testing.T) {
	now := time.Now().UTC()
	window := validRecheck(now)
	window.ProposedBy = SourceAutomation
	if err := window.ValidProposal(now); err != nil {
		t.Fatalf("automation proposal: %v", err)
	}
	if err := window.CanStart(SourceAutomation, uuid.New(), now); err == nil {
		t.Fatal("scheduled automation started an evidence window")
	}
	if err := window.CanStart(SourceApp, uuid.New(), now); err != nil {
		t.Fatalf("app could not start proposed window: %v", err)
	}
}

func TestReadyRequiresEveryExpectedEvidenceKindForEveryPlant(t *testing.T) {
	now := time.Now().UTC()
	window := validRecheck(now.Add(-3 * 24 * time.Hour))
	window.Status = WindowActive
	window.EarliestReviewAt = now.Add(-time.Hour)
	window.LatestReviewAt = now.Add(time.Hour)
	window.Expected = append(window.Expected, EvidenceExpectation{
		PlantID: window.PlantIDs[0], Kind: EvidenceReading, Instruction: "Capture the current reading.",
	})

	photo := EvidenceRef{PlantID: window.PlantIDs[0], Kind: EvidencePhoto, ID: uuid.New(), Phase: EvidenceReview}
	if err := window.CanMarkReady([]EvidenceRef{photo}, now); err == nil || !strings.Contains(err.Error(), "reading") {
		t.Fatalf("incomplete review returned %v", err)
	}
	reading := EvidenceRef{PlantID: window.PlantIDs[0], Kind: EvidenceReading, ID: uuid.New(), Phase: EvidenceReview}
	if err := window.CanMarkReady([]EvidenceRef{photo, reading}, now); err != nil {
		t.Fatalf("complete review: %v", err)
	}
}

func TestConfoundedExperimentsCannotClaimSupport(t *testing.T) {
	now := time.Now().UTC()
	window := validRecheck(now)
	window.Kind = WindowExperiment
	window.Status = WindowReady
	window.Experiment = &Experiment{
		Title: "Move", Hypothesis: "A brighter shelf helps.",
		VariableKind: "location", VariableValue: "east shelf",
		HoldConstantRules: []string{"Do not move it again."},
		SuccessCriteria:   []string{"New growth remains upright."},
	}
	window.ConfoundedAt = &now

	if err := window.CanConclude(OutcomeSupported, "The photo looks better."); err == nil {
		t.Fatal("confounded experiment claimed support")
	}
	if err := window.CanConclude(OutcomeInconclusive, "It moved again during the trial."); err != nil {
		t.Fatalf("confounded experiment could not conclude honestly: %v", err)
	}
}
