package job

import (
	"strings"
	"testing"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

func TestColdMessageNamesEveryPlantAndItsOwner(t *testing.T) {
	plants := []plant.Plant{
		{CommonName: "Peace lilies", Location: "front porch", Steward: "Maya"},
		{CommonName: "Bonsai", Location: "front porch", Steward: plant.StewardSelf},
	}

	got := coldMessage(52, plants, plant.AwayPeriod{})
	for _, want := range []string{"52F", "Peace lilies", "Bonsai", "Maya's", "front porch"} {
		if !strings.Contains(got, want) {
			t.Errorf("cold message is missing %q:\n%s", want, got)
		}
	}
}

func TestColdMessageNamesTheBackupWhenAway(t *testing.T) {
	away := plant.AwayPeriod{BackupContact: "Sam next door"}
	got := coldMessage(50, []plant.Plant{{CommonName: "Fern"}}, away)

	if !strings.Contains(got, "Sam next door") {
		t.Errorf("an away trip must name who is covering:\n%s", got)
	}
}

// The whole point of the margin is that a porch is colder than the airport the
// forecast came from, so it must actually widen the net.
func TestColdMarginBiasesTowardTheCheapMistake(t *testing.T) {
	if ColdMarginF <= 0 {
		t.Fatal("the margin must widen the net, not narrow it")
	}
}

func TestThinKeepsTheEndsAndBoundsTheLength(t *testing.T) {
	readings := make([]plant.Reading, 100)
	for i := range readings {
		readings[i] = plant.Reading{Value: float64(i)}
	}

	got := thin(readings, 10)
	if len(got) != 10 {
		t.Fatalf("got %d samples, want 10", len(got))
	}
	if got[0].Value != 0 {
		t.Errorf("first sample should be the oldest reading, got %v", got[0].Value)
	}
	if got[len(got)-1].Value != 99 {
		t.Errorf("last sample should be the newest reading, got %v", got[len(got)-1].Value)
	}
}

func TestThinLeavesShortSeriesAlone(t *testing.T) {
	readings := []plant.Reading{{Value: 1}, {Value: 2}}
	if got := thin(readings, 30); len(got) != 2 {
		t.Fatalf("a short series should pass through untouched, got %d", len(got))
	}
}

func TestSeasonShapesAdvice(t *testing.T) {
	january := season(time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC))
	july := season(time.Date(2026, time.July, 15, 0, 0, 0, 0, time.UTC))

	if january == july {
		t.Fatal("winter and summer must not give the model the same context")
	}
	if !strings.Contains(january, "winter") || !strings.Contains(july, "summer") {
		t.Errorf("season strings should name the season: %q / %q", january, july)
	}
}

func TestHumanDaysReadsNaturally(t *testing.T) {
	for _, tc := range []struct {
		ago  time.Duration
		want string
	}{
		{2 * time.Hour, "since today"},
		{30 * time.Hour, "since yesterday"},
		{5 * 24 * time.Hour, "for 5 days"},
	} {
		if got := humanDays(time.Now().Add(-tc.ago)); got != tc.want {
			t.Errorf("%v ago: got %q want %q", tc.ago, got, tc.want)
		}
	}
}

func TestWateringWindowIsLongEnoughForWaterToRegister(t *testing.T) {
	// Soil sensors report on their own schedule, often only every 20 minutes,
	// so a window shorter than that would call every watering a failure.
	if WateringWindow < time.Hour {
		t.Fatalf("window %v is too short for a sensor to report a change", WateringWindow)
	}
}
