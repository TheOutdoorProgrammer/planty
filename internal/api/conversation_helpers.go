package api

import (
	"context"

	"github.com/google/uuid"

	"github.com/TheOutdoorProgrammer/planty/internal/judge"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

// conversationHistory reconstructs one stored conversation regardless of
// whether it belongs to a plant or to a scratch question. uuid.Nil means the
// latter. Photo ids are always preserved; the caller decides whether those
// photos are offered from a plant timeline or replayed as scratch attachments.
func (s *Server) conversationHistory(ctx context.Context, ownerID uuid.UUID,
	conversationID *uuid.UUID) ([]judge.PriorAnswer, uuid.UUID, error) {
	if conversationID == nil {
		return nil, uuid.New(), nil
	}

	turns, err := s.store.Consultation(ctx, *conversationID, ownerID)
	if err != nil {
		return nil, uuid.Nil, err
	}
	prior := make([]judge.PriorAnswer, 0, len(turns))
	for _, turn := range turns {
		prior = append(prior, judge.PriorAnswer{
			Asked: turn.Asked, Reply: turn.Reply, PhotoID: turn.PhotoID,
		})
	}
	return prior, *conversationID, nil
}

// conversationResponse is the one wire shape both kinds of consultation use.
// Keeping it here means a new field cannot be returned by plant chat but
// silently disappear from scratch chat, or vice versa.
func conversationResponse(saved store.ConsultTurn) map[string]any {
	return map[string]any{
		"id":                   saved.ID,
		"conversation_id":      saved.ConversationID,
		"reply":                saved.Reply.Reply,
		"confidence":           saved.Reply.Confidence,
		"looked_at":            saved.Reply.LookedAt,
		"suggested_follow_ups": saved.Reply.Suggestions,
		"steps":                saved.Reply.Steps,
	}
}
