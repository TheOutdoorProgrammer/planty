package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
	"github.com/google/uuid"
)

// Root replaces these temporary names with generated route constants after
// the parallel OpenAPI work lands.
const (
	routeProposeRecheckPending         = "POST /v1/plants/{slug}/rechecks"
	routeGetEvidenceWindowPending      = "GET /v1/evidence-windows/{id}"
	routeStartEvidenceWindowPending    = "POST /v1/evidence-windows/{id}/start"
	routeReviewEvidenceWindowPending   = "POST /v1/evidence-windows/{id}/review"
	routeConcludeEvidenceWindowPending = "POST /v1/evidence-windows/{id}/conclude"
	routeCancelEvidenceWindowPending   = "POST /v1/evidence-windows/{id}/cancel"
	routeListGuardrailsPending         = "GET /v1/plants/{slug}/guardrails"
	routeOverrideGuardrailPending      = "POST /v1/guardrails/{id}/override"
	routeListExperimentsPending        = "GET /v1/experiments"
	routeProposeExperimentPending      = "POST /v1/experiments"
	routeGetExperimentPending          = "GET /v1/experiments/{id}"
)

func (s *Server) registerEvidenceWorkflowRoutes(mux *http.ServeMux) {
	mux.HandleFunc(routeProposeRecheckPending, s.proposeRecheck)
	mux.HandleFunc(routeGetEvidenceWindowPending, s.getEvidenceWindow)
	mux.HandleFunc(routeStartEvidenceWindowPending, s.startEvidenceWindow)
	mux.HandleFunc(routeReviewEvidenceWindowPending, s.reviewEvidenceWindow)
	mux.HandleFunc(routeConcludeEvidenceWindowPending, s.concludeEvidenceWindow)
	mux.HandleFunc(routeCancelEvidenceWindowPending, s.cancelEvidenceWindow)
	mux.HandleFunc(routeListGuardrailsPending, s.listGuardrails)
	mux.HandleFunc(routeOverrideGuardrailPending, s.overrideGuardrail)
	mux.HandleFunc(routeListExperimentsPending, s.listExperiments)
	mux.HandleFunc(routeProposeExperimentPending, s.proposeExperiment)
	mux.HandleFunc(routeGetExperimentPending, s.getExperiment)
}

type evidenceRefRequest struct {
	PlantID uuid.UUID          `json:"plant_id"`
	Kind    plant.EvidenceKind `json:"kind"`
	ID      uuid.UUID          `json:"id"`
}

func (r evidenceRefRequest) ref(phase plant.EvidencePhase, defaultPlant uuid.UUID) plant.EvidenceRef {
	plantID := r.PlantID
	if plantID == uuid.Nil {
		plantID = defaultPlant
	}
	return plant.EvidenceRef{PlantID: plantID, Kind: r.Kind, ID: r.ID, Phase: phase}
}

type evidenceExpectationRequest struct {
	PlantID     uuid.UUID          `json:"plant_id"`
	Kind        plant.EvidenceKind `json:"kind"`
	Instruction string             `json:"instruction"`
}

func (r evidenceExpectationRequest) expectation(defaultPlant uuid.UUID) plant.EvidenceExpectation {
	plantID := r.PlantID
	if plantID == uuid.Nil {
		plantID = defaultPlant
	}
	return plant.EvidenceExpectation{PlantID: plantID, Kind: r.Kind, Instruction: r.Instruction}
}

type recheckRequest struct {
	InterventionKind plant.ObservationKind        `json:"intervention_kind"`
	Baseline         []evidenceRefRequest         `json:"baseline"`
	Expected         []evidenceExpectationRequest `json:"expected"`
	EarliestReview   time.Time                    `json:"earliest_review_at"`
	LatestReview     time.Time                    `json:"latest_review_at"`
	Actor            string                       `json:"actor"`
}

func (s *Server) proposeRecheck(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetPlant(r.Context(), r.PathValue("slug"))
	if err != nil {
		workflowFail(s, w, err)
		return
	}
	var request recheckRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	baseline := make([]plant.EvidenceRef, len(request.Baseline))
	for i, ref := range request.Baseline {
		baseline[i] = ref.ref(plant.EvidenceBaseline, p.ID)
	}
	expected := make([]plant.EvidenceExpectation, len(request.Expected))
	for i, item := range request.Expected {
		expected[i] = item.expectation(p.ID)
	}
	window, err := s.store.ProposeEvidenceWindow(r.Context(), plant.EvidenceWindow{
		Kind: plant.WindowRecheck, PlantIDs: []uuid.UUID{p.ID},
		InterventionKind: request.InterventionKind,
		Baseline:         baseline, Expected: expected,
		EarliestReviewAt: request.EarliestReview, LatestReviewAt: request.LatestReview,
		ProposedBy: plant.SourceApp, ProposedActor: request.Actor,
	})
	if err != nil {
		workflowFail(s, w, err)
		return
	}
	s.ok(w, http.StatusCreated, window)
}

type experimentRequest struct {
	PlantIDs          []uuid.UUID                  `json:"plant_ids"`
	InterventionKind  plant.ObservationKind        `json:"intervention_kind"`
	Baseline          []evidenceRefRequest         `json:"baseline"`
	Expected          []evidenceExpectationRequest `json:"expected"`
	EarliestReview    time.Time                    `json:"earliest_review_at"`
	LatestReview      time.Time                    `json:"latest_review_at"`
	Actor             string                       `json:"actor"`
	Title             string                       `json:"title"`
	Hypothesis        string                       `json:"hypothesis"`
	VariableKind      string                       `json:"variable_kind"`
	VariableValue     string                       `json:"variable_value"`
	HoldConstantRules []string                     `json:"hold_constant_rules"`
	SuccessCriteria   []string                     `json:"success_criteria"`
}

func (s *Server) proposeExperiment(w http.ResponseWriter, r *http.Request) {
	var request experimentRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	baseline := make([]plant.EvidenceRef, len(request.Baseline))
	for i, ref := range request.Baseline {
		baseline[i] = ref.ref(plant.EvidenceBaseline, uuid.Nil)
	}
	expected := make([]plant.EvidenceExpectation, len(request.Expected))
	for i, item := range request.Expected {
		expected[i] = item.expectation(uuid.Nil)
	}
	window, err := s.store.ProposeEvidenceWindow(r.Context(), plant.EvidenceWindow{
		Kind: plant.WindowExperiment, PlantIDs: request.PlantIDs,
		InterventionKind: request.InterventionKind,
		Baseline:         baseline, Expected: expected,
		EarliestReviewAt: request.EarliestReview, LatestReviewAt: request.LatestReview,
		ProposedBy: plant.SourceApp, ProposedActor: request.Actor,
		Experiment: &plant.Experiment{
			Title: request.Title, Hypothesis: request.Hypothesis,
			VariableKind: request.VariableKind, VariableValue: request.VariableValue,
			HoldConstantRules: request.HoldConstantRules, SuccessCriteria: request.SuccessCriteria,
		},
	})
	if err != nil {
		workflowFail(s, w, err)
		return
	}
	s.ok(w, http.StatusCreated, window)
}

func (s *Server) getEvidenceWindow(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	window, err := s.store.EvidenceWindow(r.Context(), id)
	if err != nil {
		workflowFail(s, w, err)
		return
	}
	s.ok(w, http.StatusOK, window)
}

func (s *Server) getExperiment(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	window, err := s.store.EvidenceWindow(r.Context(), id)
	if err != nil {
		workflowFail(s, w, err)
		return
	}
	if window.Kind != plant.WindowExperiment {
		s.fail(w, http.StatusNotFound, store.ErrNotFound)
		return
	}
	s.ok(w, http.StatusOK, window)
}

func (s *Server) listExperiments(w http.ResponseWriter, r *http.Request) {
	kind := plant.WindowExperiment
	windows, err := s.store.EvidenceWindows(r.Context(), nil, &kind)
	if err != nil {
		workflowFail(s, w, err)
		return
	}
	s.ok(w, http.StatusOK, map[string]any{"experiments": windows, "count": len(windows)})
}

type startEvidenceWindowRequest struct {
	ObservationID uuid.UUID `json:"observation_id"`
	Actor         string    `json:"actor"`
}

func (s *Server) startEvidenceWindow(w http.ResponseWriter, r *http.Request) {
	id, ok := workflowID(s, w, r.PathValue("id"))
	if !ok {
		return
	}
	var request startEvidenceWindowRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	window, err := s.store.StartEvidenceWindow(r.Context(), id, request.ObservationID, plant.SourceApp, request.Actor)
	if err != nil {
		workflowFail(s, w, err)
		return
	}
	s.ok(w, http.StatusOK, window)
}

type reviewEvidenceWindowRequest struct {
	Evidence []evidenceRefRequest `json:"evidence"`
}

func (s *Server) reviewEvidenceWindow(w http.ResponseWriter, r *http.Request) {
	id, ok := workflowID(s, w, r.PathValue("id"))
	if !ok {
		return
	}
	var request reviewEvidenceWindowRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	refs := make([]plant.EvidenceRef, len(request.Evidence))
	for i, ref := range request.Evidence {
		refs[i] = ref.ref(plant.EvidenceReview, uuid.Nil)
	}
	window, err := s.store.MarkEvidenceWindowReady(r.Context(), id, refs)
	if err != nil {
		workflowFail(s, w, err)
		return
	}
	s.ok(w, http.StatusOK, window)
}

type concludeEvidenceWindowRequest struct {
	Outcome    plant.EvidenceWindowOutcome `json:"outcome"`
	Conclusion string                      `json:"conclusion"`
	Actor      string                      `json:"actor"`
}

func (s *Server) concludeEvidenceWindow(w http.ResponseWriter, r *http.Request) {
	id, ok := workflowID(s, w, r.PathValue("id"))
	if !ok {
		return
	}
	var request concludeEvidenceWindowRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	window, err := s.store.ConcludeEvidenceWindow(r.Context(), id, request.Outcome,
		request.Conclusion, plant.SourceApp, request.Actor)
	if err != nil {
		workflowFail(s, w, err)
		return
	}
	s.ok(w, http.StatusOK, window)
}

type cancelEvidenceWindowRequest struct {
	Reason string `json:"reason"`
	Actor  string `json:"actor"`
}

func (s *Server) cancelEvidenceWindow(w http.ResponseWriter, r *http.Request) {
	id, ok := workflowID(s, w, r.PathValue("id"))
	if !ok {
		return
	}
	var request cancelEvidenceWindowRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	window, err := s.store.CancelEvidenceWindow(r.Context(), id, plant.SourceApp, request.Actor, request.Reason)
	if err != nil {
		workflowFail(s, w, err)
		return
	}
	s.ok(w, http.StatusOK, window)
}

func (s *Server) listGuardrails(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetPlant(r.Context(), r.PathValue("slug"))
	if err != nil {
		workflowFail(s, w, err)
		return
	}
	windows, err := s.store.EvidenceWindows(r.Context(), &p.ID, nil)
	if err != nil {
		workflowFail(s, w, err)
		return
	}
	active := make([]plant.EvidenceWindow, 0, len(windows))
	for _, window := range windows {
		if window.Guardrail != nil && (window.Status == plant.WindowActive || window.Status == plant.WindowReady) {
			active = append(active, window)
		}
	}
	s.ok(w, http.StatusOK, map[string]any{"guardrails": active, "count": len(active)})
}

type overrideGuardrailRequest struct {
	PlantID uuid.UUID             `json:"plant_id"`
	Kind    plant.ObservationKind `json:"kind"`
	Reason  string                `json:"reason"`
	Actor   string                `json:"actor"`
}

func (s *Server) overrideGuardrail(w http.ResponseWriter, r *http.Request) {
	id, ok := workflowID(s, w, r.PathValue("id"))
	if !ok {
		return
	}
	var request overrideGuardrailRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	override, err := s.store.OverrideGuardrail(r.Context(), plant.GuardrailOverride{
		WindowID: id, PlantID: request.PlantID, Kind: request.Kind,
		Reason: request.Reason, Source: plant.SourceApp, Actor: request.Actor,
	})
	if err != nil {
		workflowFail(s, w, err)
		return
	}
	s.ok(w, http.StatusCreated, override)
}

func workflowID(s *Server, w http.ResponseWriter, raw string) (uuid.UUID, bool) {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		s.fail(w, http.StatusBadRequest, fmt.Errorf("invalid evidence window id: %w", err))
		return uuid.Nil, false
	}
	return id, true
}

func workflowFail(s *Server, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		s.fail(w, http.StatusNotFound, err)
	case errors.Is(err, plant.ErrInvalid):
		s.fail(w, http.StatusBadRequest, err)
	default:
		s.fail(w, http.StatusInternalServerError, err)
	}
}
