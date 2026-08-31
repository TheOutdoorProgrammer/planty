package plant

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	MaxActuatorDuration        = time.Hour
	MaxActuatorScheduleWindows = 12
)

type ActuatorKind string

const (
	ActuatorFan    ActuatorKind = "fan"
	ActuatorSwitch ActuatorKind = "switch"
	ActuatorLight  ActuatorKind = "light"
	ActuatorWater  ActuatorKind = "water"
)

type Actuator struct {
	ID                   uuid.UUID         `json:"id"`
	EntityID             string            `json:"entity_id"`
	Name                 string            `json:"name"`
	Kind                 ActuatorKind      `json:"kind"`
	PlantIDs             []uuid.UUID       `json:"plant_ids"`
	PolicyControlEnabled bool              `json:"policy_control_enabled"`
	CurrentState         string            `json:"current_state,omitempty"`
	ActiveLease          *ActuatorLease    `json:"active_lease,omitempty"`
	LightSchedule        *ActuatorSchedule `json:"light_schedule,omitempty"`
	FanSchedule          *ActuatorSchedule `json:"fan_schedule,omitempty"`
	CreatedAt            time.Time         `json:"created_at"`
	UpdatedAt            time.Time         `json:"updated_at"`
}

func (a Actuator) Valid() error {
	if strings.TrimSpace(a.Name) == "" {
		return invalid("actuator name is required")
	}
	if _, err := a.EntityDomain(); err != nil {
		return invalid("actuator entity_id must be a fan, switch, or light entity")
	}
	switch a.Kind {
	case ActuatorFan, ActuatorSwitch, ActuatorLight, ActuatorWater:
	default:
		return invalid("actuator kind must be fan, switch, light, or water")
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

func (a Actuator) EntityDomain() (string, error) {
	domain, _, ok := strings.Cut(strings.TrimSpace(a.EntityID), ".")
	if !ok || (domain != "fan" && domain != "switch" && domain != "light") {
		return "", invalid("actuator entity_id must be a fan, switch, or light entity")
	}
	return domain, nil
}

type ActuatorSchedule struct {
	ActuatorID       uuid.UUID                `json:"actuator_id"`
	StartMinute      int                      `json:"start_minute"`
	EndMinute        int                      `json:"end_minute"`
	Windows          []ActuatorScheduleWindow `json:"windows"`
	Timezone         string                   `json:"timezone"`
	Enabled          bool                     `json:"enabled"`
	LastAppliedState *bool                    `json:"last_applied_state,omitempty"`
	LastAppliedAt    *time.Time               `json:"last_applied_at,omitempty"`
	LastError        string                   `json:"last_error,omitempty"`
	CreatedAt        time.Time                `json:"created_at"`
	UpdatedAt        time.Time                `json:"updated_at"`
}

type ActuatorScheduleWindow struct {
	StartMinute int `json:"start_minute"`
	EndMinute   int `json:"end_minute"`
}

func (s ActuatorSchedule) EffectiveWindows() []ActuatorScheduleWindow {
	if len(s.Windows) > 0 {
		return s.Windows
	}
	return []ActuatorScheduleWindow{{StartMinute: s.StartMinute, EndMinute: s.EndMinute}}
}

func (s ActuatorSchedule) Valid() error {
	if s.ActuatorID == uuid.Nil {
		return invalid("actuator_id is required")
	}
	if _, err := time.LoadLocation(strings.TrimSpace(s.Timezone)); err != nil {
		return invalid("unknown actuator schedule timezone %q", s.Timezone)
	}
	windows := s.EffectiveWindows()
	if len(windows) > MaxActuatorScheduleWindows {
		return invalid("actuator schedule may contain at most %d windows", MaxActuatorScheduleWindows)
	}
	occupied := [24 * 60]bool{}
	for _, window := range windows {
		if err := window.Valid(); err != nil {
			return err
		}
		for minute := window.StartMinute; minute != window.EndMinute; minute = (minute + 1) % (24 * 60) {
			if occupied[minute] {
				return invalid("actuator schedule windows must not overlap")
			}
			occupied[minute] = true
		}
	}
	return nil
}

func (w ActuatorScheduleWindow) Valid() error {
	if w.StartMinute < 0 || w.StartMinute > 1439 || w.EndMinute < 0 || w.EndMinute > 1439 {
		return invalid("actuator schedule minutes must be between 0 and 1439")
	}
	if w.StartMinute == w.EndMinute {
		return invalid("actuator schedule start and end must differ")
	}
	return nil
}

func (s ActuatorSchedule) WantsOn(at time.Time) (bool, error) {
	if err := s.Valid(); err != nil {
		return false, err
	}
	if !s.Enabled {
		return false, nil
	}
	location, _ := time.LoadLocation(s.Timezone)
	local := at.In(location)
	minute := local.Hour()*60 + local.Minute()
	for _, window := range s.EffectiveWindows() {
		if window.StartMinute < window.EndMinute && minute >= window.StartMinute && minute < window.EndMinute {
			return true, nil
		}
		if window.StartMinute > window.EndMinute && (minute >= window.StartMinute || minute < window.EndMinute) {
			return true, nil
		}
	}
	return false, nil
}

type LightSchedule = ActuatorSchedule

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
