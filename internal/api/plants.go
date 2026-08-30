package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

func (s *Server) listPlants(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := store.PlantFilter{
		Domain:          plant.Domain(q.Get("domain")),
		Steward:         q.Get("steward"),
		Status:          plant.Status(q.Get("status")),
		WateringMethod:  plant.WateringMethod(q.Get("watering_method")),
		IncludeArchived: q.Get("include_archived") == "true",
	}

	plants, err := s.store.ListPlants(r.Context(), filter)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, map[string]any{
		"plants": s.withThumbnails(r, plants),
		"count":  len(plants),
	})
}

// listedPlant is a plant plus the newest picture of it, so a library of plants
// shows the plants rather than a column of identical leaf glyphs.
type listedPlant struct {
	plant.Plant
	PhotoURL       string     `json:"photo_url,omitempty"`
	PhotoTaken     *time.Time `json:"photo_taken_at,omitempty"`
	ActiveWatering bool       `json:"active_watering,omitempty"`
}

// withThumbnails attaches a link to each plant's most recent photograph.
// Best effort: a plant with no photo, or storage being down, still lists.
func (s *Server) withThumbnails(r *http.Request, plants []plant.Plant) []listedPlant {
	listed := make([]listedPlant, 0, len(plants))
	for _, p := range plants {
		listed = append(listed, listedPlant{Plant: p})
	}
	if len(plants) == 0 {
		return listed
	}
	activeWatering, err := s.store.ActiveWateringPlantIDs(r.Context())
	if err != nil {
		s.log.Warn("listing without active watering state", "error", err)
	} else {
		for i := range listed {
			_, listed[i].ActiveWatering = activeWatering[listed[i].ID]
		}
	}
	if s.photos == nil {
		return listed
	}

	ids := make([]uuid.UUID, 0, len(plants))
	for _, p := range plants {
		ids = append(ids, p.ID)
	}
	newest, err := s.store.NewestPhotos(r.Context(), ids)
	if err != nil {
		s.log.Warn("listing without thumbnails", "error", err)
		return listed
	}

	for i, entry := range listed {
		shot, ok := newest[entry.ID]
		if !ok {
			continue
		}
		link, err := s.photos.URL(r.Context(), shot.StorageKey, PhotoLinkTTL)
		if err != nil || !validPhotoURL(link) {
			continue
		}
		listed[i].PhotoURL = link
		listed[i].PhotoTaken = &shot.TakenAt
	}
	return listed
}

func (s *Server) createPlant(w http.ResponseWriter, r *http.Request) {
	var p plant.Plant
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}

	plant.ApplyDefaults(&p)

	created, err := s.store.CreatePlant(r.Context(), p)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusCreated, created)
}

func (s *Server) getPlant(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetPlant(r.Context(), r.PathValue("slug"))
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	history, next, err := s.store.ObservationsPage(r.Context(), p.ID, nil, 20)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	body := map[string]any{
		"plant":        p,
		"risk":         p.Risk(),
		"observations": history,
	}
	if lineage, err := s.store.PlantLineage(r.Context(), p.ID); err == nil {
		body["lineage"] = lineage
	} else if !errors.Is(err, store.ErrNotFound) {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	if cursor := encodeHistoryCursor(next); cursor != "" {
		body["observations_next_cursor"] = cursor
	}
	if at, err := s.store.LastWatered(r.Context(), p.ID); err == nil {
		body["last_watered"] = at
	} else if !errors.Is(err, store.ErrNotFound) {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	// Sensor links and readings have to travel together. A reading without its
	// link loses its role, calibration, and stable identity on the client.
	links, readings, err := s.sensorSnapshot(r.Context(), &p.ID)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	if len(links) > 0 {
		body["sensors"] = links
		body["readings"] = readings
	}
	proposals, err := s.store.PendingCalibrationProposals(r.Context(), p.ID)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	if len(proposals) > 0 {
		body["calibration_proposals"] = proposals
	}
	if verdict, err := s.store.LatestVerdict(r.Context(), p.ID); err == nil {
		body["verdict"] = verdict
	} else if !errors.Is(err, store.ErrNotFound) {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, body)
}

func (s *Server) sensorSnapshot(
	ctx context.Context,
	plantID *uuid.UUID,
) ([]plant.SensorLink, []plant.Reading, error) {
	ctx, span := otel.Tracer("planty/api").Start(ctx, "plant.sensor.snapshot")
	defer span.End()

	links, err := s.store.SensorLinks(ctx, plantID)
	if err != nil {
		span.RecordError(err)
		return nil, nil, err
	}

	readings := make([]plant.Reading, 0, len(links))
	for _, link := range links {
		latest, err := s.store.LatestReading(ctx, link.ID)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			span.RecordError(err)
			return nil, nil, err
		}
		readings = append(readings, latest)
	}

	span.SetAttributes(
		attribute.Int("plant.sensor.count", len(links)),
		attribute.Int("plant.sensor.reading_count", len(readings)),
	)
	return links, readings, nil
}

// updatePlant applies a sparse patch, so an agent can change one field.
func (s *Server) updatePlant(w http.ResponseWriter, r *http.Request) {
	var patch store.PlantPatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}

	updated, err := s.store.UpdatePlant(r.Context(), r.PathValue("slug"), patch)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, updated)
}

// archivePlant retires a plant. It is never a hard delete: what was done to a
// plant that died is the record most worth keeping.
func (s *Server) archivePlant(w http.ResponseWriter, r *http.Request) {
	status := plant.Status(r.URL.Query().Get("status"))
	if status == "" {
		status = plant.StatusRemoved
	}
	if err := status.ValidateArchive(); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.ArchivePlant(r.Context(), r.PathValue("slug"), status); err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, map[string]string{"archived": r.PathValue("slug")})
}

func (s *Server) derivePlant(w http.ResponseWriter, r *http.Request) {
	var request plant.DerivePlantRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	created, lineage, err := s.store.DerivePlant(r.Context(), r.PathValue("slug"), request)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusCreated, map[string]any{"plant": created, "lineage": lineage})
}

func (s *Server) restorePlant(w http.ResponseWriter, r *http.Request) {
	restored, err := s.store.RestorePlant(r.Context(), r.PathValue("slug"))
	if errors.Is(err, store.ErrNotFound) {
		s.fail(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, restored)
}

func (s *Server) listObservations(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetPlant(r.Context(), r.PathValue("slug"))
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	cursor, err := decodeHistoryCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	limit, err := pageLimit(r.URL.Query(), 50)
	if err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}

	history, next, err := s.store.ObservationsPage(r.Context(), p.ID, cursor, limit)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, map[string]any{
		"observations": history,
		"next_cursor":  encodeHistoryCursor(next),
	})
}

func (s *Server) addObservation(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetPlant(r.Context(), r.PathValue("slug"))
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	var o plant.Observation
	if err := json.NewDecoder(r.Body).Decode(&o); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	o.PlantID = p.ID
	if o.Source == "" {
		o.Source = plant.SourceApp
	}

	created, err := s.store.AddObservation(r.Context(), o)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusCreated, created)
}

// coldWatch answers which plants need bringing in for a given forecast low.
func (s *Server) coldWatch(w http.ResponseWriter, r *http.Request) {
	low, err := strconv.ParseFloat(r.URL.Query().Get("forecast_low_f"), 64)
	if err != nil {
		s.fail(w, http.StatusBadRequest, errors.New("forecast_low_f is required"))
		return
	}

	plants, err := s.store.ColdWatch(r.Context(), low)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, map[string]any{
		"forecast_low_f": low,
		"plants":         plants,
		"count":          len(plants),
	})
}
