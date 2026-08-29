package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TheOutdoorProgrammer/planty/internal/pgtest"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
	"github.com/google/uuid"
)

func TestIncidentHandlersListAcknowledgeAndResolve(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, pgtest.DSN(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	makePlant := func(name string) plant.Plant {
		p, err := db.CreatePlant(ctx, plant.Plant{CommonName: name, Domain: plant.DomainHouseplant,
			Steward: plant.StewardSelf, Status: plant.StatusAlive, Location: "office",
			Accessibility: plant.AccessEasy, WateringMethod: plant.WateringHand})
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	first, second := makePlant("API incident first"), makePlant("API incident second")
	run, err := db.StartJudgmentRun(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	firstVerdict, err := db.SaveVerdict(ctx, plant.Verdict{PlantID: first.ID, Action: plant.ActionUrgent, Confidence: 0.8, Evidence: plant.Evidence{SensorSummary: "anomaly"}})
	if err != nil {
		t.Fatal(err)
	}
	secondVerdict, err := db.SaveVerdict(ctx, plant.Verdict{PlantID: second.ID, Action: plant.ActionCheck, Confidence: 0.7, Evidence: plant.Evidence{SensorSummary: "anomaly"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.RecordJudgmentPlantResult(ctx, run.ID, store.JudgmentResultInput{PlantID: first.ID, Succeeded: true}); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordJudgmentPlantResult(ctx, run.ID, store.JudgmentResultInput{PlantID: second.ID, Succeeded: true}); err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteJudgmentRun(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	incident, _, err := db.UpsertIncidentCandidate(ctx, plant.IncidentCandidate{
		Factor: plant.FactorLocation, FactorRef: "office", Summary: "Shared factor worth checking",
		Reason:     "Both plant agents reported new leaf damage in the office.",
		Confidence: 0.7, Evidence: plant.IncidentEvidence{RunID: run.ID, VerdictIDs: []uuid.UUID{firstVerdict.ID, secondVerdict.ID}},
		Plants: []plant.IncidentPlant{{Plant: first, VerdictID: firstVerdict.ID}, {Plant: second, VerdictID: secondVerdict.ID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := New(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	list := httptest.NewRecorder()
	server.listIncidents(list, httptest.NewRequest(http.MethodGet, "/v1/incidents?status=open", nil))
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	if !bytes.Contains(list.Body.Bytes(), []byte(`"reason":"Both plant agents reported new leaf damage in the office."`)) {
		t.Fatalf("list omitted incident reason: %s", list.Body.String())
	}
	call := func(handler func(http.ResponseWriter, *http.Request), path, id, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
		req.SetPathValue("id", id)
		rec := httptest.NewRecorder()
		handler(rec, req)
		return rec
	}
	ack := call(server.acknowledgeIncident, "/v1/incidents/"+incident.ID.String()+"/acknowledge", incident.ID.String(), `{"actor":"Joey"}`)
	if ack.Code != http.StatusOK {
		t.Fatalf("ack status=%d body=%s", ack.Code, ack.Body.String())
	}
	resolveBody, _ := json.Marshal(map[string]any{"actor": "Joey", "outcome": "unrelated", "conclusion": "separate issues"})
	resolved := call(server.resolveIncident, "/v1/incidents/"+incident.ID.String()+"/resolve", incident.ID.String(), string(resolveBody))
	if resolved.Code != http.StatusOK {
		t.Fatalf("resolve status=%d body=%s", resolved.Code, resolved.Body.String())
	}
}
