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
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/pgtest"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
	"github.com/google/uuid"
)

func evidenceWorkflowServer(t *testing.T) (*http.ServeMux, *store.Store, context.Context) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, pgtest.DSN(t))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(db.Close)
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	server := New(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	mux := http.NewServeMux()
	server.registerEvidenceWorkflowRoutes(mux)
	return mux, db, ctx
}

func evidenceRequest(t *testing.T, h http.Handler, method, path string, body any, out any) *httptest.ResponseRecorder {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if out != nil {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			t.Fatalf("decode %d %s: %v", rec.Code, rec.Body.String(), err)
		}
	}
	return rec
}

func evidencePlant(t *testing.T, db *store.Store, ctx context.Context, name string) (plant.Plant, plant.Photo) {
	t.Helper()
	p, err := db.CreatePlant(ctx, plant.Plant{
		CommonName: name, Slug: store.Slugify(name) + "-" + uuid.NewString(),
		Domain: plant.DomainHouseplant, Status: plant.StatusAlive,
		Steward: plant.StewardSelf, Accessibility: plant.AccessEasy,
		WateringMethod: plant.WateringHand,
	})
	if err != nil {
		t.Fatalf("create plant: %v", err)
	}
	photo, err := db.SavePhoto(ctx, plant.Photo{
		PlantID: p.ID, StorageKey: "api-evidence/" + uuid.NewString() + ".jpg",
		TakenAt: time.Now().UTC(), Caption: "baseline",
	})
	if err != nil {
		t.Fatalf("save photo: %v", err)
	}
	return p, photo
}

func recheckBody(p plant.Plant, photo plant.Photo) map[string]any {
	now := time.Now().UTC()
	return map[string]any{
		"intervention_kind":  "moved",
		"baseline":           []map[string]any{{"kind": "photo", "id": photo.ID}},
		"expected":           []map[string]any{{"kind": "photo", "instruction": "Repeat the same viewpoint."}},
		"earliest_review_at": now.Add(48 * time.Hour),
		"latest_review_at":   now.Add(7 * 24 * time.Hour),
		"actor":              "joey",
	}
}

func TestEvidenceWorkflowAPIProposesAndGetsAVisualRecheck(t *testing.T) {
	h, db, ctx := evidenceWorkflowServer(t)
	p, photo := evidencePlant(t, db, ctx, "API recheck")

	var proposed plant.EvidenceWindow
	rec := evidenceRequest(t, h, http.MethodPost, "/v1/plants/"+p.Slug+"/rechecks", recheckBody(p, photo), &proposed)
	if rec.Code != http.StatusCreated {
		t.Fatalf("propose returned %d: %s", rec.Code, rec.Body.String())
	}
	if proposed.Kind != plant.WindowRecheck || proposed.Status != plant.WindowProposed || proposed.Guardrail == nil {
		t.Fatalf("unexpected proposal: %+v", proposed)
	}
	if len(proposed.Guardrail.RedFlags) == 0 || len(proposed.Guardrail.ConflictingKinds) == 0 {
		t.Fatalf("API omitted code-owned guardrail: %+v", proposed.Guardrail)
	}

	var fetched plant.EvidenceWindow
	rec = evidenceRequest(t, h, http.MethodGet, "/v1/evidence-windows/"+proposed.ID.String(), nil, &fetched)
	if rec.Code != http.StatusOK || fetched.ID != proposed.ID {
		t.Fatalf("get returned %d %+v", rec.Code, fetched)
	}
}

func TestExperimentAPIRejectsMissingVariableAndListsAValidProposal(t *testing.T) {
	h, db, ctx := evidenceWorkflowServer(t)
	p, photo := evidencePlant(t, db, ctx, "API experiment")
	now := time.Now().UTC()
	body := map[string]any{
		"plant_ids": []uuid.UUID{p.ID}, "intervention_kind": "moved",
		"baseline":           []map[string]any{{"plant_id": p.ID, "kind": "photo", "id": photo.ID}},
		"expected":           []map[string]any{{"plant_id": p.ID, "kind": "photo", "instruction": "Repeat the same viewpoint."}},
		"earliest_review_at": now.Add(48 * time.Hour), "latest_review_at": now.Add(7 * 24 * time.Hour),
		"actor": "joey", "title": "Shelf trial", "hypothesis": "The east shelf improves growth.",
		"variable_kind": "location", "variable_value": "",
		"hold_constant_rules": []string{"Keep watering behavior unchanged."},
		"success_criteria":    []string{"New growth remains upright."},
	}
	rec := evidenceRequest(t, h, http.MethodPost, "/v1/experiments", body, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing variable returned %d: %s", rec.Code, rec.Body.String())
	}

	body["variable_value"] = "east shelf"
	var proposed plant.EvidenceWindow
	rec = evidenceRequest(t, h, http.MethodPost, "/v1/experiments", body, &proposed)
	if rec.Code != http.StatusCreated || proposed.Experiment == nil {
		t.Fatalf("valid experiment returned %d: %s", rec.Code, rec.Body.String())
	}
	var listed struct {
		Experiments []plant.EvidenceWindow `json:"experiments"`
		Count       int                    `json:"count"`
	}
	rec = evidenceRequest(t, h, http.MethodGet, "/v1/experiments", nil, &listed)
	if rec.Code != http.StatusOK || listed.Count < 1 {
		t.Fatalf("list returned %d %+v", rec.Code, listed)
	}
}

func TestGuardrailAPIStartsListsAndAuditsAnOverride(t *testing.T) {
	h, db, ctx := evidenceWorkflowServer(t)
	p, photo := evidencePlant(t, db, ctx, "API guardrail")
	var proposed plant.EvidenceWindow
	evidenceRequest(t, h, http.MethodPost, "/v1/plants/"+p.Slug+"/rechecks", recheckBody(p, photo), &proposed)
	origin, err := db.AddObservation(ctx, plant.Observation{
		PlantID: p.ID, Kind: plant.ObservedMoved, Source: plant.SourceApp, Actor: "joey",
	})
	if err != nil {
		t.Fatalf("origin: %v", err)
	}
	var started plant.EvidenceWindow
	rec := evidenceRequest(t, h, http.MethodPost, "/v1/evidence-windows/"+proposed.ID.String()+"/start",
		map[string]any{"observation_id": origin.ID, "actor": "joey"}, &started)
	if rec.Code != http.StatusOK || started.Status != plant.WindowActive {
		t.Fatalf("start returned %d: %s", rec.Code, rec.Body.String())
	}

	var guardrails struct {
		Guardrails []plant.EvidenceWindow `json:"guardrails"`
		Count      int                    `json:"count"`
	}
	rec = evidenceRequest(t, h, http.MethodGet, "/v1/plants/"+p.Slug+"/guardrails", nil, &guardrails)
	if rec.Code != http.StatusOK || guardrails.Count != 1 {
		t.Fatalf("guardrails returned %d %+v", rec.Code, guardrails)
	}

	var override plant.GuardrailOverride
	rec = evidenceRequest(t, h, http.MethodPost, "/v1/guardrails/"+proposed.ID.String()+"/override",
		map[string]any{
			"plant_id": p.ID, "kind": "moved", "reason": "The shelf is overheating.", "actor": "joey",
		}, &override)
	if rec.Code != http.StatusCreated || override.ID == uuid.Nil {
		t.Fatalf("override returned %d: %s", rec.Code, rec.Body.String())
	}
}
