package plant

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestIncidentCandidateRequiresIndependentEvidence(t *testing.T) {
	member := IncidentPlant{Plant: Plant{ID: uuid.New()}, VerdictID: uuid.New()}
	candidate := IncidentCandidate{
		Factor: FactorHAArea, FactorRef: "office", Summary: "Shared factor worth checking",
		Confidence: 0.6, Evidence: IncidentEvidence{RunID: uuid.New(), VerdictIDs: []uuid.UUID{member.VerdictID}}, Plants: []IncidentPlant{member},
	}
	if !errors.Is(candidate.Valid(), ErrInvalid) {
		t.Fatal("one plant without independent system evidence opened an incident")
	}
	candidate.Evidence.SensorLinkIDs = []uuid.UUID{uuid.New()}
	if err := candidate.Valid(); err != nil {
		t.Fatalf("one plant plus an independent environmental signal was rejected: %v", err)
	}
	candidate.Evidence.SensorLinkIDs = nil
	second := IncidentPlant{Plant: Plant{ID: uuid.New()}, VerdictID: uuid.New()}
	candidate.Plants = append(candidate.Plants, second)
	candidate.Evidence.VerdictIDs = append(candidate.Evidence.VerdictIDs, second.VerdictID)
	if err := candidate.Valid(); err != nil {
		t.Fatalf("two independent plants were rejected: %v", err)
	}
}
