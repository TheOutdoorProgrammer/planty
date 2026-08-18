package job

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

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

	history, err := p.gather(ctx, subject)
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

func (p Postmortem) gather(ctx context.Context, subject plant.Plant) (judge.History, error) {
	history := judge.History{Plant: subject}

	observations, err := p.Store.Observations(ctx, subject.ID, 100)
	if err != nil {
		return history, err
	}
	// Oldest first: a death reads as a sequence, not a stack.
	for _, o := range slices.Backward(observations) {
		history.Observations = append(history.Observations, o)
	}

	links, err := p.Store.SensorLinks(ctx, &subject.ID)
	if err != nil {
		return history, err
	}
	for _, link := range links {
		if link.Role != plant.RoleSoilMoisture || !link.Calibrated() {
			continue
		}
		readings, err := p.Store.ReadingsSince(ctx, link.ID, subject.CreatedAt)
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

	photos, err := p.Store.Photos(ctx, subject.ID, 30)
	if err == nil {
		for _, shot := range photos {
			if shot.VisionFindings != "" {
				history.PhotoNotes = append(history.PhotoNotes,
					strings.TrimSpace(shot.VisionFindings))
			}
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
