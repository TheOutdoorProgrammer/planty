package plant

import (
	"encoding/json"
	"testing"
	"time"
)

// The whole feature turns on this one distinction. A plant nobody has looked
// up must never be presentable as a plant somebody cleared.
func TestNobodyCheckedIsNotTheSameAsSafe(t *testing.T) {
	var untouched Toxicity

	if untouched.Checked() {
		t.Error("an untouched record claims somebody checked it")
	}
	if untouched.Cats.Known() {
		t.Error("an unset rating claims to be known")
	}
	if rank(HarmUnknown) <= rank(HarmSafe) {
		t.Error("unknown sorts at or below safe, which buries the plants that need looking up")
	}
	if untouched.Worst() != HarmUnknown {
		t.Errorf("the worst of nothing is %q, not unknown", untouched.Worst())
	}
}

// The zero value has to survive validation, or every plant created without a
// toxicity lookup is rejected by the store.
func TestAnUnratedPlantIsValid(t *testing.T) {
	if err := (Toxicity{}).Valid(); err != nil {
		t.Fatalf("an unrated plant was refused: %v", err)
	}
}

// The rule that stops the field filling with confident guesses.
func TestCallingSomethingDangerousRequiresSayingWhy(t *testing.T) {
	bare := Toxicity{Cats: HarmSevere, Dogs: HarmSafe, People: HarmSafe, Basis: BasisDerived}
	if err := bare.Valid(); err == nil {
		t.Error("a severe rating was accepted with no toxic principle behind it")
	}

	bare.Principle = "unidentified nephrotoxin"
	if err := bare.Valid(); err != nil {
		t.Errorf("a justified rating was refused: %v", err)
	}
}

// Mild does not need a mechanism: most of a collection is oxalate irritation
// and demanding a citation for "it stings" would just stop the field getting
// filled in at all.
func TestAMildRatingNeedsNoPrinciple(t *testing.T) {
	mild := Toxicity{Cats: HarmMild, Dogs: HarmMild, People: HarmMild, Basis: BasisSource}
	if err := mild.Valid(); err != nil {
		t.Errorf("a mild rating was refused for lacking a principle: %v", err)
	}
}

// The ASPCA publishes toxic or non-toxic and nothing between, so a rating
// finer than that has to admit who invented the gradation.
func TestARatingMustSayWhoGradedIt(t *testing.T) {
	anonymous := Toxicity{Cats: HarmMild, Dogs: HarmMild, People: HarmMild}
	if err := anonymous.Valid(); err == nil {
		t.Error("a rating was accepted without saying whether a source or Planty graded it")
	}
}

func TestPartsAndRoutesAreCheckedAgainstAVocabulary(t *testing.T) {
	tests := []struct {
		name string
		tox  Toxicity
	}{
		{"invented part", Toxicity{Basis: BasisSource, Cats: HarmSafe, Parts: []string{"trunk"}}},
		{"invented route", Toxicity{Basis: BasisSource, Cats: HarmSafe, Routes: []string{"osmosis"}}},
		{"invented rating", Toxicity{Basis: BasisSource, Cats: Harm("deadly")}},
		{"invented basis", Toxicity{Cats: HarmSafe, Basis: Basis("vibes")}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.tox.Valid(); err == nil {
				t.Error("accepted a value outside the vocabulary")
			}
		})
	}
}

// The lily case, which is the reason the ratings are per audience at all.
func TestTheAudiencesAreRatedSeparately(t *testing.T) {
	lily := Toxicity{
		Cats:         HarmSevere,
		Dogs:         HarmMild,
		People:       HarmMild,
		Basis:        BasisSource,
		IdentifiedAs: "Lilium longiflorum",
		Principle:    "unidentified nephrotoxin",
	}

	if err := lily.Valid(); err != nil {
		t.Fatalf("the lily record is invalid: %v", err)
	}
	if !lily.Diverges() {
		t.Error("a plant that kills cats and mildly upsets dogs did not read as divergent")
	}
	if lily.Worst() != HarmSevere {
		t.Errorf("worst was %q for a plant that causes renal failure", lily.Worst())
	}
	if !lily.Dangerous() {
		t.Error("a lily did not read as dangerous")
	}
	if lily.Dogs.Urgent() {
		t.Error("a dog with a stomach ache was escalated to an emergency")
	}
	if !lily.Cats.Urgent() {
		t.Error("a cat in renal failure was not treated as urgent")
	}
}

// Most of a houseplant collection is Araceae, which treats everyone the same.
func TestTheOrdinaryCaseDoesNotDiverge(t *testing.T) {
	pothos := Toxicity{
		Cats: HarmMild, Dogs: HarmMild, People: HarmMild,
		Basis:        BasisSource,
		IdentifiedAs: "Epipremnum aureum",
		Principle:    "insoluble calcium oxalates",
	}
	if pothos.Diverges() {
		t.Error("a plant rated the same for everyone read as divergent")
	}
}

// An unchecked plant is not "divergent", it is unknown. Reporting divergence
// there would put a warning on every plant in a fresh database.
func TestAnUncheckedPlantDoesNotDiverge(t *testing.T) {
	if (Toxicity{}).Diverges() {
		t.Error("a plant nobody looked up reported divergence")
	}
}

// Losing the botanical name loses the only thing that makes a lily rating
// mean anything, so it has to survive the round trip.
func TestTheRecordSurvivesJSON(t *testing.T) {
	when := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	original := Toxicity{
		Cats: HarmSevere, Dogs: HarmMild, People: HarmMild,
		Basis:        BasisSource,
		IdentifiedAs: "Lilium longiflorum",
		Principle:    "unidentified nephrotoxin",
		Signs:        "vomiting, then anuria",
		Parts:        []string{"all", "flower"},
		Routes:       []string{"eaten", "skin"},
		FirstAid:     "a cat that groomed pollen goes to a vet before any sign appears",
		Source:       "www.aspca.org",
		CheckedAt:    &when,
	}

	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var back Toxicity
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}

	if back.IdentifiedAs != original.IdentifiedAs {
		t.Errorf("identity did not survive: %q", back.IdentifiedAs)
	}
	if back.FirstAid != original.FirstAid {
		t.Error("the first aid override did not survive")
	}
	if back.CheckedAt == nil || !back.CheckedAt.Equal(when) {
		t.Error("the check date did not survive")
	}
	if len(back.Parts) != 2 || len(back.Routes) != 2 {
		t.Error("parts or routes did not survive")
	}
}
