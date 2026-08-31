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

const actuatorScheduleColumns = `actuator_id, start_minute, end_minute, timezone, enabled,
	last_applied_state, last_applied_at, last_error, created_at, updated_at`

func (s *Store) actuatorSchedule(ctx context.Context, actuatorID uuid.UUID) (plant.ActuatorSchedule, error) {
	var schedule plant.ActuatorSchedule
	err := s.pool.QueryRow(ctx, `SELECT `+actuatorScheduleColumns+`
		FROM plant_actuator_schedules WHERE actuator_id = $1`, actuatorID).Scan(
		&schedule.ActuatorID, &schedule.StartMinute, &schedule.EndMinute, &schedule.Timezone,
		&schedule.Enabled, &schedule.LastAppliedState, &schedule.LastAppliedAt,
		&schedule.LastError, &schedule.CreatedAt, &schedule.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return plant.ActuatorSchedule{}, ErrNotFound
	}
	if err != nil {
		return plant.ActuatorSchedule{}, err
	}
	rows, err := s.pool.Query(ctx, `SELECT start_minute, end_minute
		FROM plant_actuator_schedule_windows WHERE actuator_id = $1 ORDER BY position`, actuatorID)
	if err != nil {
		return plant.ActuatorSchedule{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var window plant.ActuatorScheduleWindow
		if err := rows.Scan(&window.StartMinute, &window.EndMinute); err != nil {
			return plant.ActuatorSchedule{}, err
		}
		schedule.Windows = append(schedule.Windows, window)
	}
	return schedule, rows.Err()
}

func (s *Store) LightSchedule(ctx context.Context, actuatorID uuid.UUID) (plant.ActuatorSchedule, error) {
	return s.scheduleForKind(ctx, actuatorID, plant.ActuatorLight)
}

func (s *Store) FanSchedule(ctx context.Context, actuatorID uuid.UUID) (plant.ActuatorSchedule, error) {
	return s.scheduleForKind(ctx, actuatorID, plant.ActuatorFan)
}

func (s *Store) scheduleForKind(ctx context.Context, actuatorID uuid.UUID, kind plant.ActuatorKind) (plant.ActuatorSchedule, error) {
	actualKind, err := s.actuatorKind(ctx, actuatorID)
	if err != nil {
		return plant.ActuatorSchedule{}, err
	}
	if actualKind != kind {
		return plant.ActuatorSchedule{}, fmt.Errorf("%w: schedule requires a %s actuator", plant.ErrInvalid, kind)
	}
	return s.actuatorSchedule(ctx, actuatorID)
}

func (s *Store) SetLightSchedule(ctx context.Context, schedule plant.ActuatorSchedule, actor string, source plant.Source) (plant.ActuatorSchedule, error) {
	return s.setActuatorSchedule(ctx, schedule, actor, source, plant.ActuatorLight)
}

func (s *Store) SetFanSchedule(ctx context.Context, schedule plant.ActuatorSchedule, actor string, source plant.Source) (plant.ActuatorSchedule, error) {
	return s.setActuatorSchedule(ctx, schedule, actor, source, plant.ActuatorFan)
}

func (s *Store) setActuatorSchedule(ctx context.Context, schedule plant.ActuatorSchedule, actor string, source plant.Source, kind plant.ActuatorKind) (plant.ActuatorSchedule, error) {
	windows := schedule.EffectiveWindows()
	schedule.StartMinute = windows[0].StartMinute
	schedule.EndMinute = windows[0].EndMinute
	schedule.Windows = windows
	if err := schedule.Valid(); err != nil {
		return plant.ActuatorSchedule{}, err
	}
	if err := validActuatorActor(actor, source); err != nil {
		return plant.ActuatorSchedule{}, err
	}
	actualKind, err := s.actuatorKind(ctx, schedule.ActuatorID)
	if err != nil {
		return plant.ActuatorSchedule{}, err
	}
	if actualKind != kind {
		return plant.ActuatorSchedule{}, fmt.Errorf("%w: schedule requires a %s actuator", plant.ErrInvalid, kind)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return plant.ActuatorSchedule{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var saved plant.ActuatorSchedule
	err = tx.QueryRow(ctx, `
		INSERT INTO plant_actuator_schedules (actuator_id, start_minute, end_minute, timezone, enabled)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (actuator_id) DO UPDATE SET
			start_minute = excluded.start_minute,
			end_minute = excluded.end_minute,
			timezone = excluded.timezone,
			enabled = excluded.enabled,
			last_error = '',
			updated_at = now()
		RETURNING `+actuatorScheduleColumns, schedule.ActuatorID, schedule.StartMinute,
		schedule.EndMinute, schedule.Timezone, schedule.Enabled).Scan(
		&saved.ActuatorID, &saved.StartMinute, &saved.EndMinute, &saved.Timezone,
		&saved.Enabled, &saved.LastAppliedState, &saved.LastAppliedAt,
		&saved.LastError, &saved.CreatedAt, &saved.UpdatedAt,
	)
	if err != nil {
		return plant.ActuatorSchedule{}, classify(err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM plant_actuator_schedule_windows WHERE actuator_id = $1`, schedule.ActuatorID); err != nil {
		return plant.ActuatorSchedule{}, err
	}
	for position, window := range windows {
		if _, err := tx.Exec(ctx, `INSERT INTO plant_actuator_schedule_windows
			(actuator_id, position, start_minute, end_minute) VALUES ($1,$2,$3,$4)`,
			schedule.ActuatorID, position, window.StartMinute, window.EndMinute); err != nil {
			return plant.ActuatorSchedule{}, classify(err)
		}
	}
	saved.Windows = append([]plant.ActuatorScheduleWindow(nil), windows...)
	if err := insertActuatorEvent(ctx, tx, plant.ActuatorEvent{
		ActuatorID: schedule.ActuatorID, Action: "schedule_updated", Actor: actor,
		Source: source, Detail: fmt.Sprintf("%s %s enabled=%t", formatScheduleWindows(windows), schedule.Timezone, schedule.Enabled),
	}); err != nil {
		return plant.ActuatorSchedule{}, err
	}
	return saved, tx.Commit(ctx)
}

func formatScheduleWindows(windows []plant.ActuatorScheduleWindow) string {
	formatted := make([]string, 0, len(windows))
	for _, window := range windows {
		formatted = append(formatted, fmt.Sprintf("%04d-%04d", window.StartMinute, window.EndMinute))
	}
	return strings.Join(formatted, ",")
}

func (s *Store) DeleteLightSchedule(ctx context.Context, actuatorID uuid.UUID, actor string, source plant.Source) error {
	return s.deleteActuatorSchedule(ctx, actuatorID, actor, source, plant.ActuatorLight)
}

func (s *Store) DeleteFanSchedule(ctx context.Context, actuatorID uuid.UUID, actor string, source plant.Source) error {
	return s.deleteActuatorSchedule(ctx, actuatorID, actor, source, plant.ActuatorFan)
}

func (s *Store) deleteActuatorSchedule(ctx context.Context, actuatorID uuid.UUID, actor string, source plant.Source, kind plant.ActuatorKind) error {
	if actuatorID == uuid.Nil {
		return fmt.Errorf("%w: actuator id is required", plant.ErrInvalid)
	}
	if err := validActuatorActor(actor, source); err != nil {
		return err
	}
	actualKind, err := s.actuatorKind(ctx, actuatorID)
	if err != nil {
		return err
	}
	if actualKind != kind {
		return fmt.Errorf("%w: schedule requires a %s actuator", plant.ErrInvalid, kind)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	command, err := tx.Exec(ctx, `DELETE FROM plant_actuator_schedules WHERE actuator_id = $1`, actuatorID)
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

func (s *Store) RecordScheduledState(ctx context.Context, actuatorID uuid.UUID, on bool, actor string, source plant.Source, controlErr error) error {
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
		if _, err := tx.Exec(ctx, `UPDATE plant_actuator_schedules SET
			last_applied_state = $2, last_applied_at = now(), last_error = '', updated_at = now()
			WHERE actuator_id = $1`, actuatorID, on); err != nil {
			return err
		}
	} else if _, err := tx.Exec(ctx, `UPDATE plant_actuator_schedules SET
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

func (s *Store) RecordLightState(ctx context.Context, actuatorID uuid.UUID, on bool, actor string, source plant.Source, controlErr error) error {
	return s.RecordScheduledState(ctx, actuatorID, on, actor, source, controlErr)
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

func (s *Store) actuatorKind(ctx context.Context, actuatorID uuid.UUID) (plant.ActuatorKind, error) {
	var kind plant.ActuatorKind
	err := s.pool.QueryRow(ctx, `SELECT kind FROM plant_actuators
		WHERE id = $1 AND deleted_at IS NULL`, actuatorID).Scan(&kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return kind, err
}
