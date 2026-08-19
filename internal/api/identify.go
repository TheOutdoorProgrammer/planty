package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/judge"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

// MaxIdentifyBytes caps an upload. The app sends a cutout, not a full frame,
// so anything this large is a client that skipped the cutout.
const MaxIdentifyBytes = 8 << 20

// identify names a plant from one photograph. Deliberately not under a plant:
// nobody knows which plant it is yet, and it may not be one on record.
func (s *Server) identify(w http.ResponseWriter, r *http.Request) {
	if s.judge == nil {
		s.fail(w, http.StatusServiceUnavailable,
			errors.New("identification needs a judge, and none is configured"))
		return
	}

	image, media, err := readImage(w, r, MaxIdentifyBytes)
	if err != nil {
		s.fail(w, statusForUpload(err), err)
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

// plantFromPhoto adds a plant by photographing it, so the alternative is not
// typing a species name you do not know into a form.
func (s *Server) plantFromPhoto(w http.ResponseWriter, r *http.Request) {
	if s.judge == nil || s.photos == nil {
		s.fail(w, http.StatusServiceUnavailable,
			errors.New("adding a plant from a photograph needs the judge and photo storage"))
		return
	}

	image, media, err := readImage(w, r, MaxPhotoBytes)
	if err != nil {
		s.fail(w, statusForUpload(err), err)
		return
	}

	seen := sighting(r)
	takenAt := time.Now().UTC()
	if seen.TakenAt != nil {
		takenAt = seen.TakenAt.UTC()
	}

	candidates, err := s.judge.Identify(r.Context(),
		judge.Frame{Bytes: image, Media: media, TakenAt: takenAt}, seen)
	if err != nil {
		s.fail(w, http.StatusBadGateway, err)
		return
	}

	p, err := describedBy(r, candidates)
	if err != nil {
		s.fail(w, http.StatusUnprocessableEntity, err)
		return
	}
	if p.Slug, err = s.store.FreeSlug(r.Context(), p.CommonName); err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	created, err := s.store.CreatePlant(r.Context(), p)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	body := map[string]any{"plant": created, "candidates": candidates}

	// The plant exists either way. Losing the photograph is worth reporting
	// and not worth unwinding a record the caller can see was created.
	photo, err := s.keepPhoto(r.Context(), created, image, media, "", takenAt)
	if err != nil {
		s.log.Warn("plant created but its photograph was not kept",
			"plant", created.Slug, "error", err)
		body["photo_error"] = err.Error()
	} else {
		body["photo"] = photo
	}
	s.ok(w, http.StatusCreated, body)
}

// describedBy builds the plant from what the model saw, letting the caller
// override any of it. A named override wins outright, because somebody looking
// at the plant knows better than a model looking at a picture of it.
func describedBy(r *http.Request, candidates []judge.Candidate) (plant.Plant, error) {
	query := r.URL.Query()

	p := plant.Plant{
		CommonName:    query.Get("common_name"),
		BotanicalName: query.Get("botanical_name"),
		Location:      query.Get("location"),
		Domain:        plant.Domain(query.Get("domain")),
		Steward:       query.Get("steward"),
	}
	if p.CommonName == "" && len(candidates) > 0 {
		p.CommonName = candidates[0].CommonName
		if p.BotanicalName == "" {
			p.BotanicalName = candidates[0].ScientificName
		}
	}
	if p.CommonName == "" {
		return plant.Plant{}, errors.New(
			"nothing in the photograph could be named: pass common_name to record it anyway")
	}

	applyPlantDefaults(&p)
	return p, nil
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
func readImage(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, string, error) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		r.Body = http.MaxBytesReader(w, r.Body, limit+maxMultipartOverhead)
		if err := r.ParseMultipartForm(limit); err != nil {
			return nil, "", uploadReadError(err)
		}
		file, header, err := r.FormFile("photo")
		if err != nil {
			return nil, "", errors.New("multipart upload needs a 'photo' part")
		}
		defer func() { _ = file.Close() }()
		media := header.Header.Get("Content-Type")
		if media == "" {
			media = "image/jpeg"
		}
		bytes, err := readBounded(file, limit)
		return bytes, media, err
	}

	media := r.Header.Get("Content-Type")
	if media == "" {
		media = "image/jpeg"
	}
	bytes, err := readBounded(r.Body, limit)
	if err != nil {
		return nil, "", err
	}
	if len(bytes) == 0 {
		return nil, "", errors.New("no image: send a photo part or raw bytes")
	}
	return bytes, media, nil
}
