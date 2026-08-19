package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/photos"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

// MaxPhotoBytes bounds one upload. Phone photos are a few MB; anything much
// larger is a mistake, and the vision call has its own limits anyway.
const MaxPhotoBytes = 12 << 20

// Multipart framing is small, but the request limit must leave room for it
// beyond the file limit or a valid image at the boundary would be rejected.
const maxMultipartOverhead = 64 << 10

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

	body, contentType, caption, takenAt, err := readUpload(w, r)
	if err != nil {
		s.fail(w, statusForUpload(err), err)
		return
	}
	if takenAt.IsZero() {
		takenAt = time.Now().UTC()
	}

	saved, err := s.keepPhoto(r.Context(), p, body, contentType, caption, takenAt)
	if err != nil {
		s.fail(w, statusForPhoto(err), err)
		return
	}
	s.ok(w, http.StatusCreated, saved)
}

// ErrPhotoType and ErrPhotoSize are the two ways a photograph is refused
// before anything is written, and both are the caller's fault.
var (
	ErrPhotoType = errors.New("send image/jpeg, image/png or image/webp")
	ErrPhotoSize = errors.New("photo is too large")
)

// keepPhoto puts one image in storage and records it against a plant. Shared,
// because a plant created from a photograph keeps that photograph the same way
// an uploaded one is kept.
func (s *Server) keepPhoto(ctx context.Context, p plant.Plant, body []byte,
	contentType, caption string, takenAt time.Time) (plant.Photo, error) {
	ext, ok := photoTypes[contentType]
	if !ok {
		return plant.Photo{}, ErrPhotoType
	}
	if len(body) > MaxPhotoBytes {
		return plant.Photo{}, ErrPhotoSize
	}

	// Checked before the upload, so the same capture saved and then asked
	// about costs one object and one row rather than two of each.
	sum := fmt.Sprintf("%x", sha256.Sum256(body))
	if existing, found, err := s.store.PhotoByHash(ctx, p.ID, sum); err != nil {
		return plant.Photo{}, err
	} else if found {
		return existing, nil
	}

	key := photos.Key(p.Slug, takenAt, ext)
	if _, err := s.photos.Put(ctx, key, contentType,
		bytes.NewReader(body), int64(len(body))); err != nil {
		return plant.Photo{}, err
	}

	return s.store.SavePhoto(ctx, plant.Photo{
		PlantID:     p.ID,
		StorageKey:  key,
		TakenAt:     takenAt,
		Caption:     caption,
		ContentHash: sum,
	})
}

func statusForPhoto(err error) int {
	switch {
	case errors.Is(err, ErrPhotoType):
		return http.StatusUnsupportedMediaType
	case errors.Is(err, ErrPhotoSize):
		return http.StatusRequestEntityTooLarge
	default:
		return http.StatusBadGateway
	}
}

func statusForUpload(err error) int {
	if errors.Is(err, ErrPhotoSize) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}

// readUpload accepts either shape, because the two clients have different
// natural idioms: URLSession builds multipart, and curl or an agent posts raw
// bytes with a Content-Type. Rejecting one of them would be arbitrary.
func readUpload(
	w http.ResponseWriter,
	r *http.Request,
) (
	body []byte,
	contentType string,
	caption string,
	takenAt time.Time,
	err error,
) {
	caption = r.URL.Query().Get("caption")
	contentType = r.Header.Get("Content-Type")
	takenAt, err = parseUploadTime(r.URL.Query().Get("taken_at"))
	if err != nil {
		return nil, "", "", time.Time{}, err
	}

	if !strings.HasPrefix(contentType, "multipart/form-data") {
		body, err = readBounded(r.Body, MaxPhotoBytes)
		return body, contentType, caption, takenAt, err
	}

	r.Body = http.MaxBytesReader(w, r.Body, MaxPhotoBytes+maxMultipartOverhead)
	if err := r.ParseMultipartForm(MaxPhotoBytes); err != nil {
		return nil, "", "", time.Time{}, uploadReadError(err)
	}
	file, header, err := r.FormFile("photo")
	if err != nil {
		return nil, "", "", time.Time{}, errors.New("multipart upload needs a 'photo' part")
	}
	defer func() { _ = file.Close() }()

	if given := r.FormValue("caption"); given != "" {
		caption = given
	}
	if given := r.FormValue("taken_at"); given != "" {
		takenAt, err = parseUploadTime(given)
		if err != nil {
			return nil, "", "", time.Time{}, err
		}
	}
	body, err = readBounded(file, MaxPhotoBytes)
	return body, header.Header.Get("Content-Type"), caption, takenAt, err
}

func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, uploadReadError(err)
	}
	if int64(len(body)) > limit {
		return nil, ErrPhotoSize
	}
	return body, nil
}

func uploadReadError(err error) error {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return ErrPhotoSize
	}
	return err
}

func parseUploadTime(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	takenAt, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("taken_at must be RFC3339: %w", err)
	}
	return takenAt.UTC(), nil
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
