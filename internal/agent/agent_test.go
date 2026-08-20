package agent

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

func runVerb(t *testing.T, deps Deps, args ...string) (string, error) {
	t.Helper()
	return runVerbCtx(t, context.Background(), deps, args...)
}

func runVerbCtx(t *testing.T, ctx context.Context, deps Deps, args ...string) (string, error) {
	t.Helper()
	var out strings.Builder
	err := Run(ctx, deps, &out, args)
	return out.String(), err
}

func TestUsageAndVerbTableAgree(t *testing.T) {
	for verb := range verbs {
		if !strings.Contains(Usage, "  "+verb) {
			t.Errorf("verb %q is callable but absent from Usage", verb)
		}
	}
}

func TestHelpPrintsUsage(t *testing.T) {
	out, err := runVerb(t, Deps{}, "help")
	if err != nil {
		t.Fatal(err)
	}
	if out != Usage+"\n" {
		t.Error("help is not the exact Usage handed to the model")
	}
}

func TestUnknownVerbIsRefusedByName(t *testing.T) {
	out, err := runVerb(t, Deps{}, "migrate")
	if err == nil || !strings.Contains(err.Error(), "there is no agent verb") {
		t.Fatalf("migrate should be refused by the agent surface, got %v", err)
	}
	if out != Usage+"\n" {
		t.Error("unknown verb did not print the safe surface")
	}
}

func TestNoVerbPrintsUsageAndFails(t *testing.T) {
	out, err := runVerb(t, Deps{})
	if err == nil {
		t.Fatal("no verb should fail")
	}
	if out != Usage+"\n" {
		t.Error("no verb did not print usage")
	}
}

func TestCreateDefaultsMatchTheAPI(t *testing.T) {
	deps, _, ctx := toxicityDeps(t)
	name := "Agent defaults " + t.Name()

	out, err := runVerbCtx(t, ctx, deps, "create", "--name", name)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "houseplant") || !strings.Contains(out, "watering hand") {
		t.Fatalf("confirmation did not report defaults: %s", out)
	}

	plants, err := deps.Store.ListPlants(ctx, store.PlantFilter{Status: plant.StatusAlive})
	if err != nil {
		t.Fatal(err)
	}
	var found plant.Plant
	for _, p := range plants {
		if p.CommonName == name {
			found = p
		}
	}
	if found.ID == [16]byte{} {
		t.Fatal("created plant was not stored")
	}
	if found.Domain != plant.DomainHouseplant || found.Steward != plant.StewardSelf ||
		found.Accessibility != plant.AccessEasy || found.WateringMethod != plant.WateringHand {
		t.Fatalf("agent defaults drifted from API: %+v", found)
	}
}

func TestParseRejectsUnquotedLeftovers(t *testing.T) {
	set := newFlags("test")
	_ = set.String("name", "", "")
	if err := parse(set, []string{"--name", "Big", "Pothos"}); err == nil ||
		!strings.Contains(err.Error(), "not attached to a flag") {
		t.Fatalf("unquoted leftovers were silently ignored: %v", err)
	}
}

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
		t.Errorf("medium should parse, got %v %v", l, err)
	}
}

func TestNotWhileTalkingNamesTheConversationOwner(t *testing.T) {
	if err := notWhileTalking("self"); err == nil || !strings.Contains(err.Error(), "in this conversation") {
		t.Fatalf("asking the current person should be refused, got %v", err)
	}
	if err := notWhileTalking(""); err != nil {
		t.Fatalf("letting the store infer somebody else's steward failed: %v", err)
	}
}

func TestSplitSlugsDropsWhitespaceAndEmpties(t *testing.T) {
	got := splitSlugs(" mona, , basil ,tomato ")
	want := []string{"mona", "basil", "tomato"}
	if len(got) != len(want) {
		t.Fatalf("got %#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	}
}

func TestParseHours(t *testing.T) {
	got, err := parseHours("8, 20")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != 8 || got[1] != 20 {
		t.Fatalf("got %#v", got)
	}
	if _, err := parseHours("morning"); err == nil {
		t.Fatal("words are not hours")
	}
}

func TestDescribeReminder(t *testing.T) {
	if got := describe(plant.Reminder{EveryDays: 1, AtHours: []int{8, 20}}); got != "every day at 08:00 and 20:00" {
		t.Fatalf("got %q", got)
	}
	if got := describe(plant.Reminder{EveryDays: 7, AtHours: []int{8}}); got != "every 7 days at 08:00" {
		t.Fatalf("got %q", got)
	}
}

func TestRunUsesCallerContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := runVerbCtx(t, ctx, Deps{}, "plants")
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "database") {
		t.Fatalf("cancelled caller context did not flow through: %v", err)
	}
}

func TestParseWhenPreservesZeroMeaning(t *testing.T) {
	at, err := parseWhen("")
	if err != nil {
		t.Fatal(err)
	}
	if !at.IsZero() {
		t.Fatalf("empty time became %v", at)
	}
}

func TestTimeZonesRemainExplicit(t *testing.T) {
	at, err := parseWhen("2026-08-17T09:30:00-04:00")
	if err != nil {
		t.Fatal(err)
	}
	_, offset := at.Zone()
	if offset != -4*60*60 {
		t.Fatalf("offset was %d", offset)
	}
}

var _ io.Writer = (*strings.Builder)(nil)
var _ = time.Second
