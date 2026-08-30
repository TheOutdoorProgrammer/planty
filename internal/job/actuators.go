package job

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/ha"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
	"github.com/google/uuid"
)

type ActuatorHomeAssistant interface {
	CallService(context.Context, string, string, map[string]any) error
}

type actuatorStateReader interface {
	State(context.Context, string) (ha.State, error)
}

type ActuatorControl struct {
	Store *store.Store
	HA    ActuatorHomeAssistant
	Log   *slog.Logger
}

func (c ActuatorControl) Start(ctx context.Context, actuatorID uuid.UUID, seconds int, actor string, source plant.Source, key uuid.UUID) (plant.ActuatorLease, bool, error) {
	return c.start(ctx, actuatorID, uuid.Nil, seconds, actor, source, key)
}

func (c ActuatorControl) StartForPlant(ctx context.Context, actuatorID, plantID uuid.UUID, seconds int, actor string, source plant.Source, key uuid.UUID) (plant.ActuatorLease, bool, error) {
	return c.start(ctx, actuatorID, plantID, seconds, actor, source, key)
}

func (c ActuatorControl) start(ctx context.Context, actuatorID, plantID uuid.UUID, seconds int, actor string, source plant.Source, key uuid.UUID) (plant.ActuatorLease, bool, error) {
	if c.HA == nil {
		return plant.ActuatorLease{}, false, errors.New("Home Assistant actuation is not configured")
	}
	request := plant.ActuatorLease{
		ActuatorID: actuatorID, RequestedSeconds: seconds, Actor: actor,
		Source: source, IdempotencyKey: key,
	}
	var actuator plant.Actuator
	var lease plant.ActuatorLease
	var created bool
	var err error
	if plantID == uuid.Nil {
		actuator, lease, created, err = c.Store.BeginActuatorLease(ctx, request)
	} else {
		actuator, lease, created, err = c.Store.BeginActuatorLeaseForPlant(ctx, request, plantID)
	}
	if err != nil || !created {
		return lease, created, err
	}
	domain, err := actuator.EntityDomain()
	if err != nil {
		return lease, true, err
	}
	if err := c.HA.CallService(ctx, domain, "turn_on", map[string]any{"entity_id": actuator.EntityID}); err != nil {
		stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		stopErr := c.HA.CallService(stopCtx, domain, "turn_off", map[string]any{"entity_id": actuator.EntityID})
		cause := errors.Join(err, stopErr)
		return lease, true, errors.Join(fmt.Errorf("start actuator: %w", err), c.Store.RecordActuatorStartFailure(stopCtx, lease, cause, stopErr == nil))
	}
	if err := c.Store.MarkActuatorStarted(context.WithoutCancel(ctx), lease); err != nil {
		stopErr := c.stop(context.WithoutCancel(ctx), actuator, lease, "start audit failed", "planty recovery", plant.SourceAutomation, nil)
		return lease, true, errors.Join(fmt.Errorf("record actuator start: %w", err), stopErr)
	}
	return lease, true, nil
}

func (c ActuatorControl) Stop(ctx context.Context, actuatorID uuid.UUID, actor string, source plant.Source, key uuid.UUID) (bool, error) {
	if c.HA == nil {
		return false, errors.New("Home Assistant actuation is not configured")
	}
	if actuatorID == uuid.Nil || key == uuid.Nil || actor == "" {
		return false, fmt.Errorf("%w: actuator id, actor, and idempotency key are required", plant.ErrInvalid)
	}
	switch source {
	case plant.SourceApp, plant.SourceAgent, plant.SourceAutomation:
	default:
		return false, fmt.Errorf("%w: unknown actuator source %q", plant.ErrInvalid, source)
	}
	actuator, err := c.Store.Actuator(ctx, actuatorID)
	if err != nil {
		return false, err
	}
	domain, err := actuator.EntityDomain()
	if err != nil {
		return false, err
	}
	lease, err := c.Store.ActiveActuatorLease(ctx, actuatorID)
	if errors.Is(err, store.ErrNotFound) {
		if _, recordErr := c.Store.RecordActuatorNoopStop(ctx, actuatorID, actor, source, key); recordErr != nil {
			return false, recordErr
		}
		stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		return false, c.HA.CallService(stopCtx, domain, "turn_off", map[string]any{"entity_id": actuator.EntityID})
	}
	if err != nil {
		return false, err
	}
	newRequest, err := c.Store.RecordActuatorStopRequested(ctx, lease, actor, source, key)
	if err != nil || !newRequest {
		return false, err
	}
	if err := c.stop(ctx, actuator, lease, "requested", actor, source, nil); err != nil {
		return false, err
	}
	return true, nil
}

func (c ActuatorControl) Reconcile(ctx context.Context, now time.Time) (int, error) {
	leases, err := c.Store.OverdueActuatorLeases(ctx, now)
	if err != nil {
		return 0, err
	}
	stopped := 0
	var failures []error
	for _, lease := range leases {
		actuator, err := c.Store.Actuator(ctx, lease.ActuatorID)
		if err == nil {
			err = c.stop(ctx, actuator, lease, "deadline elapsed", "planty actuator reconciler", plant.SourceAutomation, nil)
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("stop actuator %s: %w", lease.ActuatorID, err))
			continue
		}
		stopped++
	}
	changed, err := c.reconcileSchedules(ctx, now)
	if err != nil {
		failures = append(failures, err)
	}
	return stopped + changed, errors.Join(failures...)
}

func (c ActuatorControl) SetLight(ctx context.Context, actuatorID uuid.UUID, on bool, actor string, source plant.Source) error {
	return c.setScheduledActuator(ctx, actuatorID, on, actor, source, plant.ActuatorLight)
}

func (c ActuatorControl) SetFanScheduleState(ctx context.Context, actuatorID uuid.UUID, on bool, actor string, source plant.Source) error {
	return c.setScheduledActuator(ctx, actuatorID, on, actor, source, plant.ActuatorFan)
}

func (c ActuatorControl) setScheduledActuator(ctx context.Context, actuatorID uuid.UUID, on bool, actor string, source plant.Source, kind plant.ActuatorKind) error {
	if c.HA == nil {
		return errors.New("Home Assistant actuation is not configured")
	}
	if strings.TrimSpace(actor) == "" {
		return fmt.Errorf("%w: actuator actor is required", plant.ErrInvalid)
	}
	switch source {
	case plant.SourceApp, plant.SourceAgent, plant.SourceAutomation:
	default:
		return fmt.Errorf("%w: unknown actuator source %q", plant.ErrInvalid, source)
	}
	actuator, err := c.Store.Actuator(ctx, actuatorID)
	if err != nil {
		return err
	}
	if actuator.Kind != kind {
		return fmt.Errorf("%w: direct state control requires a %s actuator", plant.ErrInvalid, kind)
	}
	service := "turn_off"
	if on {
		service = "turn_on"
	}
	domain, err := actuator.EntityDomain()
	if err != nil {
		return err
	}
	controlErr := c.HA.CallService(ctx, domain, service, map[string]any{"entity_id": actuator.EntityID})
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	recordErr := c.Store.RecordScheduledState(recordCtx, actuatorID, on, actor, source, controlErr)
	return errors.Join(controlErr, recordErr)
}

func (c ActuatorControl) reconcileSchedules(ctx context.Context, now time.Time) (int, error) {
	actuators, err := c.Store.Actuators(ctx)
	if err != nil {
		return 0, err
	}
	reader, canRead := c.HA.(actuatorStateReader)
	changed := 0
	var failures []error
	for _, actuator := range actuators {
		schedule := actuator.LightSchedule
		if actuator.Kind == plant.ActuatorFan {
			schedule = actuator.FanSchedule
			if actuator.ActiveLease != nil {
				continue
			}
		}
		if schedule == nil || (actuator.Kind != plant.ActuatorLight && actuator.Kind != plant.ActuatorFan) {
			continue
		}
		desired, err := schedule.WantsOn(now)
		if err != nil {
			failures = append(failures, fmt.Errorf("%s %s schedule: %w", actuator.Kind, actuator.ID, err))
			continue
		}
		if canRead {
			state, err := reader.State(ctx, actuator.EntityID)
			if err == nil && (state.State == "on") == desired {
				continue
			}
		}
		var controlErr error
		if actuator.Kind == plant.ActuatorLight {
			controlErr = c.SetLight(ctx, actuator.ID, desired, "planty light schedule", plant.SourceAutomation)
		} else {
			controlErr = c.SetFanScheduleState(ctx, actuator.ID, desired, "planty fan schedule", plant.SourceAutomation)
		}
		if controlErr != nil {
			failures = append(failures, fmt.Errorf("%s %s: %w", actuator.Kind, actuator.ID, controlErr))
			continue
		}
		changed++
	}
	return changed, errors.Join(failures...)
}

func (c ActuatorControl) stop(ctx context.Context, actuator plant.Actuator, lease plant.ActuatorLease, reason, actor string, source plant.Source, key *uuid.UUID) error {
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	domain, domainErr := actuator.EntityDomain()
	err := domainErr
	if err == nil {
		err = c.HA.CallService(stopCtx, domain, "turn_off", map[string]any{"entity_id": actuator.EntityID})
	}
	_, recordErr := c.Store.FinishActuatorLease(stopCtx, lease, actor, source, reason, err, key)
	return errors.Join(err, recordErr)
}
