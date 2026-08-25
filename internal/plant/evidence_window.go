package plant

import (
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

type EvidenceWindowKind string

const (
	WindowRecheck    EvidenceWindowKind = "recheck"
	WindowExperiment EvidenceWindowKind = "experiment"
)

type EvidenceWindowStatus string

const (
	WindowProposed  EvidenceWindowStatus = "proposed"
	WindowActive    EvidenceWindowStatus = "active"
	WindowReady     EvidenceWindowStatus = "ready"
	WindowCompleted EvidenceWindowStatus = "completed"
	WindowCancelled EvidenceWindowStatus = "cancelled"
)

type EvidenceWindowOutcome string

const (
	OutcomeImproved             EvidenceWindowOutcome = "improved"
	OutcomeUnchanged            EvidenceWindowOutcome = "unchanged"
	OutcomeWorsened             EvidenceWindowOutcome = "worsened"
	OutcomeInsufficientEvidence EvidenceWindowOutcome = "insufficient_evidence"
	OutcomeSupported            EvidenceWindowOutcome = "supported"
	OutcomeNotSupported         EvidenceWindowOutcome = "not_supported"
	OutcomeInconclusive         EvidenceWindowOutcome = "inconclusive"
	OutcomeStoppedForSafety     EvidenceWindowOutcome = "stopped_for_safety"
	OutcomeCancelled            EvidenceWindowOutcome = "cancelled"
)

type EvidenceKind string

const (
	EvidenceObservation EvidenceKind = "observation"
	EvidencePhoto       EvidenceKind = "photo"
	EvidenceReading     EvidenceKind = "reading"
)

type EvidencePhase string

const (
	EvidenceBaseline EvidencePhase = "baseline"
	EvidenceReview   EvidencePhase = "review"
)

const (
	MinimumEvidenceWindow = time.Hour
	MaximumEvidenceWindow = 90 * 24 * time.Hour
)

// EvidenceRef points into an existing source ledger. It never copies the
// observation, photograph, or reading into model-authored prose.
type EvidenceRef struct {
	PlantID uuid.UUID     `json:"plant_id"`
	Kind    EvidenceKind  `json:"kind"`
	ID      uuid.UUID     `json:"id"`
	Phase   EvidencePhase `json:"phase"`
}

func (r EvidenceRef) Valid() error {
	if r.PlantID == uuid.Nil || r.ID == uuid.Nil {
		return invalid("evidence plant_id and id are required")
	}
	switch r.Kind {
	case EvidenceObservation, EvidencePhoto, EvidenceReading:
	default:
		return invalid("unknown evidence kind %q", r.Kind)
	}
	switch r.Phase {
	case EvidenceBaseline, EvidenceReview:
	default:
		return invalid("unknown evidence phase %q", r.Phase)
	}
	return nil
}

// EvidenceExpectation says what future ledger entry will answer the question.
// It has no evidence id yet because that record does not exist at proposal time.
type EvidenceExpectation struct {
	PlantID     uuid.UUID    `json:"plant_id"`
	Kind        EvidenceKind `json:"kind"`
	Instruction string       `json:"instruction"`
}

func (e EvidenceExpectation) Valid() error {
	if e.PlantID == uuid.Nil {
		return invalid("evidence expectation plant_id is required")
	}
	switch e.Kind {
	case EvidenceObservation, EvidencePhoto, EvidenceReading:
	default:
		return invalid("unknown expected evidence kind %q", e.Kind)
	}
	if strings.TrimSpace(e.Instruction) == "" {
		return invalid("evidence expectation instruction is required")
	}
	return nil
}

type Experiment struct {
	Title             string   `json:"title"`
	Hypothesis        string   `json:"hypothesis"`
	VariableKind      string   `json:"variable_kind"`
	VariableValue     string   `json:"variable_value"`
	HoldConstantRules []string `json:"hold_constant_rules"`
	SuccessCriteria   []string `json:"success_criteria"`
}

func (e Experiment) Valid() error {
	for name, value := range map[string]string{
		"title": e.Title, "hypothesis": e.Hypothesis,
		"variable_kind": e.VariableKind, "variable_value": e.VariableValue,
	} {
		if strings.TrimSpace(value) == "" {
			return invalid("experiment %s is required", name)
		}
	}
	if len(e.HoldConstantRules) == 0 || len(e.SuccessCriteria) == 0 {
		return invalid("experiment needs hold-constant rules and success criteria")
	}
	for _, group := range [][]string{e.HoldConstantRules, e.SuccessCriteria} {
		for _, value := range group {
			if strings.TrimSpace(value) == "" {
				return invalid("experiment rules and criteria cannot be blank")
			}
		}
	}
	return nil
}

type Guardrail struct {
	Reason           string            `json:"reason"`
	ConflictingKinds []ObservationKind `json:"conflicting_kinds"`
	RedFlags         []string          `json:"red_flags"`
}

// GuardrailFor is deliberately code-owned. A client or model may choose a
// shorter review inside the bounds, but cannot delete red flags or conflicts.
func GuardrailFor(kind ObservationKind) (Guardrail, time.Duration, time.Duration, bool) {
	switch kind {
	case ObservedWatered:
		return Guardrail{
			Reason:           "Wait for delivery evidence before watering again.",
			ConflictingKinds: []ObservationKind{ObservedWatered},
			RedFlags:         []string{"soil remains waterlogged", "water did not reach the root ball", "rapid collapse"},
		}, time.Hour, 3 * 24 * time.Hour, true
	case ObservedMoved:
		return Guardrail{
			Reason:           "Hold the location steady long enough to compare the plant's response.",
			ConflictingKinds: []ObservationKind{ObservedMoved},
			RedFlags:         []string{"freezing or heat exposure", "rapid collapse", "severe leaf scorch"},
		}, 24 * time.Hour, 14 * 24 * time.Hour, true
	case ObservedRepotted:
		return Guardrail{
			Reason:           "Avoid another root disturbance or routine feeding while the repot settles.",
			ConflictingKinds: []ObservationKind{ObservedRepotted, ObservedFertilized},
			RedFlags:         []string{"rapid collapse", "waterlogged soil", "active root rot"},
		}, 48 * time.Hour, 21 * 24 * time.Hour, true
	case ObservedFertilized:
		return Guardrail{
			Reason:           "Do not stack another feeding before evidence shows how this one landed.",
			ConflictingKinds: []ObservationKind{ObservedFertilized, ObservedRepotted},
			RedFlags:         []string{"fertilizer burn", "rapid collapse", "salt crust on the soil"},
		}, 48 * time.Hour, 14 * 24 * time.Hour, true
	case ObservedPruned:
		return Guardrail{
			Reason:           "Let the plant respond before removing more growth or disturbing roots.",
			ConflictingKinds: []ObservationKind{ObservedPruned, ObservedRepotted},
			RedFlags:         []string{"spreading rot", "rapid collapse", "active pest damage"},
		}, 24 * time.Hour, 14 * 24 * time.Hour, true
	default:
		return Guardrail{}, 0, 0, false
	}
}

type GuardrailOverride struct {
	ID        uuid.UUID       `json:"id"`
	WindowID  uuid.UUID       `json:"window_id"`
	PlantID   uuid.UUID       `json:"plant_id"`
	Kind      ObservationKind `json:"kind"`
	Reason    string          `json:"reason"`
	Source    Source          `json:"source"`
	Actor     string          `json:"actor,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

func (o GuardrailOverride) Valid() error {
	if o.WindowID == uuid.Nil || o.PlantID == uuid.Nil {
		return invalid("guardrail override window_id and plant_id are required")
	}
	if strings.TrimSpace(o.Reason) == "" {
		return invalid("guardrail override reason is required")
	}
	if strings.TrimSpace(o.Actor) == "" {
		return invalid("guardrail override actor is required")
	}
	if err := validObservationKind(o.Kind); err != nil {
		return err
	}
	return validSource(o.Source)
}

type EvidenceWindow struct {
	ID                        uuid.UUID              `json:"id"`
	Kind                      EvidenceWindowKind     `json:"kind"`
	Status                    EvidenceWindowStatus   `json:"status"`
	PlantIDs                  []uuid.UUID            `json:"plant_ids"`
	InterventionKind          ObservationKind        `json:"intervention_kind"`
	InterventionObservationID *uuid.UUID             `json:"intervention_observation_id,omitempty"`
	Baseline                  []EvidenceRef          `json:"baseline"`
	Expected                  []EvidenceExpectation  `json:"expected"`
	Review                    []EvidenceRef          `json:"review"`
	EarliestReviewAt          time.Time              `json:"earliest_review_at"`
	LatestReviewAt            time.Time              `json:"latest_review_at"`
	StartedAt                 *time.Time             `json:"started_at,omitempty"`
	ReadyAt                   *time.Time             `json:"ready_at,omitempty"`
	CompletedAt               *time.Time             `json:"completed_at,omitempty"`
	Outcome                   *EvidenceWindowOutcome `json:"outcome,omitempty"`
	Conclusion                string                 `json:"conclusion,omitempty"`
	ConfoundedAt              *time.Time             `json:"confounded_at,omitempty"`
	ConfoundReason            string                 `json:"confound_reason,omitempty"`
	ProposedBy                Source                 `json:"proposed_by"`
	ProposedActor             string                 `json:"proposed_actor,omitempty"`
	Guardrail                 *Guardrail             `json:"guardrail,omitempty"`
	Experiment                *Experiment            `json:"experiment,omitempty"`
	Overrides                 []GuardrailOverride    `json:"overrides"`
	CreatedAt                 time.Time              `json:"created_at"`
	UpdatedAt                 time.Time              `json:"updated_at"`
}

func (w EvidenceWindow) ValidProposal(now time.Time) error {
	if w.Kind != WindowRecheck && w.Kind != WindowExperiment {
		return invalid("unknown evidence window kind %q", w.Kind)
	}
	if len(w.PlantIDs) == 0 {
		return invalid("evidence window needs at least one plant")
	}
	seen := map[uuid.UUID]bool{}
	for _, id := range w.PlantIDs {
		if id == uuid.Nil || seen[id] {
			return invalid("evidence window plant ids must be non-zero and unique")
		}
		seen[id] = true
	}
	if err := validObservationKind(w.InterventionKind); err != nil {
		return err
	}
	if len(w.Baseline) == 0 || len(w.Expected) == 0 {
		return invalid("evidence window needs baseline and expected evidence")
	}
	for _, ref := range w.Baseline {
		if err := ref.Valid(); err != nil {
			return err
		}
		if ref.Phase != EvidenceBaseline || !seen[ref.PlantID] {
			return invalid("baseline evidence must belong to a window plant")
		}
	}
	for _, expected := range w.Expected {
		if err := expected.Valid(); err != nil {
			return err
		}
		if !seen[expected.PlantID] {
			return invalid("expected evidence must belong to a window plant")
		}
	}
	if w.EarliestReviewAt.Before(now.Add(MinimumEvidenceWindow)) {
		return invalid("earliest review must be at least %s after proposal", MinimumEvidenceWindow)
	}
	if !w.LatestReviewAt.After(w.EarliestReviewAt) || w.LatestReviewAt.After(now.Add(MaximumEvidenceWindow)) {
		return invalid("latest review must follow earliest review and be within %s", MaximumEvidenceWindow)
	}
	guardrail, minimum, maximum, ok := GuardrailFor(w.InterventionKind)
	if !ok {
		return invalid("%q has no code-owned guardrail", w.InterventionKind)
	}
	if w.EarliestReviewAt.Before(now.Add(minimum)) || w.LatestReviewAt.After(now.Add(maximum)) {
		return invalid("review bounds for %q must be between %s and %s", w.InterventionKind, minimum, maximum)
	}
	_ = guardrail
	if w.Kind == WindowRecheck {
		if w.Experiment != nil {
			return invalid("a recheck cannot carry an experiment")
		}
		if !hasEvidenceKind(w.Baseline, EvidencePhoto) || !expectsEvidenceKind(w.Expected, EvidencePhoto) {
			return invalid("a visual recheck needs a baseline and expected photograph")
		}
	} else {
		if w.Experiment == nil {
			return invalid("an experiment definition is required")
		}
		if err := w.Experiment.Valid(); err != nil {
			return err
		}
	}
	return validSource(w.ProposedBy)
}

func (w EvidenceWindow) CanStart(source Source, observationID uuid.UUID, now time.Time) error {
	if w.Status != WindowProposed {
		return invalid("only a proposed evidence window can start")
	}
	if observationID == uuid.Nil {
		return invalid("starting an evidence window needs one intervention observation")
	}
	if source == SourceAutomation {
		return invalid("scheduled automation may propose an evidence window but cannot start it")
	}
	if err := validSource(source); err != nil {
		return err
	}
	if !now.Before(w.LatestReviewAt) {
		return invalid("the evidence window review deadline has passed")
	}
	return nil
}

func (w EvidenceWindow) CanMarkReady(review []EvidenceRef, now time.Time) error {
	if w.Status != WindowActive {
		return invalid("only an active evidence window can become ready")
	}
	if now.Before(w.EarliestReviewAt) || now.After(w.LatestReviewAt) {
		return invalid("review evidence is outside the window's review bounds")
	}
	if len(review) == 0 {
		return invalid("review evidence is required")
	}
	for _, ref := range review {
		if err := ref.Valid(); err != nil {
			return err
		}
		if ref.Phase != EvidenceReview || !slices.Contains(w.PlantIDs, ref.PlantID) {
			return invalid("review evidence must belong to a window plant")
		}
	}
	for _, expected := range w.Expected {
		matched := false
		for _, ref := range review {
			if ref.PlantID == expected.PlantID && ref.Kind == expected.Kind {
				matched = true
				break
			}
		}
		if !matched {
			return invalid("review evidence does not satisfy the %s expectation for plant %s", expected.Kind, expected.PlantID)
		}
	}
	return nil
}

func (w EvidenceWindow) CanConclude(outcome EvidenceWindowOutcome, conclusion string) error {
	if strings.TrimSpace(conclusion) == "" {
		return invalid("evidence window conclusion is required")
	}
	if outcome == OutcomeStoppedForSafety {
		if w.Status != WindowActive && w.Status != WindowReady {
			return invalid("only an active or ready window can stop for safety")
		}
	} else if w.Status != WindowReady {
		return invalid("only a ready evidence window can conclude")
	}
	if w.ConfoundedAt != nil {
		if w.Kind == WindowExperiment && outcome != OutcomeInconclusive && outcome != OutcomeStoppedForSafety {
			return invalid("a confounded experiment can only be inconclusive or stopped for safety")
		}
		if w.Kind == WindowRecheck && outcome != OutcomeInsufficientEvidence && outcome != OutcomeStoppedForSafety {
			return invalid("a confounded recheck can only report insufficient evidence or stop for safety")
		}
	}
	switch w.Kind {
	case WindowRecheck:
		switch outcome {
		case OutcomeImproved, OutcomeUnchanged, OutcomeWorsened, OutcomeInsufficientEvidence, OutcomeStoppedForSafety:
			return nil
		}
	case WindowExperiment:
		switch outcome {
		case OutcomeSupported, OutcomeNotSupported, OutcomeInconclusive, OutcomeStoppedForSafety:
			return nil
		}
	}
	return invalid("outcome %q does not belong to a %s", outcome, w.Kind)
}

func validObservationKind(kind ObservationKind) error {
	switch kind {
	case ObservedWatered, ObservedMisted, ObservedRepotted, ObservedFertilized,
		ObservedPruned, ObservedHarvested, ObservedMoved, ObservedSymptom,
		ObservedNote, ObservedDied:
		return nil
	default:
		return invalid("unknown observation kind %q", kind)
	}
}

func validSource(source Source) error {
	switch source {
	case SourceApp, SourceAgent, SourceAutomation:
		return nil
	default:
		return invalid("unknown source %q", source)
	}
}

func hasEvidenceKind(refs []EvidenceRef, kind EvidenceKind) bool {
	return slices.ContainsFunc(refs, func(ref EvidenceRef) bool { return ref.Kind == kind })
}

func expectsEvidenceKind(expected []EvidenceExpectation, kind EvidenceKind) bool {
	return slices.ContainsFunc(expected, func(item EvidenceExpectation) bool { return item.Kind == kind })
}
