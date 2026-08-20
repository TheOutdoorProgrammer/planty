package api

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

type completeVerdictRequest struct {
	IdempotencyKey uuid.UUID             `json:"idempotency_key"`
	Kind           plant.ObservationKind `json:"kind"`
	Body           string                `json:"body,omitempty"`
}

func (s *Server) completeVerdict(w http.ResponseWriter, r *http.Request) {
	verdictID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	var body completeVerdictRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	completed, err := s.store.CompleteVerdict(r.Context(), store.VerdictCompletion{
		IdempotencyKey: body.IdempotencyKey,
		VerdictID:      verdictID,
		Kind:           body.Kind,
		Body:           body.Body,
	})
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, completed)
}
