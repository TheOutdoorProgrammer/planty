package api

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/judge"
)

// MaxIdentifyBytes caps an upload. The app sends a cutout, not a full frame,
// so anything this large is a client that skipped the cutout.
const MaxIdentifyBytes = 8 << 20

// identify names a plant from one photograph. Deliberately not under a plant:
// nobody knows which plant it is yet, and it may not be one on record.
func (s *Server) identify(w http.ResponseWriter, r *http.Request) {
	if s.judge == nil {
		s.fail(w, http.StatusServiceUnavailable,
			errors.New("identification needs ANTHROPIC_API_KEY, which is unset"))
		return
	}

	image, media, err := readImage(r, MaxIdentifyBytes)
	if err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}

	candidates, err := s.judge.Identify(r.Context(),
		judge.Frame{Bytes: image, Media: media, TakenAt: time.Now()},
		sighting(r),
	)
	if err != nil {
		s.fail(w, http.StatusBadGateway, err)
		return
	}

	s.ok(w, http.StatusOK, map[string]any{
		"candidates": candidates,
		"count":      len(candidates),
	})
}

// sighting reads the optional where and when. Both absent is normal: a picker
// copy usually has its GPS stripped.
func sighting(r *http.Request) judge.Sighting {
	var seen judge.Sighting
	query := r.URL.Query()

	if raw := query.Get("taken_at"); raw != "" {
		if when, err := time.Parse(time.RFC3339, raw); err == nil {
			seen.TakenAt = &when
		}
	}

	latitude, latErr := strconv.ParseFloat(query.Get("lat"), 64)
	longitude, lonErr := strconv.ParseFloat(query.Get("lon"), 64)
	if latErr == nil && lonErr == nil {
		seen.Latitude = &latitude
		seen.Longitude = &longitude
	}
	return seen
}

// readImage accepts a multipart part named photo, or raw bytes with a content
// type, matching how photo upload already works.
func readImage(r *http.Request, limit int64) ([]byte, string, error) {
	body := http.MaxBytesReader(nil, r.Body, limit)

	if file, header, err := r.FormFile("photo"); err == nil {
		defer func() { _ = file.Close() }()
		media := header.Header.Get("Content-Type")
		if media == "" {
			media = "image/jpeg"
		}
		bytes, err := io.ReadAll(file)
		return bytes, media, err
	}

	media := r.Header.Get("Content-Type")
	if media == "" {
		media = "image/jpeg"
	}
	bytes, err := io.ReadAll(body)
	if err != nil {
		return nil, "", err
	}
	if len(bytes) == 0 {
		return nil, "", errors.New("no image: send a photo part or raw bytes")
	}
	return bytes, media, nil
}
