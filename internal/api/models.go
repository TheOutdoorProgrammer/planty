package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/TheOutdoorProgrammer/planty/internal/judge"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

// modelView is one selectable model and, crucially, which jobs it may be given.
// The phone filters on this rather than reimplementing the capability rules,
// so there is one definition of what a model can do and the server owns it.
type modelView struct {
	Provider string       `json:"provider"`
	ID       string       `json:"id"`
	Ref      string       `json:"ref"`
	Name     string       `json:"name"`
	Rank     int          `json:"rank"`
	Skills   judge.Skills `json:"skills"`
	Note     string       `json:"note,omitempty"`
	Jobs     []string     `json:"jobs"`
}

type jobView struct {
	Job      string `json:"job"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Ref      string `json:"ref"`
	Default  bool   `json:"default"`
}

func (s *Server) listModels(w http.ResponseWriter, r *http.Request) {
	providers, err := judge.Providers()
	if err != nil {
		s.fail(w, http.StatusServiceUnavailable, err)
		return
	}

	catalog := judge.Catalog(providers)
	out := make([]modelView, 0, len(catalog))
	for _, m := range catalog {
		jobs := make([]string, 0, len(judge.Jobs))
		for _, job := range judge.Jobs {
			if m.CanDo(job) == nil {
				jobs = append(jobs, string(job))
			}
		}
		out = append(out, modelView{
			Provider: m.Provider, ID: m.ID, Ref: m.Ref(), Name: m.Name,
			Rank: m.Rank, Skills: m.Skills, Note: m.Note, Jobs: jobs,
		})
	}
	s.ok(w, http.StatusOK, map[string]any{"models": out})
}

func (s *Server) listModelAssignments(w http.ResponseWriter, r *http.Request) {
	assigned, err := s.store.ModelAssignments(r.Context())
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	chosen := map[judge.Job]store.ModelAssignment{}
	for _, a := range assigned {
		chosen[a.Job] = a
	}

	out := make([]jobView, 0, len(judge.Jobs))
	for _, job := range judge.Jobs {
		view := jobView{Job: string(job), Default: true}
		if a, ok := chosen[job]; ok {
			view.Provider, view.Model = a.Provider, a.Model
			view.Ref = a.Provider + "/" + a.Model
			view.Default = false
		}
		out = append(out, view)
	}
	s.ok(w, http.StatusOK, map[string]any{"assignments": out})
}

type assignmentRequest struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

func (s *Server) setModelAssignment(w http.ResponseWriter, r *http.Request) {
	job, ok := requestedJob(r)
	if !ok {
		s.fail(w, http.StatusNotFound, fmt.Errorf("there is no job %q", r.PathValue("job")))
		return
	}

	var request assignmentRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.fail(w, http.StatusBadRequest, fmt.Errorf("decode assignment: %w", err))
		return
	}
	request.Provider = strings.TrimSpace(request.Provider)
	request.Model = strings.TrimSpace(request.Model)

	err := s.store.SetModelAssignment(r.Context(), store.ModelAssignment{
		Job: job, Provider: request.Provider, Model: request.Model,
	})
	if err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	s.ok(w, http.StatusOK, jobView{
		Job: string(job), Provider: request.Provider, Model: request.Model,
		Ref: request.Provider + "/" + request.Model,
	})
}

func (s *Server) clearModelAssignment(w http.ResponseWriter, r *http.Request) {
	job, ok := requestedJob(r)
	if !ok {
		s.fail(w, http.StatusNotFound, fmt.Errorf("there is no job %q", r.PathValue("job")))
		return
	}
	if err := s.store.ClearModelAssignment(r.Context(), job); err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, jobView{Job: string(job), Default: true})
}

// requestedJob reads the job from the path, refusing one Planty does not have
// so a typo becomes a 404 rather than a row nothing will ever read.
func requestedJob(r *http.Request) (judge.Job, bool) {
	asked := judge.Job(strings.TrimSpace(r.PathValue("job")))
	for _, job := range judge.Jobs {
		if job == asked {
			return job, true
		}
	}
	return "", false
}
