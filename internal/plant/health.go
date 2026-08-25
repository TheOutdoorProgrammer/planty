package plant

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
)

// HealthEvidence names the durable facts behind a health assessment. Summary
// is for evidence that cannot be addressed by an id, not a replacement for ids
// available to an automated judgment.
type HealthEvidence struct {
	PhotoIDs       []uuid.UUID `json:"photo_ids,omitempty"`
	ObservationIDs []uuid.UUID `json:"observation_ids,omitempty"`
	ReadingIDs     []uuid.UUID `json:"reading_ids,omitempty"`
	Summary        string      `json:"summary,omitempty"`
	ModelVersion   string      `json:"model_version,omitempty"`
}

// HasReferences reports whether Planty can resolve at least one piece of the
// evidence back to its own records.
func (e HealthEvidence) HasReferences() bool {
	return len(e.PhotoIDs)+len(e.ObservationIDs)+len(e.ReadingIDs) > 0
}

// Present reports whether the assessment carries anything checkable later.
func (e HealthEvidence) Present() bool {
	return e.HasReferences() || strings.TrimSpace(e.Summary) != ""
}

// HealthEvent is one append-only assessment. A nil RequestedDelta and
// AppliedDelta identify the single absolute baseline; every later row is a
// signed change whose applied value may be smaller when clamped at 0 or 100.
type HealthEvent struct {
	ID      uuid.UUID `json:"id"`
	PlantID uuid.UUID `json:"plant_id"`
	Score   float64   `json:"score"`

	RequestedDelta *float64 `json:"requested_delta,omitempty"`
	AppliedDelta   *float64 `json:"applied_delta,omitempty"`

	Rationale string         `json:"rationale"`
	Evidence  HealthEvidence `json:"evidence"`
	Source    Source         `json:"source"`
	Actor     string         `json:"actor,omitempty"`

	JudgmentRunID  *uuid.UUID `json:"judgment_run_id,omitempty"`
	IdempotencyKey *uuid.UUID `json:"idempotency_key,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// IsBaseline reports whether this event established the absolute score.
func (e HealthEvent) IsBaseline() bool { return e.RequestedDelta == nil }

// HealthChange is the write request. Exactly one of Baseline and Delta is set.
type HealthChange struct {
	PlantID  uuid.UUID
	Baseline *float64
	Delta    *float64

	Rationale string
	Evidence  HealthEvidence
	Source    Source
	Actor     string

	JudgmentRunID  *uuid.UUID
	IdempotencyKey *uuid.UUID
}

// Valid checks the parts that do not require database state.
func (c HealthChange) Valid() error {
	if c.PlantID == uuid.Nil {
		return invalid("plant_id is required")
	}
	if c.IdempotencyKey != nil && *c.IdempotencyKey == uuid.Nil {
		return invalid("idempotency_key cannot be zero")
	}
	if (c.Baseline == nil) == (c.Delta == nil) {
		return invalid("set exactly one of baseline or delta")
	}
	if c.Baseline != nil && (!finite(*c.Baseline) || *c.Baseline < 0 || *c.Baseline > 100) {
		return invalid("baseline must be between 0 and 100")
	}
	if c.Delta != nil && (!finite(*c.Delta) || *c.Delta == 0) {
		return invalid("delta must be a non-zero finite number")
	}
	if strings.TrimSpace(c.Rationale) == "" {
		return invalid("health rationale is required")
	}
	if !c.Evidence.Present() {
		return invalid("health evidence is required")
	}
	if c.Source == SourceAgent && !c.Evidence.HasReferences() {
		return invalid("agent health changes require a photo, observation, or reading id")
	}
	switch c.Source {
	case SourceApp, SourceAgent, SourceAutomation:
	default:
		return invalid("unknown source %q", c.Source)
	}
	return nil
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

// HealthMode is how a scheduled assessment treats the score.
type HealthMode string

const (
	HealthUnchanged HealthMode = "unchanged"
	HealthBaseline  HealthMode = "baseline"
	HealthAdjust    HealthMode = "adjust"
)

func (m HealthMode) Valid() error {
	switch m {
	case HealthUnchanged, HealthBaseline, HealthAdjust:
		return nil
	default:
		return fmt.Errorf("invalid health mode %q", m)
	}
}
