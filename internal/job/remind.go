package job

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

// Remind sends the chores nothing else notices. It runs hourly because a
// mushroom kit misted at eight and again at eight is two separate reminders,
// and a once-a-day job can only ever send one of them.
type Remind struct {
	Store         *store.Store
	Log           *slog.Logger
	Notifications Notifier
	Now           func() time.Time
}

func (r Remind) Run(ctx context.Context) (int, error) {
	now := time.Now()
	if r.Now != nil {
		now = r.Now()
	}

	reminders, err := r.Store.ActiveReminders(ctx)
	if err != nil {
		return 0, fmt.Errorf("active reminders: %w", err)
	}

	owed := Owed(reminders, now)
	if len(owed) == 0 {
		r.Log.Info("nothing owed", "checked", len(reminders))
		return 0, nil
	}

	if err := notify(ctx, r.Notifications, title(owed), body(owed, now), nil); err != nil {
		return 0, fmt.Errorf("notify: %w", err)
	}

	for _, due := range owed {
		if err := r.Store.MarkReminderSent(ctx, due.Reminder.ID, now); err != nil {
			r.Log.Error("could not mark reminder sent",
				"plant", due.Plant.Slug, "kind", due.Reminder.Kind, "error", err)
		}
	}
	return len(owed), nil
}

func Owed(reminders []store.DueReminder, now time.Time) []store.DueReminder {
	out := make([]store.DueReminder, 0)
	for _, due := range reminders {
		if !due.Reminder.Due(due.LastDone, now) {
			continue
		}
		if due.Reminder.AlreadySent(due.LastDone, now) {
			continue
		}
		out = append(out, due)
	}

	sort.SliceStable(out, func(i, j int) bool {
		return waited(out[i], now) > waited(out[j], now)
	})
	return out
}

func waited(due store.DueReminder, now time.Time) time.Duration {
	if due.LastDone == nil {
		return 1 << 62
	}
	return now.Sub(*due.LastDone)
}

func title(owed []store.DueReminder) string {
	if len(owed) == 1 {
		return fmt.Sprintf("%s: %s", owed[0].Plant.CommonName, verb(owed[0].Reminder.Kind))
	}
	return fmt.Sprintf("%d plants need you", len(owed))
}

func body(owed []store.DueReminder, now time.Time) string {
	var b strings.Builder
	for _, due := range owed {
		fmt.Fprintf(&b, "%s: %s", due.Plant.CommonName, verb(due.Reminder.Kind))
		if due.Plant.Location != "" {
			fmt.Fprintf(&b, " (%s)", due.Plant.Location)
		}
		if due.LastDone == nil {
			b.WriteString(", never done")
		} else if days := int(now.Sub(*due.LastDone).Hours() / 24); days > 0 {
			fmt.Fprintf(&b, ", last %d days ago", days)
		}
		if due.Reminder.Note != "" {
			fmt.Fprintf(&b, ". %s", due.Reminder.Note)
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func verb(kind plant.ObservationKind) string {
	switch kind {
	case plant.ObservedWatered:
		return "water it"
	case plant.ObservedMisted:
		return "mist it"
	case plant.ObservedFertilized:
		return "feed it"
	case plant.ObservedPruned:
		return "prune it"
	case plant.ObservedRepotted:
		return "repot it"
	default:
		return string(kind)
	}
}
