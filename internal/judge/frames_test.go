package judge

import (
	"strings"
	"testing"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

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

func TestDescribeTrustsTemperatureWithoutSoilCalibration(t *testing.T) {
	got := describe(Evidence{
		Plant: plant.Plant{CommonName: "Fern", WateringMethod: plant.WateringHand},
		Sensors: []SensorState{{
			Role: plant.RoleAmbientTemp, Raw: 68.5, Unit: "°F", TakenAt: time.Now(),
		}},
	})

	if !strings.Contains(got, "ambient_temp: 68.5 °F") {
		t.Errorf("temperature is missing from the evidence:\n%s", got)
	}
	if strings.Contains(got, "NOT CALIBRATED") {
		t.Errorf("temperature was subjected to soil calibration:\n%s", got)
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

// A cat that chews things changes what the right advice is, not merely how it
// is worded, so it has to reach the text the model actually reads.
func TestTheHouseholdReachesTheRecord(t *testing.T) {
	h := History{
		Plant: plant.Plant{CommonName: "Golden Pothos", Slug: "golden-pothos"},
		Household: []plant.Note{
			{Title: "Cat", Body: "there is a cat indoors that chews leaves"},
			{Body: "nobody is home in August"},
		},
	}

	written := record(h, ongoing)

	if !strings.Contains(written, "chews leaves") {
		t.Error("a household note never reached the model's record")
	}
	if !strings.Contains(written, "Cat: there is a cat") {
		t.Error("the note's heading was dropped")
	}
	if !strings.Contains(written, "nobody is home in August") {
		t.Error("an untitled household note was dropped")
	}
}

// An empty section reading "true of this house:" followed by nothing is worse
// than no section at all.
func TestNoHouseholdNotesMeansNoSection(t *testing.T) {
	h := History{Plant: plant.Plant{CommonName: "Fern", Slug: "fern"}}
	if strings.Contains(record(h, ongoing), "True of this house") {
		t.Error("an empty household section was written")
	}
}

// The subject line differs between the two chats, and only that line.
func TestTheSubjectLineNamesWhatIsBeingTalkedAbout(t *testing.T) {
	one := aboutOnePlant("golden-pothos")
	if !strings.Contains(one, `"golden-pothos"`) {
		t.Errorf("the plant was not named: %q", one)
	}
	if strings.Contains(aboutNothingYet, "slug") {
		t.Error("a chat with no plant was given a slug to act on")
	}
	// The person deciding to buy it is exactly when creating one is right.
	if !strings.Contains(aboutNothingYet, "create") {
		t.Error("there is no way to keep a plant the person decides to buy")
	}
}
