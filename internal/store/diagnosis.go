package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/TheOutdoorProgrammer/planty/internal/judge"
)

// DiagnosisTurn is one exchange, kept so a follow-up can refer to the answer
// before it.
type DiagnosisTurn struct {
	ID             uuid.UUID       `json:"id"`
	PlantID        uuid.UUID       `json:"plant_id"`
	ConversationID uuid.UUID       `json:"conversation_id"`
	Asked          string          `json:"asked"`
	Reply          judge.Diagnosis `json:"reply"`
	PhotoID        *uuid.UUID      `json:"photo_id,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
}

// SaveDiagnosisTurn records one exchange.
func (s *Store) SaveDiagnosisTurn(ctx context.Context, t DiagnosisTurn) (DiagnosisTurn, error) {
	reply, err := json.Marshal(t.Reply)
	if err != nil {
		return DiagnosisTurn{}, err
	}
	if t.ConversationID == uuid.Nil {
		t.ConversationID = uuid.New()
	}

	row := s.pool.QueryRow(ctx, `
		INSERT INTO diagnosis_turns (plant_id, conversation_id, asked, reply, photo_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, plant_id, conversation_id, asked, reply, photo_id, created_at`,
		t.PlantID, t.ConversationID, t.Asked, reply, t.PhotoID)

	return scanDiagnosisTurn(row)
}

// DiagnosisConversation returns a conversation's turns, oldest first.
func (s *Store) DiagnosisConversation(ctx context.Context, conversationID uuid.UUID) ([]DiagnosisTurn, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, plant_id, conversation_id, asked, reply, photo_id, created_at
		FROM diagnosis_turns
		WHERE conversation_id = $1
		ORDER BY created_at`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DiagnosisTurn
	for rows.Next() {
		turn, err := scanDiagnosisTurn(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, turn)
	}
	return out, rows.Err()
}

func scanDiagnosisTurn(row interface {
	Scan(dest ...any) error
}) (DiagnosisTurn, error) {
	var t DiagnosisTurn
	var reply []byte

	if err := row.Scan(&t.ID, &t.PlantID, &t.ConversationID, &t.Asked,
		&reply, &t.PhotoID, &t.CreatedAt); err != nil {
		return DiagnosisTurn{}, err
	}
	if len(reply) > 0 {
		if err := json.Unmarshal(reply, &t.Reply); err != nil {
			return DiagnosisTurn{}, err
		}
	}
	return t, nil
}
