package judge

import (
	"strings"
	"testing"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

func openingBlocks(question string) []Part {
	return []Part{
		text("preamble about the plant"),
		text("Taken 3 days ago:"),
		picture("image/jpeg", []byte("AAAA")),
		text("Taken 1 hour ago:"),
		picture("image/jpeg", []byte("BBBB")),
		text(question),
	}
}

func turns(n int) []PriorTurn {
	out := make([]PriorTurn, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, PriorTurn{
			Asked: "question " + string(rune('0'+i)),
			Reply: Diagnosis{Severity: SeverityFinding, Observed: "answer " + string(rune('0'+i))},
		})
	}
	return out
}

func textOf(m Turn) string {
	var b strings.Builder
	for _, part := range m.Parts {
		if part.Image == nil {
			b.WriteString(part.Text)
			b.WriteString(" ")
		}
	}
	return b.String()
}

func images(m Turn) int {
	n := 0
	for _, part := range m.Parts {
		if part.Image != nil {
			n++
		}
	}
	return n
}

// The API rejects two messages in a row from the same role, so a replay that
// ever doubles up makes every follow-up diagnosis fail outright.
func TestReplayAlwaysAlternates(t *testing.T) {
	for _, count := range []int{1, 2, 3, 6} {
		messages := replay(turns(count), openingBlocks("the newest question"), "the newest question")

		if messages[0].Role != RoleUser {
			t.Errorf("%d turns: a conversation has to open with the user", count)
		}
		last := messages[len(messages)-1]
		if last.Role != RoleUser {
			t.Errorf("%d turns: the model has to be answering something", count)
		}
		for i := 1; i < len(messages); i++ {
			if messages[i].Role == messages[i-1].Role {
				t.Fatalf("%d turns: %s twice in a row at %d", count, messages[i].Role, i)
			}
		}
	}
}

// The photographs are expensive and only need sending once; every later turn
// refers back to them.
func TestReplaySendsTheImagesOnlyOnce(t *testing.T) {
	messages := replay(turns(3), openingBlocks("newest"), "newest")

	if got := images(messages[0]); got != 2 {
		t.Errorf("the opening turn carries %d images, want 2", got)
	}
	for i, m := range messages[1:] {
		if got := images(m); got != 0 {
			t.Errorf("message %d re-sends %d images", i+1, got)
		}
	}
}

// The opening turn is built carrying whatever is being asked now, so replay has
// to swap that for the question the conversation actually started with.
func TestReplayRestoresTheOriginalQuestion(t *testing.T) {
	messages := replay(turns(2), openingBlocks("newest question"), "newest question")

	opening := textOf(messages[0])
	if !strings.Contains(opening, "question 1") {
		t.Errorf("the opening turn lost the question it started with:\n%s", opening)
	}
	if strings.Contains(opening, "newest question") {
		t.Errorf("the opening turn still carries today's question:\n%s", opening)
	}
	if !strings.Contains(opening, "preamble about the plant") {
		t.Error("the opening turn dropped the preamble")
	}
}

// Each answer has to sit against the question it answered, or the model reads
// the whole conversation shifted by one.
func TestReplayPairsAnswersWithTheirQuestions(t *testing.T) {
	messages := replay(turns(3), openingBlocks("newest"), "newest")

	want := []string{"question 1", "answer 1", "question 2", "answer 2", "question 3", "answer 3", "newest"}
	if len(messages) != len(want) {
		t.Fatalf("got %d messages, want %d", len(messages), len(want))
	}
	for i, fragment := range want {
		if got := textOf(messages[i]); !strings.Contains(got, fragment) {
			t.Errorf("message %d is %q, want it to carry %q", i, got, fragment)
		}
	}
}

func TestDescribeCarriesWhatTheOwnerSaid(t *testing.T) {
	said := "Very delicate. They need very little water."
	drainage := false

	got := describe(Evidence{
		Plant: plant.Plant{
			CommonName:     "Peace lily",
			Steward:        "Marcus",
			Location:       "front porch",
			Accessibility:  plant.AccessHard,
			WateringMethod: plant.WateringHand,
			HasDrainage:    &drainage,
			CareProfile:    plant.CareProfile{OwnerSays: said},
		},
	})

	for _, want := range []string{
		said,
		"Marcus",
		"NO drainage hole",
		"nothing happens unless a person does it",
		"this plant has no sensor",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the description drops %q:\n%s", want, got)
		}
	}
}

// An uncalibrated probe is not evidence, and the model has to be told that
// rather than handed a number it will reason about.
func TestDescribeFlagsUncalibratedProbes(t *testing.T) {
	got := describe(Evidence{
		Plant: plant.Plant{CommonName: "Fern", WateringMethod: plant.WateringHand},
		Sensors: []SensorState{{
			Role: plant.RoleSoilMoisture, Raw: 41, Calibrated: false, TakenAt: time.Now(),
		}},
	})

	if !strings.Contains(got, "NOT CALIBRATED") {
		t.Errorf("an uncalibrated reading is presented as evidence:\n%s", got)
	}
}

func TestDescribeSaysWhenNothingWasEverWatered(t *testing.T) {
	got := describe(Evidence{
		Plant: plant.Plant{CommonName: "New arrival", WateringMethod: plant.WateringHand},
	})
	if !strings.Contains(got, "No watering has ever been recorded") {
		t.Errorf("a plant with no history should say so:\n%s", got)
	}
}

// Indoors a tomato sets no fruit without agitation, and the plant looks
// perfectly healthy the whole time it yields nothing.
func TestDescribeRaisesPollination(t *testing.T) {
	needs := true
	got := describe(Evidence{
		Plant: plant.Plant{
			CommonName: "Sungold", Domain: plant.DomainEdibleIndoor,
			WateringMethod: plant.WateringLetPot,
			CareProfile:    plant.CareProfile{NeedsPollination: &needs},
		},
	})
	if !strings.Contains(got, "pollination") {
		t.Errorf("a plant that needs pollination should say so:\n%s", got)
	}
}

// A pot with no drainage hole is the most common way a plant drowns, and it
// used to be reported only when somebody had also recorded the material.
func TestDrainageIsReportedWhateverElseIsKnown(t *testing.T) {
	no, yes := false, true
	eight := 8.0

	for _, tc := range []struct {
		name   string
		pot    plant.Plant
		want   string
		absent string
	}{
		{
			name: "nothing recorded but the missing hole",
			pot:  plant.Plant{HasDrainage: &no},
			want: "NO drainage hole",
		},
		{
			name: "material as well",
			pot:  plant.Plant{PotMaterial: "ceramic", HasDrainage: &no},
			want: "ceramic, with NO drainage hole",
		},
		{
			name: "size and material",
			pot:  plant.Plant{PotSizeIn: &eight, PotMaterial: "terracotta", HasDrainage: &no},
			want: "8 inch terracotta, with NO drainage hole",
		},
		{
			name:   "a pot that drains is not worth warning about",
			pot:    plant.Plant{PotMaterial: "plastic", HasDrainage: &yes},
			want:   "plastic",
			absent: "NO drainage",
		},
		{
			name:   "nothing known about the pot at all",
			pot:    plant.Plant{},
			absent: "Pot",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := describePot(tc.pot)
			if tc.want != "" && !strings.Contains(got, tc.want) {
				t.Errorf("got %q, want it to carry %q", got, tc.want)
			}
			if tc.absent != "" && strings.Contains(got, tc.absent) {
				t.Errorf("got %q, which should not mention %q", got, tc.absent)
			}
		})
	}
}

// The autopsy reads the same description, and a missing drainage hole is
// exactly the cause it exists to identify.
func TestNarrateCarriesTheMissingDrainageHole(t *testing.T) {
	no := false
	got := narrate(History{Plant: plant.Plant{
		CommonName: "Drowned", HasDrainage: &no, WateringMethod: plant.WateringHand,
	}})

	if !strings.Contains(got, "NO drainage hole") {
		t.Errorf("the autopsy drops the likeliest cause:\n%s", got)
	}
}

func TestAgoReadsNaturally(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		since time.Duration
		want  string
	}{
		{30 * time.Minute, "minutes"},
		{5 * time.Hour, "hours"},
		{9 * 24 * time.Hour, "days"},
	} {
		if got := ago(now.Add(-tc.since)); !strings.Contains(got, tc.want) {
			t.Errorf("%v ago rendered as %q, want %q", tc.since, got, tc.want)
		}
	}
}

func TestTimelineIsCappedAtTheNewestFrames(t *testing.T) {
	if MaxTimelineImages < 2 {
		t.Fatal("a comparison needs at least two frames")
	}
	if MaxTimelineImages > 10 {
		t.Error("too many frames and the oldest is no longer the same plant in the same pot")
	}
}

// A consultation narrated in the postmortem's voice tells the model the plant
// is dead, and it then answers a question about a living plant as an autopsy.
func TestALivingPlantIsNotNarratedAsDead(t *testing.T) {
	h := History{Plant: plant.Plant{
		CommonName:  "Golden pothos",
		CareProfile: plant.CareProfile{OwnerSays: "it likes being ignored"},
	}}

	living := record(h, ongoing)
	if strings.Contains(living, "is dead") {
		t.Errorf("a living plant's record says it is dead:\n%s", living)
	}
	if !strings.Contains(living, "The owner says") {
		t.Errorf("a living plant's record uses the past tense:\n%s", living)
	}
	if !strings.Contains(living, "What has been done to it") {
		t.Errorf("a living plant's record closes its own story:\n%s", living)
	}

	dead := narrate(h)
	if !strings.Contains(dead, "is dead") {
		t.Errorf("an autopsy no longer says the plant died:\n%s", dead)
	}
	if !strings.Contains(dead, "The owner had said") {
		t.Errorf("an autopsy lost its past tense:\n%s", dead)
	}
}
