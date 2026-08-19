package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

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
	PhotoURL   string     `json:"photo_url,omitempty"`
	PhotoTaken *time.Time `json:"photo_taken_at,omitempty"`
}

// withThumbnails attaches a link to each plant's most recent photograph.
// Best effort: a plant with no photo, or storage being down, still lists.
func (s *Server) withThumbnails(r *http.Request, plants []plant.Plant) []listedPlant {
	listed := make([]listedPlant, 0, len(plants))
	for _, p := range plants {
		listed = append(listed, listedPlant{Plant: p})
	}
	if s.photos == nil || len(plants) == 0 {
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
		if err != nil {
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

	applyPlantDefaults(&p)

	created, err := s.store.CreatePlant(r.Context(), p)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusCreated, created)
}

// applyPlantDefaults fills the fields an agent describing a plant in
// conversation will rarely supply, so a sparse create still validates.
func applyPlantDefaults(p *plant.Plant) {
	if p.Domain == "" {
		p.Domain = plant.DomainHouseplant
	}
	if p.Status == "" {
		p.Status = plant.StatusAlive
	}
	if p.Steward == "" {
		p.Steward = plant.StewardSelf
	}
	if p.Accessibility == "" {
		p.Accessibility = plant.AccessEasy
	}
	if p.WateringMethod == "" {
		p.WateringMethod = plant.WateringHand
	}
}

func (s *Server) getPlant(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetPlant(r.Context(), r.PathValue("slug"))
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	history, err := s.store.Observations(r.Context(), p.ID, 20)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	body := map[string]any{
		"plant":        p,
		"risk":         p.Risk(),
		"observations": history,
	}
	if at, err := s.store.LastWatered(r.Context(), p.ID); err == nil {
		body["last_watered"] = at
	} else if !errors.Is(err, store.ErrNotFound) {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	// Readings and the current verdict, because a plant page without them
	// sends the client straight back for two more round trips.
	if readings, err := s.latestReadings(r.Context(), p); err == nil && len(readings) > 0 {
		body["readings"] = readings
	}
	if verdict, err := s.store.LatestVerdict(r.Context(), p.ID); err == nil {
		body["verdict"] = verdict
	} else if !errors.Is(err, store.ErrNotFound) {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, body)
}

// readingView is one probe's latest sample, expressed against its own
// baselines because two probes' absolute numbers are never comparable.
type readingView struct {
	Role       plant.SensorRole `json:"role"`
	EntityID   string           `json:"ha_entity_id"`
	Raw        float64          `json:"raw"`
	Fraction   *float64         `json:"fraction,omitempty"`
	Calibrated bool             `json:"calibrated"`
	TakenAt    time.Time        `json:"taken_at"`
}

func (s *Server) latestReadings(ctx context.Context, p plant.Plant) ([]readingView, error) {
	links, err := s.store.SensorLinks(ctx, &p.ID)
	if err != nil {
		return nil, err
	}

	out := make([]readingView, 0, len(links))
	for _, link := range links {
		latest, err := s.store.LatestReading(ctx, link.ID)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}

		view := readingView{
			Role:       link.Role,
			EntityID:   link.HAEntityID,
			Raw:        latest.Value,
			Calibrated: link.Calibrated(),
			TakenAt:    latest.TakenAt,
		}
		if view.Calibrated {
			if f, err := link.Fraction(latest.Value); err == nil {
				view.Fraction = &f
			}
		}
		out = append(out, view)
	}
	return out, nil
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
		status = plant.StatusGone
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

func (s *Server) listObservations(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetPlant(r.Context(), r.PathValue("slug"))
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	history, err := s.store.Observations(r.Context(), p.ID, limit)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, map[string]any{"observations": history})
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
