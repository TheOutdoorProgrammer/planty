package api_test

import (
	"net/http"
	"testing"
)

func TestEvidenceCoverageNamesOneHonestNextInput(t *testing.T) {
	h, _, _ := newServer(t)
	slug := createPlant(t, h, map[string]any{
		"common_name": "Coverage subject", "slug": unique("coverage"),
	})

	rec, body := do(t, h, http.MethodGet, "/v1/evidence-coverage", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("coverage = %d %#v", rec.Code, body)
	}
	plants, _ := body["plants"].([]any)
	var found map[string]any
	for _, raw := range plants {
		item := raw.(map[string]any)
		subject := item["plant"].(map[string]any)
		if subject["slug"] == slug {
			found = item
			break
		}
	}
	if found == nil {
		t.Fatalf("coverage omitted %s: %#v", slug, body)
	}
	if found["botanical_known"] != false || found["next_best_input"] != "Confirm the botanical identity" {
		t.Fatalf("coverage invented confidence: %#v", found)
	}
}
