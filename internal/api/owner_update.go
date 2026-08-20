package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/TheOutdoorProgrammer/planty/internal/judge"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

const ownerUpdateWindow = 7 * 24 * time.Hour

type ownerUpdateRequest struct {
	Steward string `json:"steward"`
}

type ownerUpdatePhoto struct {
	PlantName string    `json:"plant_name"`
	PlantSlug string    `json:"plant_slug"`
	TakenAt   time.Time `json:"taken_at"`
	URL       string    `json:"url"`
}

type ownerUpdateResponse struct {
	Steward string             `json:"steward"`
	Summary string             `json:"summary"`
	Photos  []ownerUpdatePhoto `json:"photos"`
}

func (s *Server) createOwnerUpdate(w http.ResponseWriter, r *http.Request) {
	if s.judge == nil {
		s.fail(w, http.StatusServiceUnavailable, fmt.Errorf("owner updates need a configured model"))
		return
	}
	var request ownerUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.fail(w, http.StatusBadRequest, fmt.Errorf("decode owner update: %w", err))
		return
	}
	request.Steward = strings.TrimSpace(request.Steward)
	if request.Steward == "" || request.Steward == plant.StewardSelf {
		s.fail(w, http.StatusBadRequest, fmt.Errorf("a friend's steward name is required"))
		return
	}

	plants, err := s.store.ListPlants(r.Context(), store.PlantFilter{Steward: request.Steward})
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	active := plants[:0]
	for _, p := range plants {
		if p.Status != plant.StatusDead && p.Status != plant.StatusGone {
			active = append(active, p)
		}
	}
	if len(active) == 0 {
		s.fail(w, http.StatusNotFound, store.ErrNotFound)
		return
	}

	plantIDs := make([]uuid.UUID, 0, len(active))
	for _, p := range active {
		plantIDs = append(plantIDs, p.ID)
	}
	// NewestPhotos already does the one-query-per-owner optimization used by the
	// library, so the update flow does not fan out one photo query per plant.
	newest, err := s.store.NewestPhotos(r.Context(), plantIDs)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	since := time.Now().UTC().Add(-ownerUpdateWindow)
	week := make([]judge.OwnerPlantWeek, 0, len(active))
	for _, p := range active {
		observations, err := s.store.ObservationsSince(r.Context(), p.ID, since)
		if err != nil {
			s.fail(w, http.StatusInternalServerError, err)
			return
		}
		verdicts, err := s.store.VerdictsSince(r.Context(), p.ID, since)
		if err != nil {
			s.fail(w, http.StatusInternalServerError, err)
			return
		}
		record := judge.OwnerPlantWeek{Plant: p, Observations: observations, Verdicts: verdicts}
		if photo, ok := newest[p.ID]; ok {
			copy := photo
			record.LatestPhoto = &copy
		}
		week = append(week, record)
	}

	summary, err := s.judge.OwnerUpdate(r.Context(), request.Steward, week)
	if err != nil {
		s.fail(w, http.StatusBadGateway, fmt.Errorf("generate owner update: %w", err))
		return
	}

	response := ownerUpdateResponse{Steward: request.Steward, Summary: summary, Photos: []ownerUpdatePhoto{}}
	if s.photos != nil {
		for _, record := range week {
			if record.LatestPhoto == nil {
				continue
			}
			url, err := s.photos.URL(r.Context(), record.LatestPhoto.StorageKey, 20*time.Minute)
			if err != nil {
				s.log.Warn("owner update photo URL unavailable", "plant", record.Plant.Slug, "error", err)
				continue
			}
			response.Photos = append(response.Photos, ownerUpdatePhoto{
				PlantName: record.Plant.CommonName,
				PlantSlug: record.Plant.Slug,
				TakenAt:   record.LatestPhoto.TakenAt,
				URL:       url,
			})
		}
	}
	s.ok(w, http.StatusOK, response)
}
