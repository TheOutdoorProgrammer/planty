package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// There is one kind of conversation now. The column stays because rows written
// by the separate photograph diagnosis still carry "diagnosis", and rewriting
// history to tidy a discriminator would be worse than keeping it.
const kindConsult = "consult"

const turnColumns = `id, plant_id, conversation_id, asked, reply, photo_id, created_at,
	status, coalesce(failure, ''), attempts, lease_id, lease_expires_at, updated_at`

// turn is one exchange with its reply still encoded, so the two conversation
// types share all the storage and differ only in what they decode into.
type turn struct {
	ID             uuid.UUID
	PlantID        uuid.UUID
	ConversationID uuid.UUID
	Asked          string
	Reply          json.RawMessage
	PhotoID        *uuid.UUID
	CreatedAt      time.Time
	Status         ConsultStatus
	Failure        string
	Attempts       int
	LeaseID        uuid.UUID
	LeaseExpiresAt *time.Time
	UpdatedAt      time.Time
}

func (s *Store) saveTurn(ctx context.Context, kind string, t turn) (turn, error) {
	return s.insertTurn(ctx, kind, t, ConsultComplete)
}

func (s *Store) queueTurn(ctx context.Context, kind string, t turn) (turn, error) {
	return s.insertTurn(ctx, kind, t, ConsultPending)
}

func (s *Store) insertTurn(ctx context.Context, kind string, t turn, status ConsultStatus) (turn, error) {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	if t.ConversationID == uuid.Nil {
		t.ConversationID = uuid.New()
	}
	// A question about a plant nobody owns has no plant, and the zero uuid is
	// how that arrives here. Writing it literally would fail the foreign key.
	var owner any
	if t.PlantID != uuid.Nil {
		owner = t.PlantID
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return turn{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// A new conversation has no row to lock, so an advisory lock serialises
	// concurrent first turns carrying the same client-supplied UUID.
	lockKey := kind + ":" + t.ConversationID.String()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		return turn{}, err
	}

	var ownersMatch *bool
	err = tx.QueryRow(ctx, `
		SELECT bool_and(plant_id IS NOT DISTINCT FROM $3::uuid)
		FROM diagnosis_turns
		WHERE kind = $1 AND conversation_id = $2`, kind, t.ConversationID, owner).Scan(&ownersMatch)
	if err != nil {
		return turn{}, err
	}
	if ownersMatch != nil && !*ownersMatch {
		return turn{}, ErrConversationOwner
	}

	var reply any
	if status == ConsultComplete {
		reply = []byte(t.Reply)
	}
	row := tx.QueryRow(ctx, `
		INSERT INTO diagnosis_turns (
			id, plant_id, conversation_id, asked, reply, photo_id, kind, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO NOTHING
		RETURNING `+turnColumns,
		t.ID, owner, t.ConversationID, t.Asked, reply, t.PhotoID, kind, status)
	saved, err := scanTurn(row)
	if errors.Is(err, pgx.ErrNoRows) {
		saved, err = s.turn(ctx, t.ID)
		if err == nil && !sameTurn(saved, t) {
			err = ErrTurnConflict
		}
	}
	if err != nil {
		return turn{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return turn{}, err
	}
	return saved, nil
}

func sameTurn(saved, requested turn) bool {
	if saved.ID != requested.ID || saved.PlantID != requested.PlantID ||
		saved.ConversationID != requested.ConversationID || saved.Asked != requested.Asked {
		return false
	}
	if saved.PhotoID == nil || requested.PhotoID == nil {
		return saved.PhotoID == nil && requested.PhotoID == nil
	}
	return *saved.PhotoID == *requested.PhotoID
}

func (s *Store) turn(ctx context.Context, id uuid.UUID) (turn, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+turnColumns+` FROM diagnosis_turns WHERE id = $1`, id)
	return scanTurn(row)
}

func (s *Store) conversation(ctx context.Context, kind string,
	conversationID, owner uuid.UUID) ([]turn, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+turnColumns+`
		FROM diagnosis_turns
		WHERE kind = $1 AND conversation_id = $2
		ORDER BY created_at, id`, kind, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []turn{}
	for rows.Next() {
		t, err := scanTurn(rows)
		if err != nil {
			return nil, err
		}
		if t.PlantID != owner {
			return nil, ErrConversationOwner
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, ErrNotFound
	}
	return out, nil
}

func scanTurn(row interface{ Scan(dest ...any) error }) (turn, error) {
	var t turn
	var reply []byte
	var owner *uuid.UUID
	var leaseID *uuid.UUID

	if err := row.Scan(&t.ID, &owner, &t.ConversationID, &t.Asked,
		&reply, &t.PhotoID, &t.CreatedAt, &t.Status, &t.Failure, &t.Attempts,
		&leaseID, &t.LeaseExpiresAt, &t.UpdatedAt); err != nil {
		return turn{}, err
	}
	if owner != nil {
		t.PlantID = *owner
	}
	if leaseID != nil {
		t.LeaseID = *leaseID
	}
	t.Reply = reply
	return t, nil
}

func (s *Store) claimConsultTurn(ctx context.Context, lease time.Duration) (turn, bool, error) {
	leaseID := uuid.New()
	row := s.pool.QueryRow(ctx, `
		WITH candidate AS (
			SELECT queued.id
			FROM diagnosis_turns queued
			WHERE queued.kind = $1
			  AND ((queued.status = $2 AND (
				queued.lease_expires_at IS NULL OR queued.lease_expires_at < now()
			  )) OR (
				queued.status = $3 AND queued.lease_expires_at < now()
			  ))
			  AND NOT EXISTS (
				SELECT 1
				FROM diagnosis_turns earlier
				WHERE earlier.kind = queued.kind
				  AND earlier.conversation_id = queued.conversation_id
				  AND (earlier.created_at, earlier.id) < (queued.created_at, queued.id)
				  AND earlier.status IN ($2, $3)
			  )
			ORDER BY queued.created_at, queued.id
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE diagnosis_turns work
		SET status = $3,
		    attempts = attempts + 1,
		    lease_expires_at = now() + make_interval(secs => $4),
		    lease_id = $5,
		    updated_at = now()
		WHERE work.id = (SELECT id FROM candidate)
		RETURNING `+turnColumns,
		kindConsult, ConsultPending, ConsultProcessing, lease.Seconds(), leaseID)
	claimed, err := scanTurn(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return turn{}, false, nil
	}
	return claimed, err == nil, err
}

func (s *Store) completeConsultTurn(ctx context.Context, id, leaseID uuid.UUID, reply []byte) (turn, error) {
	row := s.pool.QueryRow(ctx, `
		UPDATE diagnosis_turns
		SET reply = $3, status = $4, failure = NULL,
		    lease_id = NULL, lease_expires_at = NULL, updated_at = now()
		WHERE id = $1 AND lease_id = $2 AND status = $5
		RETURNING `+turnColumns, id, leaseID, reply, ConsultComplete, ConsultProcessing)
	return scanTurn(row)
}

func (s *Store) failConsultTurn(ctx context.Context, id, leaseID uuid.UUID, failure string) (turn, error) {
	row := s.pool.QueryRow(ctx, `
		UPDATE diagnosis_turns
		SET status = $3, failure = $4,
		    lease_id = NULL, lease_expires_at = NULL, updated_at = now()
		WHERE id = $1 AND lease_id = $2 AND status = $5
		RETURNING `+turnColumns, id, leaseID, ConsultFailed, failure, ConsultProcessing)
	return scanTurn(row)
}

func (s *Store) retryConsultTurn(ctx context.Context, id, leaseID uuid.UUID, after time.Duration) (turn, error) {
	row := s.pool.QueryRow(ctx, `
		UPDATE diagnosis_turns
		SET status = $3, failure = NULL, lease_id = NULL,
		    lease_expires_at = now() + make_interval(secs => $4),
		    updated_at = now()
		WHERE id = $1 AND lease_id = $2 AND status = $5
		RETURNING `+turnColumns, id, leaseID, ConsultPending, after.Seconds(), ConsultProcessing)
	return scanTurn(row)
}

func (s *Store) renewConsultTurn(ctx context.Context, id, leaseID uuid.UUID, lease time.Duration) error {
	result, err := s.pool.Exec(ctx, `
		UPDATE diagnosis_turns
		SET lease_expires_at = now() + make_interval(secs => $2),
		    updated_at = now()
		WHERE id = $1 AND lease_id = $3 AND status = $4`,
		id, lease.Seconds(), leaseID, ConsultProcessing)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
