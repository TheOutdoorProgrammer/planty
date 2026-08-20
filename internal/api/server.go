// Package api is the HTTP surface. The iOS app and the Dusk plugin are both
// clients of it and have identical powers; see docs/DATA-MODEL.md.
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"

	"github.com/TheOutdoorProgrammer/planty/internal/ha"
	"github.com/TheOutdoorProgrammer/planty/internal/judge"
	"github.com/TheOutdoorProgrammer/planty/internal/photos"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

// Server routes HTTP onto the store.
//
// There is no authentication by deliberate choice; keep it on the LAN.
type Server struct {
	store         *store.Store
	log           *slog.Logger
	photos        *photos.Store
	judge         *judge.Judge
	homeAssistant homeAssistantDiscoverer
}

// New builds a server. Photo storage and the judge are optional: without them
// the photo routes report unavailable rather than the whole service failing.
func New(s *store.Store, log *slog.Logger) *Server {
	server := &Server{store: s, log: log}
	if baseURL, token := os.Getenv("PLANTY_HA_URL"), os.Getenv("PLANTY_HA_TOKEN"); baseURL != "" && token != "" {
		server.homeAssistant = ha.New(baseURL, token)
	}
	return server
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

	mux.HandleFunc("GET /v1/plants/{slug}/notes", s.listNotes)
	mux.HandleFunc("POST /v1/plants/{slug}/notes", s.addNote)
	// No slug: true of the house rather than of anything growing in it.
	mux.HandleFunc("GET /v1/notes", s.listHouseholdNotes)
	mux.HandleFunc("POST /v1/notes", s.addHouseholdNote)
	mux.HandleFunc("PATCH /v1/notes/{id}", s.updateNote)
	mux.HandleFunc("DELETE /v1/notes/{id}", s.deleteNote)

	mux.HandleFunc("GET /v1/harvests", s.listHarvests)
	mux.HandleFunc("GET /v1/plants/{slug}/harvests", s.listHarvests)
	mux.HandleFunc("POST /v1/plants/{slug}/harvests", s.addHarvest)
	mux.HandleFunc("POST /v1/plants/{slug}/photos", s.uploadPhoto)
	mux.HandleFunc("GET /v1/plants/{slug}/timeline", s.timeline)
	mux.HandleFunc("POST /v1/plants/{slug}/ask", s.consult)

	mux.HandleFunc("GET /v1/plants/{slug}/reminders", s.listReminders)
	mux.HandleFunc("PUT /v1/plants/{slug}/reminders", s.setReminder)
	mux.HandleFunc("DELETE /v1/plants/{slug}/reminders/{kind}", s.deleteReminder)

	// No slug: a question about something in a shop is not a plant you own.
	mux.HandleFunc("POST /v1/ask", s.ask)

	mux.HandleFunc("POST /v1/identify", s.identify)
	mux.HandleFunc("POST /v1/plants/from-photo", s.plantFromPhoto)

	mux.HandleFunc("GET /v1/postmortems", s.listPostmortems)
	mux.HandleFunc("POST /v1/plants/{slug}/postmortem", s.autopsy)

	mux.HandleFunc("GET /v1/today", s.today)
	mux.HandleFunc("POST /v1/verdicts/{id}/ack", s.ackVerdict)

	mux.HandleFunc("GET /v1/home-assistant/entities", s.discoverHomeAssistantEntities)

	mux.HandleFunc("GET /v1/sensors", s.listSensors)
	mux.HandleFunc("POST /v1/sensors", s.linkSensor)
	mux.HandleFunc("PATCH /v1/sensors/{id}", s.calibrateSensor)

	mux.HandleFunc("GET /v1/questions", s.listQuestions)
	mux.HandleFunc("POST /v1/questions", s.askOwner)
	mux.HandleFunc("POST /v1/questions/{id}/answer", s.answerQuestion)

	mux.HandleFunc("GET /v1/away", s.listAway)
	mux.HandleFunc("POST /v1/away", s.goAway)
	mux.HandleFunc("PATCH /v1/away/{id}", s.updateAway)
	mux.HandleFunc("DELETE /v1/away/{id}", s.cancelAway)

	mux.HandleFunc("GET /v1/cold-watch", s.coldWatch)
	mux.HandleFunc("POST /v1/shelter", s.shelter)
	mux.HandleFunc("POST /v1/unshelter", s.unshelter)

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

// fail maps the error onto a status, so the error decides whose fault it was
// rather than every call site guessing. A database outage reporting 400 would
// send whoever is debugging it looking at the request.
func (s *Server) fail(w http.ResponseWriter, code int, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		code = http.StatusNotFound
	case errors.Is(err, store.ErrConversationOwner):
		code = http.StatusConflict
	case errors.Is(err, plant.ErrInvalid):
		code = http.StatusBadRequest
	}
	if code >= http.StatusInternalServerError {
		s.log.Error("request failed", "status", code, "error", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
