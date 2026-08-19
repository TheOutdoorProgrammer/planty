package job

import (
	"strings"
	"testing"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

func when(day, hour int) time.Time {
	return time.Date(2026, 8, day, hour, 0, 0, 0, time.UTC)
}

func owedFor(name string, kind plant.ObservationKind, hours []int, everyDays int,
	lastDone *time.Time) store.DueReminder {
	r := plant.Reminder{Kind: kind, EveryDays: everyDays, AtHours: hours, Active: true}
	r.Normalise()
	return store.DueReminder{
		Reminder: r,
		Plant:    plant.Plant{CommonName: name, Location: "Basement"},
		LastDone: lastDone,
	}
}

func TestOnlyOwedRemindersGoOut(t *testing.T) {
	mistedThisMorning := when(18, 8)
	wateredYesterday := when(17, 8)

	owed := Owed([]store.DueReminder{
		owedFor("Blue oyster", plant.ObservedMisted, []int{8, 20}, 1, &mistedThisMorning),
		owedFor("Pothos", plant.ObservedWatered, []int{8}, 10, &wateredYesterday),
		owedFor("Never touched", plant.ObservedWatered, []int{8}, 7, nil),
	}, when(18, 20))

	if len(owed) != 2 {
		t.Fatalf("%d owed, want the evening misting and the plant nobody has watered", len(owed))
	}
	if owed[0].Plant.CommonName != "Never touched" {
		t.Errorf("%q is first; the never-done one should lead", owed[0].Plant.CommonName)
	}
}

// Sending must not be what clears a reminder, or an hourly job clears its own
// backlog without anything actually being done to a plant.
func TestASentReminderIsNotSentAgainForTheSameSlot(t *testing.T) {
	done := when(17, 20)
	due := owedFor("Blue oyster", plant.ObservedMisted, []int{8, 20}, 1, &done)

	if got := Owed([]store.DueReminder{due}, when(18, 8)); len(got) != 1 {
		t.Fatalf("the morning misting was not owed")
	}

	sent := when(18, 8)
	due.Reminder.LastSentAt = &sent

	if got := Owed([]store.DueReminder{due}, when(18, 9)); len(got) != 0 {
		t.Error("the same slot was sent twice in one morning")
	}
	if got := Owed([]store.DueReminder{due}, when(18, 20)); len(got) != 1 {
		t.Error("the morning send suppressed the evening misting")
	}
}

func TestTheNotificationSaysWhatToDo(t *testing.T) {
	lastWeek := when(11, 8)
	owed := []store.DueReminder{
		owedFor("Blue oyster", plant.ObservedMisted, []int{8, 20}, 1, nil),
		owedFor("Pothos", plant.ObservedWatered, []int{8}, 7, &lastWeek),
	}

	text := body(owed, when(18, 20))
	for _, want := range []string{"mist it", "water it", "Basement", "never done", "7 days ago"} {
		if !strings.Contains(text, want) {
			t.Errorf("the notification never says %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "misted") {
		t.Errorf("the notification reports a record instead of giving an instruction:\n%s", text)
	}

	if got := title(owed); got != "2 plants need you" {
		t.Errorf("title is %q", got)
	}
	if got := title(owed[:1]); got != "Blue oyster: mist it" {
		t.Errorf("a single reminder titled %q", got)
	}
}
