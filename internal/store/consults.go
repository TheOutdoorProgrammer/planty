package store

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/TheOutdoorProgrammer/planty/internal/judge"
)

var (
	ErrConversationOwner = errors.New("conversation belongs to a different subject")
	ErrTurnConflict      = errors.New("conversation turn id belongs to different content")
)

type ConsultStatus string

const (
	ConsultPending    ConsultStatus = "pending"
	ConsultProcessing ConsultStatus = "processing"
	ConsultComplete   ConsultStatus = "complete"
	ConsultFailed     ConsultStatus = "failed"
)

// ConsultTurn is one exchange in a conversation, whether about a plant's
// record, a photograph of it, or something nobody owns.
type ConsultTurn struct {
	ID             uuid.UUID    `json:"id"`
	PlantID        uuid.UUID    `json:"plant_id"`
	ConversationID uuid.UUID    `json:"conversation_id"`
	Asked          string       `json:"asked"`
	Reply          judge.Answer `json:"reply"`

	// Set when the person attached a photograph to this turn, which is how a
	// conversation survives losing its model session: the pictures can be
	// handed over again rather than being gone.
	PhotoID   *uuid.UUID    `json:"photo_id,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
	Status    ConsultStatus `json:"status"`
	Failure   string        `json:"failure,omitempty"`
	Attempts  int           `json:"-"`
	LeaseID   uuid.UUID     `json:"-"`
}

// ConsultConversation is the stored transcript and the small amount of
// metadata needed to present it in a plant's chat history.
type ConsultConversation struct {
	ID        uuid.UUID     `json:"id"`
	Turns     []ConsultTurn `json:"turns"`
	StartedAt time.Time     `json:"started_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// SaveConsultTurn records one question and its answer.
func (s *Store) SaveConsultTurn(ctx context.Context, t ConsultTurn) (ConsultTurn, error) {
	reply, err := json.Marshal(t.Reply)
	if err != nil {
		return ConsultTurn{}, err
	}

	saved, err := s.saveTurn(ctx, kindConsult, turn{
		ID:             t.ID,
		PlantID:        t.PlantID,
		ConversationID: t.ConversationID,
		Asked:          t.Asked,
		Reply:          reply,
		PhotoID:        t.PhotoID,
	})
	if err != nil {
		return ConsultTurn{}, err
	}
	return asConsult(saved)
}

func (s *Store) QueueConsultTurn(ctx context.Context, t ConsultTurn) (ConsultTurn, error) {
	saved, err := s.queueTurn(ctx, kindConsult, turn{
		ID:             t.ID,
		PlantID:        t.PlantID,
		ConversationID: t.ConversationID,
		Asked:          t.Asked,
		PhotoID:        t.PhotoID,
	})
	if err != nil {
		return ConsultTurn{}, err
	}
	return asConsult(saved)
}

func (s *Store) ClaimConsultTurn(ctx context.Context, lease time.Duration) (ConsultTurn, bool, error) {
	claimed, ok, err := s.claimTurn(ctx, []string{kindConsult}, lease)
	if err != nil || !ok {
		return ConsultTurn{}, ok, err
	}
	turn, err := asConsult(claimed)
	return turn, err == nil, err
}

func (s *Store) CompleteConsultTurn(ctx context.Context, id, leaseID uuid.UUID,
	reply judge.Answer) (ConsultTurn, error) {
	raw, err := json.Marshal(reply)
	if err != nil {
		return ConsultTurn{}, err
	}
	completed, err := s.completeTurn(ctx, id, leaseID, raw)
	if err != nil {
		return ConsultTurn{}, err
	}
	return asConsult(completed)
}

func (s *Store) FailConsultTurn(ctx context.Context, id, leaseID uuid.UUID,
	failure string) (ConsultTurn, error) {
	failed, err := s.failTurn(ctx, id, leaseID, failure)
	if err != nil {
		return ConsultTurn{}, err
	}
	return asConsult(failed)
}

func (s *Store) RetryConsultTurn(ctx context.Context, id, leaseID uuid.UUID,
	after time.Duration) (ConsultTurn, error) {
	retried, err := s.retryTurn(ctx, id, leaseID, after)
	if err != nil {
		return ConsultTurn{}, err
	}
	return asConsult(retried)
}

func (s *Store) RenewConsultTurn(ctx context.Context, id, leaseID uuid.UUID, lease time.Duration) error {
	return s.renewTurn(ctx, id, leaseID, lease)
}

// Consultation returns a conversation's turns, oldest first.
func (s *Store) Consultation(ctx context.Context,
	conversationID, plantID uuid.UUID) ([]ConsultTurn, error) {
	turns, err := s.conversation(ctx, kindConsult, conversationID, plantID)
	if err != nil {
		return nil, err
	}

	out := make([]ConsultTurn, 0, len(turns))
	for _, t := range turns {
		decoded, err := asConsult(t)
		if err != nil {
			return nil, err
		}
		out = append(out, decoded)
	}
	return out, nil
}

// Consultations returns every saved conversation for one plant, newest first.
func (s *Store) Consultations(ctx context.Context, plantID uuid.UUID) ([]ConsultConversation, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+turnColumns+`
		FROM diagnosis_turns
		WHERE kind = $1 AND plant_id = $2
		ORDER BY created_at, id`, kindConsult, plantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byID := map[uuid.UUID]int{}
	out := []ConsultConversation{}
	for rows.Next() {
		raw, err := scanTurn(rows)
		if err != nil {
			return nil, err
		}
		turn, err := asConsult(raw)
		if err != nil {
			return nil, err
		}
		index, ok := byID[turn.ConversationID]
		if !ok {
			index = len(out)
			byID[turn.ConversationID] = index
			out = append(out, ConsultConversation{
				ID: turn.ConversationID, StartedAt: turn.CreatedAt,
			})
		}
		out[index].Turns = append(out[index].Turns, turn)
		out[index].UpdatedAt = turn.UpdatedAt
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	slices.SortFunc(out, func(a, b ConsultConversation) int {
		return b.UpdatedAt.Compare(a.UpdatedAt)
	})
	return out, nil
}

func asConsult(t turn) (ConsultTurn, error) {
	out := ConsultTurn{
		ID: t.ID, PlantID: t.PlantID, ConversationID: t.ConversationID,
		Asked: t.Asked, PhotoID: t.PhotoID, CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt, Status: t.Status, Failure: t.Failure,
		Attempts: t.Attempts, LeaseID: t.LeaseID,
	}
	if len(t.Reply) > 0 {
		if err := json.Unmarshal(t.Reply, &out.Reply); err != nil {
			return ConsultTurn{}, err
		}
	}
	return out, nil
}
