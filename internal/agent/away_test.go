package agent

import (
	"strings"
	"testing"
)

func TestAwayVerbListsEditsAndCancelsCoverage(t *testing.T) {
	deps, _, ctx := toxicityDeps(t)

	created, err := runVerbCtx(t, ctx, deps, "away",
		"--from", "2102-03-01", "--until", "2102-03-05",
		"--contact", "Sam", "--note", "first draft")
	if err != nil {
		t.Fatalf("create coverage: %v", err)
	}
	fields := strings.Fields(created)
	if len(fields) == 0 {
		t.Fatal("create returned no confirmation")
	}
	id := strings.TrimSuffix(fields[len(fields)-1], ")")

	listed, err := runVerbCtx(t, ctx, deps, "away")
	if err != nil {
		t.Fatalf("list coverage: %v", err)
	}
	if !strings.Contains(listed, id) || !strings.Contains(listed, "Sam") {
		t.Fatalf("created coverage was not inspectable:\n%s", listed)
	}

	if _, err := runVerbCtx(t, ctx, deps, "away", "--id", id,
		"--contact", "Maya", "--note", "corrected"); err != nil {
		t.Fatalf("edit coverage: %v", err)
	}
	listed, err = runVerbCtx(t, ctx, deps, "away")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listed, "Maya") || !strings.Contains(listed, "corrected") {
		t.Fatalf("edited coverage was not visible:\n%s", listed)
	}

	if _, err := runVerbCtx(t, ctx, deps, "away", "--id", id, "--cancel"); err != nil {
		t.Fatalf("cancel coverage: %v", err)
	}
	listed, err = runVerbCtx(t, ctx, deps, "away")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(listed, id) {
		t.Fatalf("cancelled coverage was still listed:\n%s", listed)
	}
}
