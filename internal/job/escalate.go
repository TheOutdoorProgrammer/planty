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

const (
	ChaseAfter     = 6 * time.Hour
	MaxEscalations = 3
)

// Escalate chases unacknowledged verdicts. Delivery is always Planty's native
// push channel; Home Assistant is not a notification fallback or speaker path.
type Escalate struct {
	Store         *store.Store
	Log           *slog.Logger
	Notifications Notifier
}

func (e Escalate) Run(ctx context.Context) error {
	due, err := e.Store.Chaseable(ctx, ChaseAfter, MaxEscalations)
	if err != nil {
		return fmt.Errorf("chaseable: %w", err)
	}
	if len(due) == 0 {
		e.Log.Info("nothing to chase")
		return nil
	}

	away := e.away(ctx)
	if err := notify(ctx, e.Notifications, chaseTitle(due), chaseBody(due, away), map[string]any{
		"data": map[string]any{"tag": "planty-chase"},
	}); err != nil {
		return err
	}

	for _, entry := range due {
		if err := e.Store.RecordEscalation(ctx, entry.Verdict.ID); err != nil {
			e.Log.Error("could not record escalation", "verdict", entry.Verdict.ID, "error", err)
		}
	}

	e.Log.Warn("chased", "verdicts", len(due), "backup", away.BackupContact)
	return nil
}

func (e Escalate) away(ctx context.Context) plant.AwayPeriod {
	away, err := e.Store.AwayAt(ctx, time.Now())
	if errors.Is(err, store.ErrNotFound) || err != nil {
		return plant.AwayPeriod{}
	}
	return away
}

func chaseTitle(due []plant.DigestEntry) string {
	if len(due) == 1 {
		return "Still waiting: " + due[0].Plant.CommonName
	}
	return fmt.Sprintf("%d plants are still waiting", len(due))
}

func chaseBody(due []plant.DigestEntry, away plant.AwayPeriod) string {
	var b strings.Builder

	for _, entry := range due {
		fmt.Fprintf(&b, "%s: %s", entry.Plant.CommonName, entry.Verdict.Reasoning)
		if entry.Plant.IsFriends() {
			fmt.Fprintf(&b, " (%s's)", entry.Plant.Steward)
		}
		if entry.Plant.NeedsChasing() {
			b.WriteString(" Nothing waters this one but you.")
		}

		left := MaxEscalations - entry.Verdict.Escalations - 1
		switch {
		case left <= 0:
			b.WriteString(" This is the last time I ask.")
		case left == 1:
			b.WriteString(" I will ask once more.")
		}
		b.WriteString("\n")
	}

	if away.BackupContact != "" {
		fmt.Fprintf(&b, "\nYou are away until %s. %s is recorded as covering.",
			away.EndsAt.Format("Jan 2"), away.BackupContact)
	}
	return b.String()
}
