package job

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

func TestThresholdsLeaveADeadBand(t *testing.T) {
	if Thirsty >= Soaked {
		t.Fatalf("dry (%v) must sit below wet (%v)", Thirsty, Soaked)
	}
	if Soaked-Thirsty < 0.2 {
		t.Errorf("dead band of %.2f is too narrow to stop oscillation", Soaked-Thirsty)
	}
}

func calibratedMoisture(t *testing.T, s *store.Store, ctx context.Context, p plant.Plant, entity string, at time.Time, value float64) plant.SensorLink {
	t.Helper()
	link, err := s.LinkSensor(ctx, plant.SensorLink{PlantID: &p.ID, HAEntityID: entity, Role: plant.RoleSoilMoisture})
	if err != nil {
		t.Fatal(err)
	}
	link, err = s.Calibrate(ctx, link.ID, 10, 50)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordReading(ctx, plant.Reading{SensorLinkID: link.ID, Value: value, TakenAt: at}); err != nil {
		t.Fatal(err)
	}
	return link
}

func TestManualWateringIsCreditedOnlyAfterEvidence(t *testing.T) {
	s, ctx := testStore(t)
	p := onTheLine(t, s, ctx, "Verified tomato")
	link := calibratedMoisture(t, s, ctx, p, "sensor.verified_tomato", time.Now().Add(-time.Minute), 10)
	f := newFakeHA(t, weatherEntity)

	if err := (Water{
		Store: s, HA: f.client(), Log: quietLog(), Notifications: f,
		PumpSwitch: "switch.letpot", RunFor: time.Millisecond,
	}).Run(ctx); err != nil {
		t.Fatalf("manual water: %v", err)
	}
	if len(f.services) != 2 || !strings.HasSuffix(f.services[0], "/turn_on") || !strings.HasSuffix(f.services[1], "/turn_off") {
		t.Fatalf("pump calls = %v, want one start followed by one stop", f.services)
	}
	if _, err := s.LastWatered(ctx, p.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unverified pump run was recorded as watered: %v", err)
	}

	attempts, err := s.WateringAttemptsReadyForEvidence(ctx, time.Now().Add(time.Hour))
	if err != nil || len(attempts) != 1 {
		t.Fatalf("durable attempt = %+v, %v", attempts, err)
	}
	started := *attempts[0].PumpStartedAt
	if err := s.RecordReading(ctx, plant.Reading{SensorLinkID: link.ID, Value: 30, TakenAt: started.Add(10 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if err := (VerifyWater{
		Store: s, Log: quietLog(), Notifications: f, SettleAfter: time.Nanosecond,
		Now: func() time.Time { return started.Add(time.Hour) },
	}).Run(ctx); err != nil {
		t.Fatalf("verify water: %v", err)
	}
	wateredAt, err := s.LastWatered(ctx, p.ID)
	if err != nil {
		t.Fatalf("verified watering missing: %v", err)
	}
	if !wateredAt.Equal(started) {
		t.Fatalf("watered at %v, want physical start %v", wateredAt, started)
	}
}

func TestWateringAlertsDistinguishAClogFromUnknownSensors(t *testing.T) {
	s, ctx := testStore(t)
	clogged := onTheLine(t, s, ctx, "Clogged basil")
	unknown := onTheLine(t, s, ctx, "Silent mint")
	started := time.Now().Add(-time.Hour).UTC()
	cloggedLink := calibratedMoisture(t, s, ctx, clogged, "sensor.clogged_basil", started.Add(-time.Minute), 20)
	_ = calibratedMoisture(t, s, ctx, unknown, "sensor.silent_mint", started.Add(-time.Minute), 20)
	if err := s.RecordReading(ctx, plant.Reading{SensorLinkID: cloggedLink.ID, Value: 20, TakenAt: started.Add(10 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	attempt, err := s.CreateWateringAttempt(ctx, "switch.letpot", "", time.Minute, []plant.Plant{clogged, unknown})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MarkWateringStarted(ctx, attempt.ID, started, store.PumpActivityConfirmed); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkWateringStopped(ctx, attempt.ID, started.Add(time.Minute), nil); err != nil {
		t.Fatal(err)
	}
	f := newFakeHA(t, weatherEntity)
	if err := (VerifyWater{
		Store: s, Log: quietLog(), Notifications: f, SettleAfter: time.Nanosecond,
		Now: func() time.Time { return started.Add(2 * time.Hour) },
	}).Run(ctx); err != nil {
		t.Fatal(err)
	}
	if len(f.notified) != 1 {
		t.Fatalf("notifications = %+v", f.notified)
	}
	message := f.notified[0].message
	for _, want := range []string{"Clogged basil", "blocked dripper", "Silent mint", "sensor readings"} {
		if !strings.Contains(message, want) {
			t.Errorf("watering alert missing %q: %s", want, message)
		}
	}
	if _, err := s.LastWatered(ctx, clogged.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("flat moisture was falsely recorded as watered")
	}
}

func TestDryThresholdBiasesAgainstOverwatering(t *testing.T) {
	if Thirsty > 0.35 {
		t.Errorf("dry threshold %.2f waters soil that is not actually dry", Thirsty)
	}
}

func TestSettleWindowOutlastsASensorReportingInterval(t *testing.T) {
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

func TestPumpDoesNotStartWithoutAPositiveDuration(t *testing.T) {
	f := newFakeHA(t, weatherEntity)
	err := (Water{
		HA: f.client(), Log: quietLog(), PumpSwitch: "switch.letpot",
	}).runLine(context.Background(), []plant.Plant{{CommonName: "Tomato"}})
	if err == nil {
		t.Fatal("an unset run duration was accepted")
	}
	if len(f.services) != 0 {
		t.Fatalf("invalid configuration called Home Assistant services: %v", f.services)
	}
}

func TestNoJudgeIsAQuietDayNotAFailure(t *testing.T) {
	s, ctx := testStore(t)
	tender(t, s, ctx, "Keyless", plant.StewardSelf, 55)

	if err := (Daily{Store: s, Log: quietLog()}).Run(ctx); err != nil {
		t.Fatalf("a Planty with no key must still run: %v", err)
	}
}

func TestThirstReportsWithoutAPumpOrAKey(t *testing.T) {
	s, ctx := testStore(t)
	onTheLine(t, s, ctx, "Tomato")

	f := newFakeHA(t, weatherEntity)
	report := Thirst{Store: s, Log: quietLog(), Notifications: f}
	if err := report.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(f.notified) != 0 {
		t.Errorf("said %v about a plant with no calibrated probe", f.notified)
	}
	if len(f.haNotifications) != 0 {
		t.Fatalf("thirst routed through Home Assistant notifications: %v", f.haNotifications)
	}
}

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

func TestNothingOnTheLineIsNotAFault(t *testing.T) {
	s, ctx := testStore(t)

	if err := (Water{Store: s, Log: quietLog()}).Run(ctx); err != nil {
		t.Fatalf("a garden with no LetPot plants has nothing to do, not a fault: %v", err)
	}
}

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
