package job

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

// PrepLead is how far ahead of leaving the pre-departure pass fires.
const PrepLead = 36 * time.Hour

// Away changes behaviour around a trip rather than muting notifications.
type Away struct {
	Store         *store.Store
	Log           *slog.Logger
	Notifications Notifier
}

// Run does whichever of the three away jobs the calendar calls for.
func (a Away) Run(ctx context.Context) error {
	if trip, err := a.Store.UpcomingAway(ctx, PrepLead); err == nil {
		return a.prepare(ctx, trip)
	} else if !errors.Is(err, store.ErrNotFound) {
		return err
	}

	trip, err := a.Store.AwayAt(ctx, time.Now())
	if errors.Is(err, store.ErrNotFound) {
		return a.briefOnReturn(ctx)
	}
	if err != nil {
		return err
	}

	a.Log.Info("away", "until", trip.EndsAt, "backup", trip.BackupContact)
	return nil
}

func (a Away) prepare(ctx context.Context, trip plant.AwayPeriod) error {
	plants, err := a.Store.ListPlants(ctx, store.PlantFilter{Status: plant.StatusAlive})
	if err != nil {
		return err
	}

	var needy []plant.Plant
	for _, p := range plants {
		if p.NeedsChasing() {
			needy = append(needy, p)
		}
	}
	if len(needy) == 0 {
		return nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "You leave in %s. These are hand-watered, so nothing happens while you are gone:\n",
		until(trip.StartsAt))
	for _, p := range needy {
		fmt.Fprintf(&b, "\n- %s", p.CommonName)
		if p.IsFriends() {
			fmt.Fprintf(&b, " (%s's)", p.Steward)
		}
	}
	if trip.BackupContact == "" {
		b.WriteString("\n\nNo backup contact is recorded for this trip.")
	} else {
		fmt.Fprintf(&b, "\n\n%s is recorded as the backup contact.", trip.BackupContact)
	}

	a.Log.Info("pre-departure pass", "plants", len(needy))
	return notify(ctx, a.Notifications, "Water these before you go", b.String(), nil)
}

func (a Away) briefOnReturn(ctx context.Context) error {
	digest, err := a.Store.Digest(ctx, plant.StaleAfter)
	if err != nil {
		return err
	}
	if digest.AllClear() {
		return nil
	}

	var b strings.Builder
	b.WriteString("While you were away:\n")
	for _, entry := range digest.Entries {
		fmt.Fprintf(&b, "\n- %s: %s", entry.Plant.CommonName, entry.Verdict.Reasoning)
	}
	if digest.StaleSince != nil {
		b.WriteString("\n\nReadings are stale, so this may not be the whole picture.")
	}

	return notify(ctx, a.Notifications, "Back home: what needs you", b.String(), nil)
}

func until(t time.Time) string {
	d := time.Until(t)
	if d < 24*time.Hour {
		return fmt.Sprintf("%d hours", int(d.Hours()))
	}
	return fmt.Sprintf("%d days", int(d.Hours()/24))
}
