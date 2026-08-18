package job

import (
	"context"
	"testing"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

// The gap between the two is the whole safety margin: inside it, doing nothing
// is the right answer, and a system with no dead band oscillates.
func TestThresholdsLeaveADeadBand(t *testing.T) {
	if Thirsty >= Soaked {
		t.Fatalf("dry (%v) must sit below wet (%v)", Thirsty, Soaked)
	}
	if Soaked-Thirsty < 0.2 {
		t.Errorf("dead band of %.2f is too narrow to stop oscillation", Soaked-Thirsty)
	}
}

// Overwatering kills more houseplants than drought, so the dry threshold has to
// sit well below halfway; watering at 45% would be watering damp soil.
func TestDryThresholdBiasesAgainstOverwatering(t *testing.T) {
	if Thirsty > 0.35 {
		t.Errorf("dry threshold %.2f waters soil that is not actually dry", Thirsty)
	}
}

func TestSettleWindowOutlastsASensorReportingInterval(t *testing.T) {
	// Zigbee soil sensors commonly report every 20 minutes; checking sooner
	// would call a perfectly good watering a failure.
	if SettleWindow.Minutes() < 30 {
		t.Errorf("settle window %v is shorter than a sensor reporting cycle", SettleWindow)
	}
}

// Planty with no API key still has to run: the cold watch and the watering
// line are what keep plants alive and neither needs a model. Failing here
// would fail the digest at eight every morning, forever.
func TestNoJudgeIsAQuietDayNotAFailure(t *testing.T) {
	s, ctx := testStore(t)
	tender(t, s, ctx, "Keyless", plant.StewardSelf, 55)

	if err := (Daily{Store: s, Log: quietLog()}).Run(ctx); err != nil {
		t.Fatalf("a Planty with no key must still run: %v", err)
	}
}

// onTheLine creates a plant the LetPot pump is responsible for.
func onTheLine(t *testing.T, s *store.Store, ctx context.Context, name string) plant.Plant {
	t.Helper()

	p, err := s.CreatePlant(ctx, plant.Plant{
		CommonName:     name,
		Slug:           store.Slugify(name) + "-" + time.Now().Format("150405.000000000"),
		Domain:         plant.DomainEdibleIndoor,
		Status:         plant.StatusAlive,
		Steward:        plant.StewardSelf,
		Location:       "greenhouse cabinet",
		Accessibility:  plant.AccessEasy,
		WateringMethod: plant.WateringLetPot,
	})
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	t.Cleanup(func() { _ = s.ArchivePlant(ctx, p.Slug, plant.StatusGone) })
	return p
}

// A garden that waters by hand is not a broken pump. Erroring on it would fail
// the hourly job forever on a system that has no LetPot line at all, and a
// failure that fires every hour is one nobody reads.
func TestNothingOnTheLineIsNotAFault(t *testing.T) {
	s, ctx := testStore(t)

	if err := (Water{Store: s, Log: quietLog()}).Run(ctx); err != nil {
		t.Fatalf("a garden with no LetPot plants has nothing to do, not a fault: %v", err)
	}
}

// The opposite case has to stay loud: this is watering silently not happening.
func TestPlantsOnTheLineWithNoPumpIsAFault(t *testing.T) {
	s, ctx := testStore(t)
	onTheLine(t, s, ctx, "Tomato")

	if err := (Water{Store: s, Log: quietLog()}).Run(ctx); err == nil {
		t.Fatal("plants on the line with no pump configured must be an error")
	}
}
