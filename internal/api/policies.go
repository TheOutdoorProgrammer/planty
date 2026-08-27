package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/TheOutdoorProgrammer/planty/internal/job"
	"github.com/TheOutdoorProgrammer/planty/internal/policy"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
	"github.com/google/uuid"
)

type policyRequest struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Source      string      `json:"source"`
	Mode        policy.Mode `json:"mode"`
	Enabled     bool        `json:"enabled"`
}

func (request policyRequest) policy() policy.Policy {
	return policy.Policy{Name: request.Name, Description: request.Description,
		Source: request.Source, Mode: request.Mode, Enabled: request.Enabled}
}

func (s *Server) listPolicies(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.Policies(r.Context())
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, map[string]any{"policies": items, "count": len(items)})
}

func (s *Server) getPolicy(w http.ResponseWriter, r *http.Request) {
	id, ok := policyID(w, r, s)
	if !ok {
		return
	}
	item, err := s.store.Policy(r.Context(), id)
	if err != nil {
		s.fail(w, policyStoreStatus(err), err)
		return
	}
	s.ok(w, http.StatusOK, item)
}

func (s *Server) createPolicy(w http.ResponseWriter, r *http.Request) {
	request, ok := decodePolicyRequest(w, r, s)
	if !ok {
		return
	}
	item := request.policy()
	if err := (policy.Engine{}).Compile(r.Context(), item.Source); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	created, err := s.store.CreatePolicy(r.Context(), item)
	if err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	s.ok(w, http.StatusCreated, created)
}

func (s *Server) updatePolicy(w http.ResponseWriter, r *http.Request) {
	id, ok := policyID(w, r, s)
	if !ok {
		return
	}
	request, ok := decodePolicyRequest(w, r, s)
	if !ok {
		return
	}
	item := request.policy()
	if err := (policy.Engine{}).Compile(r.Context(), item.Source); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	updated, err := s.store.UpdatePolicy(r.Context(), id, item)
	if err != nil {
		s.fail(w, policyStoreStatus(err), err)
		return
	}
	s.ok(w, http.StatusOK, updated)
}

func (s *Server) deletePolicy(w http.ResponseWriter, r *http.Request) {
	id, ok := policyID(w, r, s)
	if !ok {
		return
	}
	if err := s.store.ArchivePolicy(r.Context(), id); err != nil {
		s.fail(w, policyStoreStatus(err), err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type policyEvaluationRequest struct {
	PlantSlug string `json:"plant_slug"`
}

type policyPreviewRequest struct {
	policyRequest
	PlantSlug string `json:"plant_slug"`
}

func (s *Server) previewPolicy(w http.ResponseWriter, r *http.Request) {
	var request policyPreviewRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		s.fail(w, http.StatusBadRequest, fmt.Errorf("decode policy preview: %w", err))
		return
	}
	item := request.policy()
	if err := item.Valid(); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	subject, err := s.store.GetPlant(r.Context(), strings.TrimSpace(request.PlantSlug))
	if err != nil {
		s.fail(w, policyStoreStatus(err), err)
		return
	}
	runner := job.PolicyRunner{Store: s.store, Engine: policy.Engine{}}
	if s.policyRunner != nil {
		runner = *s.policyRunner
	}
	input, err := runner.BuildInput(r.Context(), subject, policy.TriggerPreview)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	decision, duration, err := runner.Preview(r.Context(), item, input)
	if err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	s.ok(w, http.StatusOK, map[string]any{
		"input": input, "decision": decision,
		"duration_ms": float64(duration.Microseconds()) / 1000,
	})
}

func (s *Server) evaluatePolicy(w http.ResponseWriter, r *http.Request) {
	if s.policyRunner == nil {
		s.fail(w, http.StatusServiceUnavailable, errors.New("policy enforcement is not configured"))
		return
	}
	id, ok := policyID(w, r, s)
	if !ok {
		return
	}
	var request policyEvaluationRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	item, err := s.store.Policy(r.Context(), id)
	if err != nil {
		s.fail(w, policyStoreStatus(err), err)
		return
	}
	subject, err := s.store.GetPlant(r.Context(), strings.TrimSpace(request.PlantSlug))
	if err != nil {
		s.fail(w, policyStoreStatus(err), err)
		return
	}
	input, err := s.policyRunner.BuildInput(r.Context(), subject, policy.TriggerManual)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	evaluation, created, err := s.policyRunner.Evaluate(r.Context(), item, input)
	if err != nil && evaluation.ID == uuid.Nil {
		s.fail(w, http.StatusBadGateway, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	s.ok(w, status, evaluation)
}

func (s *Server) listPolicyEvaluations(w http.ResponseWriter, r *http.Request) {
	limit, err := pageLimit(r.URL.Query(), 50)
	if err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	var plantID *uuid.UUID
	if raw := strings.TrimSpace(r.URL.Query().Get("plant_id")); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			s.fail(w, http.StatusBadRequest, fmt.Errorf("plant_id must be a UUID: %w", err))
			return
		}
		plantID = &parsed
	}
	evaluations, err := s.store.PolicyEvaluations(r.Context(), plantID, limit)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, map[string]any{"evaluations": evaluations, "count": len(evaluations)})
}

func (s *Server) getPolicyReference(w http.ResponseWriter, _ *http.Request) {
	s.ok(w, http.StatusOK, policy.Reference())
}

func decodePolicyRequest(w http.ResponseWriter, r *http.Request, s *Server) (policyRequest, bool) {
	var request policyRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		s.fail(w, http.StatusBadRequest, fmt.Errorf("decode policy: %w", err))
		return policyRequest{}, false
	}
	if err := request.policy().Valid(); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return policyRequest{}, false
	}
	return request, true
}

func policyID(w http.ResponseWriter, r *http.Request, s *Server) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.fail(w, http.StatusBadRequest, fmt.Errorf("policy id must be a UUID: %w", err))
		return uuid.Nil, false
	}
	return id, true
}

func policyStoreStatus(err error) int {
	if errors.Is(err, store.ErrNotFound) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}
