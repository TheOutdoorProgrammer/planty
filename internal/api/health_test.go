package api_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestHealthAPIKeepsUnknownSeparateAndReplaysWrites(t *testing.T) {
	h, _, _ := newServer(t)
	slug := createPlant(t, h, map[string]any{
		"common_name": "Health API subject", "slug": unique("health-api"),
	})

	rec, unknown := do(t, h, http.MethodGet, "/v1/plants/"+slug+"/health", nil)
	if rec.Code != http.StatusOK || unknown["current"] != nil || unknown["count"] != float64(0) {
		t.Fatalf("unknown health = %d %#v", rec.Code, unknown)
	}

	key := uuid.NewString()
	body := map[string]any{
		"kind": "baseline", "value": 76, "rationale": "initial inspection",
		"evidence": map[string]any{"summary": "upright leaves and active growth"},
		"actor":    "Joey", "idempotency_key": key,
	}
	created, event := do(t, h, http.MethodPost, "/v1/plants/"+slug+"/health-events", body)
	if created.Code != http.StatusCreated || event["score"] != float64(76) || event["source"] != "app" {
		t.Fatalf("created health = %d %#v", created.Code, event)
	}
	replayed, again := do(t, h, http.MethodPost, "/v1/plants/"+slug+"/health-events", body)
	if replayed.Code != http.StatusOK || again["id"] != event["id"] {
		t.Fatalf("replayed health = %d %#v", replayed.Code, again)
	}

	listed, history := do(t, h, http.MethodGet, "/v1/plants/"+slug+"/health", nil)
	if listed.Code != http.StatusOK || history["count"] != float64(1) {
		t.Fatalf("health history = %d %#v", listed.Code, history)
	}
}

func TestHealthAPIRejectsDeltaBeforeBaseline(t *testing.T) {
	h, _, _ := newServer(t)
	slug := createPlant(t, h, map[string]any{
		"common_name": "Unknown health subject", "slug": unique("unknown-health"),
	})
	rec, body := do(t, h, http.MethodPost, "/v1/plants/"+slug+"/health-events", map[string]any{
		"kind": "delta", "value": -5, "rationale": "drooping",
		"evidence": map[string]any{"summary": "new droop"},
	})
	if rec.Code != http.StatusBadRequest || body["code"] != "bad_request" {
		t.Fatalf("delta before baseline = %d %#v", rec.Code, body)
	}
}

func TestHealthAPINotFoundUsesPublic404(t *testing.T) {
	h, _, _ := newServer(t)
	rec, body := do(t, h, http.MethodGet, "/v1/plants/no-such-plant/health", nil)
	if rec.Code != http.StatusNotFound || body["code"] != "not_found" {
		t.Fatalf("missing plant = %d %#v", rec.Code, body)
	}
}
