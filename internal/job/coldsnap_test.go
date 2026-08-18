package job

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

const weatherEntity = "weather.nws_home"

func testStore(t *testing.T) (*store.Store, context.Context) {
	t.Helper()

	dsn := os.Getenv("PLANTY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set PLANTY_TEST_DATABASE_URL to run job tests")
	}

	ctx := context.Background()
	s, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(s.Close)

	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return s, ctx
}

// tender creates a plant that has to come in below the given temperature.
func tender(t *testing.T, s *store.Store, ctx context.Context, name, steward string, minTempF float64) plant.Plant {
	t.Helper()

	p, err := s.CreatePlant(ctx, plant.Plant{
		CommonName:     name,
		Slug:           store.Slugify(name) + "-" + time.Now().Format("150405.000000000"),
		Domain:         plant.DomainHouseplant,
		Status:         plant.StatusAlive,
		Steward:        steward,
		Location:       "front porch",
		Accessibility:  plant.AccessEasy,
		WateringMethod: plant.WateringHand,
		MinTempF:       &minTempF,
	})
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	t.Cleanup(func() { _ = s.ArchivePlant(ctx, p.Slug, plant.StatusGone) })
	return p
}

func coldWatch(s *store.Store, f *fakeHA) ColdWatch {
	return ColdWatch{
		Store: s, HA: f.client(), Log: quietLog(),
		Weather: weatherEntity, Notifier: "notify",
	}
}

// The whole point of the job: a cold night has to reach the person who can
// carry the pots indoors, and it has to name them.
func TestAColdNightNamesThePlantsAndTheirOwner(t *testing.T) {
	s, ctx := testStore(t)
	p := tender(t, s, ctx, "Peace lily", "Marcus", 55)

	f := newFakeHA(t, weatherEntity)
	f.forecast(70, 48)

	if err := coldWatch(s, f).Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(f.notified) == 0 {
		t.Fatal("a 48F night with a 55F plant outside said nothing")
	}
	if !f.said(p.CommonName) {
		t.Errorf("the warning does not name the plant:\n%+v", f.notified)
	}
	if !f.said("Marcus") {
		t.Errorf("the warning does not say whose it is:\n%+v", f.notified)
	}
	if !f.said("48") {
		t.Errorf("the warning does not give the forecast:\n%+v", f.notified)
	}
}

// The margin exists because a porch at 3am is colder than the airport the
// forecast came from, so a night at the threshold still warns.
func TestTheMarginWarnsBeforeTheThresholdIsReached(t *testing.T) {
	s, ctx := testStore(t)
	tender(t, s, ctx, "Marginal", plant.StewardSelf, 55)

	f := newFakeHA(t, weatherEntity)
	f.forecast(70, 57) // above 55, but inside the 3F margin

	if err := coldWatch(s, f).Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(f.notified) == 0 {
		t.Error("57F with a 55F plant is inside the margin and should still warn")
	}
}

func TestAWarmNightSaysNothing(t *testing.T) {
	s, ctx := testStore(t)
	tender(t, s, ctx, "Comfortable", plant.StewardSelf, 55)

	f := newFakeHA(t, weatherEntity)
	f.forecast(80, 68)

	if err := coldWatch(s, f).Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, n := range f.notified {
		if n.title == "Bring the plants in" {
			t.Errorf("a 68F night warned about a 55F plant: %+v", n)
		}
	}
}

// A plant already indoors must not be warned about again, or the job repeats
// itself every afternoon until the weather turns.
func TestAShelteredPlantIsNotWarnedAboutTwice(t *testing.T) {
	s, ctx := testStore(t)
	p := tender(t, s, ctx, "Already inside", plant.StewardSelf, 55)

	if _, err := s.Shelter(ctx, []string{p.Slug}); err != nil {
		t.Fatalf("shelter: %v", err)
	}

	f := newFakeHA(t, weatherEntity)
	f.forecast(70, 45)

	if err := coldWatch(s, f).Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if f.said("Already inside") {
		t.Errorf("warned about a plant that is already indoors:\n%+v", f.notified)
	}
}

// The half nobody builds. Left in a dark room for a week is its own way of
// killing them, and nothing else would ever say so.
func TestPlantsAreToldToGoBackOutOnceItIsWarm(t *testing.T) {
	s, ctx := testStore(t)
	p := tender(t, s, ctx, "Wants out", plant.StewardSelf, 55)

	if _, err := s.Shelter(ctx, []string{p.Slug}); err != nil {
		t.Fatalf("shelter: %v", err)
	}

	f := newFakeHA(t, weatherEntity)
	f.forecast(78, 66) // clears 55 plus the margin

	if err := coldWatch(s, f).Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !f.said("Put the plants back out") && !f.said("can go back out") {
		t.Errorf("nothing told him the plants could go back outside:\n%+v", f.notified)
	}
	if !f.said(p.CommonName) {
		t.Errorf("the message does not name the plant:\n%+v", f.notified)
	}
}

// Every sheltered plant has to clear its own threshold, not just the hardiest,
// or the tender one goes back out into a night it cannot survive.
func TestTheTenderestShelteredPlantGatesGoingBackOut(t *testing.T) {
	s, ctx := testStore(t)
	hardy := tender(t, s, ctx, "Hardy one", plant.StewardSelf, 40)
	fragile := tender(t, s, ctx, "Fragile one", plant.StewardSelf, 60)

	if _, err := s.Shelter(ctx, []string{hardy.Slug, fragile.Slug}); err != nil {
		t.Fatalf("shelter: %v", err)
	}

	f := newFakeHA(t, weatherEntity)
	f.forecast(70, 55) // fine for the hardy one, too cold for the fragile one

	if err := coldWatch(s, f).Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if f.said("can go back out") {
		t.Errorf("sent them out at 55F with a 60F plant among them:\n%+v", f.notified)
	}
}

// While away, the warning has to reach whoever is actually there. Nagging a
// phone nobody is holding protects nothing.
func TestWhileAwayTheWarningGoesToTheBackup(t *testing.T) {
	s, ctx := testStore(t)
	tender(t, s, ctx, "Left behind", "Marcus", 55)

	now := time.Now().UTC()
	if _, err := s.GoAway(ctx, plant.AwayPeriod{
		StartsAt:      now.Add(-24 * time.Hour),
		EndsAt:        now.Add(72 * time.Hour),
		BackupContact: "Sam next door",
		BackupNotify:  "mobile_app_sam",
	}); err != nil {
		t.Fatalf("go away: %v", err)
	}

	f := newFakeHA(t, weatherEntity)
	f.forecast(70, 44)

	if err := coldWatch(s, f).Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(f.notified) == 0 {
		t.Fatal("nothing was sent")
	}
	if got := f.notified[0].service; got != "mobile_app_sam" {
		t.Errorf("sent to %q, want the backup contact's device", got)
	}
	if !f.said("Sam next door") {
		t.Errorf("the message should say who is covering:\n%+v", f.notified)
	}
}

// A plant with no threshold recorded is not a plant that tolerates anything.
func TestAPlantWithNoThresholdIsNotWarnedAbout(t *testing.T) {
	s, ctx := testStore(t)

	p, err := s.CreatePlant(ctx, plant.Plant{
		CommonName:     "No threshold",
		Slug:           "no-threshold-" + time.Now().Format("150405.000000000"),
		Domain:         plant.DomainHouseplant,
		Status:         plant.StatusAlive,
		Steward:        plant.StewardSelf,
		Accessibility:  plant.AccessEasy,
		WateringMethod: plant.WateringHand,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _ = s.ArchivePlant(ctx, p.Slug, plant.StatusGone) })

	f := newFakeHA(t, weatherEntity)
	f.forecast(50, 20)

	if err := coldWatch(s, f).Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if f.said("No threshold") {
		t.Error("warned about a plant whose tolerance nobody recorded")
	}
}

// A forecast that cannot be read is an error, not a quiet decision to do
// nothing on the night it matters.
func TestAnUnreadableForecastIsAnError(t *testing.T) {
	s, ctx := testStore(t)
	tender(t, s, ctx, "At risk", plant.StewardSelf, 55)

	f := newFakeHA(t, weatherEntity)
	f.periods = nil

	if err := coldWatch(s, f).Run(ctx); err == nil {
		t.Fatal("an empty forecast passed silently")
	}
}
