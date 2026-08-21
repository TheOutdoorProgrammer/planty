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

// VerdictCompletion is one user claim that the care requested by a verdict was
// performed. IdempotencyKey belongs to the user action, not the HTTP attempt.
type VerdictCompletion struct {
	IdempotencyKey uuid.UUID
	VerdictID      uuid.UUID
	Kind           plant.ObservationKind
	Body           string
}

// CompleteVerdict records the care observation and acknowledges the verdict in
// one transaction. Replaying the same key and payload returns the original
// observation; reusing a key for a different operation is rejected.
func (s *Store) CompleteVerdict(ctx context.Context, c VerdictCompletion) (plant.Observation, error) {
	if c.IdempotencyKey == uuid.Nil || c.VerdictID == uuid.Nil {
		return plant.Observation{}, fmt.Errorf("%w: completion needs an idempotency key and verdict", plant.ErrInvalid)
	}
	// Validate the application enum before Postgres sees it. Otherwise an
	// unknown observation kind is rejected by the database enum as SQLSTATE 22
	// and looks like a server failure instead of a bad request.
	validation := plant.Observation{
		PlantID:    uuid.New(),
		Kind:       c.Kind,
		OccurredAt: time.Now().UTC(),
		Source:     plant.SourceApp,
	}
	if err := validation.Valid(); err != nil {
		return plant.Observation{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return plant.Observation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		INSERT INTO care_completions (idempotency_key, verdict_id, kind, body)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (idempotency_key) DO NOTHING`,
		c.IdempotencyKey, c.VerdictID, c.Kind, c.Body)
	if err != nil {
		return plant.Observation{}, classify(err)
	}

	if tag.RowsAffected() == 0 {
		return replayCompletion(ctx, tx, c)
	}

	var plantID uuid.UUID
	var acknowledged *time.Time
	if err := tx.QueryRow(ctx, `
		SELECT plant_id, acknowledged_at
		FROM verdicts
		WHERE id = $1
		FOR UPDATE`, c.VerdictID).Scan(&plantID, &acknowledged); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return plant.Observation{}, ErrNotFound
		}
		return plant.Observation{}, err
	}
	if acknowledged != nil {
		return plant.Observation{}, fmt.Errorf("%w: that verdict is already handled", plant.ErrInvalid)
	}

	o := plant.Observation{
		PlantID:    plantID,
		Kind:       c.Kind,
		Body:       c.Body,
		OccurredAt: time.Now().UTC(),
		Source:     plant.SourceApp,
	}

	created, err := addObservationTx(ctx, tx, o)
	if err != nil {
		return plant.Observation{}, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE verdicts SET acknowledged_at = now() WHERE id = $1`, c.VerdictID); err != nil {
		return plant.Observation{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE care_completions SET observation_id = $2
		WHERE idempotency_key = $1`, c.IdempotencyKey, created.ID); err != nil {
		return plant.Observation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return plant.Observation{}, err
	}
	return created, nil
}

func replayCompletion(ctx context.Context, tx pgx.Tx, c VerdictCompletion) (plant.Observation, error) {
	var verdictID uuid.UUID
	var kind plant.ObservationKind
	var body string
	var observationID *uuid.UUID
	if err := tx.QueryRow(ctx, `
		SELECT verdict_id, kind, body, observation_id
		FROM care_completions
		WHERE idempotency_key = $1`, c.IdempotencyKey).
		Scan(&verdictID, &kind, &body, &observationID); err != nil {
		return plant.Observation{}, err
	}
	if verdictID != c.VerdictID || kind != c.Kind || body != c.Body {
		return plant.Observation{}, fmt.Errorf("%w: idempotency key was already used for different care", plant.ErrInvalid)
	}
	if observationID == nil {
		return plant.Observation{}, errors.New("care completion exists without its observation")
	}

	out, err := observationTx(ctx, tx, *observationID)
	if err != nil {
		return plant.Observation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return plant.Observation{}, err
	}
	return out, nil
}

func addObservationTx(ctx context.Context, tx pgx.Tx, o plant.Observation) (plant.Observation, error) {
	row := tx.QueryRow(ctx, `
		INSERT INTO observations (plant_id, kind, body, occurred_at, source, actor)
		VALUES ($1, $2, $3, $4, $5, nullif($6,''))
		RETURNING id, plant_id, kind, body, occurred_at, source, coalesce(actor,''), created_at`,
		o.PlantID, o.Kind, o.Body, o.OccurredAt, o.Source, o.Actor)
	var out plant.Observation
	err := row.Scan(&out.ID, &out.PlantID, &out.Kind, &out.Body,
		&out.OccurredAt, &out.Source, &out.Actor, &out.CreatedAt)
	return out, classify(err)
}

func observationTx(ctx context.Context, tx pgx.Tx, id uuid.UUID) (plant.Observation, error) {
	var out plant.Observation
	err := tx.QueryRow(ctx, `
		SELECT id, plant_id, kind, body, occurred_at, source, coalesce(actor,''), created_at
		FROM observations WHERE id = $1`, id).
		Scan(&out.ID, &out.PlantID, &out.Kind, &out.Body,
			&out.OccurredAt, &out.Source, &out.Actor, &out.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return plant.Observation{}, ErrNotFound
	}
	return out, err
}
