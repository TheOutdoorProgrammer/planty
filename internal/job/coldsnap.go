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

	at := c.Store
	plants, err := at.ColdWatch(ctx, low-ColdMarginF)
	if err != nil {
		return fmt.Errorf("cold watch query: %w", err)
	}
	if len(plants) == 0 {
		c.Log.Info("cold watch clear", "forecast_low_f", low)
		return nil
	}

	away, hasBackup := c.backup(ctx)
	message := coldMessage(low, plants, away)

	target := c.Notifier
	if hasBackup && away.BackupNotify != "" {
		target = away.BackupNotify
	}

	c.Log.Warn("cold watch triggered", "forecast_low_f", low, "plants", len(plants))
	return c.HA.Notify(ctx, target, "Bring the plants in", message, map[string]any{
		"data": map[string]any{"tag": "planty-cold", "importance": "high"},
	})
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
