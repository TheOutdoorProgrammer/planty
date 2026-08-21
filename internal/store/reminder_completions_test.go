package store

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

func TestReminderCompletionLogsConfiguredKindAndIsIdempotent(t *testing.T) {
	s, ctx := testStore(t)
	p := newPlant(t, s, ctx, "Remembered mushroom")
	now := time.Now().UTC()

	reminder, err := s.SaveReminder(ctx, plant.Reminder{
		PlantID:   p.ID,
		Kind:      plant.ObservedMisted,
		EveryDays: 1,
		AtHours:   []int{now.Hour()},
		Active:    true,
		Note:      "mist the surface lightly",
	})
	if err != nil {
		t.Fatal(err)
	}

	due, err := s.DueReminders(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	var occurrence DueReminder
	for _, candidate := range due {
		if candidate.Reminder.ID == reminder.ID {
			occurrence = candidate
			break
		}
	}
	if occurrence.Reminder.ID == uuid.Nil {
		t.Fatal("a due reminder was missing from Today's work")
	}

	completion := ReminderCompletion{
		IdempotencyKey: uuid.New(),
		ReminderID:     reminder.ID,
		DueAt:          occurrence.DueAt,
	}
	first, err := s.CompleteReminder(ctx, completion)
	if err != nil {
		t.Fatal(err)
	}
	if first.Kind != plant.ObservedMisted || first.Body != reminder.Note {
		t.Fatalf("completion recorded %+v, want the reminder's mist action and note", first)
	}

	second, err := s.CompleteReminder(ctx, completion)
	if err != nil {
		t.Fatalf("retry should replay the committed reminder: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("retry duplicated the observation: %s != %s", first.ID, second.ID)
	}

	otherPhone, err := s.CompleteReminder(ctx, ReminderCompletion{
		IdempotencyKey: uuid.New(),
		ReminderID:     reminder.ID,
		DueAt:          occurrence.DueAt,
	})
	if err != nil {
		t.Fatalf("the same occurrence from another device should replay: %v", err)
	}
	if first.ID != otherPhone.ID {
		t.Fatalf("two devices produced two observations: %s != %s", first.ID, otherPhone.ID)
	}

	var observations int
	if err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM observations
		WHERE plant_id = $1 AND kind = 'misted'`, p.ID).Scan(&observations); err != nil {
		t.Fatal(err)
	}
	if observations != 1 {
		t.Fatalf("reminder completion wrote %d observations, want 1", observations)
	}

	due, err = s.DueReminders(ctx, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range due {
		if candidate.Reminder.ID == reminder.ID {
			t.Fatal("a completed reminder occurrence stayed on Today")
		}
	}
}
