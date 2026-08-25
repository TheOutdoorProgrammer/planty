package api

import (
	"encoding/json"
	"net/http"
	"time"

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

type completeReminderRequest struct {
	IdempotencyKey uuid.UUID `json:"idempotency_key"`
	DueAt          time.Time `json:"due_at"`
}

type resolveReminderRequest struct {
	IdempotencyKey uuid.UUID                 `json:"idempotency_key"`
	DueAt          time.Time                 `json:"due_at"`
	Disposition    store.ReminderDisposition `json:"disposition"`
	Note           string                    `json:"note,omitempty"`
}

func (s *Server) resolveReminder(w http.ResponseWriter, r *http.Request) {
	reminderID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	var body resolveReminderRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	resolved, err := s.store.ResolveReminder(r.Context(), store.ReminderResolution{
		IdempotencyKey: body.IdempotencyKey,
		ReminderID:     reminderID,
		DueAt:          body.DueAt,
		Disposition:    body.Disposition,
		Note:           body.Note,
	})
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, resolved)
}

func (s *Server) completeReminder(w http.ResponseWriter, r *http.Request) {
	reminderID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	var body completeReminderRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	completed, err := s.store.CompleteReminder(r.Context(), store.ReminderResolution{
		IdempotencyKey: body.IdempotencyKey,
		ReminderID:     reminderID,
		DueAt:          body.DueAt,
	})
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, completed)
}
