package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

const awayColumns = `
	id, starts_at, ends_at, coalesce(backup_contact,''),
	coalesce(backup_notify,''), coalesce(note,''), created_at`

func scanAway(row pgx.Row) (plant.AwayPeriod, error) {
	var a plant.AwayPeriod
	err := row.Scan(&a.ID, &a.StartsAt, &a.EndsAt, &a.BackupContact,
		&a.BackupNotify, &a.Note, &a.CreatedAt)
	return a, err
}

// createAway serializes coverage writes before checking for overlap. That makes
// "no overlapping away periods" true even when two clients save at once,
// without adding a migration that could strand an existing installation with
// historical overlap already in the table.
func (s *Store) createAway(ctx context.Context, a plant.AwayPeriod) (plant.AwayPeriod, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return plant.AwayPeriod{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockAwayPeriods(ctx, tx); err != nil {
		return plant.AwayPeriod{}, err
	}
	if err := ensureNoAwayOverlap(ctx, tx, uuid.Nil, a.StartsAt, a.EndsAt); err != nil {
		return plant.AwayPeriod{}, err
	}

	created, err := scanAway(tx.QueryRow(ctx, `
		INSERT INTO away_periods (starts_at, ends_at, backup_contact, backup_notify, note)
		VALUES ($1, $2, nullif($3,''), nullif($4,''), nullif($5,''))
		RETURNING `+awayColumns,
		a.StartsAt, a.EndsAt, a.BackupContact, a.BackupNotify, a.Note))
	if err != nil {
		return plant.AwayPeriod{}, classify(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return plant.AwayPeriod{}, err
	}
	return created, nil
}

// AwayPeriods lists coverage that still matters. Past periods are available for
// history/debugging when includePast is true, but the normal API and agent view
// stay focused on active and upcoming plans.
func (s *Store) AwayPeriods(ctx context.Context, includePast bool) ([]plant.AwayPeriod, error) {
	query := `SELECT ` + awayColumns + ` FROM away_periods`
	if !includePast {
		query += ` WHERE ends_at > now()`
	}
	query += ` ORDER BY starts_at, id`

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []plant.AwayPeriod{}
	for rows.Next() {
		a, err := scanAway(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// AwayPeriod returns one plan by id so sparse updates can preserve fields the
// caller did not mention.
func (s *Store) AwayPeriod(ctx context.Context, id uuid.UUID) (plant.AwayPeriod, error) {
	a, err := scanAway(s.pool.QueryRow(ctx, `SELECT `+awayColumns+` FROM away_periods WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return plant.AwayPeriod{}, ErrNotFound
	}
	return a, err
}

// UpdateAway replaces one plan after validating its time window and checking
// that the replacement does not overlap any other plan.
func (s *Store) UpdateAway(ctx context.Context, id uuid.UUID, a plant.AwayPeriod) (plant.AwayPeriod, error) {
	if !a.EndsAt.After(a.StartsAt) {
		return plant.AwayPeriod{}, fmt.Errorf(
			"%w: ends_at must be after starts_at", plant.ErrInvalid)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return plant.AwayPeriod{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockAwayPeriods(ctx, tx); err != nil {
		return plant.AwayPeriod{}, err
	}
	if err := ensureNoAwayOverlap(ctx, tx, id, a.StartsAt, a.EndsAt); err != nil {
		return plant.AwayPeriod{}, err
	}

	updated, err := scanAway(tx.QueryRow(ctx, `
		UPDATE away_periods
		SET starts_at = $2,
		    ends_at = $3,
		    backup_contact = nullif($4,''),
		    backup_notify = nullif($5,''),
		    note = nullif($6,'')
		WHERE id = $1
		RETURNING `+awayColumns,
		id, a.StartsAt, a.EndsAt, a.BackupContact, a.BackupNotify, a.Note))
	if errors.Is(err, pgx.ErrNoRows) {
		return plant.AwayPeriod{}, ErrNotFound
	}
	if err != nil {
		return plant.AwayPeriod{}, classify(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return plant.AwayPeriod{}, err
	}
	return updated, nil
}

// DeleteAway cancels a plan. It is intentionally a real delete: an accidental
// future coverage window is not a care event worth preserving as history.
func (s *Store) DeleteAway(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM away_periods WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func lockAwayPeriods(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('planty-away-periods')::bigint)`)
	return err
}

func ensureNoAwayOverlap(
	ctx context.Context,
	tx pgx.Tx,
	excludeID uuid.UUID,
	startsAt, endsAt time.Time,
) error {
	var conflict uuid.UUID
	err := tx.QueryRow(ctx, `
		SELECT id
		FROM away_periods
		WHERE id <> $1
		  AND starts_at < $3
		  AND ends_at > $2
		ORDER BY starts_at
		LIMIT 1`, excludeID, startsAt, endsAt).Scan(&conflict)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf(
		"%w: another away period already covers part of that time", plant.ErrInvalid)
}
