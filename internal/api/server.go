// Package api is the HTTP surface. The iOS app and the Dusk plugin are both
// clients of it and have identical powers; see docs/DATA-MODEL.md.
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/TheOutdoorProgrammer/planty/internal/judge"
	"github.com/TheOutdoorProgrammer/planty/internal/photos"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

// Server routes HTTP onto the store.
//
// There is no authentication by deliberate choice; keep it on the LAN.
type Server struct {
	store  *store.Store
	log    *slog.Logger
	photos *photos.Store
	judge  *judge.Judge
}

// New builds a server. Photo storage and the judge are optional: without them
// the photo routes report unavailable rather than the whole service failing.
func New(s *store.Store, log *slog.Logger) *Server {
	return &Server{store: s, log: log}
}

// WithPhotos enables the photo timeline and vision diagnosis routes.
func (s *Server) WithPhotos(p *photos.Store, j *judge.Judge) *Server {
	s.photos, s.judge = p, j
	return s
}

// Handler returns the routed, authenticated mux.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", s.health)

	mux.HandleFunc("GET /v1/plants", s.listPlants)
	mux.HandleFunc("POST /v1/plants", s.createPlant)
	mux.HandleFunc("GET /v1/plants/{slug}", s.getPlant)
	mux.HandleFunc("PATCH /v1/plants/{slug}", s.updatePlant)
	mux.HandleFunc("DELETE /v1/plants/{slug}", s.archivePlant)

	mux.HandleFunc("GET /v1/plants/{slug}/observations", s.listObservations)
	mux.HandleFunc("POST /v1/plants/{slug}/observations", s.addObservation)

	mux.HandleFunc("POST /v1/plants/{slug}/harvests", s.addHarvest)
	mux.HandleFunc("POST /v1/plants/{slug}/photos", s.uploadPhoto)
	mux.HandleFunc("GET /v1/plants/{slug}/timeline", s.timeline)
	mux.HandleFunc("POST /v1/plants/{slug}/diagnosis", s.diagnose)

	mux.HandleFunc("GET /v1/postmortems", s.listPostmortems)

	mux.HandleFunc("GET /v1/today", s.today)
	mux.HandleFunc("POST /v1/verdicts/{id}/ack", s.ackVerdict)

	mux.HandleFunc("GET /v1/sensors", s.listSensors)
	mux.HandleFunc("POST /v1/sensors", s.linkSensor)
	mux.HandleFunc("PATCH /v1/sensors/{id}", s.calibrateSensor)

	mux.HandleFunc("GET /v1/questions", s.listQuestions)
	mux.HandleFunc("POST /v1/questions", s.askOwner)
	mux.HandleFunc("POST /v1/questions/{id}/answer", s.answerQuestion)

	mux.HandleFunc("POST /v1/away", s.goAway)

	mux.HandleFunc("GET /v1/cold-watch", s.coldWatch)

	return mux
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Healthy(r.Context()); err != nil {
		s.fail(w, http.StatusServiceUnavailable, err)
		return
	}
	s.ok(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) ok(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		s.log.Error("encode response", "error", err)
	}
}

// fail maps a store error onto a status, so a missing plant reads the same to
// the app and to an agent.
func (s *Server) fail(w http.ResponseWriter, code int, err error) {
	if errors.Is(err, store.ErrNotFound) {
		code = http.StatusNotFound
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
