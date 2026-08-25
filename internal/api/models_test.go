package api_test

import (
	"net/http"
	"testing"
)

func TestTheCatalogueIsListedSmartestFirstWithTheJobsEachCanDo(t *testing.T) {
	h, _, _ := newServer(t)

	rec, body := do(t, h, http.MethodGet, "/v1/models", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("listing models: %d %s", rec.Code, rec.Body.String())
	}

	models, _ := body["models"].([]any)
	if len(models) == 0 {
		t.Fatal("no models were offered")
	}

	last := -1
	for _, entry := range models {
		m, _ := entry.(map[string]any)
		rank, _ := m["rank"].(float64)
		if int(rank) < last {
			t.Fatalf("the catalogue is not smartest first: %v after rank %d", m["ref"], last)
		}
		last = int(rank)

		jobs, _ := m["jobs"].([]any)
		skills, _ := m["skills"].(map[string]any)
		sees, _ := skills["vision"].(bool)
		if _, declared := skills["offered_photos"]; !declared {
			t.Errorf("%v did not declare offered-photo access separately", m["ref"])
		}
		for _, job := range jobs {
			if job == "identify" && !sees {
				t.Errorf("%v is offered for identification but cannot see", m["ref"])
			}
		}
	}
}

func TestEveryJobIsListedAndDefaultsUntilAssigned(t *testing.T) {
	h, _, _ := newServer(t)

	_, body := do(t, h, http.MethodGet, "/v1/model-assignments", nil)
	assignments, _ := body["assignments"].([]any)
	if len(assignments) != 6 {
		t.Fatalf("expected six jobs, got %d", len(assignments))
	}
	for _, entry := range assignments {
		a, _ := entry.(map[string]any)
		if isDefault, _ := a["default"].(bool); !isDefault {
			t.Errorf("%v was not reported as using its default", a["job"])
		}
	}
}

func TestAssigningAndClearingAJob(t *testing.T) {
	h, _, _ := newServer(t)

	rec, body := do(t, h, http.MethodPut, "/v1/model-assignments/assess",
		map[string]string{"provider": "opencode-go", "model": "deepseek-v4-flash"})
	if rec.Code != http.StatusOK {
		t.Fatalf("assigning: %d %s", rec.Code, rec.Body.String())
	}
	if body["ref"] != "opencode-go/deepseek-v4-flash" {
		t.Errorf("the assignment came back as %v", body["ref"])
	}

	_, listed := do(t, h, http.MethodGet, "/v1/model-assignments", nil)
	for _, entry := range listed["assignments"].([]any) {
		a, _ := entry.(map[string]any)
		if a["job"] != "assess" {
			continue
		}
		if isDefault, _ := a["default"].(bool); isDefault {
			t.Error("assess still reports as using its default")
		}
	}

	if rec, _ := do(t, h, http.MethodDelete, "/v1/model-assignments/assess", nil); rec.Code != http.StatusOK {
		t.Fatalf("clearing: %d", rec.Code)
	}
	_, after := do(t, h, http.MethodGet, "/v1/model-assignments", nil)
	for _, entry := range after["assignments"].([]any) {
		a, _ := entry.(map[string]any)
		if a["job"] == "assess" {
			if isDefault, _ := a["default"].(bool); !isDefault {
				t.Error("assess did not return to its default")
			}
		}
	}
}

// The gate is enforced server-side, so a client that ignores the catalogue,
// or a hand-written request, still cannot put a blind model on identification.
func TestAnIncapableModelIsRefusedOverHTTP(t *testing.T) {
	h, _, _ := newServer(t)

	rec, body := do(t, h, http.MethodPut, "/v1/model-assignments/identify",
		map[string]string{"provider": "opencode-go", "model": "deepseek-v4-flash"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("a blind model was accepted for identification: %d", rec.Code)
	}
	if message, _ := body["error"].(string); message == "" {
		t.Error("the refusal carried no explanation")
	}
}

func TestAnUnknownJobIs404(t *testing.T) {
	h, _, _ := newServer(t)

	rec, _ := do(t, h, http.MethodPut, "/v1/model-assignments/invented",
		map[string]string{"provider": "opencode-go", "model": "qwen3.8-max"})
	if rec.Code != http.StatusNotFound {
		t.Errorf("an unknown job returned %d", rec.Code)
	}
}
