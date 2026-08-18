package job

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/ha"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

// PrepLead is how far ahead of leaving the pre-departure pass fires.
const PrepLead = 36 * time.Hour

// Away changes behaviour around a trip rather than muting notifications.
// Nagging a phone nobody is holding protects nothing.
type Away struct {
	Store    *store.Store
	HA       *ha.Client
	Log      *slog.Logger
	Notifier string
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

// prepare names what to water before leaving, weighted toward the plants that
// depend on a person being there.
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
		b.WriteString("\n\nNo backup contact is recorded for this trip. Nobody will be told if something goes wrong.")
	}

	a.Log.Info("pre-departure pass", "plants", len(needy))
	return a.HA.Notify(ctx, a.Notifier, "Water these before you go", b.String(), nil)
}

// briefOnReturn reports what happened while away, once, on the first run after
// a trip ends. Anything unacknowledged from the trip is what needs you.
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

	return a.HA.Notify(ctx, a.Notifier, "Back home: what needs you", b.String(), nil)
}

func until(t time.Time) string {
	d := time.Until(t)
	if d < 24*time.Hour {
		return fmt.Sprintf("%d hours", int(d.Hours()))
	}
	return fmt.Sprintf("%d days", int(d.Hours()/24))
}
