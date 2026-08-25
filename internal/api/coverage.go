package api

import (
	"net/http"
	"time"
)

func (s *Server) evidenceCoverage(w http.ResponseWriter, r *http.Request) {
	coverage, err := s.store.EvidenceCoverage(r.Context(), time.Now().UTC())
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	complete := 0
	for _, item := range coverage {
		if item.Complete() {
			complete++
		}
	}
	s.ok(w, http.StatusOK, map[string]any{
		"plants": coverage, "count": len(coverage), "complete": complete,
	})
}
