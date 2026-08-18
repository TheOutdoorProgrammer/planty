// Package job holds the scheduled work: sensor ingest, the daily judgment run,
// and the cold snap watch.
package job

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/ha"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

// ColdMarginF pads the threshold: a porch at 3am beats the airport forecast.
const ColdMarginF = 3

// ColdWatch decides which plants need bringing indoors tonight.
type ColdWatch struct {
	Store    *store.Store
	HA       *ha.Client
	Log      *slog.Logger
	Weather  string
	Notifier string
}

// Run reads the forecast, not the thermometer: once it is cold, it is too late.
func (c ColdWatch) Run(ctx context.Context) error {
	low, err := c.HA.TonightLow(ctx, c.Weather, 18*time.Hour)
	if err != nil {
		return fmt.Errorf("forecast: %w", err)
	}

	plants, err := c.Store.ColdWatch(ctx, low-ColdMarginF)
	if err != nil {
		return fmt.Errorf("cold watch query: %w", err)
	}

	var atRisk []plant.Plant
	for _, p := range plants {
		if p.ShelteredAt == nil {
			atRisk = append(atRisk, p)
		}
	}

	if len(atRisk) > 0 {
		return c.warn(ctx, low, atRisk)
	}
	c.Log.Info("cold watch clear", "forecast_low_f", low)
	return c.putBackOut(ctx, low)
}

func (c ColdWatch) warn(ctx context.Context, low float64, atRisk []plant.Plant) error {
	away, hasBackup := c.backup(ctx)

	target := c.Notifier
	if hasBackup && away.BackupNotify != "" {
		target = away.BackupNotify
	}

	c.Log.Warn("cold watch triggered", "forecast_low_f", low, "plants", len(atRisk))
	return c.HA.Notify(ctx, target, "Bring the plants in",
		coldMessage(low, atRisk, away), map[string]any{
			"data": map[string]any{"tag": "planty-cold", "importance": "high"},
		})
}

// putBackOut names plants stuck indoors once the weather has actually turned.
func (c ColdWatch) putBackOut(ctx context.Context, low float64) error {
	sheltered, since, err := c.Store.Sheltered(ctx)
	if err != nil || len(sheltered) == 0 {
		return err
	}

	// Every sheltered plant must clear its own threshold, not just the hardiest.
	needed := 0.0
	for _, p := range sheltered {
		if p.MinTempF != nil && *p.MinTempF > needed {
			needed = *p.MinTempF
		}
	}
	if low < needed+ColdMarginF {
		c.Log.Info("still too cold to put them back out", "forecast_low_f", low)
		return nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Tonight only drops to %.0fF. These have been inside %s and can go back out:\n",
		low, humanDays(since))
	for _, p := range sheltered {
		fmt.Fprintf(&b, "\n- %s", p.CommonName)
	}

	c.Log.Info("safe to put plants back out", "plants", len(sheltered))
	return c.HA.Notify(ctx, c.Notifier, "Put the plants back out", b.String(), nil)
}

func humanDays(since time.Time) string {
	days := int(time.Since(since).Hours() / 24)
	switch {
	case days < 1:
		return "since today"
	case days == 1:
		return "since yesterday"
	default:
		return fmt.Sprintf("for %d days", days)
	}
}

// backup returns the away period covering tonight, if there is one.
func (c ColdWatch) backup(ctx context.Context) (plant.AwayPeriod, bool) {
	away, err := c.Store.AwayAt(ctx, time.Now())
	if err != nil {
		return plant.AwayPeriod{}, false
	}
	return away, true
}

func coldMessage(low float64, plants []plant.Plant, away plant.AwayPeriod) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Tonight's low is %.0fF. Bring these in before dark:\n", low)

	for _, p := range plants {
		fmt.Fprintf(&b, "\n- %s", p.CommonName)
		if p.Location != "" {
			fmt.Fprintf(&b, " (%s)", p.Location)
		}
		if p.IsFriends() {
			fmt.Fprintf(&b, " - %s's", p.Steward)
		}
	}
	if away.BackupContact != "" {
		fmt.Fprintf(&b, "\n\nJoey is away. %s is covering.", away.BackupContact)
	}
	return b.String()
}

// WarmEnough gates putting them back out; indoors for a week also kills them.
func (c ColdWatch) WarmEnough(ctx context.Context, minHighF float64) (bool, error) {
	periods, err := c.HA.Forecast(ctx, c.Weather)
	if err != nil {
		return false, err
	}
	if len(periods) == 0 {
		return false, fmt.Errorf("no forecast periods")
	}

	// Both today's high and tonight's low have to clear, or they go straight
	// back out into the next cold night.
	today := periods[0]
	if today.Temperature < minHighF {
		return false, nil
	}
	return today.Low() >= minHighF-ColdMarginF, nil
}
