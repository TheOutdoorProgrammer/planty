package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

type plantConversationSummary struct {
	ID          uuid.UUID           `json:"id"`
	FirstAsked  string              `json:"first_asked"`
	LatestReply string              `json:"latest_reply"`
	TurnCount   int                 `json:"turn_count"`
	Status      store.ConsultStatus `json:"status"`
	StartedAt   time.Time           `json:"started_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
}

type enqueuePlantMessageRequest struct {
	ID      uuid.UUID `json:"id"`
	Message string    `json:"message"`
	Photo   string    `json:"photo,omitempty"`
}

func (s *Server) enqueuePlantMessage(w http.ResponseWriter, r *http.Request) {
	if s.judge == nil {
		s.fail(w, http.StatusServiceUnavailable,
			errors.New("asking about a plant needs a judge, and none is configured"))
		return
	}

	p, err := s.store.GetPlant(r.Context(), r.PathValue("slug"))
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	conversationID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}

	var request enqueuePlantMessageRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, MaxPhotoBytes*2)).Decode(&request); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	if request.ID == uuid.Nil || strings.TrimSpace(request.Message) == "" {
		s.fail(w, http.StatusBadRequest, errors.New("message id and text are required"))
		return
	}

	var attached *uuid.UUID
	if request.Photo != "" {
		shot, _, _, err := s.keepAnswerPhoto(r.Context(), p, request.Photo)
		if err != nil {
			s.fail(w, statusForPhoto(err), err)
			return
		}
		attached = &shot.ID
	}

	queued, err := s.store.QueueConsultTurn(r.Context(), store.ConsultTurn{
		ID: request.ID, PlantID: p.ID, ConversationID: conversationID,
		Asked: request.Message, PhotoID: attached,
	})
	if errors.Is(err, store.ErrTurnConflict) || errors.Is(err, store.ErrConversationOwner) {
		s.fail(w, http.StatusConflict, err)
		return
	}
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	s.ok(w, http.StatusAccepted, conversationTurnResponse(queued))
}

func (s *Server) enqueueScratchMessage(w http.ResponseWriter, r *http.Request) {
	if s.judge == nil {
		s.fail(w, http.StatusServiceUnavailable,
			errors.New("asking about a plant needs a judge, and none is configured"))
		return
	}
	conversationID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	var request enqueuePlantMessageRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, MaxPhotoBytes*2)).Decode(&request); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	if request.ID == uuid.Nil || (strings.TrimSpace(request.Message) == "" && request.Photo == "") {
		s.fail(w, http.StatusBadRequest,
			errors.New("message id and a question or photograph are required"))
		return
	}

	_, attached, err := s.attach(r.Context(), conversationID, request.Photo, nil)
	if err != nil {
		s.fail(w, statusForPhoto(err), err)
		return
	}
	queued, err := s.store.QueueConsultTurn(r.Context(), store.ConsultTurn{
		ID: request.ID, ConversationID: conversationID,
		Asked: strings.TrimSpace(request.Message), PhotoID: attached,
	})
	if errors.Is(err, store.ErrTurnConflict) || errors.Is(err, store.ErrConversationOwner) {
		s.fail(w, http.StatusConflict, err)
		return
	}
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	response := conversationTurnResponse(queued)
	response["asked"] = queued.Asked
	s.ok(w, http.StatusAccepted, response)
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
			Status:      conversation.Turns[len(conversation.Turns)-1].Status,
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
		transcript = append(transcript, conversationTurnResponse(turn))
	}
	s.ok(w, http.StatusOK, map[string]any{
		"id": id, "turns": transcript,
	})
}

func (s *Server) getScratchConversation(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	turns, err := s.store.Consultation(r.Context(), id, uuid.Nil)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	transcript := make([]map[string]any, 0, len(turns))
	for _, turn := range turns {
		entry := conversationTurnResponse(turn)
		entry["asked"] = turn.Asked
		transcript = append(transcript, entry)
	}
	s.ok(w, http.StatusOK, map[string]any{"id": id, "turns": transcript})
}

func conversationTurnResponse(turn store.ConsultTurn) map[string]any {
	entry := conversationResponse(turn)
	entry["asked"] = visibleQuestion(turn.Asked)
	entry["photo_id"] = turn.PhotoID
	entry["created_at"] = turn.CreatedAt
	entry["updated_at"] = turn.UpdatedAt
	return entry
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
