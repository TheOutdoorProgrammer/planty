package api

import (
	"errors"
	"net/http"

	"github.com/TheOutdoorProgrammer/planty/internal/job"
)

func (s *Server) assessPlant(w http.ResponseWriter, r *http.Request) {
	if s.judge == nil {
		s.fail(w, http.StatusServiceUnavailable,
			errors.New("analyzing a plant needs a judge, and none is configured"))
		return
	}

	p, err := s.store.GetPlant(r.Context(), r.PathValue("slug"))
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	verdict, err := (job.Daily{
		Store: s.store, Judge: s.judge, Photos: s.photos, Log: s.log,
		Policies: s.policyRunner,
	}).AssessPlant(r.Context(), p)
	if err != nil {
		s.fail(w, http.StatusBadGateway, err)
		return
	}
	s.ok(w, http.StatusOK, verdict)
}
