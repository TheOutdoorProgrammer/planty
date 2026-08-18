package api

import (
	"encoding/json"
	"errors"
	"net/http"
)

// shelterRequest names what moved. `all` is the honest default for the actual
// interaction, which happens at dusk with an armful of pots.
type shelterRequest struct {
	Slugs []string `json:"slugs,omitempty"`
	All   bool     `json:"all,omitempty"`
}

// shelter answers the cold warning; without it the warning repeats forever and
// nothing is ever eligible to go back out.
func (s *Server) shelter(w http.ResponseWriter, r *http.Request) {
	s.moveIndoors(w, r, true)
}

// unshelter records that plants went back outside.
func (s *Server) unshelter(w http.ResponseWriter, r *http.Request) {
	s.moveIndoors(w, r, false)
}

func (s *Server) moveIndoors(w http.ResponseWriter, r *http.Request, inside bool) {
	var ask shelterRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&ask)
	}
	if !ask.All && len(ask.Slugs) == 0 {
		s.fail(w, http.StatusBadRequest,
			errors.New("name the plants in slugs, or pass all"))
		return
	}

	var (
		moved int64
		err   error
	)
	switch {
	case ask.All && inside:
		moved, err = s.store.ShelterAll(r.Context())
	case ask.All:
		moved, err = s.store.UnshelterAll(r.Context())
	case inside:
		moved, err = s.store.Shelter(r.Context(), ask.Slugs)
	default:
		moved, err = s.store.Unshelter(r.Context(), ask.Slugs)
	}
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	where := "outside"
	if inside {
		where = "indoors"
	}
	s.log.Info("plants moved", "where", where, "count", moved)
	s.ok(w, http.StatusOK, map[string]any{"moved": moved, "where": where})
}
