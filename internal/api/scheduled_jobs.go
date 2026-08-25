package api

import (
	"fmt"
	"net/http"

	"github.com/TheOutdoorProgrammer/planty/internal/scheduledjob"
)

func (s *Server) listScheduledJobs(w http.ResponseWriter, r *http.Request) {
	if s.scheduledJobs == nil {
		s.fail(w, http.StatusServiceUnavailable, scheduledjob.ErrUnavailable)
		return
	}
	jobs, err := s.scheduledJobs.List(r.Context())
	if err != nil {
		s.fail(w, http.StatusServiceUnavailable, err)
		return
	}
	s.ok(w, http.StatusOK, map[string]any{"jobs": jobs})
}

func (s *Server) runScheduledJob(w http.ResponseWriter, r *http.Request) {
	id, err := scheduledjob.ValidateID(r.PathValue("job"))
	if err != nil {
		s.fail(w, http.StatusNotFound, fmt.Errorf("scheduled job: %w", err))
		return
	}
	if s.scheduledJobs == nil {
		s.fail(w, http.StatusServiceUnavailable, scheduledjob.ErrUnavailable)
		return
	}
	run, created, err := s.scheduledJobs.Start(r.Context(), id)
	if err != nil {
		s.fail(w, http.StatusServiceUnavailable, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusAccepted
	}
	s.ok(w, status, run)
}
