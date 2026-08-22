package store

import (
	"context"
	"fmt"

	"github.com/TheOutdoorProgrammer/planty/internal/judge"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

// ModelAssignment is which model answers one job.
type ModelAssignment struct {
	Job      judge.Job
	Provider string
	Model    string
}

// SetModelAssignment records which model answers a job, refusing one that
// cannot do it. Enforced here as well as in the handler so a direct write
// cannot leave the service holding a pairing it will only fail on later.
func (s *Store) SetModelAssignment(ctx context.Context, a ModelAssignment) error {
	model, ok := judge.Lookup(a.Provider, a.Model)
	if !ok {
		return fmt.Errorf("%w: there is no model %s/%s", plant.ErrInvalid, a.Provider, a.Model)
	}
	if err := model.CanDo(a.Job); err != nil {
		return fmt.Errorf("%w: %s", plant.ErrInvalid, err)
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO model_assignments (job, provider, model)
		VALUES ($1, $2, $3)
		ON CONFLICT (job)
		DO UPDATE SET provider = EXCLUDED.provider,
		              model = EXCLUDED.model,
		              updated_at = now()`,
		string(a.Job), a.Provider, a.Model)
	return err
}

// ModelAssignments is every job that has been moved off its default.
func (s *Store) ModelAssignments(ctx context.Context) ([]ModelAssignment, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT job, provider, model FROM model_assignments ORDER BY job`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ModelAssignment
	for rows.Next() {
		var a ModelAssignment
		var job string
		if err := rows.Scan(&job, &a.Provider, &a.Model); err != nil {
			return nil, err
		}
		a.Job = judge.Job(job)
		out = append(out, a)
	}
	return out, rows.Err()
}

// ClearModelAssignment returns a job to its default.
func (s *Store) ClearModelAssignment(ctx context.Context, job judge.Job) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM model_assignments WHERE job = $1`, string(job))
	return err
}

// For names the model a job should use, satisfying judge.Assignments. A row
// naming something the catalogue no longer offers is ignored rather than
// fatal, so removing a model from the table cannot strand a running service.
func (s *Store) For(ctx context.Context, job judge.Job) (judge.Model, bool) {
	var provider, model string
	err := s.pool.QueryRow(ctx,
		`SELECT provider, model FROM model_assignments WHERE job = $1`,
		string(job)).Scan(&provider, &model)
	if err != nil {
		return judge.Model{}, false
	}

	found, ok := judge.Lookup(provider, model)
	if !ok || found.CanDo(job) != nil {
		return judge.Model{}, false
	}
	return found, true
}
