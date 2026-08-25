package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/judge"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

// MaxPromptInstructionsLength bounds a settings write before it becomes an
// accidental document upload and gets repeated into every request for a job.
const MaxPromptInstructionsLength = 12_000

// PromptInstruction is the editable instruction overlay for one model job.
// The immutable prompt remains code-owned in judge.
type PromptInstruction struct {
	Job          judge.Job `json:"job"`
	Instructions string    `json:"instructions"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// SetPromptInstruction creates or replaces one job's overlay.
func (s *Store) SetPromptInstruction(ctx context.Context, instruction PromptInstruction) (PromptInstruction, error) {
	if !knownJob(instruction.Job) {
		return PromptInstruction{}, fmt.Errorf("%w: there is no job %q", plant.ErrInvalid, instruction.Job)
	}
	instruction.Instructions = strings.TrimSpace(instruction.Instructions)
	if instruction.Instructions == "" {
		return PromptInstruction{}, fmt.Errorf("%w: prompt instructions cannot be blank; clear the override instead", plant.ErrInvalid)
	}
	if len(instruction.Instructions) > MaxPromptInstructionsLength {
		return PromptInstruction{}, fmt.Errorf("%w: prompt instructions exceed %d bytes", plant.ErrInvalid, MaxPromptInstructionsLength)
	}

	err := s.pool.QueryRow(ctx, `
		INSERT INTO prompt_instructions (job, instructions)
		VALUES ($1, $2)
		ON CONFLICT (job)
		DO UPDATE SET instructions = EXCLUDED.instructions, updated_at = now()
		RETURNING updated_at`, string(instruction.Job), instruction.Instructions).
		Scan(&instruction.UpdatedAt)
	return instruction, err
}

// PromptInstructions lists only jobs with an editable overlay. API callers
// merge this with judge.Jobs so jobs at their empty default remain visible.
func (s *Store) PromptInstructions(ctx context.Context) ([]PromptInstruction, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT job, instructions, updated_at
		FROM prompt_instructions
		ORDER BY job`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PromptInstruction
	for rows.Next() {
		var instruction PromptInstruction
		var job string
		if err := rows.Scan(&job, &instruction.Instructions, &instruction.UpdatedAt); err != nil {
			return nil, err
		}
		instruction.Job = judge.Job(job)
		out = append(out, instruction)
	}
	return out, rows.Err()
}

// ClearPromptInstruction returns a job to the immutable code-owned prompt.
func (s *Store) ClearPromptInstruction(ctx context.Context, job judge.Job) error {
	if !knownJob(job) {
		return fmt.Errorf("%w: there is no job %q", plant.ErrInvalid, job)
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM prompt_instructions WHERE job = $1`, string(job))
	return err
}

// PromptInstructionsFor supplies judge with the current overlay. Missing and
// stale unknown-job rows are ignored so configuration cannot strand a job.
func (s *Store) PromptInstructionsFor(ctx context.Context, job judge.Job) (string, bool) {
	if !knownJob(job) {
		return "", false
	}
	var instructions string
	err := s.pool.QueryRow(ctx,
		`SELECT instructions FROM prompt_instructions WHERE job = $1`, string(job)).
		Scan(&instructions)
	return instructions, err == nil
}

func knownJob(candidate judge.Job) bool {
	for _, job := range judge.Jobs {
		if candidate == job {
			return true
		}
	}
	return false
}
