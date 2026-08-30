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
	if s.homeAssistant != nil {
		entities, stateErr := s.homeAssistant.Entities(r.Context())
		if stateErr != nil {
			s.log.Warn("listing actuators without Home Assistant state", "error", stateErr)
		} else {
			states := make(map[string]string, len(entities))
			for _, entity := range entities {
				states[entity.EntityID] = entity.State
			}
			for i := range actuators {
				actuators[i].CurrentState = states[actuators[i].EntityID]
			}
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	s.ok(w, http.StatusOK, map[string]any{"actuators": actuators, "count": len(actuators)})
}

func (s *Server) registerActuator(w http.ResponseWriter, r *http.Request) {
	if s.homeAssistant == nil {
		s.fail(w, http.StatusServiceUnavailable, errors.New("Home Assistant discovery is not configured"))
		return
	}
	var request struct {
		EntityID string      `json:"entity_id"`
		Name     string      `json:"name"`
		PlantIDs []uuid.UUID `json:"plant_ids"`
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
	var candidateState string
	for _, entity := range filterDiscoveredActuators(entities, "") {
		if entity.EntityID == strings.TrimSpace(request.EntityID) {
			candidateDomain = entity.Domain
			candidateState = entity.State
			if request.Name == "" {
				request.Name = entity.FriendlyName
			}
			break
		}
	}
	if candidateDomain == "" {
		s.fail(w, http.StatusBadRequest, errors.New("entity_id is not a discovered Home Assistant fan, switch, or light"))
		return
	}
	created, err := s.store.RegisterActuator(r.Context(), plant.Actuator{
		EntityID: strings.TrimSpace(request.EntityID), Name: request.Name,
		Kind: plant.ActuatorKind(candidateDomain), PlantIDs: request.PlantIDs,
	})
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	created.CurrentState = candidateState
	s.ok(w, http.StatusCreated, created)
}

func (s *Server) setActuatorState(w http.ResponseWriter, r *http.Request) {
	if s.actuatorHA == nil {
		s.fail(w, http.StatusServiceUnavailable, errors.New("Home Assistant actuation is not configured"))
		return
	}
	id, ok := actuatorID(w, r, s)
	if !ok {
		return
	}
	var request struct {
		On     bool         `json:"on"`
		Actor  string       `json:"actor"`
		Source plant.Source `json:"source,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	if err := s.actuatorControl().SetLight(r.Context(), id, request.On, request.Actor, sourceOrApp(request.Source)); err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, map[string]any{"on": request.On})
}

func (s *Server) setLightSchedule(w http.ResponseWriter, r *http.Request) {
	id, ok := actuatorID(w, r, s)
	if !ok {
		return
	}
	var request struct {
		StartMinute int          `json:"start_minute"`
		EndMinute   int          `json:"end_minute"`
		Timezone    string       `json:"timezone"`
		Enabled     bool         `json:"enabled"`
		Actor       string       `json:"actor"`
		Source      plant.Source `json:"source,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	schedule, err := s.store.SetLightSchedule(r.Context(), plant.LightSchedule{
		ActuatorID: id, StartMinute: request.StartMinute, EndMinute: request.EndMinute,
		Timezone: request.Timezone, Enabled: request.Enabled,
	}, request.Actor, sourceOrApp(request.Source))
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, schedule)
}

func (s *Server) deleteLightSchedule(w http.ResponseWriter, r *http.Request) {
	id, ok := actuatorID(w, r, s)
	if !ok {
		return
	}
	if err := s.store.DeleteLightSchedule(r.Context(), id, "owner", plant.SourceApp); err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) updateActuator(w http.ResponseWriter, r *http.Request) {
	id, ok := actuatorID(w, r, s)
	if !ok {
		return
	}
	var request struct {
		Name                 string      `json:"name"`
		PlantIDs             []uuid.UUID `json:"plant_ids"`
		PolicyControlEnabled bool        `json:"policy_control_enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	updated, err := s.store.UpdateActuator(r.Context(), id, request.Name, request.PlantIDs,
		request.PolicyControlEnabled)
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
		DurationSeconds int          `json:"duration_seconds"`
		Actor           string       `json:"actor"`
		Source          plant.Source `json:"source,omitempty"`
		IdempotencyKey  uuid.UUID    `json:"idempotency_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	lease, created, err := s.actuatorControl().Start(r.Context(), id, request.DurationSeconds, request.Actor, sourceOrApp(request.Source), request.IdempotencyKey)
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
		Actor          string       `json:"actor"`
		Source         plant.Source `json:"source,omitempty"`
		IdempotencyKey uuid.UUID    `json:"idempotency_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	stopped, err := s.actuatorControl().Stop(r.Context(), id, request.Actor, sourceOrApp(request.Source), request.IdempotencyKey)
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
