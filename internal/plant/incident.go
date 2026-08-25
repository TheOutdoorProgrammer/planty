package plant

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type IncidentStatus string

const (
	IncidentOpen         IncidentStatus = "open"
	IncidentAcknowledged IncidentStatus = "acknowledged"
	IncidentResolved     IncidentStatus = "resolved"
)

type IncidentFactor string

const (
	FactorHAArea             IncidentFactor = "ha_area"
	FactorLocation           IncidentFactor = "location"
	FactorCommonCare         IncidentFactor = "common_care"
	FactorEnvironmentFailure IncidentFactor = "environment_failure"
	FactorActuatorFailure    IncidentFactor = "actuator_failure"
)

type IncidentResolution string

const (
	IncidentConfirmedCommonCause IncidentResolution = "confirmed_common_cause"
	IncidentUnrelated            IncidentResolution = "unrelated"
	IncidentContained            IncidentResolution = "contained"
	IncidentInconclusive         IncidentResolution = "inconclusive"
)

type IncidentEvidence struct {
	RunID            uuid.UUID   `json:"run_id"`
	VerdictIDs       []uuid.UUID `json:"verdict_ids,omitempty"`
	ObservationIDs   []uuid.UUID `json:"observation_ids,omitempty"`
	SensorLinkIDs    []uuid.UUID `json:"sensor_link_ids,omitempty"`
	ActuatorEventIDs []uuid.UUID `json:"actuator_event_ids,omitempty"`
	Note             string      `json:"note"`
}

type IncidentPlant struct {
	Plant      Plant          `json:"plant"`
	Role       string         `json:"role"`
	VerdictID  uuid.UUID      `json:"verdict_id"`
	Action     Action         `json:"action"`
	Confidence float64        `json:"confidence"`
	Evidence   map[string]any `json:"evidence"`
	FirstSeen  time.Time      `json:"first_seen_at"`
	LastSeen   time.Time      `json:"last_seen_at"`
}

type GardenIncident struct {
	ID                  uuid.UUID           `json:"id"`
	Status              IncidentStatus      `json:"status"`
	SuspectedFactorType IncidentFactor      `json:"suspected_factor_type"`
	SuspectedFactorRef  string              `json:"suspected_factor_ref"`
	Summary             string              `json:"summary"`
	Confidence          float64             `json:"confidence"`
	Evidence            IncidentEvidence    `json:"evidence"`
	DetectedRunID       uuid.UUID           `json:"detected_run_id"`
	Plants              []IncidentPlant     `json:"plants"`
	FirstSeenAt         time.Time           `json:"first_seen_at"`
	LastSeenAt          time.Time           `json:"last_seen_at"`
	AcknowledgedAt      *time.Time          `json:"acknowledged_at,omitempty"`
	AcknowledgedBy      string              `json:"acknowledged_by,omitempty"`
	ResolvedAt          *time.Time          `json:"resolved_at,omitempty"`
	ResolvedBy          string              `json:"resolved_by,omitempty"`
	Resolution          *IncidentResolution `json:"resolution,omitempty"`
	Conclusion          string              `json:"conclusion,omitempty"`
	CreatedAt           time.Time           `json:"created_at"`
	UpdatedAt           time.Time           `json:"updated_at"`
}

type IncidentCandidate struct {
	Factor     IncidentFactor
	FactorRef  string
	Summary    string
	Confidence float64
	Evidence   IncidentEvidence
	Plants     []IncidentPlant
}

func (c IncidentCandidate) Valid() error {
	if c.Evidence.RunID == uuid.Nil || strings.TrimSpace(c.FactorRef) == "" || strings.TrimSpace(c.Summary) == "" {
		return invalid("incident run, factor reference, and summary are required")
	}
	if !finite(c.Confidence) || c.Confidence < 0 || c.Confidence > 1 {
		return invalid("incident confidence must be between zero and one")
	}
	switch c.Factor {
	case FactorHAArea, FactorLocation, FactorCommonCare, FactorEnvironmentFailure, FactorActuatorFailure:
	default:
		return invalid("unknown incident factor %q", c.Factor)
	}
	unique := map[uuid.UUID]bool{}
	verdicts := map[uuid.UUID]bool{}
	for _, id := range c.Evidence.VerdictIDs {
		verdicts[id] = true
	}
	for _, member := range c.Plants {
		if member.Plant.ID == uuid.Nil || member.VerdictID == uuid.Nil {
			return invalid("incident plant and verdict ids are required")
		}
		unique[member.Plant.ID] = true
		if !verdicts[member.VerdictID] {
			return invalid("incident evidence must include every affected verdict id")
		}
	}
	independentFailure := len(c.Evidence.SensorLinkIDs)+len(c.Evidence.ActuatorEventIDs) > 0
	if len(unique) < 2 && !(len(unique) == 1 && independentFailure) {
		return invalid("incident needs two plants or one plant plus independent system evidence")
	}
	return nil
}

func (r IncidentResolution) Valid() error {
	switch r {
	case IncidentConfirmedCommonCause, IncidentUnrelated, IncidentContained, IncidentInconclusive:
		return nil
	default:
		return fmt.Errorf("%w: invalid incident resolution %q", ErrInvalid, r)
	}
}
