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

// ReminderCompletion is one scheduled occurrence the user says they performed.
// DueAt distinguishes two occurrences of the same reminder, such as morning and
// evening misting. The kind is deliberately read from the reminder on the
// server so a client cannot clear "mist it" while recording something else.
type ReminderCompletion struct {
	IdempotencyKey uuid.UUID
	ReminderID     uuid.UUID
	DueAt          time.Time
}

// CompleteReminder records the reminder's configured care kind exactly once.
// Replaying either the same idempotency key or the same reminder occurrence
// returns the original observation instead of duplicating the plant history.
func (s *Store) CompleteReminder(ctx context.Context, c ReminderCompletion) (plant.Observation, error) {
	if c.IdempotencyKey == uuid.Nil || c.ReminderID == uuid.Nil || c.DueAt.IsZero() {
		return plant.Observation{}, fmt.Errorf(
			"%w: reminder completion needs an idempotency key, reminder and due_at",
			plant.ErrInvalid,
		)
	}
	if c.DueAt.After(time.Now().Add(time.Minute)) {
		return plant.Observation{}, fmt.Errorf("%w: a future reminder cannot be completed", plant.ErrInvalid)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return plant.Observation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var reminder plant.Reminder
	if err := tx.QueryRow(ctx, `
		SELECT `+reminderColumns+`
		FROM reminders
		WHERE id = $1
		FOR UPDATE`, c.ReminderID).Scan(reminderFields(&reminder)...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return plant.Observation{}, ErrNotFound
		}
		return plant.Observation{}, err
	}

	if replayed, found, err := reminderCompletionByKey(ctx, tx, c); err != nil {
		return plant.Observation{}, err
	} else if found {
		return replayed, tx.Commit(ctx)
	}

	// A second phone may have completed the occurrence with its own key. The
	// occurrence identity is authoritative, so both callers receive one record.
	if replayed, found, err := reminderCompletionByOccurrence(ctx, tx, c); err != nil {
		return plant.Observation{}, err
	} else if found {
		return replayed, tx.Commit(ctx)
	}

	if !reminder.Active {
		return plant.Observation{}, fmt.Errorf("%w: that reminder is inactive", plant.ErrInvalid)
	}

	var lastDone *time.Time
	if err := tx.QueryRow(ctx, `
		SELECT max(occurred_at)
		FROM observations
		WHERE plant_id = $1 AND kind = $2`, reminder.PlantID, reminder.Kind).Scan(&lastDone); err != nil {
		return plant.Observation{}, err
	}

	slot, ok := reminder.LastSlot(lastDone, c.DueAt)
	if !ok || !slot.Equal(c.DueAt) || !reminder.Due(lastDone, c.DueAt) {
		return plant.Observation{}, fmt.Errorf(
			"%w: that reminder occurrence is no longer due",
			plant.ErrInvalid,
		)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO reminder_completions (idempotency_key, reminder_id, due_at)
		VALUES ($1, $2, $3)`, c.IdempotencyKey, c.ReminderID, c.DueAt); err != nil {
		return plant.Observation{}, classify(err)
	}

	completed, err := addObservationTx(ctx, tx, plant.Observation{
		PlantID:    reminder.PlantID,
		Kind:       reminder.Kind,
		Body:       reminder.Note,
		OccurredAt: time.Now().UTC(),
		Source:     plant.SourceApp,
	})
	if err != nil {
		return plant.Observation{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE reminder_completions SET observation_id = $2
		WHERE idempotency_key = $1`, c.IdempotencyKey, completed.ID); err != nil {
		return plant.Observation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return plant.Observation{}, err
	}
	return completed, nil
}

func reminderCompletionByKey(
	ctx context.Context,
	tx pgx.Tx,
	c ReminderCompletion,
) (plant.Observation, bool, error) {
	var reminderID uuid.UUID
	var dueAt time.Time
	var observationID *uuid.UUID
	err := tx.QueryRow(ctx, `
		SELECT reminder_id, due_at, observation_id
		FROM reminder_completions
		WHERE idempotency_key = $1`, c.IdempotencyKey).
		Scan(&reminderID, &dueAt, &observationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return plant.Observation{}, false, nil
	}
	if err != nil {
		return plant.Observation{}, false, err
	}
	if reminderID != c.ReminderID || !dueAt.Equal(c.DueAt) {
		return plant.Observation{}, false, fmt.Errorf(
			"%w: idempotency key was already used for a different reminder occurrence",
			plant.ErrInvalid,
		)
	}
	if observationID == nil {
		return plant.Observation{}, false, errors.New("reminder completion exists without its observation")
	}
	out, err := observationTx(ctx, tx, *observationID)
	return out, true, err
}

func reminderCompletionByOccurrence(
	ctx context.Context,
	tx pgx.Tx,
	c ReminderCompletion,
) (plant.Observation, bool, error) {
	var observationID *uuid.UUID
	err := tx.QueryRow(ctx, `
		SELECT observation_id
		FROM reminder_completions
		WHERE reminder_id = $1 AND due_at = $2`, c.ReminderID, c.DueAt).
		Scan(&observationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return plant.Observation{}, false, nil
	}
	if err != nil {
		return plant.Observation{}, false, err
	}
	if observationID == nil {
		return plant.Observation{}, false, errors.New("reminder completion exists without its observation")
	}
	out, err := observationTx(ctx, tx, *observationID)
	return out, true, err
}
