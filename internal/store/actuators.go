package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const actuatorColumns = `id, entity_id, name, kind, policy_control_enabled, created_at, updated_at`
const actuatorLeaseColumns = `id, actuator_id, requested_seconds, deadline, actor, source,
	idempotency_key, started_at, stopped_at, stop_reason, created_at`

func (s *Store) Actuators(ctx context.Context) ([]plant.Actuator, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+actuatorColumns+` FROM plant_actuators
		WHERE deleted_at IS NULL ORDER BY lower(name), entity_id`)
	if err != nil {
		return nil, err
	}
	out := []plant.Actuator{}
	for rows.Next() {
		actuator, err := scanActuator(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, actuator)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for i := range out {
		if err := s.hydrateActuator(ctx, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Store) Actuator(ctx context.Context, id uuid.UUID) (plant.Actuator, error) {
	actuator, err := scanActuator(s.pool.QueryRow(ctx, `SELECT `+actuatorColumns+`
		FROM plant_actuators WHERE id = $1 AND deleted_at IS NULL`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return plant.Actuator{}, ErrNotFound
	}
	if err != nil {
		return plant.Actuator{}, err
	}
	if err := s.hydrateActuator(ctx, &actuator); err != nil {
		return plant.Actuator{}, err
	}
	return actuator, nil
}

func (s *Store) RegisterActuator(ctx context.Context, actuator plant.Actuator) (plant.Actuator, error) {
	actuator.EntityID = strings.TrimSpace(actuator.EntityID)
	actuator.Name = strings.TrimSpace(actuator.Name)
	if err := actuator.Valid(); err != nil {
		return plant.Actuator{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return plant.Actuator{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	created, err := scanActuator(tx.QueryRow(ctx, `
		INSERT INTO plant_actuators (entity_id, name, kind) VALUES ($1,$2,$3)
		RETURNING `+actuatorColumns, actuator.EntityID, actuator.Name, actuator.Kind))
	if err != nil {
		return plant.Actuator{}, classify(err)
	}
	if err := replaceActuatorPlants(ctx, tx, created.ID, actuator.PlantIDs); err != nil {
		return plant.Actuator{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return plant.Actuator{}, err
	}
	return s.Actuator(ctx, created.ID)
}

func (s *Store) UpdateActuator(ctx context.Context, id uuid.UUID, name string, kind plant.ActuatorKind, plantIDs []uuid.UUID, policyControlEnabled bool) (plant.Actuator, error) {
	name = strings.TrimSpace(name)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return plant.Actuator{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	actuator, err := scanActuator(tx.QueryRow(ctx, `SELECT `+actuatorColumns+`
		FROM plant_actuators WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return plant.Actuator{}, ErrNotFound
	}
	if err != nil {
		return plant.Actuator{}, err
	}
	if kind == "" {
		kind = actuator.Kind
	}
	actuator.Name = name
	actuator.Kind = kind
	actuator.PlantIDs = plantIDs
	actuator.PolicyControlEnabled = policyControlEnabled
	if err := actuator.Valid(); err != nil {
		return plant.Actuator{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE plant_actuators
		SET name = $2, kind = $3, policy_control_enabled = $4, updated_at = now()
		WHERE id = $1`, id, name, kind, policyControlEnabled); err != nil {
		return plant.Actuator{}, err
	}
	if actuator.Kind != plant.ActuatorLight && actuator.Kind != plant.ActuatorFan {
		if _, err := tx.Exec(ctx, `DELETE FROM plant_actuator_schedules WHERE actuator_id = $1`, id); err != nil {
			return plant.Actuator{}, err
		}
	}
	if err := replaceActuatorPlants(ctx, tx, id, plantIDs); err != nil {
		return plant.Actuator{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return plant.Actuator{}, err
	}
	return s.Actuator(ctx, id)
}

func replaceActuatorPlants(ctx context.Context, tx pgx.Tx, actuatorID uuid.UUID, plantIDs []uuid.UUID) error {
	var living int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM plants
		WHERE id = ANY($1) AND archived_at IS NULL`, plantIDs).Scan(&living); err != nil {
		return err
	}
	if living != len(plantIDs) {
		return fmt.Errorf("%w: every actuator plant_id must name a living plant", plant.ErrInvalid)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM plant_actuator_plants WHERE actuator_id = $1`, actuatorID); err != nil {
		return err
	}
	for _, plantID := range plantIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO plant_actuator_plants (actuator_id, plant_id)
			VALUES ($1,$2)`, actuatorID, plantID); err != nil {
			return classify(err)
		}
	}
	return nil
}

func (s *Store) hydrateActuator(ctx context.Context, actuator *plant.Actuator) error {
	rows, err := s.pool.Query(ctx, `SELECT plant_id FROM plant_actuator_plants
		WHERE actuator_id = $1 ORDER BY plant_id`, actuator.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var plantID uuid.UUID
		if err := rows.Scan(&plantID); err != nil {
			rows.Close()
			return err
		}
		actuator.PlantIDs = append(actuator.PlantIDs, plantID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	lease, err := s.ActiveActuatorLease(ctx, actuator.ID)
	if err == nil {
		actuator.ActiveLease = &lease
	} else if !errors.Is(err, ErrNotFound) {
		return err
	}
	if actuator.Kind == plant.ActuatorLight {
		schedule, err := s.LightSchedule(ctx, actuator.ID)
		if err == nil {
			actuator.LightSchedule = &schedule
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
	} else if actuator.Kind == plant.ActuatorFan {
		schedule, err := s.FanSchedule(ctx, actuator.ID)
		if err == nil {
			actuator.FanSchedule = &schedule
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
	}
	return nil
}

func (s *Store) DeleteActuator(ctx context.Context, id uuid.UUID) error {
	command, err := s.pool.Exec(ctx, `UPDATE plant_actuators SET deleted_at = now(), updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL
		AND NOT EXISTS (SELECT 1 FROM plant_actuator_leases WHERE actuator_id = $1 AND stopped_at IS NULL)`, id)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 1 {
		return nil
	}
	if _, err := s.Actuator(ctx, id); errors.Is(err, ErrNotFound) {
		return ErrNotFound
	}
	return fmt.Errorf("%w: stop the actuator before removing it", plant.ErrInvalid)
}

// BeginActuatorLease commits the deadline before Home Assistant is called.
// A crash after this returns therefore leaves reconciliation enough state to
// issue turn_off even when nobody knows whether turn_on reached the device.
func (s *Store) BeginActuatorLease(ctx context.Context, lease plant.ActuatorLease) (plant.Actuator, plant.ActuatorLease, bool, error) {
	return s.beginActuatorLease(ctx, lease, uuid.Nil)
}

// BeginActuatorLeaseForPlant applies the same durable lease path while proving
// the caller's plant is currently assigned to the actuator under the actuator lock.
func (s *Store) BeginActuatorLeaseForPlant(ctx context.Context, lease plant.ActuatorLease, plantID uuid.UUID) (plant.Actuator, plant.ActuatorLease, bool, error) {
	if plantID == uuid.Nil {
		return plant.Actuator{}, plant.ActuatorLease{}, false, fmt.Errorf("%w: plant id is required", plant.ErrInvalid)
	}
	return s.beginActuatorLease(ctx, lease, plantID)
}

func (s *Store) beginActuatorLease(ctx context.Context, lease plant.ActuatorLease, plantID uuid.UUID) (plant.Actuator, plant.ActuatorLease, bool, error) {
	if err := lease.Valid(); err != nil {
		return plant.Actuator{}, plant.ActuatorLease{}, false, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return plant.Actuator{}, plant.ActuatorLease{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	actuator, err := scanActuator(tx.QueryRow(ctx, `SELECT `+actuatorColumns+`
		FROM plant_actuators WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, lease.ActuatorID))
	if errors.Is(err, pgx.ErrNoRows) {
		return plant.Actuator{}, plant.ActuatorLease{}, false, ErrNotFound
	}
	if err != nil {
		return plant.Actuator{}, plant.ActuatorLease{}, false, err
	}
	if actuator.Kind != plant.ActuatorFan {
		return plant.Actuator{}, plant.ActuatorLease{}, false, fmt.Errorf("%w: bounded runs are only available for fans", plant.ErrInvalid)
	}
	existing, err := scanActuatorLease(tx.QueryRow(ctx, `SELECT `+actuatorLeaseColumns+`
		FROM plant_actuator_leases WHERE idempotency_key = $1`, lease.IdempotencyKey))
	if err == nil {
		if existing.ActuatorID != lease.ActuatorID {
			return plant.Actuator{}, plant.ActuatorLease{}, false, fmt.Errorf("%w: idempotency key belongs to another actuator", plant.ErrInvalid)
		}
		return actuator, existing, false, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return plant.Actuator{}, plant.ActuatorLease{}, false, err
	}
	if plantID != uuid.Nil {
		var assigned bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM plant_actuator_plants WHERE actuator_id = $1 AND plant_id = $2
		)`, actuator.ID, plantID).Scan(&assigned); err != nil {
			return plant.Actuator{}, plant.ActuatorLease{}, false, err
		}
		if !assigned {
			return plant.Actuator{}, plant.ActuatorLease{}, false, fmt.Errorf("%w: actuator is not assigned to that plant", plant.ErrInvalid)
		}
	}

	var active uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM plant_actuator_leases
		WHERE actuator_id = $1 AND stopped_at IS NULL`, lease.ActuatorID).Scan(&active)
	if err == nil {
		return plant.Actuator{}, plant.ActuatorLease{}, false, fmt.Errorf("%w: actuator is already running", plant.ErrInvalid)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return plant.Actuator{}, plant.ActuatorLease{}, false, err
	}

	lease.Deadline = time.Now().UTC().Add(time.Duration(lease.RequestedSeconds) * time.Second)
	created, err := scanActuatorLease(tx.QueryRow(ctx, `INSERT INTO plant_actuator_leases
		(actuator_id, requested_seconds, deadline, actor, source, idempotency_key)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING `+actuatorLeaseColumns,
		lease.ActuatorID, lease.RequestedSeconds, lease.Deadline, lease.Actor, lease.Source, lease.IdempotencyKey))
	if err != nil {
		return plant.Actuator{}, plant.ActuatorLease{}, false, classify(err)
	}
	if err := insertActuatorEvent(ctx, tx, plant.ActuatorEvent{
		ActuatorID: actuator.ID, LeaseID: &created.ID, Action: "start_requested",
		Actor: lease.Actor, Source: lease.Source, IdempotencyKey: &lease.IdempotencyKey,
		Detail: fmt.Sprintf("requested %d seconds; deadline %s", lease.RequestedSeconds, created.Deadline.Format(time.RFC3339)),
	}); err != nil {
		return plant.Actuator{}, plant.ActuatorLease{}, false, err
	}
	return actuator, created, true, tx.Commit(ctx)
}

func (s *Store) MarkActuatorStarted(ctx context.Context, lease plant.ActuatorLease) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var actuatorName string
	if err := tx.QueryRow(ctx, `SELECT name FROM plant_actuators
		WHERE id = $1 AND deleted_at IS NULL FOR SHARE`, lease.ActuatorID).Scan(&actuatorName); err != nil {
		return classify(err)
	}
	var startedAt time.Time
	if err := tx.QueryRow(ctx, `UPDATE plant_actuator_leases SET started_at = now()
		WHERE id = $1 AND stopped_at IS NULL AND started_at IS NULL
		RETURNING started_at`, lease.ID).Scan(&startedAt); errors.Is(err, pgx.ErrNoRows) {
		return tx.Commit(ctx)
	} else if err != nil {
		return err
	}
	if err := insertActuatorEvent(ctx, tx, plant.ActuatorEvent{
		ActuatorID: lease.ActuatorID, LeaseID: &lease.ID, Action: "started",
		Actor: lease.Actor, Source: lease.Source,
	}); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT plant_id FROM plant_actuator_plants
		WHERE actuator_id = $1 ORDER BY plant_id`, lease.ActuatorID)
	if err != nil {
		return err
	}
	var plantIDs []uuid.UUID
	for rows.Next() {
		var plantID uuid.UUID
		if err := rows.Scan(&plantID); err != nil {
			rows.Close()
			return err
		}
		plantIDs = append(plantIDs, plantID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if len(plantIDs) == 0 {
		return fmt.Errorf("%w: actuator has no plant assignments", plant.ErrInvalid)
	}
	body := fmt.Sprintf("%s started for up to %s.", actuatorName, airflowDuration(lease.RequestedSeconds))
	for _, plantID := range plantIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO observations
			(plant_id, kind, body, occurred_at, source, actor)
			VALUES ($1,$2,$3,$4,$5,nullif($6,''))`, plantID, plant.ObservedAirflow,
			body, startedAt, lease.Source, lease.Actor); err != nil {
			return classify(err)
		}
	}
	return tx.Commit(ctx)
}

func airflowDuration(seconds int) string {
	duration := time.Duration(seconds) * time.Second
	if duration%time.Hour == 0 {
		hours := int(duration / time.Hour)
		return fmt.Sprintf("%d %s", hours, plural(hours, "hour"))
	}
	if duration%time.Minute == 0 {
		minutes := int(duration / time.Minute)
		return fmt.Sprintf("%d %s", minutes, plural(minutes, "minute"))
	}
	return fmt.Sprintf("%d %s", seconds, plural(seconds, "second"))
}

func plural(value int, unit string) string {
	if value == 1 {
		return unit
	}
	return unit + "s"
}

func (s *Store) ActiveActuatorLease(ctx context.Context, actuatorID uuid.UUID) (plant.ActuatorLease, error) {
	lease, err := scanActuatorLease(s.pool.QueryRow(ctx, `SELECT `+actuatorLeaseColumns+`
		FROM plant_actuator_leases WHERE actuator_id = $1 AND stopped_at IS NULL`, actuatorID))
	if errors.Is(err, pgx.ErrNoRows) {
		return plant.ActuatorLease{}, ErrNotFound
	}
	return lease, err
}

func (s *Store) OverdueActuatorLeases(ctx context.Context, now time.Time) ([]plant.ActuatorLease, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+actuatorLeaseColumns+` FROM plant_actuator_leases
		WHERE stopped_at IS NULL AND deadline <= $1 ORDER BY deadline`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []plant.ActuatorLease{}
	for rows.Next() {
		lease, err := scanActuatorLease(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, lease)
	}
	return out, rows.Err()
}

// FinishActuatorLease leaves failed stops active so the next reconciliation
// retries them. Successful completion is conditional, making concurrent and
// repeated stops idempotent.
func (s *Store) FinishActuatorLease(ctx context.Context, lease plant.ActuatorLease, actor string, source plant.Source, reason string, stopErr error, key *uuid.UUID) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	action, detail := "stopped", reason
	if stopErr != nil {
		action, detail = "stop_failed", stopErr.Error()
	} else {
		command, err := tx.Exec(ctx, `UPDATE plant_actuator_leases
			SET stopped_at = now(), stop_reason = $2 WHERE id = $1 AND stopped_at IS NULL`, lease.ID, reason)
		if err != nil {
			return false, err
		}
		if command.RowsAffected() == 0 {
			return false, tx.Commit(ctx)
		}
	}
	if err := insertActuatorEvent(ctx, tx, plant.ActuatorEvent{
		ActuatorID: lease.ActuatorID, LeaseID: &lease.ID, Action: action,
		Actor: actor, Source: source, IdempotencyKey: key, Detail: detail,
	}); err != nil {
		return false, classify(err)
	}
	return true, tx.Commit(ctx)
}

func (s *Store) RecordActuatorStartFailure(ctx context.Context, lease plant.ActuatorLease, cause error, stopConfirmed bool) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if stopConfirmed {
		if _, err := tx.Exec(ctx, `UPDATE plant_actuator_leases SET stopped_at = now(), stop_reason = 'start_failed'
			WHERE id = $1 AND stopped_at IS NULL`, lease.ID); err != nil {
			return err
		}
	}
	if err := insertActuatorEvent(ctx, tx, plant.ActuatorEvent{
		ActuatorID: lease.ActuatorID, LeaseID: &lease.ID, Action: "start_failed",
		Actor: lease.Actor, Source: lease.Source, Detail: cause.Error(),
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) RecordActuatorNoopStop(ctx context.Context, actuatorID uuid.UUID, actor string, source plant.Source, key uuid.UUID) (bool, error) {
	_, err := s.Actuator(ctx, actuatorID)
	if err != nil {
		return false, err
	}
	event := plant.ActuatorEvent{ActuatorID: actuatorID, Action: "stop_noop", Actor: actor, Source: source, IdempotencyKey: &key, Detail: "no active lease; issued turn_off defensively"}
	err = insertActuatorEvent(ctx, s.pool, event)
	if err == nil {
		return true, nil
	}
	var duplicate *pgconn.PgError
	if errors.As(err, &duplicate) && duplicate.Code == "23505" && duplicate.ConstraintName == "plant_actuator_event_idempotency" {
		var existingActuator uuid.UUID
		if queryErr := s.pool.QueryRow(ctx, `SELECT actuator_id FROM plant_actuator_events
			WHERE idempotency_key = $1`, key).Scan(&existingActuator); queryErr != nil {
			return false, queryErr
		}
		if existingActuator != actuatorID {
			return false, fmt.Errorf("%w: idempotency key belongs to another actuator", plant.ErrInvalid)
		}
		return false, nil
	}
	return false, classify(err)
}

func (s *Store) RecordActuatorStopRequested(ctx context.Context, lease plant.ActuatorLease, actor string, source plant.Source, key uuid.UUID) (bool, error) {
	event := plant.ActuatorEvent{
		ActuatorID: lease.ActuatorID, LeaseID: &lease.ID, Action: "stop_requested",
		Actor: actor, Source: source, IdempotencyKey: &key,
	}
	err := insertActuatorEvent(ctx, s.pool, event)
	if err == nil {
		return true, nil
	}
	var duplicate *pgconn.PgError
	if errors.As(err, &duplicate) && duplicate.Code == "23505" && duplicate.ConstraintName == "plant_actuator_event_idempotency" {
		var existingActuator uuid.UUID
		if queryErr := s.pool.QueryRow(ctx, `SELECT actuator_id FROM plant_actuator_events
			WHERE idempotency_key = $1`, key).Scan(&existingActuator); queryErr != nil {
			return false, queryErr
		}
		if existingActuator != lease.ActuatorID {
			return false, fmt.Errorf("%w: idempotency key belongs to another actuator", plant.ErrInvalid)
		}
		return false, nil
	}
	return false, classify(err)
}

func (s *Store) ActuatorEvents(ctx context.Context, actuatorID uuid.UUID, limit int) ([]plant.ActuatorEvent, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `SELECT id, actuator_id, lease_id, action, actor, source,
		idempotency_key, detail, created_at FROM plant_actuator_events
		WHERE actuator_id = $1 ORDER BY created_at DESC, id DESC LIMIT $2`, actuatorID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []plant.ActuatorEvent{}
	for rows.Next() {
		var event plant.ActuatorEvent
		if err := rows.Scan(&event.ID, &event.ActuatorID, &event.LeaseID, &event.Action,
			&event.Actor, &event.Source, &event.IdempotencyKey, &event.Detail, &event.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

type actuatorEventExecer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func insertActuatorEvent(ctx context.Context, db actuatorEventExecer, event plant.ActuatorEvent) error {
	_, err := db.Exec(ctx, `INSERT INTO plant_actuator_events
		(actuator_id, lease_id, action, actor, source, idempotency_key, detail)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`, event.ActuatorID, event.LeaseID, event.Action,
		event.Actor, event.Source, event.IdempotencyKey, event.Detail)
	return err
}

func scanActuator(row interface{ Scan(...any) error }) (plant.Actuator, error) {
	var actuator plant.Actuator
	err := row.Scan(&actuator.ID, &actuator.EntityID, &actuator.Name, &actuator.Kind,
		&actuator.PolicyControlEnabled, &actuator.CreatedAt, &actuator.UpdatedAt)
	return actuator, err
}

func scanActuatorLease(row interface{ Scan(...any) error }) (plant.ActuatorLease, error) {
	var lease plant.ActuatorLease
	err := row.Scan(&lease.ID, &lease.ActuatorID, &lease.RequestedSeconds, &lease.Deadline,
		&lease.Actor, &lease.Source, &lease.IdempotencyKey, &lease.StartedAt,
		&lease.StoppedAt, &lease.StopReason, &lease.CreatedAt)
	return lease, err
}
