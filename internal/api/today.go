package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/TheOutdoorProgrammer/planty/internal/job"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

// today answers "what should I do right now" for both clients.
func (s *Server) today(w http.ResponseWriter, r *http.Request) {
	digest, err := s.store.ReliableDigest(r.Context(), plant.StaleAfter)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	dueReminders, err := s.store.DueReminders(r.Context(), time.Now())
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	// Questions the model queued for somebody who is not in the conversation,
	// usually a friend whose plants these are. Without an outlet they piled up
	// in a table nobody opened.
	waiting, err := s.store.Questions(r.Context(), "", plant.QuestionOpen)
	if err != nil {
		s.log.Warn("open questions unavailable", "error", err)
	}

	incidents, err := s.store.Incidents(r.Context(), plant.IncidentOpen)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	failures := digest.Failures
	if failures == nil {
		failures = []plant.JudgmentFailure{}
	}
	s.ok(w, http.StatusOK, map[string]any{
		"date":           digest.Date,
		"entries":        digest.Entries,
		"due_reminders":  dueReminders,
		"checked":        digest.Checked,
		"expected":       digest.Expected,
		"failed":         digest.Failed,
		"failures":       failures,
		"run_complete":   digest.RunComplete,
		"stale_since":    digest.StaleSince,
		"never_run":      digest.NeverRun,
		"all_clear":      digest.AllClear() && len(dueReminders) == 0 && len(incidents) == 0,
		"open_questions": waiting,
		"incidents":      incidents,
	})
}

func (s *Server) ackVerdict(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.AckVerdict(r.Context(), id); err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, map[string]string{"acknowledged": id.String()})
}

// listPostmortems is what the Dusk plugin reads to attach each lesson to its
// plant as a gotcha, so a death teaches every future session and not just the
// one that noticed.
func (s *Server) listPostmortems(w http.ResponseWriter, r *http.Request) {
	records, err := s.store.Postmortems(r.Context())
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, map[string]any{
		"postmortems": records,
		"count":       len(records),
	})
}

// autopsy asks what killed one plant, now. The daily sweep writes these
// unprompted, but a death somebody wants explained should not need a shell.
func (s *Server) autopsy(w http.ResponseWriter, r *http.Request) {
	if s.judge == nil {
		s.fail(w, http.StatusServiceUnavailable,
			errors.New("an autopsy needs a judge, and none is configured"))
		return
	}

	record, err := job.Postmortem{Store: s.store, Judge: s.judge, Log: s.log}.
		Run(r.Context(), r.PathValue("slug"))
	if err != nil {
		s.fail(w, http.StatusBadGateway, err)
		return
	}
	s.ok(w, http.StatusCreated, record)
}

func (s *Server) listSensors(w http.ResponseWriter, r *http.Request) {
	links, err := s.store.SensorLinks(r.Context(), nil)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, map[string]any{"sensors": links, "count": len(links)})
}

func (s *Server) linkSensor(w http.ResponseWriter, r *http.Request) {
	var link plant.SensorLink
	if err := json.NewDecoder(r.Body).Decode(&link); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	created, err := s.store.LinkSensor(r.Context(), link)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusCreated, created)
}

func (s *Server) calibrateSensor(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}

	var body struct {
		Dry float64 `json:"dry_baseline"`
		Wet float64 `json:"wet_baseline"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}

	link, err := s.store.Calibrate(r.Context(), id, body.Dry, body.Wet)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, link)
}

func (s *Server) listQuestions(w http.ResponseWriter, r *http.Request) {
	status := plant.QuestionStatus(r.URL.Query().Get("status"))
	if status == "" {
		status = plant.QuestionOpen
	}
	questions, err := s.store.Questions(r.Context(), r.URL.Query().Get("asked_of"), status)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, map[string]any{
		"questions": questions,
		"count":     len(questions),
		"as_text":   questionText(questions),
	})
}

// questionText renders the queue as one message worth sending, which is the
// whole point of batching them.
func questionText(questions []plant.Question) string {
	if len(questions) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("A few questions about the plants:\n")
	for i, q := range questions {
		fmt.Fprintf(&b, "\n%d. %s", i+1, q.Question)
	}
	return b.String()
}

func (s *Server) askOwner(w http.ResponseWriter, r *http.Request) {
	var q plant.Question
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	created, err := s.store.AskOwner(r.Context(), q)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusCreated, created)
}

func (s *Server) answerQuestion(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	var body struct {
		Answer string `json:"answer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	answered, err := s.store.AnswerQuestion(r.Context(), id, body.Answer)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, answered)
}

func (s *Server) goAway(w http.ResponseWriter, r *http.Request) {
	var a plant.AwayPeriod
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	created, err := s.store.GoAway(r.Context(), a)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusCreated, created)
}

func (s *Server) addHarvest(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetPlant(r.Context(), r.PathValue("slug"))
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	var h plant.Harvest
	if err := json.NewDecoder(r.Body).Decode(&h); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	h.PlantID = p.ID

	created, err := s.store.AddHarvest(r.Context(), h)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, plant.ErrInvalid) {
			status = http.StatusBadRequest
		}
		s.fail(w, status, err)
		return
	}
	s.ok(w, http.StatusCreated, created)
}

func (s *Server) updateHarvest(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	current, err := s.store.Harvest(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		s.fail(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	var patch struct {
		OccurredAt time.Time `json:"occurred_at"`
		Quantity   float64   `json:"quantity"`
		Unit       string    `json:"unit"`
		Notes      string    `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	current.OccurredAt = patch.OccurredAt
	current.Quantity = patch.Quantity
	current.Unit = patch.Unit
	current.Notes = patch.Notes
	updated, err := s.store.UpdateHarvest(r.Context(), current)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, plant.ErrInvalid) {
			status = http.StatusBadRequest
		}
		s.fail(w, status, err)
		return
	}
	s.ok(w, http.StatusOK, updated)
}

func (s *Server) deleteHarvest(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.DeleteHarvest(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		s.fail(w, http.StatusNotFound, err)
		return
	} else if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) harvestSummary(w http.ResponseWriter, r *http.Request) {
	summary, err := s.store.HarvestSummary(r.Context())
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, map[string]any{"summary": summary, "count": len(summary)})
}

func (s *Server) listHarvests(w http.ResponseWriter, r *http.Request) {
	var plantID *uuid.UUID
	if slug := r.PathValue("slug"); slug != "" {
		p, err := s.store.GetPlant(r.Context(), slug)
		if err != nil {
			s.fail(w, http.StatusInternalServerError, err)
			return
		}
		plantID = &p.ID
	}

	harvests, err := s.store.Harvests(r.Context(), plantID)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, map[string]any{
		"harvests": harvests,
		"count":    len(harvests),
	})
}
