package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// manual is every command deliberately not on a schedule, and why. A command
// absent from both this and the CronJobs is one nothing ever runs.
var manual = map[string]string{
	"serve":   "runs as a Deployment, not a CronJob",
	"migrate": "every command migrates on start, so it is only ever run by hand",
	"autopsy": "takes a slug, and is asked for when a plant dies",
	"seed":    "one-off, against a new database",

	// Joey's rule, and the reason `thirst` exists: nothing waters a plant on a
	// timer. He is told it needs doing and turns the pump on himself.
	"water": "moves water, so it is never scheduled",
}

var scheduledArgs = regexp.MustCompile(`args:\s*\["(\w+)"\]`)

// The LetPot line was implemented, wired into main, covered by tests, and
// scheduled nowhere, so watering would never once have happened. Nothing was
// checking that a command anybody can run is a command something does run.
func TestEveryCommandIsScheduledOrDeliberatelyManual(t *testing.T) {
	documented := commandsInUsage(t)
	scheduled := commandsInCronJobs(t)

	for _, name := range documented {
		if _, byHand := manual[name]; byHand {
			continue
		}
		if !scheduled[name] {
			t.Errorf("%q is documented but never scheduled: add a CronJob, or say in `manual` why it is run by hand", name)
		}
	}
}

// The other direction: a CronJob calling a command that no longer exists fails
// on its own schedule, in a log nobody is reading.
func TestEveryScheduledCommandStillExists(t *testing.T) {
	documented := map[string]bool{}
	for _, name := range commandsInUsage(t) {
		documented[name] = true
	}

	for name := range commandsInCronJobs(t) {
		if !documented[name] {
			t.Errorf("a CronJob runs %q, which planty does not offer", name)
		}
	}
}

// commandsInUsage reads the names out of the usage text, which is the list a
// person is told about and so the one that has to be true.
func commandsInUsage(t *testing.T) []string {
	t.Helper()

	var names []string
	for line := range strings.SplitSeq(usage, "\n") {
		if !strings.HasPrefix(line, "  ") {
			continue
		}
		if fields := strings.Fields(line); len(fields) > 0 {
			names = append(names, fields[0])
		}
	}
	if len(names) == 0 {
		t.Fatal("no commands found in the usage text")
	}
	return names
}

func commandsInCronJobs(t *testing.T) map[string]bool {
	t.Helper()

	raw, err := os.ReadFile("../../deploy/cronjobs.yaml")
	if err != nil {
		t.Fatalf("read cronjobs: %v", err)
	}

	found := map[string]bool{}
	for _, match := range scheduledArgs.FindAllStringSubmatch(string(raw), -1) {
		found[match[1]] = true
	}
	if len(found) == 0 {
		t.Fatal("no scheduled commands found: the manifest shape changed and this test now proves nothing")
	}
	return found
}
