package api_test

import (
	"net/http"
	"testing"
	"time"
)

func apiAwayWindow() (time.Time, time.Time) {
	start := time.Date(2101, 1, 1, 12, 0, 0, 0, time.UTC).
		Add(time.Duration(time.Now().UnixNano()%1_000_000) * time.Second)
	return start, start.Add(72 * time.Hour)
}

func TestAwayPeriodHTTPManagement(t *testing.T) {
	h, _, _ := newServer(t)
	starts, ends := apiAwayWindow()

	created, body := do(t, h, http.MethodPost, "/v1/away", map[string]any{
		"starts_at":      starts,
		"ends_at":        ends,
		"backup_contact": "Sam",
		"note":           "draft",
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create got %d: %s", created.Code, created.Body.String())
	}
	id, _ := body["id"].(string)
	if id == "" {
		t.Fatal("create returned no id")
	}
	defer func() { _, _ = do(t, h, http.MethodDelete, "/v1/away/"+id, nil) }()

	listed, listBody := do(t, h, http.MethodGet, "/v1/away", nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("list got %d: %s", listed.Code, listed.Body.String())
	}
	periods, _ := listBody["away_periods"].([]any)
	var visible bool
	for _, raw := range periods {
		period, _ := raw.(map[string]any)
		if period["id"] == id {
			visible = true
		}
	}
	if !visible {
		t.Fatal("created away period was not listed")
	}

	patched, patchBody := do(t, h, http.MethodPatch, "/v1/away/"+id, map[string]any{
		"backup_contact": "Maya",
		"note":           "corrected",
	})
	if patched.Code != http.StatusOK {
		t.Fatalf("patch got %d: %s", patched.Code, patched.Body.String())
	}
	if patchBody["backup_contact"] != "Maya" || patchBody["note"] != "corrected" {
		t.Fatalf("patch response lost changes: %#v", patchBody)
	}

	deleted, _ := do(t, h, http.MethodDelete, "/v1/away/"+id, nil)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete got %d: %s", deleted.Code, deleted.Body.String())
	}

	listed, listBody = do(t, h, http.MethodGet, "/v1/away", nil)
	periods, _ = listBody["away_periods"].([]any)
	for _, raw := range periods {
		period, _ := raw.(map[string]any)
		if period["id"] == id {
			t.Fatal("cancelled away period was still listed")
		}
	}
}

func TestAwayPeriodHTTPRejectsOverlap(t *testing.T) {
	h, _, _ := newServer(t)
	starts, ends := apiAwayWindow()

	first, body := do(t, h, http.MethodPost, "/v1/away", map[string]any{
		"starts_at": starts,
		"ends_at":   ends,
	})
	if first.Code != http.StatusCreated {
		t.Fatalf("first create got %d: %s", first.Code, first.Body.String())
	}
	id, _ := body["id"].(string)
	defer func() { _, _ = do(t, h, http.MethodDelete, "/v1/away/"+id, nil) }()

	overlap, overlapBody := do(t, h, http.MethodPost, "/v1/away", map[string]any{
		"starts_at": starts.Add(time.Hour),
		"ends_at":   ends.Add(time.Hour),
	})
	if overlap.Code != http.StatusBadRequest {
		t.Fatalf("overlap got %d, want 400: %s", overlap.Code, overlap.Body.String())
	}
	if overlapBody["error"] == nil {
		t.Fatal("overlap response did not explain the conflict")
	}
}
