package store

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func wateringDetectionFixture(t *testing.T, s *Store, ctx context.Context) (plant.Plant, plant.SensorLink, plant.Verdict, time.Time) {
	t.Helper()
	p := newPlant(t, s, ctx, "Detected watering")
	now := time.Now().UTC().Truncate(time.Second)
	link, err := s.LinkSensor(ctx, plant.SensorLink{PlantID: &p.ID, HAEntityID: "sensor." + p.ID.String(), Role: plant.RoleSoilMoisture})
	if err != nil {
		t.Fatal(err)
	}
	link, err = s.Calibrate(ctx, link.ID, 30, 50)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE sensor_links SET assigned_at = $2, calibrated_at = $2 WHERE id = $1`, link.ID, now.Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	v, err := s.SaveVerdict(ctx, plant.Verdict{PlantID: p.ID, Action: plant.ActionWater, Confidence: 0.9, Reasoning: "Dry soil"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE verdicts SET created_at = $2 WHERE id = $1`, v.ID, now.Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	for n, value := range []float64{31, 31.1, 31, 40, 39} {
		at := now.Add(time.Duration(n-4) * 20 * time.Minute)
		if err := s.RecordReading(ctx, plant.Reading{SensorLinkID: link.ID, Value: value, Unit: "%", TakenAt: at, ReportedAt: &at}); err != nil {
			t.Fatal(err)
		}
	}
	return p, link, v, now
}

func TestDetectedWateringCompletesCareExactlyOnce(t *testing.T) {
	s, ctx := testStore(t)
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(previous); _ = provider.Shutdown(context.Background()) })
	p, _, v, now := wateringDetectionFixture(t, s, ctx)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := s.DetectWatering(ctx, now); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	detected := int64(0)
	for _, span := range recorder.Ended() {
		if span.Name() != "planty.watering.detect" {
			continue
		}
		for _, attr := range span.Attributes() {
			if attr.Key == "watering.detected" {
				detected += attr.Value.AsInt64()
			}
		}
	}
	if detected != 1 {
		t.Fatalf("telemetry reported %d detections", detected)
	}
	history, err := s.Observations(ctx, p.ID, 10)
	if err != nil || len(history) != 1 {
		t.Fatalf("history=%+v, err=%v", history, err)
	}
	o := history[0]
	if o.Kind != plant.ObservedWatered || o.Source != plant.SourceAutomation || o.Actor != "soil-moisture-detection" || !strings.Contains(o.Body, "Watering detected automatically") || !o.OccurredAt.Equal(now.Add(-20*time.Minute)) {
		t.Fatalf("unexpected observation: %+v", o)
	}
	var evidenceCount int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM watering_detections d
		JOIN readings b ON b.id = d.baseline_reading_id
		JOIN readings r ON r.id = d.rise_reading_id
		JOIN readings c ON c.id = d.confirmation_reading_id
		WHERE d.observation_id = $1 AND d.verdict_id = $2
		AND b.value = 31 AND r.value = 40 AND c.value = 39
		AND d.dry_baseline = 30 AND d.wet_baseline = 50`, o.ID, v.ID).Scan(&evidenceCount); err != nil || evidenceCount != 1 {
		t.Fatalf("evidence count=%d, err=%v", evidenceCount, err)
	}
	verdict, err := s.LatestVerdict(ctx, p.ID)
	if err != nil || verdict.ID != v.ID || verdict.AcknowledgedAt == nil {
		t.Fatalf("verdict=%+v, err=%v", verdict, err)
	}
	last, err := s.LastWatered(ctx, p.ID)
	if err != nil || !last.Equal(o.OccurredAt) {
		t.Fatalf("last watered=%v, err=%v", last, err)
	}
	digest, err := s.Digest(ctx, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range digest.Entries {
		if entry.Plant.ID == p.ID {
			t.Fatal("watering action remains in digest")
		}
	}
}

func TestDetectedWateringRacesReminderCompletion(t *testing.T) {
	s, ctx := testStore(t)
	for range 10 {
		p, _, _, now := wateringDetectionFixture(t, s, ctx)
		r, err := s.SaveReminder(ctx, plant.Reminder{PlantID: p.ID, Kind: plant.ObservedWatered, EveryDays: 7, AtHours: []int{now.Add(-time.Hour).Hour()}, Active: true})
		if err != nil {
			t.Fatal(err)
		}
		due, ok := r.LastSlot(nil, now)
		if !ok {
			t.Fatal("missing due slot")
		}
		var wg sync.WaitGroup
		wg.Go(func() {
			if _, err := s.DetectWatering(ctx, now); err != nil {
				t.Error(err)
			}
		})
		wg.Go(func() {
			_, err := s.CompleteReminder(ctx, ReminderCompletion{IdempotencyKey: uuid.New(), ReminderID: r.ID, DueAt: due})
			if err != nil && !errors.Is(err, plant.ErrInvalid) {
				t.Error(err)
			}
		})
		wg.Wait()
		history, err := s.Observations(ctx, p.ID, 10)
		if err != nil || len(history) != 1 {
			t.Fatalf("history=%+v, err=%v", history, err)
		}
	}
}

func TestDetectedWateringRollsBackObservationWhenCompletionFails(t *testing.T) {
	s, ctx := testStore(t)
	p, _, _, now := wateringDetectionFixture(t, s, ctx)
	_, err := s.pool.Exec(ctx, `CREATE FUNCTION reject_detected_ack() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'completion unavailable'; END $$;
		CREATE TRIGGER reject_detected_ack BEFORE UPDATE OF acknowledged_at ON verdicts
		FOR EACH ROW EXECUTE FUNCTION reject_detected_ack()`)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = s.pool.Exec(ctx, `DROP TRIGGER reject_detected_ack ON verdicts; DROP FUNCTION reject_detected_ack()`)
	})
	if _, err := s.DetectWatering(ctx, now); err == nil {
		t.Fatal("expected completion error")
	}
	history, err := s.Observations(ctx, p.ID, 10)
	if err != nil || len(history) != 0 {
		t.Fatalf("orphaned observation=%+v, err=%v", history, err)
	}
	v, err := s.LatestVerdict(ctx, p.ID)
	if err != nil || v.AcknowledgedAt != nil {
		t.Fatalf("verdict=%+v, err=%v", v, err)
	}
}

func TestDetectedWateringRejectsUnsafeEvidence(t *testing.T) {
	s, ctx := testStore(t)
	for _, scenario := range []string{"uncalibrated", "new assignment", "reassigned", "recalibrated", "stale", "cached", "noise", "spike", "not water", "already handled", "newer verdict", "manual care", "repotted", "archived", "dead", "letpot"} {
		t.Run(scenario, func(t *testing.T) {
			p, link, v, now := wateringDetectionFixture(t, s, ctx)
			var err error
			switch scenario {
			case "uncalibrated":
				_, err = s.pool.Exec(ctx, `UPDATE sensor_links SET wet_baseline = NULL WHERE id = $1`, link.ID)
			case "new assignment":
				_, err = s.pool.Exec(ctx, `UPDATE sensor_links SET assigned_at = $2 WHERE id = $1`, link.ID, now)
			case "reassigned":
				other := newPlant(t, s, ctx, "Other pot")
				_, err = s.AssignSensor(ctx, link.ID, plant.SensorAssignment{PlantID: &other.ID})
				if err == nil {
					_, err = s.AssignSensor(ctx, link.ID, plant.SensorAssignment{PlantID: &p.ID})
				}
			case "recalibrated":
				_, err = s.Calibrate(ctx, link.ID, 30, 50)
			case "stale":
				now = now.Add(2 * time.Hour)
			case "cached":
				_, err = s.pool.Exec(ctx, `UPDATE readings SET reported_at = $2 WHERE sensor_link_id = $1 AND taken_at = $3`, link.ID, now.Add(-20*time.Minute), now)
			case "noise":
				_, err = s.pool.Exec(ctx, `UPDATE readings SET value = 32 WHERE sensor_link_id = $1 AND value > 35`, link.ID)
			case "spike":
				_, err = s.pool.Exec(ctx, `UPDATE readings SET value = 31 WHERE sensor_link_id = $1 AND taken_at = $2`, link.ID, now)
			case "not water":
				_, err = s.pool.Exec(ctx, `UPDATE verdicts SET action = 'check' WHERE id = $1`, v.ID)
			case "already handled":
				err = s.AckVerdict(ctx, v.ID)
			case "newer verdict":
				_, err = s.pool.Exec(ctx, `UPDATE verdicts SET created_at = $2 WHERE id = $1`, v.ID, now)
			case "manual care", "repotted":
				kind := plant.ObservedWatered
				if scenario == "repotted" {
					kind = plant.ObservedRepotted
				}
				_, err = s.AddObservation(ctx, plant.Observation{PlantID: p.ID, Kind: kind, Source: plant.SourceApp, OccurredAt: now.Add(-time.Hour)})
			case "archived":
				err = s.ArchivePlant(ctx, p.Slug, plant.StatusRemoved)
			case "dead":
				_, err = s.pool.Exec(ctx, `UPDATE plants SET status = 'dead' WHERE id = $1`, p.ID)
			case "letpot":
				_, err = s.pool.Exec(ctx, `UPDATE plants SET watering_method = 'letpot' WHERE id = $1`, p.ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			if n, err := s.DetectWatering(ctx, now); err != nil || n != 0 {
				t.Fatalf("detected=%d, err=%v", n, err)
			}
		})
	}
}

func TestDetectedWateringRacesManualCompletion(t *testing.T) {
	s, ctx := testStore(t)
	for range 10 {
		p, _, v, now := wateringDetectionFixture(t, s, ctx)
		var wg sync.WaitGroup
		wg.Go(func() {
			if _, err := s.DetectWatering(ctx, now); err != nil {
				t.Error(err)
			}
		})
		wg.Go(func() {
			_, err := s.CompleteVerdict(ctx, VerdictCompletion{IdempotencyKey: uuid.New(), VerdictID: v.ID, Kind: plant.ObservedWatered})
			if err != nil && !errors.Is(err, plant.ErrInvalid) {
				t.Error(err)
			}
		})
		wg.Wait()
		history, err := s.Observations(ctx, p.ID, 10)
		if err != nil || len(history) != 1 {
			t.Fatalf("history=%+v, err=%v", history, err)
		}
	}
}
