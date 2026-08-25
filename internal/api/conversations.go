package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

type plantConversationSummary struct {
	ID          uuid.UUID `json:"id"`
	FirstAsked  string    `json:"first_asked"`
	LatestReply string    `json:"latest_reply"`
	TurnCount   int       `json:"turn_count"`
	StartedAt   time.Time `json:"started_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (s *Server) listPlantConversations(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetPlant(r.Context(), r.PathValue("slug"))
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	conversations, err := s.store.Consultations(r.Context(), p.ID)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	summaries := make([]plantConversationSummary, 0, len(conversations))
	for _, conversation := range conversations {
		if len(conversation.Turns) == 0 {
			continue
		}
		summaries = append(summaries, plantConversationSummary{
			ID:          conversation.ID,
			FirstAsked:  visibleQuestion(conversation.Turns[0].Asked),
			LatestReply: conversation.Turns[len(conversation.Turns)-1].Reply.Reply,
			TurnCount:   len(conversation.Turns),
			StartedAt:   conversation.StartedAt,
			UpdatedAt:   conversation.UpdatedAt,
		})
	}
	s.ok(w, http.StatusOK, map[string]any{"conversations": summaries})
}

func (s *Server) getPlantConversation(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetPlant(r.Context(), r.PathValue("slug"))
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	turns, err := s.store.Consultation(r.Context(), id, p.ID)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	transcript := make([]map[string]any, 0, len(turns))
	for _, turn := range turns {
		entry := conversationResponse(turn)
		entry["asked"] = visibleQuestion(turn.Asked)
		entry["photo_id"] = turn.PhotoID
		entry["created_at"] = turn.CreatedAt
		transcript = append(transcript, entry)
	}
	s.ok(w, http.StatusOK, map[string]any{
		"id": id, "turns": transcript,
	})
}

func visibleQuestion(asked string) string {
	const marker = "User's question:\n"
	if index := strings.LastIndex(asked, marker); index >= 0 {
		asked = asked[index+len(marker):]
	}
	asked = strings.TrimSpace(asked)
	if asked == "" {
		return "Asked about today's finding"
	}
	return asked
}
