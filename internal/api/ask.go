package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/TheOutdoorProgrammer/planty/internal/judge"
	"github.com/TheOutdoorProgrammer/planty/internal/photos"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

// askRequest is a question about a plant nobody owns. Both fields are
// optional on their own, because a photograph with no words is the commonest
// version of this question and typing one out adds nothing.
type askRequest struct {
	Message        string     `json:"message"`
	Photo          string     `json:"photo,omitempty"`
	ConversationID *uuid.UUID `json:"conversation_id,omitempty"`
}

// ask answers about something photographed in a shop or somebody else's house.
// Nothing here creates a plant: the answer is usually "do not buy it", and
// filing a record first would leave a row for a plant never brought home.
func (s *Server) ask(w http.ResponseWriter, r *http.Request) {
	if s.judge == nil {
		s.fail(w, http.StatusServiceUnavailable,
			errors.New("asking about a plant needs a judge, and none is configured"))
		return
	}

	var req askRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, MaxPhotoBytes*2)).Decode(&req); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	if req.Message == "" && req.Photo == "" {
		s.fail(w, http.StatusBadRequest,
			errors.New("send a question, a photograph, or both"))
		return
	}

	prior, conversation, err := s.priorScratchAnswers(r, req.ConversationID)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	shown, attached, err := s.attach(r.Context(), conversation, req.Photo, prior)
	if err != nil {
		s.fail(w, statusForPhoto(err), err)
		return
	}

	answer, err := s.judge.Ask(r.Context(), req.Message, shown, prior, conversation)
	if err != nil {
		s.fail(w, http.StatusBadGateway, err)
		return
	}

	saved, err := s.store.SaveConsultTurn(r.Context(), store.ConsultTurn{
		ConversationID: conversation,
		Asked:          req.Message,
		Reply:          answer,
		PhotoID:        attached,
	})
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	// The same shape a plant consultation returns, so one client decoder
	// covers both conversations rather than two that drift.
	s.ok(w, http.StatusOK, map[string]any{
		"id":                   saved.ID,
		"conversation_id":      saved.ConversationID,
		"reply":                answer.Reply,
		"confidence":           answer.Confidence,
		"looked_at":            answer.LookedAt,
		"suggested_follow_ups": answer.Suggestions,
	})
}

// priorScratchAnswers reads back a conversation about nothing in particular.
func (s *Server) priorScratchAnswers(r *http.Request,
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
		prior = append(prior, judge.PriorAnswer{
			Asked: turn.Asked, Reply: turn.Reply, PhotoID: turn.PhotoID,
		})
	}
	return prior, *conversationID, nil
}

// attach stores a photograph sent with this turn and gathers the ones sent
// with earlier turns, so a conversation that loses its model session can be
// replayed with its pictures instead of without them.
func (s *Server) attach(ctx context.Context, conversation uuid.UUID, encoded string,
	prior []judge.PriorAnswer) ([]judge.Offer, *uuid.UUID, error) {
	var shown []judge.Offer
	for i, earlier := range prior {
		if earlier.PhotoID == nil {
			continue
		}
		raw, media, err := s.photoBytes(ctx, *earlier.PhotoID)
		if err != nil {
			continue
		}
		shown = append(shown, judge.Offer{
			Label: fmt.Sprintf("sent earlier, with message %d", i+1),
			Media: media, Bytes: raw,
		})
	}

	if encoded == "" {
		return shown, nil, nil
	}

	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, nil, fmt.Errorf("the photograph is not valid base64: %w", err)
	}
	media := http.DetectContentType(raw)
	ext, ok := photoTypes[media]
	if !ok {
		return nil, nil, ErrPhotoType
	}
	if len(raw) > MaxPhotoBytes {
		return nil, nil, ErrPhotoSize
	}

	// Keyed by conversation rather than by plant, since there is no plant. The
	// pictures of one question stay together and can be swept as a unit.
	taken := time.Now().UTC()
	key := photos.Key("scratch/"+conversation.String(), taken, ext)
	if s.photos == nil {
		return nil, nil, errors.New("keeping a photograph needs object storage, and none is configured")
	}
	if _, err := s.photos.Put(ctx, key, media,
		bytes.NewReader(raw), int64(len(raw))); err != nil {
		return nil, nil, err
	}

	saved, err := s.store.SavePhoto(ctx, plant.Photo{StorageKey: key, TakenAt: taken})
	if err != nil {
		return nil, nil, err
	}

	shown = append(shown, judge.Offer{
		Label: "just sent, the subject of this question",
		Media: media, Bytes: raw,
	})
	return shown, &saved.ID, nil
}

// keepAnswerPhoto files a photograph sent mid-conversation against its plant,
// captioned so a later reader knows it was taken to answer a question rather
// than as one of the routine timeline shots.
func (s *Server) keepAnswerPhoto(ctx context.Context, p plant.Plant,
	encoded string) (plant.Photo, []byte, string, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return plant.Photo{}, nil, "", fmt.Errorf("the photograph is not valid base64: %w", err)
	}

	media := http.DetectContentType(raw)
	shot, err := s.keepPhoto(ctx, p, raw, media, "asked for in conversation", time.Now().UTC())
	if err != nil {
		return plant.Photo{}, nil, "", err
	}
	return shot, raw, media, nil
}

// photoBytes reads one stored image back out.
func (s *Server) photoBytes(ctx context.Context, id uuid.UUID) ([]byte, string, error) {
	if s.photos == nil {
		return nil, "", errors.New("no object storage")
	}
	shot, err := s.store.Photo(ctx, id)
	if err != nil {
		return nil, "", err
	}
	body, err := s.photos.Get(ctx, shot.StorageKey)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(body, MaxPhotoBytes))
	if err != nil {
		return nil, "", err
	}
	return raw, mediaFor(shot.StorageKey), nil
}
