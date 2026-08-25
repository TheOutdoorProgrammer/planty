package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const healthColumns = `
	id, plant_id, score, requested_delta, applied_delta, rationale, evidence,
	source, actor, judgment_run_id, idempotency_key, created_at`

// LatestHealth returns the newest assessment, or ErrNotFound while health is
// genuinely unknown.
func (s *Store) LatestHealth(ctx context.Context, plantID uuid.UUID) (plant.HealthEvent, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+healthColumns+`
		FROM plant_health_events WHERE plant_id = $1
		ORDER BY created_at DESC, id DESC LIMIT 1`, plantID)
	event, err := scanHealth(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return plant.HealthEvent{}, ErrNotFound
	}
	return event, err
}

// HealthHistory returns newest first. Health is sparse, so a bounded list is
// enough for both the agent and the initial API without inventing a second
// cursor shape.
func (s *Store) HealthHistory(ctx context.Context, plantID uuid.UUID, limit int) ([]plant.HealthEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `SELECT `+healthColumns+`
		FROM plant_health_events WHERE plant_id = $1
		ORDER BY created_at DESC, id DESC LIMIT $2`, plantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []plant.HealthEvent{}
	for rows.Next() {
		event, err := scanHealth(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

// RecordHealth serializes through the plant row, clamps a requested change,
// and appends one event. The current row is never rewritten.
func (s *Store) RecordHealth(ctx context.Context, change plant.HealthChange) (plant.HealthEvent, bool, error) {
	if err := change.Valid(); err != nil {
		return plant.HealthEvent{}, false, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return plant.HealthEvent{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var exists bool
	if err := tx.QueryRow(ctx, `SELECT true FROM plants WHERE id = $1 FOR UPDATE`, change.PlantID).Scan(&exists); err != nil {
		return plant.HealthEvent{}, false, classify(err)
	}

	if prior, found, err := existingHealthWrite(ctx, tx, change); err != nil {
		return plant.HealthEvent{}, false, err
	} else if found {
		return prior, false, tx.Commit(ctx)
	}

	current, hasCurrent, err := latestHealthTx(ctx, tx, change.PlantID)
	if err != nil {
		return plant.HealthEvent{}, false, err
	}

	var score float64
	var requested, applied *float64
	switch {
	case change.Baseline != nil:
		if hasCurrent {
			return plant.HealthEvent{}, false, fmt.Errorf("%w: health already has a baseline", plant.ErrInvalid)
		}
		score = *change.Baseline
	case !hasCurrent:
		return plant.HealthEvent{}, false, fmt.Errorf("%w: health is unknown; establish a baseline before applying a delta", plant.ErrInvalid)
	default:
		score = min(100, max(0, current.Score+*change.Delta))
		actual := score - current.Score
		if actual == 0 {
			return plant.HealthEvent{}, false, fmt.Errorf("%w: that delta leaves health unchanged at %.0f", plant.ErrInvalid, score)
		}
		asked := *change.Delta
		requested, applied = &asked, &actual
	}

	newestEvidence, err := validateHealthEvidence(ctx, tx, change.PlantID, change.Evidence)
	if err != nil {
		return plant.HealthEvent{}, false, err
	}
	if change.JudgmentRunID != nil {
		if !change.Evidence.HasReferences() {
			return plant.HealthEvent{}, false, fmt.Errorf("%w: automated health changes require record-backed evidence", plant.ErrInvalid)
		}
		if hasCurrent && !newestEvidence.After(current.CreatedAt) {
			return plant.HealthEvent{}, false, fmt.Errorf("%w: no evidence is newer than the current health assessment", plant.ErrInvalid)
		}
	}

	raw, err := json.Marshal(change.Evidence)
	if err != nil {
		return plant.HealthEvent{}, false, err
	}
	row := tx.QueryRow(ctx, `
		INSERT INTO plant_health_events (
			plant_id, score, requested_delta, applied_delta, rationale, evidence,
			source, actor, judgment_run_id, idempotency_key
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING `+healthColumns,
		change.PlantID, score, requested, applied, change.Rationale, raw,
		change.Source, change.Actor, change.JudgmentRunID, change.IdempotencyKey)
	event, err := scanHealth(row)
	if err != nil {
		return plant.HealthEvent{}, false, classify(err)
	}
	return event, true, tx.Commit(ctx)
}

func latestHealthTx(ctx context.Context, tx pgx.Tx, plantID uuid.UUID) (plant.HealthEvent, bool, error) {
	event, err := scanHealth(tx.QueryRow(ctx, `SELECT `+healthColumns+`
		FROM plant_health_events WHERE plant_id = $1
		ORDER BY created_at DESC, id DESC LIMIT 1`, plantID))
	if errors.Is(err, pgx.ErrNoRows) {
		return plant.HealthEvent{}, false, nil
	}
	return event, err == nil, err
}

func existingHealthWrite(ctx context.Context, tx pgx.Tx, change plant.HealthChange) (plant.HealthEvent, bool, error) {
	var row pgx.Row
	switch {
	case change.IdempotencyKey != nil:
		row = tx.QueryRow(ctx, `SELECT `+healthColumns+` FROM plant_health_events WHERE idempotency_key = $1`, change.IdempotencyKey)
	case change.JudgmentRunID != nil:
		row = tx.QueryRow(ctx, `SELECT `+healthColumns+` FROM plant_health_events WHERE judgment_run_id = $1 AND plant_id = $2`, change.JudgmentRunID, change.PlantID)
	default:
		return plant.HealthEvent{}, false, nil
	}
	event, err := scanHealth(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return plant.HealthEvent{}, false, nil
	}
	if err != nil {
		return plant.HealthEvent{}, false, err
	}
	if event.PlantID != change.PlantID {
		return plant.HealthEvent{}, false, fmt.Errorf("%w: idempotency key was used for another plant", plant.ErrInvalid)
	}
	return event, true, nil
}

func validateHealthEvidence(ctx context.Context, tx pgx.Tx, plantID uuid.UUID, evidence plant.HealthEvidence) (time.Time, error) {
	var newest time.Time
	checks := []struct {
		name  string
		ids   []uuid.UUID
		query string
	}{
		{"observations", evidence.ObservationIDs, `SELECT count(*), max(occurred_at) FROM observations WHERE plant_id = $1 AND id = ANY($2)`},
		{"photos", evidence.PhotoIDs, `SELECT count(*), max(taken_at) FROM photos WHERE plant_id = $1 AND deletion_requested_at IS NULL AND id = ANY($2)`},
		{"readings", evidence.ReadingIDs, `SELECT count(*), max(r.taken_at) FROM readings r JOIN sensor_links s ON s.id = r.sensor_link_id WHERE s.plant_id = $1 AND r.id = ANY($2)`},
	}
	for _, check := range checks {
		if len(check.ids) == 0 {
			continue
		}
		var count int
		var latest *time.Time
		if err := tx.QueryRow(ctx, check.query, plantID, check.ids).Scan(&count, &latest); err != nil {
			return time.Time{}, err
		}
		if count != len(check.ids) {
			return time.Time{}, fmt.Errorf("%w: health evidence includes %s not belonging to this plant", plant.ErrInvalid, check.name)
		}
		if latest != nil && latest.After(newest) {
			newest = *latest
		}
	}
	return newest, nil
}

func scanHealth(row interface{ Scan(...any) error }) (plant.HealthEvent, error) {
	var event plant.HealthEvent
	var raw []byte
	err := row.Scan(&event.ID, &event.PlantID, &event.Score,
		&event.RequestedDelta, &event.AppliedDelta, &event.Rationale, &raw,
		&event.Source, &event.Actor, &event.JudgmentRunID, &event.IdempotencyKey, &event.CreatedAt)
	if err != nil {
		return plant.HealthEvent{}, err
	}
	if err := json.Unmarshal(raw, &event.Evidence); err != nil {
		return plant.HealthEvent{}, err
	}
	return event, nil
}
