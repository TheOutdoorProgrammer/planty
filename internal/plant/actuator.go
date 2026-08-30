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
	ActuatorLight  ActuatorKind = "light"
)

type Actuator struct {
	ID                   uuid.UUID      `json:"id"`
	EntityID             string         `json:"entity_id"`
	Name                 string         `json:"name"`
	Kind                 ActuatorKind   `json:"kind"`
	PlantIDs             []uuid.UUID    `json:"plant_ids"`
	PolicyControlEnabled bool           `json:"policy_control_enabled"`
	CurrentState         string         `json:"current_state,omitempty"`
	ActiveLease          *ActuatorLease `json:"active_lease,omitempty"`
	LightSchedule        *LightSchedule `json:"light_schedule,omitempty"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
}

func (a Actuator) Valid() error {
	if strings.TrimSpace(a.Name) == "" {
		return invalid("actuator name is required")
	}
	domain, _, ok := strings.Cut(strings.TrimSpace(a.EntityID), ".")
	if !ok || (domain != string(ActuatorFan) && domain != string(ActuatorSwitch) && domain != string(ActuatorLight)) {
		return invalid("actuator entity_id must be a fan, switch, or light entity")
	}
	if a.Kind != ActuatorKind(domain) {
		return invalid("actuator kind must match its entity_id domain")
	}
	if a.PolicyControlEnabled && a.Kind != ActuatorFan {
		return invalid("policy control is only available for fans")
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

type LightSchedule struct {
	ActuatorID       uuid.UUID  `json:"actuator_id"`
	StartMinute      int        `json:"start_minute"`
	EndMinute        int        `json:"end_minute"`
	Timezone         string     `json:"timezone"`
	Enabled          bool       `json:"enabled"`
	LastAppliedState *bool      `json:"last_applied_state,omitempty"`
	LastAppliedAt    *time.Time `json:"last_applied_at,omitempty"`
	LastError        string     `json:"last_error,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

func (s LightSchedule) Valid() error {
	if s.ActuatorID == uuid.Nil {
		return invalid("actuator_id is required")
	}
	if s.StartMinute < 0 || s.StartMinute > 1439 || s.EndMinute < 0 || s.EndMinute > 1439 {
		return invalid("light schedule minutes must be between 0 and 1439")
	}
	if s.StartMinute == s.EndMinute {
		return invalid("light schedule start and end must differ")
	}
	if _, err := time.LoadLocation(strings.TrimSpace(s.Timezone)); err != nil {
		return invalid("unknown light schedule timezone %q", s.Timezone)
	}
	return nil
}

func (s LightSchedule) WantsOn(at time.Time) (bool, error) {
	if err := s.Valid(); err != nil {
		return false, err
	}
	if !s.Enabled {
		return false, nil
	}
	location, _ := time.LoadLocation(s.Timezone)
	local := at.In(location)
	minute := local.Hour()*60 + local.Minute()
	if s.StartMinute < s.EndMinute {
		return minute >= s.StartMinute && minute < s.EndMinute, nil
	}
	return minute >= s.StartMinute || minute < s.EndMinute, nil
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
