package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

// listReminders returns what is set for a plant, with what is owed right now
// worked out here rather than in the client, so the app and an agent agree.
func (s *Server) listReminders(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetPlant(r.Context(), r.PathValue("slug"))
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	reminders, err := s.store.Reminders(r.Context(), p.ID)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	now := time.Now()
	type entry struct {
		plant.Reminder
		LastDone *time.Time `json:"last_done,omitempty"`
		Due      bool       `json:"due"`
	}
	out := make([]entry, 0, len(reminders))
	for _, reminder := range reminders {
		e := entry{Reminder: reminder}
		if done, err := s.store.LastObserved(r.Context(), p.ID, reminder.Kind); err == nil {
			e.LastDone = &done
		} else if !errors.Is(err, store.ErrNotFound) {
			s.fail(w, http.StatusInternalServerError, err)
			return
		}
		e.Due = reminder.Due(e.LastDone, now)
		if e.Due {
			slot, ok := reminder.LastSlot(e.LastDone, now)
			if ok {
				resolved, err := s.store.ReminderOccurrenceResolved(r.Context(), reminder.ID, slot)
				if err != nil {
					s.fail(w, http.StatusInternalServerError, err)
					return
				}
				e.Due = !resolved
			}
		}
		out = append(out, e)
	}
	s.ok(w, http.StatusOK, map[string]any{"reminders": out, "count": len(out)})
}

// reminderRequest is what the app sends. Hours are optional because most
// reminders happen once in the morning; mushrooms are why they can be a list.
type reminderRequest struct {
	Kind      plant.ObservationKind `json:"kind"`
	EveryDays int                   `json:"every_days"`
	AtHours   []int                 `json:"at_hours"`
	Active    *bool                 `json:"active"`
	Note      string                `json:"note"`
}

func (s *Server) setReminder(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetPlant(r.Context(), r.PathValue("slug"))
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	var ask reminderRequest
	if err := json.NewDecoder(r.Body).Decode(&ask); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}

	// Setting a reminder means wanting it, so it is on unless switched off.
	active := true
	if ask.Active != nil {
		active = *ask.Active
	}

	saved, err := s.store.SaveReminder(r.Context(), plant.Reminder{
		PlantID:   p.ID,
		Kind:      ask.Kind,
		EveryDays: ask.EveryDays,
		AtHours:   ask.AtHours,
		Active:    active,
		Note:      ask.Note,
	})
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusCreated, saved)
}

func (s *Server) deleteReminder(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetPlant(r.Context(), r.PathValue("slug"))
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	if err := s.store.DeleteReminder(r.Context(),
		p.ID, plant.ObservationKind(r.PathValue("kind"))); err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, map[string]any{"deleted": true})
}
