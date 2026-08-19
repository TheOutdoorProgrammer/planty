package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

// noteRequest writes or changes a note. On a change, a field left out is left
// alone, which is why both are pointers rather than plain strings.
type noteRequest struct {
	Title *string `json:"title,omitempty"`
	Body  *string `json:"body,omitempty"`
}

// listHouseholdNotes serves what is true of the house rather than of a plant.
func (s *Server) listHouseholdNotes(w http.ResponseWriter, r *http.Request) {
	notes, err := s.store.Notes(r.Context(), uuid.Nil)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, map[string]any{"notes": notes})
}

func (s *Server) addHouseholdNote(w http.ResponseWriter, r *http.Request) {
	var req noteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	if req.Body == nil {
		s.fail(w, http.StatusBadRequest, errors.New("a note needs a body"))
		return
	}

	written, err := s.store.AddNote(r.Context(), plant.Note{
		Title: derefOr(req.Title), Body: *req.Body,
	})
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusCreated, written)
}

func (s *Server) listNotes(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetPlant(r.Context(), r.PathValue("slug"))
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	notes, err := s.store.Notes(r.Context(), p.ID)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, map[string]any{"notes": notes})
}

func (s *Server) addNote(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetPlant(r.Context(), r.PathValue("slug"))
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	var req noteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	if req.Body == nil {
		s.fail(w, http.StatusBadRequest, errors.New("a note needs a body"))
		return
	}

	written, err := s.store.AddNote(r.Context(), plant.Note{
		PlantID: p.ID, Title: derefOr(req.Title), Body: *req.Body,
	})
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusCreated, written)
}

func (s *Server) updateNote(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}

	var req noteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}

	changed, err := s.store.UpdateNote(r.Context(), id, req.Title, req.Body)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, changed)
}

func (s *Server) deleteNote(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.DeleteNote(r.Context(), id); err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func derefOr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
