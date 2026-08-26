package plant

import (
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ObservationKind is an event a person or agent reported. Sensor data is a
// Reading instead.
type ObservationKind string

const (
	ObservedWatered    ObservationKind = "watered"
	ObservedAirflow    ObservationKind = "airflow"
	ObservedMisted     ObservationKind = "misted"
	ObservedRepotted   ObservationKind = "repotted"
	ObservedFertilized ObservationKind = "fertilized"
	ObservedPruned     ObservationKind = "pruned"
	ObservedHarvested  ObservationKind = "harvested"
	ObservedMoved      ObservationKind = "moved"
	ObservedSymptom    ObservationKind = "symptom"
	ObservedNote       ObservationKind = "note"
	ObservedDied       ObservationKind = "died"
)

// Source answers the first question asked when something went wrong: did the
// system do this, or did a person.
type Source string

const (
	SourceApp        Source = "app"
	SourceAgent      Source = "agent"
	SourceAutomation Source = "automation"
)

// Observation is one recorded event in a plant's life.
type Observation struct {
	ID      uuid.UUID       `json:"id"`
	PlantID uuid.UUID       `json:"plant_id"`
	Kind    ObservationKind `json:"kind"`
	Body    string          `json:"body,omitempty"`

	OccurredAt time.Time `json:"occurred_at"`
	Source     Source    `json:"source"`
	Actor      string    `json:"actor,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}

// Verifiable: dry soil goes hydrophobic, so water can miss the roots entirely.
func (o Observation) Verifiable() bool { return o.Kind == ObservedWatered }

// Valid reports whether this observation can be stored.
func (o Observation) Valid() error {
	switch o.Kind {
	case ObservedWatered, ObservedAirflow, ObservedMisted, ObservedRepotted, ObservedFertilized,
		ObservedPruned, ObservedHarvested, ObservedMoved, ObservedSymptom,
		ObservedNote, ObservedDied:
	default:
		return invalid("unknown observation kind %q", o.Kind)
	}
	switch o.Source {
	case SourceApp, SourceAgent, SourceAutomation:
	default:
		return invalid("unknown source %q", o.Source)
	}
	if o.PlantID == uuid.Nil {
		return invalid("plant_id is required")
	}
	if o.OccurredAt.IsZero() {
		return invalid("occurred_at is required")
	}
	return nil
}

// Harvest is separate from Observation because yield per plant per season needs
// to aggregate, which free text bodies cannot.
type Harvest struct {
	ID         uuid.UUID `json:"id"`
	PlantID    uuid.UUID `json:"plant_id"`
	OccurredAt time.Time `json:"occurred_at"`
	Quantity   float64   `json:"quantity"`
	Unit       string    `json:"unit"`
	Notes      string    `json:"notes,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (h Harvest) Valid() error {
	if h.PlantID == uuid.Nil {
		return invalid("plant_id is required")
	}
	if h.OccurredAt.IsZero() {
		return invalid("occurred_at is required")
	}
	if h.Quantity <= 0 || math.IsNaN(h.Quantity) || math.IsInf(h.Quantity, 0) {
		return invalid("harvest quantity must be greater than zero")
	}
	if strings.TrimSpace(h.Unit) == "" {
		return invalid("harvest unit is required")
	}
	return nil
}
