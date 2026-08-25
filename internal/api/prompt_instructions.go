package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/judge"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

// These route constants are temporary names until contract generation is run
// for the OpenAPI additions. Keeping them local avoids editing generated code.
const (
	routeListPromptInstructionsPending = "GET /v1/prompt-instructions"
	routeSetPromptInstructionPending   = "PUT /v1/prompt-instructions/{job}"
	routeClearPromptInstructionPending = "DELETE /v1/prompt-instructions/{job}"
)

type promptInstructionView struct {
	Job          string     `json:"job"`
	Instructions string     `json:"instructions"`
	UpdatedAt    *time.Time `json:"updated_at,omitempty"`
}

func (s *Server) listPromptInstructions(w http.ResponseWriter, r *http.Request) {
	stored, err := s.store.PromptInstructions(r.Context())
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	configured := make(map[judge.Job]store.PromptInstruction, len(stored))
	for _, instruction := range stored {
		configured[instruction.Job] = instruction
	}

	out := make([]promptInstructionView, 0, len(judge.Jobs))
	for _, job := range judge.Jobs {
		view := promptInstructionView{Job: string(job)}
		if instruction, ok := configured[job]; ok {
			view.Instructions = instruction.Instructions
			view.UpdatedAt = &instruction.UpdatedAt
		}
		out = append(out, view)
	}
	s.ok(w, http.StatusOK, map[string]any{"instructions": out})
}

type promptInstructionRequest struct {
	Instructions string `json:"instructions"`
}

func (s *Server) setPromptInstruction(w http.ResponseWriter, r *http.Request) {
	job, ok := requestedJob(r)
	if !ok {
		s.fail(w, http.StatusNotFound, fmt.Errorf("there is no job %q", r.PathValue("job")))
		return
	}

	var request promptInstructionRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.fail(w, http.StatusBadRequest, fmt.Errorf("decode prompt instructions: %w", err))
		return
	}

	saved, err := s.store.SetPromptInstruction(r.Context(), store.PromptInstruction{
		Job: job, Instructions: strings.TrimSpace(request.Instructions),
	})
	if err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	s.ok(w, http.StatusOK, promptInstructionView{
		Job: string(job), Instructions: saved.Instructions, UpdatedAt: &saved.UpdatedAt,
	})
}

func (s *Server) clearPromptInstruction(w http.ResponseWriter, r *http.Request) {
	job, ok := requestedJob(r)
	if !ok {
		s.fail(w, http.StatusNotFound, fmt.Errorf("there is no job %q", r.PathValue("job")))
		return
	}
	if err := s.store.ClearPromptInstruction(r.Context(), job); err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, promptInstructionView{Job: string(job)})
}
