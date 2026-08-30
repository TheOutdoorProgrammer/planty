package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

func (s *Server) approveCalibrationProposal(w http.ResponseWriter, r *http.Request) {
	s.resolveCalibrationProposal(w, r, true)
}

func (s *Server) denyCalibrationProposal(w http.ResponseWriter, r *http.Request) {
	s.resolveCalibrationProposal(w, r, false)
}

func (s *Server) resolveCalibrationProposal(w http.ResponseWriter, r *http.Request, approve bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	var request struct {
		Actor string `json:"actor"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			s.fail(w, http.StatusBadRequest, err)
			return
		}
	}
	if strings.TrimSpace(request.Actor) == "" {
		request.Actor = "owner"
	}
	proposal, err := s.store.ResolveCalibrationProposal(r.Context(), id, approve, request.Actor)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, proposal)
}
