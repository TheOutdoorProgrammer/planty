package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// JudgmentRun records how much of one garden-wide daily check actually
// completed. Expected is fixed at the start so plants appearing or disappearing
// midway through a model run cannot rewrite what that run claimed to cover.
type JudgmentRun struct {
	ID          uuid.UUID
	StartedAt   time.Time
	CompletedAt *time.Time
	Expected    int
	Succeeded   int
	Failed      int
}

// StartJudgmentRun makes incompleteness durable before the first model call.
func (s *Store) StartJudgmentRun(ctx context.Context, expected int) (JudgmentRun, error) {
	var run JudgmentRun
	err := s.pool.QueryRow(ctx, `
		INSERT INTO judgment_runs (expected)
		VALUES ($1)
		RETURNING id, started_at, completed_at, expected, succeeded, failed`, expected).
		Scan(&run.ID, &run.StartedAt, &run.CompletedAt, &run.Expected, &run.Succeeded, &run.Failed)
	return run, classify(err)
}

// RecordJudgmentResult advances exactly one plant in a run. Recording after
// each plant means a process crash still leaves an honest partial run behind.
func (s *Store) RecordJudgmentResult(ctx context.Context, id uuid.UUID, succeeded bool) error {
	column := "failed"
	if succeeded {
		column = "succeeded"
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE judgment_runs
		SET `+column+` = `+column+` + 1
		WHERE id = $1 AND completed_at IS NULL
		  AND succeeded + failed < expected`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CompleteJudgmentRun seals the counts. A run may complete with failures; that
// distinction is what lets Today say "incomplete" instead of "all clear".
func (s *Store) CompleteJudgmentRun(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE judgment_runs SET completed_at = now()
		WHERE id = $1 AND completed_at IS NULL`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// LatestJudgmentRun is the newest attempt, including an unfinished attempt. An
// interrupted run must outrank yesterday's complete run or Today could still
// claim yesterday's confidence after today's check crashed halfway through.
func (s *Store) LatestJudgmentRun(ctx context.Context) (JudgmentRun, error) {
	var run JudgmentRun
	err := s.pool.QueryRow(ctx, `
		SELECT id, started_at, completed_at, expected, succeeded, failed
		FROM judgment_runs
		ORDER BY started_at DESC, id DESC
		LIMIT 1`).Scan(
		&run.ID, &run.StartedAt, &run.CompletedAt,
		&run.Expected, &run.Succeeded, &run.Failed,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return JudgmentRun{}, ErrNotFound
	}
	return run, err
}
