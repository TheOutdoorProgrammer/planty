package plant

import (
	"slices"
	"time"
)

// Harm is how badly a plant treats whatever ate it, graded by what the owner
// should do rather than by pharmacology, because that is the only scale a
// houseplant app can honestly defend. adr/0003 has the reasoning.
type Harm string

const (
	// The default, and the only honest answer for a plant nobody looked up.
	// Distinct from safe because silence about a lily is not a lily being fine.
	HarmUnknown Harm = "unknown"

	// No toxic principle reported. Which the ASPCA means as "not poisonous",
	// not "harmless": enough of anything causes vomiting.
	HarmSafe Harm = "safe"

	// Self-limiting. Rinse the mouth, offer water, watch it.
	HarmMild Harm = "mild"

	// Systemic effects are plausible. Ring the vet or the poison line.
	HarmModerate Harm = "moderate"

	// Can kill, or has a narrow treatment window. Go now, before signs appear.
	HarmSevere Harm = "severe"
)

// Basis records who did the grading. The ASPCA publishes toxic or non-toxic
// and nothing between, so any finer level is Planty's own judgement and must
// say so rather than borrow the source's authority.
type Basis string

const (
	// The source stated this level itself.
	BasisSource Basis = "source"

	// Graded here from the toxic principle. An honest guess, labelled.
	BasisDerived Basis = "derived"
)

// Known reports whether anyone has actually looked this up.
func (h Harm) Known() bool { return h != "" && h != HarmUnknown }

// Urgent reports whether eating it means calling someone rather than watching.
func (h Harm) Urgent() bool { return h == HarmModerate || h == HarmSevere }

// Toxicity is what a plant does to whatever eats it, rated per audience: an
// Easter lily is renal failure in a cat and a stomach ache in a dog, and one
// flag for both either panics dog owners or kills cats.
type Toxicity struct {
	Cats   Harm `json:"cats"`
	Dogs   Harm `json:"dogs"`
	People Harm `json:"people"`

	Basis Basis `json:"basis,omitempty"`

	// The botanical name this was looked up under. "Lily" is six unrelated
	// plants across three mechanisms, so a rating that cannot say what it
	// resolved to is not evidence of anything.
	IdentifiedAs string `json:"identified_as,omitempty"`

	// The mechanism, in the reference works' vocabulary ("insoluble calcium
	// oxalates"). Justifies the ratings and outlives them when re-checked.
	Principle string `json:"principle,omitempty"`

	// What you would see in an animal that ate it.
	Signs string `json:"signs,omitempty"`

	// Which parts hurt. Structured rather than prose because a tomato's fruit
	// is dinner and its leaves are not, and burying that distinction in a
	// paragraph is how someone gets hurt.
	Parts []string `json:"parts,omitempty"`

	// How it gets you. Euphorbia sap damages eyes without anyone eating it.
	Routes []string `json:"routes,omitempty"`

	// What the ratings cannot carry: why the audiences diverge, dose caveats.
	Notes string `json:"notes,omitempty"`

	// Set only where the advice implied by the rating is actively wrong: a cat
	// that groomed lily pollen needs a vet before any sign appears, and
	// Euphorbia sap in an eye is irrigation rather than anything oral.
	FirstAid string `json:"first_aid,omitempty"`

	// Where this came from, so a doubtful entry can be re-read rather than
	// argued about.
	Source string `json:"source,omitempty"`

	// Separates "looked it up and found nothing" from "never looked", which
	// otherwise wear the same HarmUnknown.
	CheckedAt *time.Time `json:"checked_at,omitempty"`
}

// PartNames is every plant part a hazard can be pinned to.
var PartNames = []string{
	"all", "bulb", "leaf", "stem", "sap", "flower", "fruit", "seed", "root",
}

// RouteNames is every way a plant can reach you.
var RouteNames = []string{"eaten", "skin", "eyes", "breathed"}

// Rating pairs an audience with its verdict.
type Rating struct {
	Who  string
	Harm Harm
}

// Audiences is every rating, in display order. Anything iterating them uses
// this rather than repeating the list.
func (t Toxicity) Audiences() []Rating {
	return []Rating{{"cats", t.Cats}, {"dogs", t.Dogs}, {"people", t.People}}
}

// Checked reports whether anybody has established anything at all.
func (t Toxicity) Checked() bool {
	return t.CheckedAt != nil || t.Cats.Known() || t.Dogs.Known() || t.People.Known()
}

// Worst is the highest rating across the audiences.
func (t Toxicity) Worst() Harm {
	worst := HarmUnknown
	for _, a := range t.Audiences() {
		if rank(a.Harm) > rank(worst) {
			worst = a.Harm
		}
	}
	return worst
}

// Dangerous reports whether this plant is worth warning about for anyone.
func (t Toxicity) Dangerous() bool { return t.Worst().Urgent() }

// Diverges reports whether the audiences disagree. Most of a collection is
// Araceae, which treats all three identically, so the rare plant that does not
// is worth saying out loud rather than leaving someone to spot it in chips.
func (t Toxicity) Diverges() bool {
	return t.Checked() && (t.Cats != t.Dogs || t.Dogs != t.People)
}

// rank orders Harm. Unknown deliberately outranks safe, so sorting by risk
// surfaces the plants nobody has checked instead of burying them.
func rank(h Harm) int {
	switch h {
	case HarmSafe:
		return 1
	case HarmUnknown, "":
		return 2
	case HarmMild:
		return 3
	case HarmModerate:
		return 4
	case HarmSevere:
		return 5
	}
	return 0
}

// ValidHarm reports whether a rating is one the store accepts. The empty
// string counts, because an untouched plant is unrated rather than invalid.
func ValidHarm(h Harm) bool {
	switch h {
	case "", HarmUnknown, HarmSafe, HarmMild, HarmModerate, HarmSevere:
		return true
	}
	return false
}

// Valid holds the two rules that keep this field honest: a claim that
// something is dangerous has to name the mechanism, and a level finer than the
// sources publish has to admit it was derived here.
func (t Toxicity) Valid() error {
	for _, a := range t.Audiences() {
		if !ValidHarm(a.Harm) {
			return invalid("unknown toxicity rating %q for %s", a.Harm, a.Who)
		}
	}
	for _, part := range t.Parts {
		if !slices.Contains(PartNames, part) {
			return invalid("%q is not a plant part; use one of %v", part, PartNames)
		}
	}
	for _, route := range t.Routes {
		if !slices.Contains(RouteNames, route) {
			return invalid("%q is not an exposure route; use one of %v", route, RouteNames)
		}
	}
	switch t.Basis {
	case "", BasisSource, BasisDerived:
	default:
		return invalid("unknown basis %q", t.Basis)
	}
	if t.Worst().Urgent() && t.Principle == "" {
		return invalid("a rating of %q needs a toxic principle to justify it", t.Worst())
	}
	if t.Checked() && t.Basis == "" {
		return invalid("say whether the rating came from the source or was derived here")
	}
	return nil
}
