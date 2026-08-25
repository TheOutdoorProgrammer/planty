package agent

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"
)

func runVerb(t *testing.T, d Deps, args ...string) (string, error) {
	t.Helper()
	return runVerbCtx(t, context.Background(), d, args...)
}

func runVerbCtx(t *testing.T, ctx context.Context, d Deps, args ...string) (string, error) {
	t.Helper()

	var out bytes.Buffer
	err := Run(ctx, d, &out, args)
	return out.String(), err
}

// The gate passes anything behind `planty agent `, so keeping autopsy (which
// spends tokens recursively) and the operational commands out of the dispatch
// table is Run's job. This fails the moment somebody adds one.
func TestExcludedCommandsAreNotAgentVerbs(t *testing.T) {
	for _, name := range []string{
		"autopsy", "migrate", "seed", "serve",
		"daily", "cold", "chase", "thirst", "ingest",
		"gate", "version",
	} {
		_, err := runVerb(t, Deps{}, name, "golden-pothos")
		if err == nil || !strings.Contains(err.Error(), "no agent verb") {
			t.Errorf("%q must not be an agent verb, got error %v", name, err)
		}
	}
}

var usageVerb = regexp.MustCompile(`(?m)^  ([a-z]+)\b`)

// Usage is the contract handed to the model; a verb missing from it cannot be
// used, and a verb only in it is a lie. Both directions are checked.
func TestUsageAndTheDispatchTableAgree(t *testing.T) {
	documented := map[string]bool{}
	for _, match := range usageVerb.FindAllStringSubmatch(Usage, -1) {
		documented[match[1]] = true
	}
	if len(documented) == 0 {
		t.Fatal("no verbs found in the usage text; the format changed and this test proves nothing")
	}

	for name := range verbs {
		if !documented[name] {
			t.Errorf("verb %q exists but the usage text never mentions it", name)
		}
	}
	for name := range documented {
		if _, ok := verbs[name]; !ok {
			t.Errorf("the usage text promises %q, which does not exist", name)
		}
	}
}

func TestHelpPrintsTheReference(t *testing.T) {
	out, err := runVerb(t, Deps{}, "help")
	if err != nil {
		t.Fatalf("help errored: %v", err)
	}
	if !strings.Contains(out, "planty agent <verb>") {
		t.Errorf("help did not print the reference: %q", out[:min(80, len(out))])
	}
}

func TestNoVerbStillTeaches(t *testing.T) {
	out, err := runVerb(t, Deps{})
	if err == nil {
		t.Error("no verb given must be an error")
	}
	if !strings.Contains(out, "planty agent <verb>") {
		t.Error("the usage text was not printed for an empty call")
	}

	out, err = runVerb(t, Deps{}, "prune-everything")
	if err == nil || !strings.Contains(err.Error(), "no agent verb") {
		t.Errorf("unknown verb error: %v", err)
	}
	if !strings.Contains(out, "planty agent <verb>") {
		t.Error("the usage text was not printed for an unknown verb")
	}
}

// Every verb acting on one plant refuses plainly before touching anything
// when no --plant was given.
func TestSubjectVerbsDemandAPlant(t *testing.T) {
	for _, name := range []string{
		"show", "observations", "health", "reminders",
		"log", "harvest", "healthchange", "remind", "forget", "update", "archive", "ack",
	} {
		_, err := runVerb(t, Deps{}, name)
		if err == nil || !strings.Contains(err.Error(), "--plant") {
			t.Errorf("%s with no plant: want an error naming --plant, got %v", name, err)
		}
	}
}

func TestWaterRunsTheConfiguredPass(t *testing.T) {
	ran := false
	out, err := runVerb(t, Deps{Water: func(context.Context) error {
		ran = true
		return nil
	}}, "water")
	if err != nil {
		t.Fatalf("water errored: %v", err)
	}
	if !ran {
		t.Error("the configured watering pass never ran")
	}
	if !strings.Contains(out, "LetPot pass ran") {
		t.Errorf("no confirmation was printed: %q", out)
	}
}

func TestWaterRefusesWhenNotWired(t *testing.T) {
	_, err := runVerb(t, Deps{}, "water")
	if err == nil || !strings.Contains(err.Error(), "not wired up") {
		t.Errorf("want a plain refusal, got %v", err)
	}
}

// Flag mistakes that must come back as sentences, before any store call.
func TestFlagMistakesFailPlainly(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"create without a name", []string{"create"}, "--name"},
		{"shelter with no subject", []string{"shelter"}, "--all"},
		{"shelter with both subjects", []string{"shelter", "--plant", "a", "--all"}, "not both"},
		{"unshelter with no subject", []string{"unshelter"}, "--all"},
		{"calibrate without an entity", []string{"calibrate", "--dry", "1", "--wet", "2"}, "--entity"},
		{"calibrate without baselines", []string{"calibrate", "--entity", "sensor.x"}, "--dry"},
		{"link with two subjects", []string{"link", "--plant", "a", "--zone", "b"}, "not both"},
		{"answer with a junk id", []string{"answer", "--question", "nope", "--answer", "yes"}, "not a question id"},
		{"answer with no text", []string{"answer", "--question", "nope"}, "--answer"},
		{"ask with no question", []string{"ask", "--plant", "a"}, "--question"},
		{"away create missing until", []string{"away", "--from", "2026-08-20"}, "--until"},
		{"coldwatch with no low", []string{"coldwatch"}, "--low"},
		{"log with a junk time", []string{"log", "--plant", "a", "--kind", "watered", "--when", "yesterdayish"}, "not a time"},
		{"health without a shape", []string{"healthchange", "--plant", "a", "--reason", "x", "--evidence", "y", "--key", "not-reached"}, "exactly one"},
		{"a flag that does not exist", []string{"plants", "--vibe", "good"}, "vibe"},
	} {
		_, err := runVerb(t, Deps{}, tc.args...)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: want an error mentioning %q, got %v", tc.name, tc.want, err)
		}
	}
}

func TestParseWhenReadsBothShapes(t *testing.T) {
	if at, err := parseWhen("2026-08-17"); err != nil || at.Format("2006-01-02") != "2026-08-17" {
		t.Errorf("bare date: %v %v", at, err)
	}
	if at, err := parseWhen("2026-08-17T09:30:00Z"); err != nil || at.Hour() != 9 {
		t.Errorf("RFC3339: %v %v", at, err)
	}
	if at, err := parseWhen(""); err != nil || !at.IsZero() {
		t.Errorf("empty must mean the store's own now: %v %v", at, err)
	}
	if _, err := parseWhen("next tuesday"); err == nil {
		t.Error("prose is not a time")
	}
}

func TestLightExposureNamesTheChoices(t *testing.T) {
	if _, err := lightExposure("shady"); err == nil ||
		!strings.Contains(err.Error(), "bright_indirect") {
		t.Errorf("a bad light value must list the real ones, got %v", err)
	}
	if l, err := lightExposure("medium"); err != nil || string(l) != "medium" {
		t.Errorf("medium is valid: %v %v", l, err)
	}
}

// An unquoted multi-word value used to set the flag to its first word and drop
// the rest silently, renaming a plant to something nobody asked for. A model
// constructing a command line gets this wrong, so it has to be loud.
func TestAStrayWordIsRefusedRatherThanDropped(t *testing.T) {
	for _, args := range [][]string{
		{"update", "--plant", "golden-pothos", "--name", "Big", "Pothos"},
		{"log", "--plant", "golden-pothos", "--kind", "note", "--note", "looked", "droopy"},
		{"create", "--name", "Blue", "Oyster"},
	} {
		_, err := runVerb(t, Deps{}, args...)
		if err == nil || !strings.Contains(err.Error(), "double quotes") {
			t.Errorf("%v: want a refusal naming the loose words, got %v", args, err)
		}
	}
}

// A properly quoted value still works, or the guard above has broken the verb
// it was meant to protect.
func TestAQuotedValueSurvives(t *testing.T) {
	set := newFlags("update")
	name := set.String("name", "", "")
	if err := parse(set, []string{"--name", "Big Pothos"}); err != nil {
		t.Fatalf("a quoted value was refused: %v", err)
	}
	if *name != "Big Pothos" {
		t.Errorf("name is %q", *name)
	}
}

// "The pass completed" reads as "water moved", and a garden with nothing on
// the line completes without watering anything. A model relaying that would be
// telling somebody their plant was watered when it was not.
func TestWateringNeverClaimsWaterMoved(t *testing.T) {
	out, err := runVerb(t, Deps{Water: func(context.Context) error { return nil }}, "water")
	if err != nil {
		t.Fatalf("water: %v", err)
	}
	if !strings.Contains(out, "may well have watered none") {
		t.Errorf("the report reads as a completed watering: %q", out)
	}
}
