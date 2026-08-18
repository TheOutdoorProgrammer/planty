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

const (
	// ChaseAfter is how long a verdict is left alone between chases.
	ChaseAfter = 6 * time.Hour

	// MaxEscalations caps the ladder. Past this Planty stops asking, because a
	// notification that never stops is one you stop reading.
	MaxEscalations = 3

	// SpeakRisk is the neglect score at which the house speakers are allowed.
	// Reached by a hand-watered plant that belongs to someone else.
	SpeakRisk = 4
)

// Escalate chases unacknowledged verdicts, hand-watered plants hardest: a line
// fails rarely, a hand-watered plant fails every time its owner is busy.
type Escalate struct {
	Store    *store.Store
	HA       *ha.Client
	Log      *slog.Logger
	Notifier string
}

// Run chases everything due, one rung up the ladder.
func (e Escalate) Run(ctx context.Context) error {
	due, err := e.Store.Chaseable(ctx, ChaseAfter, MaxEscalations)
	if err != nil {
		return fmt.Errorf("chaseable: %w", err)
	}
	if len(due) == 0 {
		e.Log.Info("nothing to chase")
		return nil
	}

	target, away := e.target(ctx)

	var speak []plant.DigestEntry
	for _, entry := range due {
		if e.shouldSpeak(entry) {
			speak = append(speak, entry)
		}
	}

	if err := e.HA.Notify(ctx, target, chaseTitle(due), chaseBody(due, away), map[string]any{
		"data": map[string]any{"tag": "planty-chase"},
	}); err != nil {
		return err
	}

	// Recorded even when the notify succeeded but the announce did not, so a
	// speaker failure cannot restart the ladder from the bottom.
	for _, entry := range due {
		if err := e.Store.RecordEscalation(ctx, entry.Verdict.ID); err != nil {
			e.Log.Error("could not record escalation", "verdict", entry.Verdict.ID, "error", err)
		}
	}

	e.Log.Warn("chased", "verdicts", len(due), "spoken", len(speak),
		"backup", away.BackupContact)

	if len(speak) == 0 {
		return nil
	}
	return e.HA.Announce(ctx, announcement(speak))
}

// shouldSpeak reserves the speakers for the last rung, and only for a plant
// whose neglect actually costs something.
func (e Escalate) shouldSpeak(entry plant.DigestEntry) bool {
	last := entry.Verdict.Escalations >= MaxEscalations-1
	serious := entry.Risk >= SpeakRisk || entry.Verdict.Action == plant.ActionUrgent
	return last && serious
}

func (e Escalate) target(ctx context.Context) (string, plant.AwayPeriod) {
	away, err := e.Store.AwayAt(ctx, time.Now())
	if errors.Is(err, store.ErrNotFound) || err != nil {
		return e.Notifier, plant.AwayPeriod{}
	}
	if away.BackupNotify != "" {
		return away.BackupNotify, away
	}
	return e.Notifier, away
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
		fmt.Fprintf(&b, "\nJoey is away until %s. %s is covering.",
			away.EndsAt.Format("Jan 2"), away.BackupContact)
	}
	return b.String()
}

func announcement(speak []plant.DigestEntry) string {
	if len(speak) == 1 {
		p := speak[0].Plant
		if p.IsFriends() {
			return fmt.Sprintf("%s's %s still needs water and nobody has done it.",
				p.Steward, p.CommonName)
		}
		return fmt.Sprintf("The %s still needs water.", p.CommonName)
	}
	return fmt.Sprintf("%d plants still need attention. Check Planty.", len(speak))
}
