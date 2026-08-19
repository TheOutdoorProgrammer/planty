package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/pgtest"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/google/uuid"
)

// Needs a real Postgres: the digest join once emitted invalid SQL that
// compiled fine. Set PLANTY_TEST_DATABASE_URL; CI always does.
func testStore(t *testing.T) (*Store, context.Context) {
	t.Helper()

	ctx := context.Background()
	s, err := Open(ctx, pgtest.DSN(t))
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

func TestANewerVerdictSupersedesTheOldInstruction(t *testing.T) {
	s, ctx := testStore(t)
	p := newPlant(t, s, ctx, "Superseded subject")
	old, err := s.SaveVerdict(ctx, plant.Verdict{
		PlantID: p.ID, ForDate: time.Now().Add(-24 * time.Hour),
		Action: plant.ActionWater, Reasoning: "was dry",
	})
	if err != nil {
		t.Fatal(err)
	}
	latest, err := s.SaveVerdict(ctx, plant.Verdict{
		PlantID: p.ID, ForDate: time.Now(),
		Action: plant.ActionNone, Reasoning: "fine now",
	})
	if err != nil {
		t.Fatal(err)
	}
	if old.ID == latest.ID {
		t.Fatal("different judgment days unexpectedly shared a row")
	}

	digest, err := s.Digest(ctx, plant.StaleAfter)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range digest.Entries {
		if entry.Plant.ID == p.ID {
			t.Fatal("the superseded watering instruction remained actionable")
		}
	}
	if err := s.AckVerdict(ctx, old.ID); err != nil {
		t.Fatal("the superseded verdict was not retained as history")
	}
}

func TestConversationCannotMoveBetweenSubjects(t *testing.T) {
	s, ctx := testStore(t)
	first := newPlant(t, s, ctx, "Conversation owner")
	second := newPlant(t, s, ctx, "Conversation intruder")

	started, err := s.SaveConsultTurn(ctx, ConsultTurn{
		PlantID: first.ID,
		Asked:   "What is happening?",
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.Consultation(ctx, started.ConversationID, second.ID); !errors.Is(err, ErrConversationOwner) {
		t.Fatalf("reading through another plant returned %v, want ErrConversationOwner", err)
	}
	if _, err := s.SaveConsultTurn(ctx, ConsultTurn{
		PlantID:        second.ID,
		ConversationID: started.ConversationID,
		Asked:          "Continue under the wrong plant",
	}); !errors.Is(err, ErrConversationOwner) {
		t.Fatalf("writing through another plant returned %v, want ErrConversationOwner", err)
	}
	if _, err := s.Consultation(ctx, started.ConversationID, first.ID); err != nil {
		t.Fatalf("the owning plant lost its conversation: %v", err)
	}
	if _, err := s.Consultation(ctx, started.ConversationID, uuid.Nil); !errors.Is(err, ErrConversationOwner) {
		t.Fatalf("scratch access returned %v, want ErrConversationOwner", err)
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

// Every command migrates on start, so a deployment and a CronJob landing
// together race on a fresh database. This reproduces that and expects the
// advisory lock to serialise them instead of one failing.
func TestConcurrentMigrationsDoNotRace(t *testing.T) {
	dsn := pgtest.DSN(t)
	ctx := context.Background()

	const racers = 6
	errs := make(chan error, racers)

	var start sync.WaitGroup
	start.Add(1)

	for range racers {
		go func() {
			s, err := Open(ctx, dsn)
			if err != nil {
				errs <- err
				return
			}
			defer s.Close()

			start.Wait()
			errs <- s.Migrate(ctx)
		}()
	}

	start.Done()
	for range racers {
		if err := <-errs; err != nil {
			t.Errorf("a concurrent migration failed: %v", err)
		}
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

// Photographing a second pothos has to work. Slugs are unique, so naming a
// plant after its species collides the moment somebody owns two.
func TestFreeSlugNumbersTheSecondOfASpecies(t *testing.T) {
	s, ctx := testStore(t)

	first, err := s.FreeSlug(ctx, "Golden Pothos")
	if err != nil {
		t.Fatalf("first slug: %v", err)
	}
	if first != "golden-pothos" {
		t.Errorf("the first one got %q, want the plain slug", first)
	}

	claim(t, s, ctx, first, "Golden Pothos")

	second, err := s.FreeSlug(ctx, "Golden Pothos")
	if err != nil {
		t.Fatalf("second slug: %v", err)
	}
	if second != "golden-pothos-2" {
		t.Errorf("the second one got %q, want golden-pothos-2", second)
	}

	claim(t, s, ctx, second, "Golden Pothos")

	third, err := s.FreeSlug(ctx, "golden pothos")
	if err != nil {
		t.Fatalf("third slug: %v", err)
	}
	if third != "golden-pothos-3" {
		t.Errorf("the third one got %q, want golden-pothos-3", third)
	}
}

func TestFreeSlugRefusesANameWithNothingInIt(t *testing.T) {
	s, ctx := testStore(t)

	if _, err := s.FreeSlug(ctx, "  !!!  "); err == nil {
		t.Error("a name with no usable characters produced a slug")
	}
}

func TestRemindersRoundTripTheirSchedule(t *testing.T) {
	s, ctx := testStore(t)
	kit := newPlant(t, s, ctx, "Blue oyster kit")

	saved, err := s.SaveReminder(ctx, plant.Reminder{
		PlantID: kit.ID, Kind: plant.ObservedMisted,
		EveryDays: 1, AtHours: []int{20, 8, 8}, Active: true, Note: "surface looks dry",
	})
	if err != nil {
		t.Fatalf("save reminder: %v", err)
	}
	if len(saved.AtHours) != 2 || saved.AtHours[0] != 8 || saved.AtHours[1] != 20 {
		t.Errorf("hours stored as %v, want a sorted deduped [8 20]", saved.AtHours)
	}

	// One per kind per plant: two misting schedules for one kit is a typo.
	again, err := s.SaveReminder(ctx, plant.Reminder{
		PlantID: kit.ID, Kind: plant.ObservedMisted,
		EveryDays: 1, AtHours: []int{6, 12, 18}, Active: true,
	})
	if err != nil {
		t.Fatalf("replace reminder: %v", err)
	}
	if again.ID != saved.ID {
		t.Error("saving the same kind twice made a second reminder")
	}

	listed, err := s.Reminders(ctx, kit.ID)
	if err != nil {
		t.Fatalf("list reminders: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("listed %d reminders, want 1", len(listed))
	}
	if len(listed[0].AtHours) != 3 {
		t.Errorf("the replacement schedule did not stick: %v", listed[0].AtHours)
	}

	if err := s.DeleteReminder(ctx, kit.ID, plant.ObservedMisted); err != nil {
		t.Fatalf("delete reminder: %v", err)
	}
	if err := s.DeleteReminder(ctx, kit.ID, plant.ObservedMisted); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleting a gone reminder returned %v", err)
	}
}

// The reminder job needs the plant and the last real action in one pass, and
// the lateral join that fetches them is exactly the kind of SQL that compiles
// as a string and fails on a server.
func TestActiveRemindersCarryTheLastTimeItWasDone(t *testing.T) {
	s, ctx := testStore(t)
	kit := newPlant(t, s, ctx, "Reminder subject")

	misted := time.Now().UTC().Add(-30 * time.Minute)
	if _, err := s.AddObservation(ctx, plant.Observation{
		PlantID: kit.ID, Kind: plant.ObservedMisted,
		OccurredAt: misted, Source: plant.SourceApp,
	}); err != nil {
		t.Fatalf("record misting: %v", err)
	}
	// A different kind must not satisfy a misting reminder.
	if _, err := s.AddObservation(ctx, plant.Observation{
		PlantID: kit.ID, Kind: plant.ObservedWatered,
		OccurredAt: time.Now().UTC(), Source: plant.SourceApp,
	}); err != nil {
		t.Fatalf("record watering: %v", err)
	}

	if _, err := s.SaveReminder(ctx, plant.Reminder{
		PlantID: kit.ID, Kind: plant.ObservedMisted,
		EveryDays: 1, AtHours: []int{8, 20}, Active: true,
	}); err != nil {
		t.Fatalf("save reminder: %v", err)
	}

	due, err := s.ActiveReminders(ctx)
	if err != nil {
		t.Fatalf("active reminders: %v", err)
	}

	var found *DueReminder
	for i := range due {
		if due[i].Plant.ID == kit.ID {
			found = &due[i]
		}
	}
	if found == nil {
		t.Fatal("the reminder never came back")
	}
	if found.Plant.CommonName != "Reminder subject" {
		t.Errorf("carried plant %q", found.Plant.CommonName)
	}
	if found.LastDone == nil {
		t.Fatal("no last-done time, so nothing can decide whether it is owed")
	}
	if delta := found.LastDone.Sub(misted); delta > time.Second || delta < -time.Second {
		t.Errorf("last done is %s, want the misting at %s", found.LastDone, misted)
	}
}

func TestAnInactiveReminderIsNotCollected(t *testing.T) {
	s, ctx := testStore(t)
	p := newPlant(t, s, ctx, "Switched off")

	if _, err := s.SaveReminder(ctx, plant.Reminder{
		PlantID: p.ID, Kind: plant.ObservedWatered,
		EveryDays: 3, AtHours: []int{8}, Active: false,
	}); err != nil {
		t.Fatalf("save reminder: %v", err)
	}

	due, err := s.ActiveReminders(ctx)
	if err != nil {
		t.Fatalf("active reminders: %v", err)
	}
	for _, d := range due {
		if d.Plant.ID == p.ID {
			t.Error("a switched-off reminder was collected")
		}
	}
}

// claim creates a plant holding an exact slug, which newPlant deliberately
// cannot do because it uniquifies every slug it is given.
func claim(t *testing.T, s *Store, ctx context.Context, slug, name string) {
	t.Helper()

	p, err := s.CreatePlant(ctx, plant.Plant{
		CommonName:     name,
		Slug:           slug,
		Domain:         plant.DomainHouseplant,
		Status:         plant.StatusAlive,
		Steward:        plant.StewardSelf,
		Accessibility:  plant.AccessEasy,
		WateringMethod: plant.WateringHand,
	})
	if err != nil {
		t.Fatalf("claim %s: %v", slug, err)
	}
	t.Cleanup(func() { _ = s.ArchivePlant(ctx, p.Slug, plant.StatusGone) })
}
