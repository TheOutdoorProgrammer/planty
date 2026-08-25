package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/TheOutdoorProgrammer/planty/internal/judge"
)

func TestEveryJobPromptIsListedAndDefaultsToNoOverlay(t *testing.T) {
	h, db, ctx := newServer(t)
	for _, job := range judge.Jobs {
		_ = db.ClearPromptInstruction(ctx, job)
	}

	rec, body := do(t, h, http.MethodGet, "/v1/prompt-instructions", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("listing prompt instructions: %d %s", rec.Code, rec.Body.String())
	}
	instructions, _ := body["instructions"].([]any)
	if len(instructions) != len(judge.Jobs) {
		t.Fatalf("listed %d jobs, want %d", len(instructions), len(judge.Jobs))
	}
	for _, entry := range instructions {
		instruction, _ := entry.(map[string]any)
		if instruction["instructions"] != "" {
			t.Errorf("%v had unexpected editable instructions", instruction["job"])
		}
	}
}

func TestPromptInstructionsCanBeSetReviewedAndClearedOverHTTP(t *testing.T) {
	h, db, ctx := newServer(t)
	_ = db.ClearPromptInstruction(ctx, judge.JobAssess)
	t.Cleanup(func() { _ = db.ClearPromptInstruction(ctx, judge.JobAssess) })

	rec, saved := do(t, h, http.MethodPut, "/v1/prompt-instructions/assess",
		map[string]string{"instructions": "  Mention meaningful changes since the prior photo.  "})
	if rec.Code != http.StatusOK {
		t.Fatalf("setting prompt instructions: %d %s", rec.Code, rec.Body.String())
	}
	if saved["instructions"] != "Mention meaningful changes since the prior photo." {
		t.Fatalf("unexpected saved view: %v", saved)
	}
	if saved["updated_at"] == nil {
		t.Fatal("saved view omitted its update time")
	}

	_, listed := do(t, h, http.MethodGet, "/v1/prompt-instructions", nil)
	found := false
	for _, entry := range listed["instructions"].([]any) {
		instruction := entry.(map[string]any)
		if instruction["job"] == "assess" {
			found = instruction["instructions"] == saved["instructions"]
		}
	}
	if !found {
		t.Fatalf("list did not return the saved overlay: %v", listed)
	}

	rec, cleared := do(t, h, http.MethodDelete, "/v1/prompt-instructions/assess", nil)
	if rec.Code != http.StatusOK || cleared["instructions"] != "" {
		t.Fatalf("clearing returned %d %v", rec.Code, cleared)
	}
}

func TestPromptInstructionHTTPValidation(t *testing.T) {
	h, _, _ := newServer(t)

	for name, test := range map[string]struct {
		path         string
		instructions string
		status       int
	}{
		"unknown job": {"/v1/prompt-instructions/invented", "Do something.", http.StatusNotFound},
		"blank":       {"/v1/prompt-instructions/assess", "  ", http.StatusBadRequest},
		"oversized":   {"/v1/prompt-instructions/assess", strings.Repeat("x", 12_001), http.StatusBadRequest},
	} {
		t.Run(name, func(t *testing.T) {
			rec, _ := do(t, h, http.MethodPut, test.path, map[string]string{"instructions": test.instructions})
			if rec.Code != test.status {
				t.Errorf("got %d, want %d: %s", rec.Code, test.status, rec.Body.String())
			}
		})
	}
}
