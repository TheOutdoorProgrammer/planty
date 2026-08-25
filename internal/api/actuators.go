package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/TheOutdoorProgrammer/planty/internal/job"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/google/uuid"
)

const (
	routeDiscoverActuatorsPending = "GET /v1/home-assistant/actuators"
	routeListActuatorsPending     = "GET /v1/actuators"
	routeRegisterActuatorPending  = "POST /v1/actuators"
	routeUpdateActuatorPending    = "PATCH /v1/actuators/{id}"
	routeDeleteActuatorPending    = "DELETE /v1/actuators/{id}"
	routeActuatorEventsPending    = "GET /v1/actuators/{id}/events"
	routeStartActuatorPending     = "POST /v1/actuators/{id}/start"
	routeStopActuatorPending      = "POST /v1/actuators/{id}/stop"
)

func (s *Server) actuatorControl() job.ActuatorControl {
	return job.ActuatorControl{Store: s.store, HA: s.actuatorHA, Log: s.log}
}

func (s *Server) discoverActuators(w http.ResponseWriter, r *http.Request) {
	if s.homeAssistant == nil {
		s.fail(w, http.StatusServiceUnavailable, errors.New("Home Assistant discovery is not configured"))
		return
	}
	entities, err := s.homeAssistant.Entities(r.Context())
	if err != nil {
		s.fail(w, http.StatusBadGateway, errors.New("Home Assistant discovery failed"))
		return
	}
	entities = filterDiscoveredActuators(entities, r.URL.Query().Get("q"))
	w.Header().Set("Cache-Control", "no-store")
	s.ok(w, http.StatusOK, map[string]any{"entities": entities, "count": len(entities)})
}

func (s *Server) listActuators(w http.ResponseWriter, r *http.Request) {
	actuators, err := s.store.Actuators(r.Context())
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, map[string]any{"actuators": actuators, "count": len(actuators)})
}

func (s *Server) registerActuator(w http.ResponseWriter, r *http.Request) {
	if s.homeAssistant == nil {
		s.fail(w, http.StatusServiceUnavailable, errors.New("Home Assistant discovery is not configured"))
		return
	}
	var request struct {
		EntityID string `json:"entity_id"`
		Name     string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	entities, err := s.homeAssistant.Entities(r.Context())
	if err != nil {
		s.fail(w, http.StatusBadGateway, errors.New("Home Assistant discovery failed"))
		return
	}
	var candidateDomain string
	for _, entity := range filterDiscoveredActuators(entities, "") {
		if entity.EntityID == strings.TrimSpace(request.EntityID) {
			candidateDomain = entity.Domain
			if request.Name == "" {
				request.Name = entity.FriendlyName
			}
			break
		}
	}
	if candidateDomain == "" {
		s.fail(w, http.StatusBadRequest, errors.New("entity_id is not a discovered Home Assistant fan or switch"))
		return
	}
	created, err := s.store.RegisterActuator(r.Context(), plant.Actuator{
		EntityID: strings.TrimSpace(request.EntityID), Name: request.Name, Kind: plant.ActuatorKind(candidateDomain),
	})
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusCreated, created)
}

func (s *Server) updateActuator(w http.ResponseWriter, r *http.Request) {
	id, ok := actuatorID(w, r, s)
	if !ok {
		return
	}
	var request struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	updated, err := s.store.RenameActuator(r.Context(), id, request.Name)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, updated)
}

func (s *Server) deleteActuator(w http.ResponseWriter, r *http.Request) {
	id, ok := actuatorID(w, r, s)
	if !ok {
		return
	}
	if err := s.store.DeleteActuator(r.Context(), id); err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) startActuator(w http.ResponseWriter, r *http.Request) {
	if s.actuatorHA == nil {
		s.fail(w, http.StatusServiceUnavailable, errors.New("Home Assistant actuation is not configured"))
		return
	}
	id, ok := actuatorID(w, r, s)
	if !ok {
		return
	}
	var request struct {
		DurationSeconds int       `json:"duration_seconds"`
		Actor           string    `json:"actor"`
		IdempotencyKey  uuid.UUID `json:"idempotency_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	lease, created, err := s.actuatorControl().Start(r.Context(), id, request.DurationSeconds, request.Actor, plant.SourceApp, request.IdempotencyKey)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	status := http.StatusCreated
	if !created {
		status = http.StatusOK
	}
	s.ok(w, status, lease)
}

func (s *Server) stopActuator(w http.ResponseWriter, r *http.Request) {
	if s.actuatorHA == nil {
		s.fail(w, http.StatusServiceUnavailable, errors.New("Home Assistant actuation is not configured"))
		return
	}
	id, ok := actuatorID(w, r, s)
	if !ok {
		return
	}
	var request struct {
		Actor          string    `json:"actor"`
		IdempotencyKey uuid.UUID `json:"idempotency_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	stopped, err := s.actuatorControl().Stop(r.Context(), id, request.Actor, plant.SourceApp, request.IdempotencyKey)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, map[string]any{"stopped": stopped})
}

func (s *Server) actuatorEvents(w http.ResponseWriter, r *http.Request) {
	id, ok := actuatorID(w, r, s)
	if !ok {
		return
	}
	limit, err := pageLimit(r.URL.Query(), 50)
	if err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	events, err := s.store.ActuatorEvents(r.Context(), id, limit)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, map[string]any{"events": events, "count": len(events)})
}

func actuatorID(w http.ResponseWriter, r *http.Request, s *Server) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.fail(w, http.StatusBadRequest, fmt.Errorf("actuator id must be a UUID: %w", err))
		return uuid.Nil, false
	}
	return id, true
}
