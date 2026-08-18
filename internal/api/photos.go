package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/TheOutdoorProgrammer/planty/internal/judge"
	"github.com/TheOutdoorProgrammer/planty/internal/photos"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

// MaxPhotoBytes bounds one upload. Phone photos are a few MB; anything much
// larger is a mistake, and the vision call has its own limits anyway.
const MaxPhotoBytes = 12 << 20

// PhotoLinkTTL is how long a presigned image link stays valid.
const PhotoLinkTTL = 30 * time.Minute

var photoTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

// uploadPhoto takes raw image bytes with the content type in the header, which
// keeps the phone client from having to build a multipart body.
func (s *Server) uploadPhoto(w http.ResponseWriter, r *http.Request) {
	if s.photos == nil {
		s.fail(w, http.StatusServiceUnavailable, errors.New("photo storage is not configured"))
		return
	}

	p, err := s.store.GetPlant(r.Context(), r.PathValue("slug"))
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	body, contentType, caption, err := readUpload(r)
	if err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	ext, ok := photoTypes[contentType]
	if !ok {
		s.fail(w, http.StatusUnsupportedMediaType,
			errors.New("send image/jpeg, image/png or image/webp"))
		return
	}
	if len(body) > MaxPhotoBytes {
		s.fail(w, http.StatusRequestEntityTooLarge, errors.New("photo is too large"))
		return
	}

	takenAt := time.Now().UTC()
	key := photos.Key(p.Slug, takenAt, ext)
	if _, err := s.photos.Put(r.Context(), key, contentType,
		bytes.NewReader(body), int64(len(body))); err != nil {
		s.fail(w, http.StatusBadGateway, err)
		return
	}

	saved, err := s.store.SavePhoto(r.Context(), plant.Photo{
		PlantID:    p.ID,
		StorageKey: key,
		TakenAt:    takenAt,
		Caption:    caption,
	})
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusCreated, saved)
}

// readUpload accepts either shape, because the two clients have different
// natural idioms: URLSession builds multipart, and curl or an agent posts raw
// bytes with a Content-Type. Rejecting one of them would be arbitrary.
func readUpload(r *http.Request) (body []byte, contentType, caption string, err error) {
	caption = r.URL.Query().Get("caption")
	contentType = r.Header.Get("Content-Type")

	if !strings.HasPrefix(contentType, "multipart/form-data") {
		body, err = io.ReadAll(io.LimitReader(r.Body, MaxPhotoBytes+1))
		return body, contentType, caption, err
	}

	if err := r.ParseMultipartForm(MaxPhotoBytes); err != nil {
		return nil, "", "", err
	}
	file, header, err := r.FormFile("photo")
	if err != nil {
		return nil, "", "", errors.New("multipart upload needs a 'photo' part")
	}
	defer func() { _ = file.Close() }()

	if given := r.FormValue("caption"); given != "" {
		caption = given
	}
	body, err = io.ReadAll(io.LimitReader(file, MaxPhotoBytes+1))
	return body, header.Header.Get("Content-Type"), caption, err
}

// timeline returns a plant's photos with short-lived links, so the app renders
// images straight from storage instead of proxying every byte through here.
func (s *Server) timeline(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetPlant(r.Context(), r.PathValue("slug"))
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	shots, err := s.store.Photos(r.Context(), p.ID, 24)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	type entry struct {
		plant.Photo
		URL string `json:"url,omitempty"`
	}
	out := make([]entry, 0, len(shots))
	for _, shot := range shots {
		e := entry{Photo: shot}
		if s.photos != nil {
			if link, err := s.photos.URL(r.Context(), shot.StorageKey, PhotoLinkTTL); err == nil {
				e.URL = link
			}
		}
		out = append(out, e)
	}
	s.ok(w, http.StatusOK, map[string]any{"photos": out, "count": len(out)})
}

// diagnosisRequest is what the app sends: an opening question, or a follow-up
// carrying the conversation it belongs to.
type diagnosisRequest struct {
	PhotoID        *uuid.UUID `json:"photo_id,omitempty"`
	Message        string     `json:"message"`
	ConversationID *uuid.UUID `json:"conversation_id,omitempty"`
}

// diagnose answers one question about a plant's photo timeline.
func (s *Server) diagnose(w http.ResponseWriter, r *http.Request) {
	if s.photos == nil || s.judge == nil {
		s.fail(w, http.StatusServiceUnavailable,
			errors.New("photo storage and the judge must both be configured"))
		return
	}

	p, err := s.store.GetPlant(r.Context(), r.PathValue("slug"))
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	var ask diagnosisRequest
	if r.Body != nil {
		// An empty body is a valid opening question, so a decode failure on no
		// content is not an error.
		_ = json.NewDecoder(r.Body).Decode(&ask)
	}

	var prior []judge.PriorTurn
	conversation := uuid.New()
	if ask.ConversationID != nil {
		conversation = *ask.ConversationID
		turns, err := s.store.DiagnosisConversation(r.Context(), conversation)
		if err != nil {
			s.fail(w, http.StatusInternalServerError, err)
			return
		}
		for _, turn := range turns {
			prior = append(prior, judge.PriorTurn{Asked: turn.Asked, Reply: turn.Reply})
		}
	}

	shots, err := s.store.Photos(r.Context(), p.ID, judge.MaxTimelineImages)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	if len(shots) == 0 {
		s.fail(w, http.StatusBadRequest, errors.New("no photographs of this plant yet"))
		return
	}

	frames := make([]judge.Frame, 0, len(shots))
	for _, shot := range shots {
		body, err := s.photos.Get(r.Context(), shot.StorageKey)
		if err != nil {
			s.log.Warn("photo missing from storage", "key", shot.StorageKey)
			continue
		}
		raw, err := io.ReadAll(io.LimitReader(body, MaxPhotoBytes))
		_ = body.Close()
		if err != nil {
			continue
		}
		frames = append(frames, judge.Frame{
			TakenAt: shot.TakenAt,
			Media:   mediaFor(shot.StorageKey),
			Bytes:   raw,
			Caption: shot.Caption,
		})
	}
	if len(frames) == 0 {
		s.fail(w, http.StatusBadGateway, errors.New("no photographs could be read back"))
		return
	}

	diagnosis, err := s.judge.Diagnose(r.Context(), p, frames, ask.Message, prior)
	if err != nil {
		s.fail(w, http.StatusBadGateway, err)
		return
	}

	saved, err := s.store.SaveDiagnosisTurn(r.Context(), store.DiagnosisTurn{
		PlantID:        p.ID,
		ConversationID: conversation,
		Asked:          ask.Message,
		Reply:          diagnosis,
		PhotoID:        ask.PhotoID,
	})
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	// Attach the reading to the newest frame, which is the one being asked about.
	newest := shots[len(shots)-1]
	if err := s.store.RecordVision(r.Context(), newest.ID, diagnosis.Observed); err != nil {
		s.log.Warn("could not record vision finding", "photo", newest.ID, "error", err)
	}

	s.ok(w, http.StatusOK, map[string]any{
		"id":                   saved.ID,
		"conversation_id":      saved.ConversationID,
		"severity":             diagnosis.Severity,
		"observed":             diagnosis.Observed,
		"interpretation":       diagnosis.Interpretation,
		"action_today":         diagnosis.ActionToday,
		"follow_up_plan":       diagnosis.FollowUpPlan,
		"citation":             diagnosis.Citation,
		"suggested_follow_ups": diagnosis.SuggestedFollowUps,
	})
}

func mediaFor(key string) string {
	switch strings.ToLower(path.Ext(key)) {
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	default:
		return "image/jpeg"
	}
}
