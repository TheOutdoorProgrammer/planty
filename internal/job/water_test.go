package job

import (
	"context"
	"fmt"
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

func TestStaleReadingsCannotDriveWatering(t *testing.T) {
	reading := plant.Reading{TakenAt: time.Now().Add(-MaxWateringReadingAge - time.Second)}
	if freshForWatering(reading, time.Now()) {
		t.Fatal("a stale reading was allowed to drive the pump")
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

// Nothing waters a plant on a timer, so the reporting half has to work on its
// own: no pump configured, no API key, nothing but probes.
func TestThirstReportsWithoutAPumpOrAKey(t *testing.T) {
	s, ctx := testStore(t)
	onTheLine(t, s, ctx, "Tomato")

	f := newFakeHA(t, weatherEntity)
	report := Thirst{Store: s, HA: f.client(), Log: quietLog(), Notifier: "notify"}
	if err := report.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// No calibrated probe means nothing can be said, which is silence rather
	// than a guess that the plant is fine.
	if len(f.notified) != 0 {
		t.Errorf("said %v about a plant with no calibrated probe", f.notified)
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

func TestAStaleDryReadingDoesNotTurnOnThePump(t *testing.T) {
	s, ctx := testStore(t)
	p := onTheLine(t, s, ctx, "Stale tomato")
	link, err := s.LinkSensor(ctx, plant.SensorLink{
		PlantID: &p.ID, HAEntityID: "sensor.stale_tomato", Role: plant.RoleSoilMoisture,
	})
	if err != nil {
		t.Fatal(err)
	}
	link, err = s.Calibrate(ctx, link.ID, 10, 50)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordReading(ctx, plant.Reading{
		SensorLinkID: link.ID,
		Value:        10,
		TakenAt:      time.Now().Add(-MaxWateringReadingAge - time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	f := newFakeHA(t, weatherEntity)
	err = (Water{
		Store: s, HA: f.client(), Log: quietLog(), PumpSwitch: "switch.letpot",
	}).Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.services) != 0 {
		t.Fatalf("stale evidence called Home Assistant services: %v", f.services)
	}
}

func TestAnyCalibratedProbeCanConfirmWaterArrived(t *testing.T) {
	s, ctx := testStore(t)
	p := onTheLine(t, s, ctx, "Two probe tomato")
	started := time.Now().Add(-time.Hour)

	for index, readings := range [][]float64{{20, 20}, {20, 30}} {
		link, err := s.LinkSensor(ctx, plant.SensorLink{
			PlantID:    &p.ID,
			HAEntityID: fmt.Sprintf("sensor.two_probe_%d_%d", index, time.Now().UnixNano()),
			Role:       plant.RoleSoilMoisture,
		})
		if err != nil {
			t.Fatal(err)
		}
		link, err = s.Calibrate(ctx, link.ID, 10, 50)
		if err != nil {
			t.Fatal(err)
		}
		for offset, value := range readings {
			if err := s.RecordReading(ctx, plant.Reading{
				SensorLinkID: link.ID,
				Value:        value,
				TakenAt:      started.Add(time.Duration(offset*10-1) * time.Minute),
			}); err != nil {
				t.Fatal(err)
			}
		}
	}

	rose, err := (Ingest{Store: s}).VerifyWatering(ctx, p, started)
	if err != nil {
		t.Fatal(err)
	}
	if !rose {
		t.Fatal("the first flat probe hid the second probe's confirmed rise")
	}
}
