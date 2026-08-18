package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

// Needs a real Postgres: the digest join once emitted invalid SQL that
// compiled fine. Set PLANTY_TEST_DATABASE_URL; CI always does.
func testStore(t *testing.T) (*Store, context.Context) {
	t.Helper()

	dsn := os.Getenv("PLANTY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set PLANTY_TEST_DATABASE_URL to run store integration tests")
	}

	ctx := context.Background()
	s, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(s.Close)

	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return s, ctx
}

func newPlant(t *testing.T, s *Store, ctx context.Context, name string) plant.Plant {
	t.Helper()

	p, err := s.CreatePlant(ctx, plant.Plant{
		CommonName:     name,
		Slug:           Slugify(name) + "-" + time.Now().Format("150405.000000"),
		Domain:         plant.DomainHouseplant,
		Status:         plant.StatusAlive,
		Steward:        plant.StewardSelf,
		Accessibility:  plant.AccessEasy,
		WateringMethod: plant.WateringHand,
	})
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	t.Cleanup(func() { _ = s.ArchivePlant(ctx, p.Slug, plant.StatusGone) })
	return p
}

// The regression test for the bug that shipped: the digest joins plants to
// verdicts, and the aliased column list has to be valid SQL.
func TestDigestJoinProducesValidSQL(t *testing.T) {
	s, ctx := testStore(t)

	p := newPlant(t, s, ctx, "Digest subject")
	if _, err := s.SaveVerdict(ctx, plant.Verdict{
		PlantID:   p.ID,
		ForDate:   time.Now().UTC(),
		Action:    plant.ActionWater,
		Reasoning: "soil is dry",
	}); err != nil {
		t.Fatalf("save verdict: %v", err)
	}

	digest, err := s.Digest(ctx, plant.StaleAfter)
	if err != nil {
		t.Fatalf("digest: %v", err)
	}

	var found bool
	for _, entry := range digest.Entries {
		if entry.Plant.ID == p.ID {
			found = true
			if entry.Verdict.Reasoning != "soil is dry" {
				t.Errorf("verdict did not survive the join: %q", entry.Verdict.Reasoning)
			}
			if entry.Plant.CommonName != "Digest subject" {
				t.Errorf("plant columns misaligned in the join: %q", entry.Plant.CommonName)
			}
		}
	}
	if !found {
		t.Error("an unacknowledged, actionable verdict should appear in the digest")
	}
}

func TestAcknowledgedVerdictsLeaveTheDigest(t *testing.T) {
	s, ctx := testStore(t)

	p := newPlant(t, s, ctx, "Ack subject")
	v, err := s.SaveVerdict(ctx, plant.Verdict{
		PlantID: p.ID, ForDate: time.Now().UTC(),
		Action: plant.ActionWater, Reasoning: "dry",
	})
	if err != nil {
		t.Fatalf("save verdict: %v", err)
	}
	if err := s.AckVerdict(ctx, v.ID); err != nil {
		t.Fatalf("ack: %v", err)
	}

	digest, err := s.Digest(ctx, plant.StaleAfter)
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	for _, entry := range digest.Entries {
		if entry.Plant.ID == p.ID {
			t.Fatal("an acknowledged verdict must not keep nagging")
		}
	}
}

// The constraint exists so bad data cannot enter through the app or an agent.
func TestDatabaseRejectsDripperOnHandWateredPlant(t *testing.T) {
	s, ctx := testStore(t)

	dripper := 3
	_, err := s.CreatePlant(ctx, plant.Plant{
		CommonName:     "Constraint subject",
		Domain:         plant.DomainHouseplant,
		Status:         plant.StatusAlive,
		Accessibility:  plant.AccessEasy,
		WateringMethod: plant.WateringHand,
		LetPotDripper:  &dripper,
	})
	if err == nil {
		t.Fatal("the database must reject a dripper number on a hand-watered plant")
	}
}

func TestColdWatchFindsPlantsAtOrAboveTheForecast(t *testing.T) {
	s, ctx := testStore(t)

	tender := newPlant(t, s, ctx, "Tender")
	threshold := 55.0
	if _, err := s.UpdatePlant(ctx, tender.Slug, PlantPatch{MinTempF: &threshold}); err != nil {
		t.Fatalf("set threshold: %v", err)
	}

	atRisk, err := s.ColdWatch(ctx, 50)
	if err != nil {
		t.Fatalf("cold watch: %v", err)
	}
	var found bool
	for _, p := range atRisk {
		if p.ID == tender.ID {
			found = true
		}
	}
	if !found {
		t.Error("a plant needing 55F should be at risk when the forecast is 50F")
	}

	safe, err := s.ColdWatch(ctx, 70)
	if err != nil {
		t.Fatalf("cold watch: %v", err)
	}
	for _, p := range safe {
		if p.ID == tender.ID {
			t.Error("a plant needing 55F should not be at risk when the forecast is 70F")
		}
	}
}

func TestSparsePatchLeavesUnnamedFieldsAlone(t *testing.T) {
	s, ctx := testStore(t)

	p := newPlant(t, s, ctx, "Patch subject")
	hard := plant.AccessHard
	updated, err := s.UpdatePlant(ctx, p.Slug, PlantPatch{Accessibility: &hard})
	if err != nil {
		t.Fatalf("patch: %v", err)
	}

	if updated.Accessibility != plant.AccessHard {
		t.Errorf("patch did not apply: %v", updated.Accessibility)
	}
	if updated.CommonName != p.CommonName {
		t.Errorf("patch clobbered an unnamed field: %q", updated.CommonName)
	}
	if updated.Domain != p.Domain {
		t.Errorf("patch clobbered the domain: %q", updated.Domain)
	}
}

func TestShelterRoundTrip(t *testing.T) {
	s, ctx := testStore(t)

	p := newPlant(t, s, ctx, "Shelter subject")
	if n, err := s.Shelter(ctx, []string{p.Slug}); err != nil || n != 1 {
		t.Fatalf("shelter: n=%d err=%v", n, err)
	}

	sheltered, since, err := s.Sheltered(ctx)
	if err != nil {
		t.Fatalf("sheltered: %v", err)
	}
	if since.IsZero() {
		t.Error("sheltered set should report when the first plant came in")
	}
	var found bool
	for _, got := range sheltered {
		if got.ID == p.ID {
			found = true
			if got.ShelteredAt == nil {
				t.Error("sheltered_at should be readable on the plant record")
			}
		}
	}
	if !found {
		t.Fatal("a sheltered plant should be listed")
	}

	if _, err := s.Unshelter(ctx, []string{p.Slug}); err != nil {
		t.Fatalf("unshelter: %v", err)
	}
}

func TestChaseableRespectsTheCapAndTheCooldown(t *testing.T) {
	s, ctx := testStore(t)

	p := newPlant(t, s, ctx, "Chase subject")
	v, err := s.SaveVerdict(ctx, plant.Verdict{
		PlantID: p.ID, ForDate: time.Now().UTC(),
		Action: plant.ActionWater, Reasoning: "dry",
	})
	if err != nil {
		t.Fatalf("save verdict: %v", err)
	}

	// Just written, so the cooldown has not elapsed.
	fresh, err := s.Chaseable(ctx, time.Hour, 3)
	if err != nil {
		t.Fatalf("chaseable: %v", err)
	}
	for _, entry := range fresh {
		if entry.Plant.ID == p.ID {
			t.Fatal("a verdict written seconds ago must not be chased yet")
		}
	}

	// A zero cooldown makes it due immediately.
	due, err := s.Chaseable(ctx, 0, 3)
	if err != nil {
		t.Fatalf("chaseable: %v", err)
	}
	var found bool
	for _, entry := range due {
		if entry.Plant.ID == p.ID {
			found = true
			if entry.Verdict.Escalations != 0 {
				t.Errorf("escalations should start at zero, got %d", entry.Verdict.Escalations)
			}
		}
	}
	if !found {
		t.Fatal("an overdue unacknowledged verdict should be chaseable")
	}

	for range 3 {
		if err := s.RecordEscalation(ctx, v.ID); err != nil {
			t.Fatalf("record escalation: %v", err)
		}
	}

	capped, err := s.Chaseable(ctx, 0, 3)
	if err != nil {
		t.Fatalf("chaseable: %v", err)
	}
	for _, entry := range capped {
		if entry.Plant.ID == p.ID {
			t.Fatal("the ladder must stop at the cap rather than nagging forever")
		}
	}
}

// Re-judging a plant is a new ask, so the ladder starts over.
func TestResavingAVerdictResetsTheLadder(t *testing.T) {
	s, ctx := testStore(t)

	p := newPlant(t, s, ctx, "Reset subject")
	today := time.Now().UTC()
	v, err := s.SaveVerdict(ctx, plant.Verdict{
		PlantID: p.ID, ForDate: today, Action: plant.ActionWater, Reasoning: "dry",
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := s.RecordEscalation(ctx, v.ID); err != nil {
		t.Fatalf("escalate: %v", err)
	}

	if _, err := s.SaveVerdict(ctx, plant.Verdict{
		PlantID: p.ID, ForDate: today, Action: plant.ActionUrgent, Reasoning: "worse",
	}); err != nil {
		t.Fatalf("resave: %v", err)
	}

	due, err := s.Chaseable(ctx, 0, 3)
	if err != nil {
		t.Fatalf("chaseable: %v", err)
	}
	for _, entry := range due {
		if entry.Plant.ID == p.ID && entry.Verdict.Escalations != 0 {
			t.Errorf("a re-judged plant should start the ladder over, got %d",
				entry.Verdict.Escalations)
		}
	}
}

func TestMoistureRoseAfterDetectsWaterThatNeverArrived(t *testing.T) {
	s, ctx := testStore(t)

	p := newPlant(t, s, ctx, "Hydrophobic subject")
	dry, wet := 20.0, 60.0
	link, err := s.LinkSensor(ctx, plant.SensorLink{
		PlantID:    &p.ID,
		HAEntityID: "sensor.test_" + p.Slug,
		Role:       plant.RoleSoilMoisture,
	})
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	if _, err := s.Calibrate(ctx, link.ID, dry, wet); err != nil {
		t.Fatalf("calibrate: %v", err)
	}

	watered := time.Now().UTC().Add(-2 * time.Hour)
	for _, r := range []struct {
		at    time.Time
		value float64
	}{
		{watered.Add(-30 * time.Minute), 22},
		{watered.Add(30 * time.Minute), 22}, // water ran straight through
	} {
		if err := s.RecordReading(ctx, plant.Reading{
			SensorLinkID: link.ID, Value: r.value, TakenAt: r.at,
		}); err != nil {
			t.Fatalf("record reading: %v", err)
		}
	}

	rose, err := s.MoistureRoseAfter(ctx, link.ID, watered, 3*time.Hour)
	if err != nil {
		t.Fatalf("moisture check: %v", err)
	}
	if rose {
		t.Error("moisture did not actually rise; the claim should not verify")
	}
}
