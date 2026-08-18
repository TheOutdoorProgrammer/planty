package plant

import (
	"time"

	"github.com/google/uuid"
)

// QuestionStatus tracks whether an owner has answered yet.
type QuestionStatus string

const (
	QuestionOpen     QuestionStatus = "open"
	QuestionAnswered QuestionStatus = "answered"
	QuestionDropped  QuestionStatus = "dropped"
)

// Question is something only the plant's owner can settle.
type Question struct {
	ID      uuid.UUID  `json:"id"`
	PlantID *uuid.UUID `json:"plant_id,omitempty"`

	AskedOf  string `json:"asked_of"`
	Question string `json:"question"`
	Why      string `json:"why,omitempty"`

	Status     QuestionStatus `json:"status"`
	Answer     string         `json:"answer,omitempty"`
	AnsweredAt *time.Time     `json:"answered_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}

// AwayPeriod is a window where nobody is home to act on a notification.
type AwayPeriod struct {
	ID       uuid.UUID `json:"id"`
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`

	BackupContact string `json:"backup_contact,omitempty"`
	BackupNotify  string `json:"backup_notify,omitempty"`
	Note          string `json:"note,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}

// Covers reports whether a moment falls inside this away period.
func (a AwayPeriod) Covers(t time.Time) bool {
	return !t.Before(a.StartsAt) && t.Before(a.EndsAt)
}

// Postmortem is what a dead plant taught, written once when it dies.
type Postmortem struct {
	ID      uuid.UUID `json:"id"`
	PlantID uuid.UUID `json:"plant_id"`

	LikelyCause string   `json:"likely_cause"`
	Narrative   string   `json:"narrative"`
	Evidence    Evidence `json:"evidence"`
	Lesson      string   `json:"lesson,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}
