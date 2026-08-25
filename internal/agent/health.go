package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/google/uuid"
)

func (d Deps) health(ctx context.Context, out io.Writer, args []string) error {
	set := newFlags("health")
	slug := set.String("plant", "", "the plant's slug")
	limit := set.Int("limit", 20, "how many health events to show")
	if err := parse(set, args); err != nil {
		return err
	}
	if strings.TrimSpace(*slug) == "" {
		return errors.New("--plant is required")
	}
	p, err := d.lookUp(ctx, *slug)
	if err != nil {
		return err
	}
	history, err := d.Store.HealthHistory(ctx, p.ID, *limit)
	if err != nil {
		return err
	}
	if len(history) == 0 {
		_, _ = fmt.Fprintf(out, "%s: health unknown; no evidence-backed baseline exists\n", p.CommonName)
		return nil
	}
	_, _ = fmt.Fprintf(out, "%s: %.0f%% health\n", p.CommonName, history[0].Score)
	for _, event := range history {
		change := "baseline"
		if event.AppliedDelta != nil {
			change = fmt.Sprintf("%+.1f requested, %+.1f applied", *event.RequestedDelta, *event.AppliedDelta)
		}
		who := string(event.Source)
		if event.Actor != "" {
			who += ", " + event.Actor
		}
		_, _ = fmt.Fprintf(out, "%s  %.1f%% (%s; %s): %s\n",
			event.CreatedAt.Format(stamp), event.Score, change, who, event.Rationale)
	}
	return nil
}

func (d Deps) healthChange(ctx context.Context, out io.Writer, args []string) error {
	set := newFlags("healthchange")
	slug := set.String("plant", "", "the plant's slug")
	baseline := set.String("baseline", "", "absolute starting score, 0 to 100")
	delta := set.String("delta", "", "signed change to the current score")
	reason := set.String("reason", "", "why the score is changing")
	evidence := set.String("evidence", "", "what observation or image supports it")
	photo := set.String("photo", "", "a supporting Planty photo id")
	observation := set.String("observation", "", "a supporting Planty observation id")
	reading := set.String("reading", "", "a supporting Planty sensor reading id")
	key := set.String("key", "", "idempotency UUID for this assessment")
	if err := parse(set, args); err != nil {
		return err
	}
	if strings.TrimSpace(*slug) == "" {
		return errors.New("--plant is required")
	}
	if (*baseline == "") == (*delta == "") {
		return errors.New("set exactly one of --baseline or --delta")
	}
	if strings.TrimSpace(*reason) == "" {
		return errors.New("--reason is required")
	}
	if strings.TrimSpace(*evidence) == "" {
		return errors.New("--evidence is required")
	}
	healthEvidence := plant.HealthEvidence{Summary: *evidence}
	for _, ref := range []struct {
		raw  string
		name string
		to   *[]uuid.UUID
	}{
		{*photo, "--photo", &healthEvidence.PhotoIDs},
		{*observation, "--observation", &healthEvidence.ObservationIDs},
		{*reading, "--reading", &healthEvidence.ReadingIDs},
	} {
		if strings.TrimSpace(ref.raw) == "" {
			continue
		}
		id, err := uuid.Parse(ref.raw)
		if err != nil {
			return fmt.Errorf("%s must be a Planty record UUID", ref.name)
		}
		*ref.to = append(*ref.to, id)
	}
	if !healthEvidence.HasReferences() {
		return errors.New("one of --photo, --observation, or --reading is required")
	}
	idempotency, err := uuid.Parse(*key)
	if err != nil {
		return errors.New("--key must be an idempotency UUID")
	}
	p, err := d.lookUp(ctx, *slug)
	if err != nil {
		return err
	}
	change := plant.HealthChange{
		PlantID: p.ID, Rationale: *reason,
		Evidence: healthEvidence,
		Source:   plant.SourceAgent, Actor: "planty agent", IdempotencyKey: &idempotency,
	}
	if *baseline != "" {
		value, err := strconv.ParseFloat(*baseline, 64)
		if err != nil {
			return fmt.Errorf("--baseline is not a number: %w", err)
		}
		change.Baseline = &value
	} else {
		value, err := strconv.ParseFloat(*delta, 64)
		if err != nil {
			return fmt.Errorf("--delta is not a number: %w", err)
		}
		change.Delta = &value
	}
	event, inserted, err := d.Store.RecordHealth(ctx, change)
	if err != nil {
		return err
	}
	verb := "recorded"
	if !inserted {
		verb = "already recorded"
	}
	_, _ = fmt.Fprintf(out, "%s health for %s at %.1f%%\n", verb, p.CommonName, event.Score)
	return nil
}
