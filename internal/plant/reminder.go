package plant

import (
	"slices"
	"time"

	"github.com/google/uuid"
)

// Reminder covers the jobs no sensor checks: nothing confirms misting the way
// a probe confirms watering. AtHours is separate from EveryDays because
// misting twice a day cannot be said as an interval in days.
type Reminder struct {
	ID      uuid.UUID       `json:"id"`
	PlantID uuid.UUID       `json:"plant_id"`
	Kind    ObservationKind `json:"kind"`

	EveryDays int    `json:"every_days"`
	AtHours   []int  `json:"at_hours"`
	Active    bool   `json:"active"`
	Note      string `json:"note,omitempty"`

	LastSentAt *time.Time `json:"last_sent_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// MaxReminderDays is a year. Past that it is not a reminder, it is a diary.
const MaxReminderDays = 365

// Remindable are the kinds worth nagging about: things a person does on a
// rhythm. There is no reminder to notice a symptom or to have a plant die.
var Remindable = []ObservationKind{
	ObservedWatered, ObservedMisted, ObservedFertilized, ObservedPruned, ObservedRepotted,
}

// Normalise sorts and dedupes the hours and fills the defaults, so two callers
// describing the same schedule store the same row.
func (r *Reminder) Normalise() {
	if r.EveryDays == 0 {
		r.EveryDays = 1
	}
	if len(r.AtHours) == 0 {
		r.AtHours = []int{DefaultReminderHour}
	}
	slices.Sort(r.AtHours)
	r.AtHours = slices.Compact(r.AtHours)
}

// DefaultReminderHour matches the daily digest, so one morning notification
// carries everything unless somebody asked for another time.
const DefaultReminderHour = 8

// Valid reports whether this reminder can be stored.
func (r Reminder) Valid() error {
	if !slices.Contains(Remindable, r.Kind) {
		return invalid("nothing is reminded about %q", r.Kind)
	}
	if r.EveryDays < 1 || r.EveryDays > MaxReminderDays {
		return invalid("every_days is %d, want 1 to %d", r.EveryDays, MaxReminderDays)
	}
	if len(r.AtHours) == 0 {
		return invalid("a reminder needs at least one hour to fire at")
	}
	for _, hour := range r.AtHours {
		if hour < 0 || hour > 23 {
			return invalid("at_hours has %d, want 0 to 23", hour)
		}
	}
	return nil
}

// slotSearchDays bounds the walk back through the calendar. Two days is enough
// to find the previous slot however the hours are arranged.
const slotSearchDays = 2

// LastSlot is the most recent moment this was scheduled to happen. Past a
// day's cadence the slot must clear when the thing was last done; within one
// day the hours are the schedule, so twice-daily misting fires twice.
func (r Reminder) LastSlot(lastDone *time.Time, now time.Time) (time.Time, bool) {
	// Counted in whole days from the day it was done, not in hours from the
	// minute. Watering at 08:30 must not push an 08:00 slot to the next day,
	// which is how a weekly chore quietly becomes an eight-day one.
	var earliest time.Time
	if lastDone != nil && r.EveryDays > 1 {
		earliest = midnight(lastDone.In(now.Location())).AddDate(0, 0, r.EveryDays)
	}

	hours := slices.Clone(r.AtHours)
	slices.Sort(hours)

	for back := 0; back <= slotSearchDays; back++ {
		day := now.AddDate(0, 0, -back)
		for _, hour := range slices.Backward(hours) {
			slot := time.Date(day.Year(), day.Month(), day.Day(), hour, 0, 0, 0, now.Location())
			if slot.After(now) {
				continue
			}
			if slot.Before(earliest) {
				return time.Time{}, false
			}
			return slot, true
		}
	}
	return time.Time{}, false
}

func midnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// Due reports whether this reminder is owed. Never done counts as due, and the
// clock runs from the last real action rather than the last notification, so
// one nobody acted on stays due instead of quietly rescheduling itself.
func (r Reminder) Due(lastDone *time.Time, now time.Time) bool {
	if !r.Active {
		return false
	}
	slot, ok := r.LastSlot(lastDone, now)
	if !ok {
		return false
	}
	return lastDone == nil || lastDone.Before(slot)
}

// AlreadySent reports whether the notification for the current slot has gone
// out, which is what stops an hourly job sending the same thing all day. It
// tracks the slot rather than the date so the evening misting still sends.
func (r Reminder) AlreadySent(lastDone *time.Time, now time.Time) bool {
	if r.LastSentAt == nil {
		return false
	}
	slot, ok := r.LastSlot(lastDone, now)
	if !ok {
		return true
	}
	return !r.LastSentAt.Before(slot)
}
