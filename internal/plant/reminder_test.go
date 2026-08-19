package plant

import (
	"testing"
	"time"
)

func at(day, hour, minute int) time.Time {
	return time.Date(2026, 8, day, hour, minute, 0, 0, time.UTC)
}

func misting() Reminder {
	r := Reminder{Kind: ObservedMisted, EveryDays: 1, AtHours: []int{8, 20}, Active: true}
	r.Normalise()
	return r
}

func watering(everyDays int) Reminder {
	r := Reminder{Kind: ObservedWatered, EveryDays: everyDays, AtHours: []int{8}, Active: true}
	r.Normalise()
	return r
}

// The mushroom case, and the reason a reminder is not just an interval in days:
// a kit misted at breakfast is due again that evening.
func TestTwiceDailyMistingFiresTwiceInOneDay(t *testing.T) {
	r := misting()

	morning := at(18, 8, 30)
	if evening := at(18, 20, 15); !r.Due(&morning, evening) {
		t.Error("misted at breakfast, and the evening misting never came due")
	}

	justMisted := at(18, 20, 5)
	if r.Due(&justMisted, at(18, 20, 30)) {
		t.Error("came due again minutes after being done")
	}
	if !r.Due(&justMisted, at(19, 8, 1)) {
		t.Error("the next morning never came due")
	}
}

func TestAnHourThatHasNotArrivedIsNotDue(t *testing.T) {
	r := misting()
	lastNight := at(18, 20, 5)

	if r.Due(&lastNight, at(19, 7, 30)) {
		t.Error("due at half seven for an eight o'clock slot")
	}
	if !r.Due(&lastNight, at(19, 8, 0)) {
		t.Error("not due at exactly the scheduled hour")
	}
}

func TestAMultiDayCadenceWaitsItsDays(t *testing.T) {
	r := watering(7)
	watered := at(11, 8, 30)

	if r.Due(&watered, at(15, 9, 0)) {
		t.Error("a weekly watering came due after four days")
	}
	if !r.Due(&watered, at(18, 9, 0)) {
		t.Error("a weekly watering never came due after seven days")
	}
}

// A plant nobody has ever watered is exactly the plant a reminder was set for.
func TestNeverDoneIsDue(t *testing.T) {
	if !watering(30).Due(nil, at(18, 9, 0)) {
		t.Error("a reminder for something never done is not due")
	}
}

func TestAnInactiveReminderIsNeverDue(t *testing.T) {
	r := misting()
	r.Active = false

	if r.Due(nil, at(18, 20, 0)) {
		t.Error("a switched-off reminder still fired")
	}
}

// The whole point of computing from observations: a notification nobody acted
// on has to stay due, or the reminder silently cancels itself.
func TestAnIgnoredNotificationStaysDue(t *testing.T) {
	r := watering(7)
	sent := at(18, 8, 1)
	r.LastSentAt = &sent
	watered := at(4, 8, 0)

	if !r.Due(&watered, at(18, 12, 0)) {
		t.Error("sending the notification cleared the reminder on its own")
	}
	if !r.AlreadySent(&watered, at(18, 12, 0)) {
		t.Error("the same slot would be notified twice in one day")
	}
	if r.AlreadySent(&watered, at(19, 9, 0)) {
		t.Error("yesterday's notification suppressed today's")
	}
}

func TestEachMistingSlotNotifiesSeparately(t *testing.T) {
	r := misting()
	morningDone := at(18, 8, 10)
	sentAtBreakfast := at(18, 8, 0)
	r.LastSentAt = &sentAtBreakfast

	if !r.AlreadySent(&morningDone, at(18, 9, 0)) {
		t.Error("the breakfast reminder would be sent again an hour later")
	}
	if r.AlreadySent(&morningDone, at(18, 20, 30)) {
		t.Error("the morning notification suppressed the evening one")
	}
}

func TestNormaliseTidiesTheSchedule(t *testing.T) {
	r := Reminder{Kind: ObservedMisted, AtHours: []int{20, 8, 20}}
	r.Normalise()

	if r.EveryDays != 1 {
		t.Errorf("every_days defaulted to %d", r.EveryDays)
	}
	if len(r.AtHours) != 2 || r.AtHours[0] != 8 || r.AtHours[1] != 20 {
		t.Errorf("hours came out %v, want [8 20]", r.AtHours)
	}

	bare := Reminder{Kind: ObservedWatered}
	bare.Normalise()
	if len(bare.AtHours) != 1 || bare.AtHours[0] != DefaultReminderHour {
		t.Errorf("an unscheduled reminder got %v", bare.AtHours)
	}
}

func TestUnremindableAndImpossibleSchedulesAreRejected(t *testing.T) {
	cases := map[string]Reminder{
		"a symptom is not a chore": {Kind: ObservedSymptom, EveryDays: 1, AtHours: []int{8}},
		"zero days":                {Kind: ObservedWatered, EveryDays: 0, AtHours: []int{8}},
		"beyond a year":            {Kind: ObservedWatered, EveryDays: 400, AtHours: []int{8}},
		"no hours at all":          {Kind: ObservedWatered, EveryDays: 1},
		"a twenty-fifth hour":      {Kind: ObservedWatered, EveryDays: 1, AtHours: []int{24}},
	}
	for name, r := range cases {
		if err := r.Valid(); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}

	good := misting()
	if err := good.Valid(); err != nil {
		t.Errorf("twice-daily misting was rejected: %v", err)
	}
}

func TestOvernightHoursStillFindYesterdaysSlot(t *testing.T) {
	r := Reminder{Kind: ObservedMisted, EveryDays: 1, AtHours: []int{22}, Active: true}
	r.Normalise()
	done := at(17, 22, 5)

	slot, ok := r.LastSlot(&done, at(18, 3, 0))
	if !ok {
		t.Fatal("no slot found at three in the morning")
	}
	if slot.Day() != 17 || slot.Hour() != 22 {
		t.Errorf("slot is %s, want the previous night", slot)
	}
	if r.Due(&done, at(18, 3, 0)) {
		t.Error("due again five hours after being done")
	}
}
