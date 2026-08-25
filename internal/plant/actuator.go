package plant

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const MaxActuatorDuration = time.Hour

type ActuatorKind string

const (
	ActuatorFan    ActuatorKind = "fan"
	ActuatorSwitch ActuatorKind = "switch"
)

type Actuator struct {
	ID          uuid.UUID      `json:"id"`
	EntityID    string         `json:"entity_id"`
	Name        string         `json:"name"`
	Kind        ActuatorKind   `json:"kind"`
	PlantIDs    []uuid.UUID    `json:"plant_ids"`
	ActiveLease *ActuatorLease `json:"active_lease,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

func (a Actuator) Valid() error {
	if strings.TrimSpace(a.Name) == "" {
		return invalid("actuator name is required")
	}
	domain, _, ok := strings.Cut(strings.TrimSpace(a.EntityID), ".")
	if !ok || (domain != string(ActuatorFan) && domain != string(ActuatorSwitch)) {
		return invalid("actuator entity_id must be a fan or switch entity")
	}
	if a.Kind != ActuatorKind(domain) {
		return invalid("actuator kind must match its entity_id domain")
	}
	if len(a.PlantIDs) == 0 {
		return invalid("actuator must be assigned to at least one plant")
	}
	seen := make(map[uuid.UUID]struct{}, len(a.PlantIDs))
	for _, id := range a.PlantIDs {
		if id == uuid.Nil {
			return invalid("actuator plant_ids must not contain an empty id")
		}
		if _, duplicate := seen[id]; duplicate {
			return invalid("actuator plant_ids must not contain duplicates")
		}
		seen[id] = struct{}{}
	}
	return nil
}

type ActuatorLease struct {
	ID               uuid.UUID  `json:"id"`
	ActuatorID       uuid.UUID  `json:"actuator_id"`
	RequestedSeconds int        `json:"requested_seconds"`
	Deadline         time.Time  `json:"deadline"`
	Actor            string     `json:"actor"`
	Source           Source     `json:"source"`
	IdempotencyKey   uuid.UUID  `json:"idempotency_key"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	StoppedAt        *time.Time `json:"stopped_at,omitempty"`
	StopReason       string     `json:"stop_reason,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

func (l ActuatorLease) Valid() error {
	if l.ActuatorID == uuid.Nil || l.IdempotencyKey == uuid.Nil {
		return invalid("actuator_id and idempotency_key are required")
	}
	if l.RequestedSeconds < 1 || time.Duration(l.RequestedSeconds)*time.Second > MaxActuatorDuration {
		return invalid("actuator duration must be between 1 and %d seconds", int(MaxActuatorDuration/time.Second))
	}
	if strings.TrimSpace(l.Actor) == "" {
		return invalid("actuator actor is required")
	}
	switch l.Source {
	case SourceApp, SourceAgent, SourceAutomation:
		return nil
	default:
		return fmt.Errorf("%w: unknown actuator source %q", ErrInvalid, l.Source)
	}
}

type ActuatorEvent struct {
	ID             uuid.UUID  `json:"id"`
	ActuatorID     uuid.UUID  `json:"actuator_id"`
	LeaseID        *uuid.UUID `json:"lease_id,omitempty"`
	Action         string     `json:"action"`
	Actor          string     `json:"actor"`
	Source         Source     `json:"source"`
	IdempotencyKey *uuid.UUID `json:"idempotency_key,omitempty"`
	Detail         string     `json:"detail,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}
