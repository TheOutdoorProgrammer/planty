// Package api is the HTTP surface. The iOS app and the Dusk plugin are both
// clients of it and have identical powers; see docs/DATA-MODEL.md.
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

// Server routes HTTP onto the store.
type Server struct {
	store *store.Store
	log   *slog.Logger
	token string
}

// New builds a server; an empty token disables auth (loopback only).
func New(s *store.Store, log *slog.Logger, token string) *Server {
	return &Server{store: s, log: log, token: token}
}

// Handler returns the routed, authenticated mux.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", s.health)

	mux.HandleFunc("GET /v1/plants", s.listPlants)
	mux.HandleFunc("POST /v1/plants", s.createPlant)
	mux.HandleFunc("GET /v1/plants/{slug}", s.getPlant)
	mux.HandleFunc("DELETE /v1/plants/{slug}", s.archivePlant)

	mux.HandleFunc("GET /v1/plants/{slug}/observations", s.listObservations)
	mux.HandleFunc("POST /v1/plants/{slug}/observations", s.addObservation)

	mux.HandleFunc("GET /v1/cold-watch", s.coldWatch)

	return s.authenticate(mux)
}

// authenticate rejects anything without the shared token. Health is exempt so a
// probe never needs a credential.
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.token == "" || r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+s.token {
			s.fail(w, http.StatusUnauthorized, errors.New("unauthorized"))
			return
		}
		next.ServeHTTP(w, r)
	})
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
