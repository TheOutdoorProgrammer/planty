package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/google/uuid"
)

// These patterns also live in api/openapi.json. Root regenerates the shared
// contract after parallel slices land; keeping temporary names avoids editing
// the generated file from this slice.
type healthChangeRequest struct {
	Kind           string               `json:"kind"`
	Value          float64              `json:"value"`
	Rationale      string               `json:"rationale"`
	Evidence       plant.HealthEvidence `json:"evidence"`
	Actor          string               `json:"actor,omitempty"`
	IdempotencyKey *uuid.UUID           `json:"idempotency_key,omitempty"`
}

func (s *Server) getPlantHealth(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetPlant(r.Context(), r.PathValue("slug"))
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	limit, err := pageLimit(r.URL.Query(), 50)
	if err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	history, err := s.store.HealthHistory(r.Context(), p.ID, limit)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	var current *plant.HealthEvent
	if len(history) > 0 {
		current = &history[0]
	}
	s.ok(w, http.StatusOK, map[string]any{
		"current": current,
		"events":  history,
		"count":   len(history),
	})
}

func (s *Server) addHealthEvent(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetPlant(r.Context(), r.PathValue("slug"))
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	var request healthChangeRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	change := plant.HealthChange{
		PlantID: p.ID, Rationale: request.Rationale, Evidence: request.Evidence,
		Source: plant.SourceApp, Actor: request.Actor, IdempotencyKey: request.IdempotencyKey,
	}
	switch request.Kind {
	case "baseline":
		change.Baseline = &request.Value
	case "delta":
		change.Delta = &request.Value
	default:
		s.fail(w, http.StatusBadRequest, errors.New("kind must be baseline or delta"))
		return
	}
	event, inserted, err := s.store.RecordHealth(r.Context(), change)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	status := http.StatusCreated
	if !inserted {
		status = http.StatusOK
	}
	s.ok(w, status, event)
}
