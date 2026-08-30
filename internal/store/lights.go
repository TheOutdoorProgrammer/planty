package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const lightScheduleColumns = `actuator_id, start_minute, end_minute, timezone, enabled,
	last_applied_state, last_applied_at, last_error, created_at, updated_at`

func (s *Store) LightSchedule(ctx context.Context, actuatorID uuid.UUID) (plant.LightSchedule, error) {
	var schedule plant.LightSchedule
	err := s.pool.QueryRow(ctx, `SELECT `+lightScheduleColumns+`
		FROM plant_light_schedules WHERE actuator_id = $1`, actuatorID).Scan(
		&schedule.ActuatorID, &schedule.StartMinute, &schedule.EndMinute, &schedule.Timezone,
		&schedule.Enabled, &schedule.LastAppliedState, &schedule.LastAppliedAt,
		&schedule.LastError, &schedule.CreatedAt, &schedule.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return plant.LightSchedule{}, ErrNotFound
	}
	return schedule, err
}

func (s *Store) SetLightSchedule(ctx context.Context, schedule plant.LightSchedule, actor string, source plant.Source) (plant.LightSchedule, error) {
	if err := schedule.Valid(); err != nil {
		return plant.LightSchedule{}, err
	}
	if err := validActuatorActor(actor, source); err != nil {
		return plant.LightSchedule{}, err
	}
	actuator, err := s.Actuator(ctx, schedule.ActuatorID)
	if err != nil {
		return plant.LightSchedule{}, err
	}
	if actuator.Kind != plant.ActuatorLight {
		return plant.LightSchedule{}, fmt.Errorf("%w: schedules are only available for light actuators", plant.ErrInvalid)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return plant.LightSchedule{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var saved plant.LightSchedule
	err = tx.QueryRow(ctx, `
		INSERT INTO plant_light_schedules (actuator_id, start_minute, end_minute, timezone, enabled)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (actuator_id) DO UPDATE SET
			start_minute = excluded.start_minute,
			end_minute = excluded.end_minute,
			timezone = excluded.timezone,
			enabled = excluded.enabled,
			last_error = '',
			updated_at = now()
		RETURNING `+lightScheduleColumns, schedule.ActuatorID, schedule.StartMinute,
		schedule.EndMinute, schedule.Timezone, schedule.Enabled).Scan(
		&saved.ActuatorID, &saved.StartMinute, &saved.EndMinute, &saved.Timezone,
		&saved.Enabled, &saved.LastAppliedState, &saved.LastAppliedAt,
		&saved.LastError, &saved.CreatedAt, &saved.UpdatedAt,
	)
	if err != nil {
		return plant.LightSchedule{}, classify(err)
	}
	if err := insertActuatorEvent(ctx, tx, plant.ActuatorEvent{
		ActuatorID: schedule.ActuatorID, Action: "schedule_updated", Actor: actor,
		Source: source, Detail: fmt.Sprintf("%04d-%04d %s enabled=%t", schedule.StartMinute, schedule.EndMinute, schedule.Timezone, schedule.Enabled),
	}); err != nil {
		return plant.LightSchedule{}, err
	}
	return saved, tx.Commit(ctx)
}

func (s *Store) DeleteLightSchedule(ctx context.Context, actuatorID uuid.UUID, actor string, source plant.Source) error {
	if actuatorID == uuid.Nil {
		return fmt.Errorf("%w: actuator id is required", plant.ErrInvalid)
	}
	if err := validActuatorActor(actor, source); err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	command, err := tx.Exec(ctx, `DELETE FROM plant_light_schedules WHERE actuator_id = $1`, actuatorID)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return ErrNotFound
	}
	if err := insertActuatorEvent(ctx, tx, plant.ActuatorEvent{
		ActuatorID: actuatorID, Action: "schedule_disabled", Actor: actor,
		Source: source, Detail: "schedule removed",
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) RecordLightState(ctx context.Context, actuatorID uuid.UUID, on bool, actor string, source plant.Source, controlErr error) error {
	if actuatorID == uuid.Nil {
		return fmt.Errorf("%w: actuator id is required", plant.ErrInvalid)
	}
	if err := validActuatorActor(actor, source); err != nil {
		return err
	}
	action := "state_changed"
	detail := "off"
	if on {
		detail = "on"
	}
	lastError := ""
	if controlErr != nil {
		action = "schedule_failed"
		lastError = controlErr.Error()
		detail += ": " + lastError
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if controlErr == nil {
		if _, err := tx.Exec(ctx, `UPDATE plant_light_schedules SET
			last_applied_state = $2, last_applied_at = now(), last_error = '', updated_at = now()
			WHERE actuator_id = $1`, actuatorID, on); err != nil {
			return err
		}
	} else if _, err := tx.Exec(ctx, `UPDATE plant_light_schedules SET
		last_error = $2, updated_at = now() WHERE actuator_id = $1`, actuatorID, lastError); err != nil {
		return err
	}
	if err := insertActuatorEvent(ctx, tx, plant.ActuatorEvent{
		ActuatorID: actuatorID, Action: action, Actor: actor, Source: source, Detail: detail,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func validActuatorActor(actor string, source plant.Source) error {
	if strings.TrimSpace(actor) == "" {
		return fmt.Errorf("%w: actuator actor is required", plant.ErrInvalid)
	}
	switch source {
	case plant.SourceApp, plant.SourceAgent, plant.SourceAutomation:
		return nil
	default:
		return fmt.Errorf("%w: unknown actuator source %q", plant.ErrInvalid, source)
	}
}
