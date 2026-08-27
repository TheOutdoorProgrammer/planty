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
		if turn.Status != store.ConsultComplete {
			continue
		}
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
	response := map[string]any{
		"id":              saved.ID,
		"conversation_id": saved.ConversationID,
		"status":          saved.Status,
	}
	if saved.Failure != "" {
		response["failure"] = saved.Failure
	}
	if saved.Status != store.ConsultComplete {
		return response
	}

	suggestions := saved.Reply.Suggestions
	if suggestions == nil {
		suggestions = []string{}
	}
	steps := saved.Reply.Steps
	if steps == nil {
		steps = []judge.Step{}
	}
	response["reply"] = saved.Reply.Reply
	response["confidence"] = saved.Reply.Confidence
	response["looked_at"] = saved.Reply.LookedAt
	response["suggested_follow_ups"] = suggestions
	response["steps"] = steps
	return response
}
