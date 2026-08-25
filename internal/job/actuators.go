package job

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
	"github.com/google/uuid"
)

type ActuatorHomeAssistant interface {
	CallService(context.Context, string, string, map[string]any) error
}

type ActuatorControl struct {
	Store *store.Store
	HA    ActuatorHomeAssistant
	Log   *slog.Logger
}

func (c ActuatorControl) Start(ctx context.Context, actuatorID uuid.UUID, seconds int, actor string, source plant.Source, key uuid.UUID) (plant.ActuatorLease, bool, error) {
	if c.HA == nil {
		return plant.ActuatorLease{}, false, errors.New("Home Assistant actuation is not configured")
	}
	actuator, lease, created, err := c.Store.BeginActuatorLease(ctx, plant.ActuatorLease{
		ActuatorID: actuatorID, RequestedSeconds: seconds, Actor: actor,
		Source: source, IdempotencyKey: key,
	})
	if err != nil || !created {
		return lease, created, err
	}
	if err := c.HA.CallService(ctx, string(actuator.Kind), "turn_on", map[string]any{"entity_id": actuator.EntityID}); err != nil {
		stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		stopErr := c.HA.CallService(stopCtx, string(actuator.Kind), "turn_off", map[string]any{"entity_id": actuator.EntityID})
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
	lease, err := c.Store.ActiveActuatorLease(ctx, actuatorID)
	if errors.Is(err, store.ErrNotFound) {
		if _, recordErr := c.Store.RecordActuatorNoopStop(ctx, actuatorID, actor, source, key); recordErr != nil {
			return false, recordErr
		}
		stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		return false, c.HA.CallService(stopCtx, string(actuator.Kind), "turn_off", map[string]any{"entity_id": actuator.EntityID})
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
	return stopped, errors.Join(failures...)
}

func (c ActuatorControl) stop(ctx context.Context, actuator plant.Actuator, lease plant.ActuatorLease, reason, actor string, source plant.Source, key *uuid.UUID) error {
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	err := c.HA.CallService(stopCtx, string(actuator.Kind), "turn_off", map[string]any{"entity_id": actuator.EntityID})
	_, recordErr := c.Store.FinishActuatorLease(stopCtx, lease, actor, source, reason, err, key)
	return errors.Join(err, recordErr)
}
