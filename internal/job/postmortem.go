package job

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/judge"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

// Postmortem works out what killed a plant and writes it down.
type Postmortem struct {
	Store *store.Store
	Judge *judge.Judge
	Log   *slog.Logger
}

// Sweep analyses every dead plant that has not been looked at yet.
func (p Postmortem) Sweep(ctx context.Context) (int, error) {
	pending, err := p.Store.DeadWithoutPostmortem(ctx)
	if err != nil {
		return 0, err
	}

	var written int
	for _, subject := range pending {
		if _, err := p.Run(ctx, subject.Slug); err != nil {
			// One failure must not block the rest, or a single bad record
			// silently stops every future autopsy.
			p.Log.Error("autopsy failed", "plant", subject.Slug, "error", err)
			continue
		}
		written++
	}
	return written, nil
}

// Run analyses one dead plant. Idempotent: an existing autopsy is left alone.
func (p Postmortem) Run(ctx context.Context, slug string) (plant.Postmortem, error) {
	subject, err := p.Store.GetPlant(ctx, slug)
	if err != nil {
		return plant.Postmortem{}, err
	}
	if subject.Status != plant.StatusDead {
		return plant.Postmortem{}, fmt.Errorf("%s is not dead", slug)
	}

	if existing, err := p.Store.Postmortem(ctx, subject.ID); err == nil {
		return existing, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return plant.Postmortem{}, err
	}

	// From the beginning: the fatal mistake is usually weeks upstream of the
	// symptom, so a death is read over the plant's whole life.
	history, err := Gather(ctx, p.Store, subject, subject.CreatedAt)
	if err != nil {
		return plant.Postmortem{}, err
	}

	autopsy, err := p.Judge.Postmortem(ctx, history)
	if err != nil {
		return plant.Postmortem{}, err
	}

	record := plant.Postmortem{
		PlantID:     subject.ID,
		LikelyCause: autopsy.LikelyCause,
		Narrative:   autopsy.Narrative,
		Lesson:      autopsy.Lesson,
	}
	if err := p.Store.SavePostmortem(ctx, record); err != nil {
		return plant.Postmortem{}, err
	}

	p.Log.Info("postmortem written", "plant", slug,
		"cause", autopsy.LikelyCause, "preventable", autopsy.Preventable)
	return record, nil
}

// Gather assembles what is known about a plant since a point in time. Shared
// by the autopsy and the consultation, which want the same picture over
// different windows: a death over a whole life, a question over weeks.
func Gather(ctx context.Context, db *store.Store, subject plant.Plant,
	since time.Time) (judge.History, error) {
	history := judge.History{Plant: subject}

	// Read for every plant, because "there is a cat here" changes the advice
	// for all of them and belongs to none of them.
	household, err := db.Notes(ctx, uuid.Nil)
	if err != nil {
		return history, err
	}
	history.Household = household

	observations, err := db.Observations(ctx, subject.ID, 100)
	if err != nil {
		return history, err
	}
	// Oldest first: a life reads as a sequence, not a stack.
	for _, o := range slices.Backward(observations) {
		if o.OccurredAt.Before(since) {
			continue
		}
		history.Observations = append(history.Observations, o)
	}

	links, err := db.SensorLinks(ctx, &subject.ID)
	if err != nil {
		return history, err
	}
	for _, link := range links {
		if link.Role != plant.RoleSoilMoisture || !link.Calibrated() {
			continue
		}
		readings, err := db.ReadingsSince(ctx, link.ID, since)
		if err != nil {
			return history, err
		}
		for _, r := range thin(readings, 30) {
			if f, err := link.Fraction(r.Value); err == nil {
				history.Readings = append(history.Readings,
					judge.Sample{TakenAt: r.TakenAt, Fraction: f})
			}
		}
	}

	photos, err := db.Photos(ctx, subject.ID, 30)
	if err == nil {
		for _, shot := range photos {
			if shot.TakenAt.Before(since) || shot.VisionFindings == "" {
				continue
			}
			history.PhotoNotes = append(history.PhotoNotes,
				strings.TrimSpace(shot.VisionFindings))
		}
	}
	return history, nil
}

// thin evenly samples a long reading series down to at most n points, keeping
// the shape of the trend without sending months of noise.
func thin(readings []plant.Reading, n int) []plant.Reading {
	if len(readings) <= n || n <= 0 {
		return readings
	}
	step := float64(len(readings)-1) / float64(n-1)

	out := make([]plant.Reading, 0, n)
	for i := range n {
		out = append(out, readings[int(float64(i)*step)])
	}
	return out
}
