package store

import (
	"errors"
	"testing"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/google/uuid"
)

func TestIncidentLifecycleRequiresCompleteRunAndPreservesMembers(t *testing.T) {
	s, ctx := testStore(t)
	first := newPlant(t, s, ctx, "Incident first")
	second := newPlant(t, s, ctx, "Incident second")
	run, err := s.StartJudgmentRun(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	firstVerdict, err := s.SaveVerdict(ctx, plant.Verdict{PlantID: first.ID, Action: plant.ActionUrgent, Confidence: 0.8, Evidence: plant.Evidence{SensorSummary: "anomaly"}})
	if err != nil {
		t.Fatal(err)
	}
	secondVerdict, err := s.SaveVerdict(ctx, plant.Verdict{PlantID: second.ID, Action: plant.ActionCheck, Confidence: 0.7, Evidence: plant.Evidence{SensorSummary: "anomaly"}})
	if err != nil {
		t.Fatal(err)
	}
	candidate := plant.IncidentCandidate{
		Factor: plant.FactorLocation, FactorRef: "office", Summary: "Shared factor worth checking",
		Confidence: 0.75, Evidence: plant.IncidentEvidence{RunID: run.ID, VerdictIDs: []uuid.UUID{firstVerdict.ID, secondVerdict.ID}},
		Plants: []plant.IncidentPlant{
			{Plant: first, VerdictID: firstVerdict.ID, Action: plant.ActionUrgent},
			{Plant: second, VerdictID: secondVerdict.ID, Action: plant.ActionCheck},
		},
	}
	if _, _, err := s.UpsertIncidentCandidate(ctx, candidate); !errors.Is(err, plant.ErrInvalid) {
		t.Fatalf("partial run opened an incident: %v", err)
	}
	if err := s.RecordJudgmentPlantResult(ctx, run.ID, JudgmentResultInput{PlantID: first.ID, Succeeded: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordJudgmentPlantResult(ctx, run.ID, JudgmentResultInput{PlantID: second.ID, Succeeded: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteJudgmentRun(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	incident, created, err := s.UpsertIncidentCandidate(ctx, candidate)
	if err != nil || !created || len(incident.Plants) != 2 {
		t.Fatalf("incident=%#v created=%v err=%v", incident, created, err)
	}
	incident, err = s.AcknowledgeIncident(ctx, incident.ID, "Joey")
	if err != nil || incident.Status != plant.IncidentAcknowledged {
		t.Fatalf("acknowledge=%#v err=%v", incident, err)
	}
	incident, err = s.ResolveIncident(ctx, incident.ID, plant.IncidentInconclusive, "Joey", "Inspection did not establish a common cause")
	if err != nil || incident.Status != plant.IncidentResolved || incident.Resolution == nil || *incident.Resolution != plant.IncidentInconclusive {
		t.Fatalf("resolve=%#v err=%v", incident, err)
	}
}
