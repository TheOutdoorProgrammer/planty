package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

type awayPatch struct {
	StartsAt      *time.Time `json:"starts_at"`
	EndsAt        *time.Time `json:"ends_at"`
	BackupContact *string    `json:"backup_contact"`
	BackupNotify  *string    `json:"backup_notify"`
	Note          *string    `json:"note"`
}

func (s *Server) listAway(w http.ResponseWriter, r *http.Request) {
	includePast := false
	if raw := r.URL.Query().Get("include_past"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			s.fail(w, http.StatusBadRequest, errors.New("include_past must be true or false"))
			return
		}
		includePast = parsed
	}

	periods, err := s.store.AwayPeriods(r.Context(), includePast)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, map[string]any{
		"away_periods": periods,
		"count":        len(periods),
	})
}

func (s *Server) updateAway(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}

	current, err := s.store.AwayPeriod(r.Context(), id)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	var patch awayPatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}

	changed := false
	if patch.StartsAt != nil {
		current.StartsAt = *patch.StartsAt
		changed = true
	}
	if patch.EndsAt != nil {
		current.EndsAt = *patch.EndsAt
		changed = true
	}
	if patch.BackupContact != nil {
		current.BackupContact = *patch.BackupContact
		changed = true
	}
	if patch.BackupNotify != nil {
		current.BackupNotify = *patch.BackupNotify
		changed = true
	}
	if patch.Note != nil {
		current.Note = *patch.Note
		changed = true
	}
	if !changed {
		s.fail(w, http.StatusBadRequest, errors.New("away period patch changes nothing"))
		return
	}

	updated, err := s.store.UpdateAway(r.Context(), id, current)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, updated)
}

func (s *Server) cancelAway(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.DeleteAway(r.Context(), id); err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Keep the plant package in this file's dependency set explicit: malformed
// time windows are classified as plant.ErrInvalid by the store and mapped to
// HTTP 400 by Server.fail.
var _ = plant.ErrInvalid
