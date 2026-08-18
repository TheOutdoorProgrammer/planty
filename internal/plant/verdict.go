package plant

import (
	"time"

	"github.com/google/uuid"
)

// Action is what the daily run concluded should happen to one plant.
type Action string

const (
	ActionNone    Action = "none"
	ActionWater   Action = "water"
	ActionCheck   Action = "check"
	ActionUrgent  Action = "urgent"
	ActionHarvest Action = "harvest"
)

// Verdict is one plant's judgment for one day.
type Verdict struct {
	ID      uuid.UUID `json:"id"`
	PlantID uuid.UUID `json:"plant_id"`
	ForDate time.Time `json:"for_date"`

	Action     Action  `json:"action"`
	Reasoning  string  `json:"reasoning"`
	Confidence float64 `json:"confidence"`

	// Evidence is required: without it a wrong verdict cannot be debugged and a
	// right one cannot be trusted.
	Evidence Evidence `json:"evidence"`

	CreatedAt      time.Time  `json:"created_at"`
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
}

// Evidence records what the verdict was built from.
type Evidence struct {
	ReadingIDs     []uuid.UUID `json:"reading_ids,omitempty"`
	ObservationIDs []uuid.UUID `json:"observation_ids,omitempty"`
	PhotoIDs       []uuid.UUID `json:"photo_ids,omitempty"`
	SensorSummary  string      `json:"sensor_summary,omitempty"`
	ModelVersion   string      `json:"model_version,omitempty"`
}

// NeedsAction reports whether this verdict asks the operator to do something.
func (v Verdict) NeedsAction() bool { return v.Action != ActionNone }

// Photo is one image of a plant. Bytes live in object storage, never Postgres.
type Photo struct {
	ID      uuid.UUID `json:"id"`
	PlantID uuid.UUID `json:"plant_id"`

	StorageKey string    `json:"storage_key"`
	TakenAt    time.Time `json:"taken_at"`
	Caption    string    `json:"caption,omitempty"`

	// Kept apart from anything a human wrote so a wrong machine reading is never
	// later mistaken for a first-hand observation.
	VisionFindings string     `json:"vision_findings,omitempty"`
	AnalyzedAt     *time.Time `json:"analyzed_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}

// DigestEntry is one line of what needs doing, as both clients render it.
type DigestEntry struct {
	Plant   Plant   `json:"plant"`
	Verdict Verdict `json:"verdict"`
	Risk    int     `json:"risk"`
}

// Digest is the answer to "what should I do right now".
type Digest struct {
	Date    time.Time     `json:"date"`
	Entries []DigestEntry `json:"entries"`

	// Checked and StaleSince let a client tell "nothing needs doing" apart from
	// "no fresh data", which must never look the same.
	Checked    int        `json:"checked"`
	StaleSince *time.Time `json:"stale_since,omitempty"`
}

// AllClear reports whether nothing needs doing, given data is actually fresh.
func (d Digest) AllClear() bool { return len(d.Entries) == 0 && d.StaleSince == nil }
