package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// A plant has two kinds of conversation and they share a table: both are an
// ordered list of question and reply against one plant.
const (
	kindDiagnosis = "diagnosis"
	kindConsult   = "consult"
)

const turnColumns = `id, plant_id, conversation_id, asked, reply, photo_id, created_at`

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
}

func (s *Store) saveTurn(ctx context.Context, kind string, t turn) (turn, error) {
	if t.ConversationID == uuid.Nil {
		t.ConversationID = uuid.New()
	}
	row := s.pool.QueryRow(ctx, `
		INSERT INTO diagnosis_turns (plant_id, conversation_id, asked, reply, photo_id, kind)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+turnColumns,
		t.PlantID, t.ConversationID, t.Asked, []byte(t.Reply), t.PhotoID, kind)
	return scanTurn(row)
}

func (s *Store) conversation(ctx context.Context, kind string,
	conversationID uuid.UUID) ([]turn, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+turnColumns+`
		FROM diagnosis_turns
		WHERE kind = $1 AND conversation_id = $2
		ORDER BY created_at`, kind, conversationID)
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
		out = append(out, t)
	}
	return out, rows.Err()
}

func scanTurn(row interface{ Scan(dest ...any) error }) (turn, error) {
	var t turn
	var reply []byte

	if err := row.Scan(&t.ID, &t.PlantID, &t.ConversationID, &t.Asked,
		&reply, &t.PhotoID, &t.CreatedAt); err != nil {
		return turn{}, err
	}
	t.Reply = reply
	return t, nil
}
