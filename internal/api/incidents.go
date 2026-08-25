package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/google/uuid"
)

// Root wires these patterns after the parallel OpenAPI integration pass.
func (s *Server) listIncidents(w http.ResponseWriter, r *http.Request) {
	var status plant.IncidentStatus
	if raw := strings.TrimSpace(r.URL.Query().Get("status")); raw != "" {
		value := plant.IncidentStatus(raw)
		switch value {
		case plant.IncidentOpen, plant.IncidentAcknowledged, plant.IncidentResolved:
			status = value
		default:
			s.fail(w, http.StatusBadRequest, errors.New("status must be open, acknowledged, or resolved"))
			return
		}
	}
	incidents, err := s.store.Incidents(r.Context(), status)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, map[string]any{"incidents": incidents, "count": len(incidents)})
}

func (s *Server) getIncident(w http.ResponseWriter, r *http.Request) {
	id, ok := requestedIncidentID(w, r, s)
	if !ok {
		return
	}
	incident, err := s.store.Incident(r.Context(), id)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, incident)
}

func (s *Server) acknowledgeIncident(w http.ResponseWriter, r *http.Request) {
	id, ok := requestedIncidentID(w, r, s)
	if !ok {
		return
	}
	var request struct {
		Actor string `json:"actor"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	incident, err := s.store.AcknowledgeIncident(r.Context(), id, request.Actor)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, incident)
}

func (s *Server) resolveIncident(w http.ResponseWriter, r *http.Request) {
	id, ok := requestedIncidentID(w, r, s)
	if !ok {
		return
	}
	var request struct {
		Outcome    plant.IncidentResolution `json:"outcome"`
		Actor      string                   `json:"actor"`
		Conclusion string                   `json:"conclusion"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	incident, err := s.store.ResolveIncident(r.Context(), id, request.Outcome, request.Actor, request.Conclusion)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, incident)
}

func requestedIncidentID(w http.ResponseWriter, r *http.Request, s *Server) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.fail(w, http.StatusBadRequest, fmt.Errorf("incident id must be a UUID: %w", err))
		return uuid.Nil, false
	}
	return id, true
}
