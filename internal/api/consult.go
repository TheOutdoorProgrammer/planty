package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/TheOutdoorProgrammer/planty/internal/job"
	"github.com/TheOutdoorProgrammer/planty/internal/judge"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

// MaxOfferedPhotos bounds what one question is allowed to look at. Enough to
// cover a season of a plant that gets photographed weekly.
const MaxOfferedPhotos = 12

// consultRequest is a question about a plant, optionally continuing an earlier
// conversation. No photograph: the record is the subject.
type consultRequest struct {
	Message        string     `json:"message"`
	ConversationID *uuid.UUID `json:"conversation_id,omitempty"`
}

// consult answers a question about a plant from its record. Separate from
// diagnosis because most questions are not about a photograph, and requiring
// one to ask anything is what made this hard to use.
func (s *Server) consult(w http.ResponseWriter, r *http.Request) {
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

	var ask consultRequest
	if err := json.NewDecoder(r.Body).Decode(&ask); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	if ask.Message == "" {
		s.fail(w, http.StatusBadRequest, errors.New("no question was asked"))
		return
	}

	prior, conversation, err := s.priorAnswers(r, ask.ConversationID)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	history, err := job.Gather(r.Context(), s.store, p, time.Now().Add(-judge.ConsultWindow))
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	answer, err := s.judge.Consult(r.Context(), history, s.offerPhotos(r, p), ask.Message, prior)
	if err != nil {
		s.fail(w, http.StatusBadGateway, err)
		return
	}

	saved, err := s.store.SaveConsultTurn(r.Context(), store.ConsultTurn{
		PlantID:        p.ID,
		ConversationID: conversation,
		Asked:          ask.Message,
		Reply:          answer,
	})
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	s.ok(w, http.StatusOK, map[string]any{
		"id":                   saved.ID,
		"conversation_id":      saved.ConversationID,
		"reply":                answer.Reply,
		"confidence":           answer.Confidence,
		"looked_at":            answer.LookedAt,
		"suggested_follow_ups": answer.Suggestions,
	})
}

func (s *Server) priorAnswers(r *http.Request,
	conversationID *uuid.UUID) ([]judge.PriorAnswer, uuid.UUID, error) {
	if conversationID == nil {
		return nil, uuid.New(), nil
	}

	turns, err := s.store.Consultation(r.Context(), *conversationID)
	if err != nil {
		return nil, uuid.Nil, err
	}

	prior := make([]judge.PriorAnswer, 0, len(turns))
	for _, turn := range turns {
		prior = append(prior, judge.PriorAnswer{Asked: turn.Asked, Reply: turn.Reply})
	}
	return prior, *conversationID, nil
}

// offerPhotos hands over recent photographs without attaching any, so the
// model decides. A photograph that cannot be read is dropped rather than
// failing the question, because it was optional to begin with.
func (s *Server) offerPhotos(r *http.Request, p plant.Plant) []judge.Offer {
	if s.photos == nil {
		return nil
	}

	shots, err := s.store.Photos(r.Context(), p.ID, MaxOfferedPhotos)
	if err != nil {
		s.log.Warn("no photographs offered", "plant", p.Slug, "error", err)
		return nil
	}

	return judge.Offers(shots, func(shot plant.Photo) ([]byte, string, bool) {
		if shot.TakenAt.Before(time.Now().Add(-judge.ConsultWindow)) {
			return nil, "", false
		}
		body, err := s.photos.Get(r.Context(), shot.StorageKey)
		if err != nil {
			return nil, "", false
		}
		defer func() { _ = body.Close() }()

		raw, err := io.ReadAll(io.LimitReader(body, MaxPhotoBytes))
		if err != nil {
			return nil, "", false
		}
		return raw, mediaFor(shot.StorageKey), true
	})
}
