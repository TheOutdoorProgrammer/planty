package job

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/ha"
	"github.com/TheOutdoorProgrammer/planty/internal/judge"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

// Daily produces one verdict per plant, then a single digest notification.
type Daily struct {
	Store    *store.Store
	HA       *ha.Client
	Judge    *judge.Judge
	Log      *slog.Logger
	Notifier string
}

// Run judges every live plant and notifies only if something needs doing or the
// check was incomplete. The garden-wide run is persisted before model work so a
// process interruption cannot leave yesterday's all-clear looking current.
func (d Daily) Run(ctx context.Context) error {
	plants, err := d.Store.ListPlants(ctx, store.PlantFilter{Status: plant.StatusAlive})
	if err != nil {
		return fmt.Errorf("list plants: %w", err)
	}

	run, err := d.Store.StartJudgmentRun(ctx, len(plants))
	if err != nil {
		return fmt.Errorf("start judgment run: %w", err)
	}

	// No key is a Planty running without its opinion, not a broken cold-watch or
	// watering system. Still record that this daily check covered zero plants so
	// Today cannot inherit a false all-clear from an older successful run.
	if d.Judge == nil {
		for range plants {
			if err := d.Store.RecordJudgmentResult(ctx, run.ID, false); err != nil {
				return fmt.Errorf("record unavailable judgment: %w", err)
			}
		}
		if err := d.Store.CompleteJudgmentRun(ctx, run.ID); err != nil {
			return fmt.Errorf("complete unavailable judgment run: %w", err)
		}
		d.Log.Warn("no judgment: no model backend is configured",
			"plants", len(plants), "still_running", "cold watch, watering, escalation")
		return nil
	}

	var failed int
	for _, p := range plants {
		judged := true
		if err := d.judgeOne(ctx, p); err != nil {
			// One plant failing must not silence the rest of the digest.
			d.Log.Error("judgment failed", "plant", p.Slug, "error", err)
			failed++
			judged = false
		}
		if err := d.Store.RecordJudgmentResult(ctx, run.ID, judged); err != nil {
			return fmt.Errorf("record judgment result for %s: %w", p.Slug, err)
		}
	}
	if err := d.Store.CompleteJudgmentRun(ctx, run.ID); err != nil {
		return fmt.Errorf("complete judgment run: %w", err)
	}

	// A death is worth understanding, and nobody remembers to ask for it.
	if written, err := (Postmortem{Store: d.Store, Judge: d.Judge, Log: d.Log}).
		Sweep(ctx); err != nil {
		d.Log.Error("postmortem sweep failed", "error", err)
	} else if written > 0 {
		d.Log.Info("postmortems written", "count", written)
	}

	digest, err := d.Store.ReliableDigest(ctx, plant.StaleAfter)
	if err != nil {
		return fmt.Errorf("digest: %w", err)
	}

	if failed == len(plants) && len(plants) > 0 {
		return fmt.Errorf("every plant failed judgment (%d)", failed)
	}
	if digest.AllClear() {
		d.Log.Info("nothing to do", "checked", digest.Checked)
		return nil
	}
	return d.notify(ctx, digest)
}

func (d Daily) judgeOne(ctx context.Context, p plant.Plant) error {
	evidence, err := d.gather(ctx, p)
	if err != nil {
		return err
	}

	result, err := d.Judge.Assess(ctx, evidence)
	if err != nil {
		return err
	}

	_, err = d.Store.SaveVerdict(ctx, plant.Verdict{
		PlantID:    p.ID,
		ForDate:    time.Now().UTC(),
		Action:     result.Action,
		Reasoning:  result.Reasoning,
		Confidence: result.Confidence,
		Evidence:   plant.Evidence{SensorSummary: result.Summary, ModelVersion: judge.Model},
	})
	return err
}

func (d Daily) gather(ctx context.Context, p plant.Plant) (judge.Evidence, error) {
	evidence := judge.Evidence{Plant: p, Season: season(time.Now())}

	links, err := d.Store.SensorLinks(ctx, &p.ID)
	if err != nil {
		return evidence, err
	}
	for _, link := range links {
		reading, err := d.Store.LatestReading(ctx, link.ID)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return evidence, err
		}

		state := judge.SensorState{
			Role:       link.Role,
			Raw:        reading.Value,
			Calibrated: link.Calibrated(),
			TakenAt:    reading.TakenAt,
		}
		if state.Calibrated {
			if f, err := link.Fraction(reading.Value); err == nil {
				state.Fraction = &f
			}
		}
		evidence.Sensors = append(evidence.Sensors, state)
	}

	if recent, err := d.Store.Observations(ctx, p.ID, 8); err == nil {
		evidence.Recent = recent
	}
	if at, err := d.Store.LastWatered(ctx, p.ID); err == nil {
		evidence.LastWatered = &at
	}
	return evidence, nil
}

func (d Daily) notify(ctx context.Context, digest plant.Digest) error {
	var b strings.Builder
	urgent := false

	for _, entry := range digest.Entries {
		fmt.Fprintf(&b, "%s: %s\n", entry.Plant.CommonName, entry.Verdict.Reasoning)
		if entry.Verdict.Action == plant.ActionUrgent {
			urgent = true
		}
	}
	incomplete := !digest.RunComplete || digest.Failed > 0 || digest.Checked != digest.Expected
	if incomplete {
		fmt.Fprintf(&b, "\nWarning: Planty checked %d of %d plants; %d failed. This check is incomplete.\n",
			digest.Checked, digest.Expected, digest.Failed)
	}
	if digest.StaleSince != nil {
		b.WriteString("\nWarning: the last complete check is stale.\n")
	}

	title := fmt.Sprintf("%d plants need you", len(digest.Entries))
	switch {
	case incomplete && len(digest.Entries) == 0:
		title = "Planty's check was incomplete"
	case len(digest.Entries) == 1:
		title = "One plant needs you"
	}

	if err := d.HA.Notify(ctx, d.Notifier, title, b.String(), nil); err != nil {
		return err
	}
	// Speakers are reserved for real risk; anything less trains him to tune out.
	if urgent {
		return d.HA.Announce(ctx, "A plant needs attention. Check Planty.")
	}
	return nil
}

// season shapes advice: the same soil reading means different things in winter.
func season(t time.Time) string {
	switch t.Month() {
	case time.December, time.January, time.February:
		return "winter, so growth is slow and water needs are lower"
	case time.March, time.April, time.May:
		return "spring, the start of active growth"
	case time.June, time.July, time.August:
		return "summer, peak growth and fastest drying"
	default:
		return "autumn, growth slowing down"
	}
}
