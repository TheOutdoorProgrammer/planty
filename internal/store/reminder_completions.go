package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

type ReminderDisposition string

const (
	ReminderCompleted ReminderDisposition = "completed"
	ReminderMissed    ReminderDisposition = "missed"
)

func (d ReminderDisposition) valid() bool {
	return d == ReminderCompleted || d == ReminderMissed
}

// ReminderResolution settles exactly one scheduled occurrence. A completed
// occurrence writes the configured care observation; a missed one records only
// that the slot passed without pretending care happened.
type ReminderResolution struct {
	IdempotencyKey uuid.UUID
	ReminderID     uuid.UUID
	DueAt          time.Time
	Disposition    ReminderDisposition
	Note           string
}

// ReminderCompletion keeps source compatibility for callers that can only
// report successful care.
type ReminderCompletion = ReminderResolution

type ResolvedReminder struct {
	IdempotencyKey uuid.UUID           `json:"idempotency_key"`
	ReminderID     uuid.UUID           `json:"reminder_id"`
	DueAt          time.Time           `json:"due_at"`
	Disposition    ReminderDisposition `json:"disposition"`
	Note           string              `json:"note,omitempty"`
	Observation    *plant.Observation  `json:"observation,omitempty"`
	RespondedAt    time.Time           `json:"responded_at"`
}

// CompleteReminder remains the compatibility surface for existing clients.
// New callers should use ResolveReminder so a missed occurrence is explicit.
func (s *Store) CompleteReminder(ctx context.Context, resolution ReminderResolution) (plant.Observation, error) {
	resolution.Disposition = ReminderCompleted
	resolved, err := s.ResolveReminder(ctx, resolution)
	if err != nil {
		return plant.Observation{}, err
	}
	if resolved.Observation == nil {
		return plant.Observation{}, errors.New("completed reminder has no observation")
	}
	return *resolved.Observation, nil
}

// ResolveReminder records a completed or missed reminder occurrence exactly
// once. The reminder row lock serializes two phones choosing different outcomes;
// the first committed disposition is the historical truth returned to both.
func (s *Store) ResolveReminder(ctx context.Context, resolution ReminderResolution) (ResolvedReminder, error) {
	resolution.Note = strings.TrimSpace(resolution.Note)
	if resolution.IdempotencyKey == uuid.Nil || resolution.ReminderID == uuid.Nil || resolution.DueAt.IsZero() {
		return ResolvedReminder{}, fmt.Errorf(
			"%w: reminder resolution needs an idempotency key, reminder and due_at",
			plant.ErrInvalid,
		)
	}
	if !resolution.Disposition.valid() {
		return ResolvedReminder{}, fmt.Errorf(
			"%w: reminder disposition %q is not supported", plant.ErrInvalid, resolution.Disposition)
	}
	if len(resolution.Note) > 500 {
		return ResolvedReminder{}, fmt.Errorf("%w: reminder note is longer than 500 characters", plant.ErrInvalid)
	}
	if resolution.DueAt.After(time.Now().Add(time.Minute)) {
		return ResolvedReminder{}, fmt.Errorf("%w: a future reminder cannot be resolved", plant.ErrInvalid)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ResolvedReminder{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var reminder plant.Reminder
	if err := tx.QueryRow(ctx, `
		SELECT `+reminderColumns+`
		FROM reminders
		WHERE id = $1
		FOR UPDATE`, resolution.ReminderID).Scan(reminderFields(&reminder)...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ResolvedReminder{}, ErrNotFound
		}
		return ResolvedReminder{}, err
	}

	if replayed, found, err := reminderResolutionByKey(ctx, tx, resolution); err != nil {
		return ResolvedReminder{}, err
	} else if found {
		return replayed, tx.Commit(ctx)
	}

	if replayed, found, err := reminderResolutionByOccurrence(ctx, tx, resolution); err != nil {
		return ResolvedReminder{}, err
	} else if found {
		return replayed, tx.Commit(ctx)
	}

	if !reminder.Active {
		return ResolvedReminder{}, fmt.Errorf("%w: that reminder is inactive", plant.ErrInvalid)
	}

	var lastDone *time.Time
	if err := tx.QueryRow(ctx, `
		SELECT max(occurred_at)
		FROM observations
		WHERE plant_id = $1 AND kind = $2`, reminder.PlantID, reminder.Kind).Scan(&lastDone); err != nil {
		return ResolvedReminder{}, err
	}

	slot, ok := reminder.LastSlot(lastDone, resolution.DueAt)
	if !ok || !slot.Equal(resolution.DueAt) || !reminder.Due(lastDone, resolution.DueAt) {
		return ResolvedReminder{}, fmt.Errorf(
			"%w: that reminder occurrence is no longer due",
			plant.ErrInvalid,
		)
	}

	resolved := ResolvedReminder{
		IdempotencyKey: resolution.IdempotencyKey,
		ReminderID:     resolution.ReminderID,
		DueAt:          resolution.DueAt,
		Disposition:    resolution.Disposition,
		Note:           resolution.Note,
		RespondedAt:    time.Now().UTC(),
	}
	if resolution.Disposition == ReminderCompleted {
		completed, err := addObservationTx(ctx, tx, plant.Observation{
			PlantID:    reminder.PlantID,
			Kind:       reminder.Kind,
			Body:       reminder.Note,
			OccurredAt: resolved.RespondedAt,
			Source:     plant.SourceApp,
		})
		if err != nil {
			return ResolvedReminder{}, err
		}
		resolved.Observation = &completed
	}

	var observationID *uuid.UUID
	if resolved.Observation != nil {
		observationID = &resolved.Observation.ID
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO reminder_completions (
			idempotency_key, reminder_id, due_at, observation_id,
			disposition, note, responded_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		resolved.IdempotencyKey, resolved.ReminderID, resolved.DueAt, observationID,
		resolved.Disposition, resolved.Note, resolved.RespondedAt); err != nil {
		return ResolvedReminder{}, classify(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ResolvedReminder{}, err
	}
	return resolved, nil
}

func reminderResolutionByKey(
	ctx context.Context,
	tx pgx.Tx,
	resolution ReminderResolution,
) (ResolvedReminder, bool, error) {
	resolved, found, err := scanReminderResolution(ctx, tx, `
		WHERE rc.idempotency_key = $1`, resolution.IdempotencyKey)
	if err != nil || !found {
		return ResolvedReminder{}, found, err
	}
	if resolved.ReminderID != resolution.ReminderID ||
		!resolved.DueAt.Equal(resolution.DueAt) ||
		resolved.Disposition != resolution.Disposition ||
		resolved.Note != resolution.Note {
		return ResolvedReminder{}, false, fmt.Errorf(
			"%w: idempotency key was already used for a different reminder resolution",
			plant.ErrInvalid,
		)
	}
	return resolved, true, nil
}

func reminderResolutionByOccurrence(
	ctx context.Context,
	tx pgx.Tx,
	resolution ReminderResolution,
) (ResolvedReminder, bool, error) {
	return scanReminderResolution(ctx, tx, `
		WHERE rc.reminder_id = $1 AND rc.due_at = $2`, resolution.ReminderID, resolution.DueAt)
}

func scanReminderResolution(
	ctx context.Context,
	tx pgx.Tx,
	where string,
	args ...any,
) (ResolvedReminder, bool, error) {
	var resolved ResolvedReminder
	var observationID *uuid.UUID
	err := tx.QueryRow(ctx, `
		SELECT rc.idempotency_key, rc.reminder_id, rc.due_at, rc.disposition,
			rc.note, rc.responded_at, rc.observation_id
		FROM reminder_completions rc `+where, args...).Scan(
		&resolved.IdempotencyKey, &resolved.ReminderID, &resolved.DueAt,
		&resolved.Disposition, &resolved.Note, &resolved.RespondedAt, &observationID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ResolvedReminder{}, false, nil
	}
	if err != nil {
		return ResolvedReminder{}, false, err
	}
	if observationID != nil {
		observation, err := observationTx(ctx, tx, *observationID)
		if err != nil {
			return ResolvedReminder{}, false, err
		}
		resolved.Observation = &observation
	}
	return resolved, true, nil
}

func (s *Store) ReminderOccurrenceResolved(
	ctx context.Context,
	reminderID uuid.UUID,
	dueAt time.Time,
) (bool, error) {
	var resolved bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM reminder_completions
			WHERE reminder_id = $1 AND due_at = $2
		)`, reminderID, dueAt).Scan(&resolved)
	return resolved, err
}
