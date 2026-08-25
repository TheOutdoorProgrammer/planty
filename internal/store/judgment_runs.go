package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
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

// JudgmentResult is the durable outcome for one plant in one garden-wide run.
// OriginalError and OriginalOutput describe the first malformed answer even
// when a bounded repair later succeeded.
type JudgmentResult struct {
	plant.JudgmentFailure
	Succeeded bool `json:"succeeded"`
}

// JudgmentResultInput is what a worker learned after judging one plant.
type JudgmentResultInput struct {
	PlantID        uuid.UUID
	Succeeded      bool
	Attempts       int
	Model          string
	OriginalError  string
	OriginalOutput string
	FinalError     string
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

// RecordJudgmentPlantResult writes the per-plant diagnostic and advances the
// aggregate counts in one transaction. An existing failed row may be retried;
// an existing success is immutable so a retry cannot duplicate good work.
func (s *Store) RecordJudgmentPlantResult(ctx context.Context, id uuid.UUID, result JudgmentResultInput) error {
	if result.Attempts < 1 {
		result.Attempts = 1
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var completedAt *time.Time
	if err := tx.QueryRow(ctx, `
		SELECT completed_at FROM judgment_runs WHERE id = $1 FOR UPDATE`, id).
		Scan(&completedAt); err != nil {
		return classify(err)
	}
	if completedAt != nil {
		return fmt.Errorf("judgment run %s is already complete", id)
	}

	var previous bool
	err = tx.QueryRow(ctx, `
		SELECT succeeded FROM judgment_results
		WHERE run_id = $1 AND plant_id = $2`, id, result.PlantID).Scan(&previous)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		_, err = tx.Exec(ctx, `
			INSERT INTO judgment_results (
				run_id, plant_id, succeeded, attempts, model,
				original_error, original_output, final_error
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			id, result.PlantID, result.Succeeded, result.Attempts, result.Model,
			result.OriginalError, result.OriginalOutput, result.FinalError)
		if err != nil {
			return err
		}
		column := "failed"
		if result.Succeeded {
			column = "succeeded"
		}
		if _, err := tx.Exec(ctx, `
			UPDATE judgment_runs SET `+column+` = `+column+` + 1
			WHERE id = $1 AND succeeded + failed < expected`, id); err != nil {
			return err
		}
	case err != nil:
		return err
	case previous:
		return fmt.Errorf("plant %s already succeeded in judgment run %s", result.PlantID, id)
	default:
		_, err = tx.Exec(ctx, `
			UPDATE judgment_results SET
				succeeded = $3,
				attempts = attempts + $4,
				model = $5,
				original_error = CASE WHEN original_error = '' THEN $6 ELSE original_error END,
				original_output = CASE WHEN original_output = '' THEN $7 ELSE original_output END,
				final_error = $8,
				updated_at = now()
			WHERE run_id = $1 AND plant_id = $2`,
			id, result.PlantID, result.Succeeded, result.Attempts, result.Model,
			result.OriginalError, result.OriginalOutput, result.FinalError)
		if err != nil {
			return err
		}
		if result.Succeeded {
			if _, err := tx.Exec(ctx, `
				UPDATE judgment_runs SET succeeded = succeeded + 1, failed = failed - 1
				WHERE id = $1 AND failed > 0`, id); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
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

// FailedJudgments names every plant the latest run could not judge.
func (s *Store) FailedJudgments(ctx context.Context, runID uuid.UUID) ([]JudgmentResult, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT jr.run_id, `+plantColumnsFor("p")+`, jr.succeeded, jr.attempts,
		       jr.model, jr.original_error, jr.original_output, jr.final_error, jr.updated_at
		FROM judgment_results jr
		JOIN plants p ON p.id = jr.plant_id
		WHERE jr.run_id = $1 AND jr.succeeded = false
		ORDER BY p.common_name, p.id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []JudgmentResult
	for rows.Next() {
		var result JudgmentResult
		var ps plantScan
		dest := []any{&result.RunID}
		dest = append(dest, ps.targets(&result.Plant)...)
		dest = append(dest, &result.Succeeded, &result.Attempts, &result.Model,
			&result.OriginalError, &result.OriginalOutput, &result.FinalError, &result.UpdatedAt)
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		if err := ps.finish(&result.Plant); err != nil {
			return nil, err
		}
		out = append(out, result)
	}
	return out, rows.Err()
}

// BeginLatestJudgmentRetry reopens the latest partial run and returns only its
// failed plants. The original expected count and successful rows stay intact.
func (s *Store) BeginLatestJudgmentRetry(ctx context.Context) (JudgmentRun, []JudgmentResult, error) {
	run, err := s.LatestJudgmentRun(ctx)
	if err != nil {
		return JudgmentRun{}, nil, err
	}
	failed, err := s.FailedJudgments(ctx, run.ID)
	if err != nil {
		return JudgmentRun{}, nil, err
	}
	if len(failed) == 0 {
		return JudgmentRun{}, nil, ErrNotFound
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE judgment_runs SET completed_at = NULL
		WHERE id = $1 AND completed_at IS NOT NULL AND failed > 0`, run.ID)
	if err != nil {
		return JudgmentRun{}, nil, err
	}
	if tag.RowsAffected() == 0 {
		return JudgmentRun{}, nil, fmt.Errorf("judgment run %s is already being retried", run.ID)
	}
	run.CompletedAt = nil
	return run, failed, nil
}
