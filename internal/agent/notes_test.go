package agent

import (
	"strings"
	"testing"
)

func TestWritingReadingAndRemovingANote(t *testing.T) {
	deps, p, ctx := toxicityDeps(t)

	out, err := runVerbCtx(t, ctx, deps, "note", "--plant", p.Slug,
		"--title", "The cat", "--text", "she keeps chewing this one")
	if err != nil {
		t.Fatalf("writing a note failed: %v", err)
	}
	id := strings.Fields(out)[len(strings.Fields(out))-1]

	listed, err := runVerbCtx(t, ctx, deps, "notes", "--plant", p.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listed, "she keeps chewing") || !strings.Contains(listed, "The cat") {
		t.Errorf("the note did not come back:\n%s", listed)
	}

	if _, err := runVerbCtx(t, ctx, deps, "note", "--id", id, "--delete"); err != nil {
		t.Fatalf("removing failed: %v", err)
	}
	gone, _ := runVerbCtx(t, ctx, deps, "notes", "--plant", p.Slug)
	if strings.Contains(gone, "she keeps chewing") {
		t.Error("the note survived being removed")
	}
}

// Rewording the body must not silently discard a title nobody mentioned.
func TestRewordingABodyKeepsTheTitle(t *testing.T) {
	deps, p, ctx := toxicityDeps(t)

	out, err := runVerbCtx(t, ctx, deps, "note", "--plant", p.Slug,
		"--title", "Repotting", "--text", "went up a size in March")
	if err != nil {
		t.Fatal(err)
	}
	id := strings.Fields(out)[len(strings.Fields(out))-1]

	if _, err := runVerbCtx(t, ctx, deps, "note", "--id", id,
		"--text", "went up two sizes in March"); err != nil {
		t.Fatal(err)
	}

	listed, _ := runVerbCtx(t, ctx, deps, "notes", "--plant", p.Slug)
	if !strings.Contains(listed, "Repotting") {
		t.Errorf("rewording the body dropped the title:\n%s", listed)
	}
	if !strings.Contains(listed, "two sizes") {
		t.Errorf("the new body is missing:\n%s", listed)
	}
}

func TestANoteNeedsToSayWhichPlantOrWhichNote(t *testing.T) {
	deps, p, ctx := toxicityDeps(t)

	if _, err := runVerbCtx(t, ctx, deps, "note", "--text", "orphan"); err == nil {
		t.Error("a note with neither a plant nor an id was accepted")
	}
	if _, err := runVerbCtx(t, ctx, deps, "note", "--plant", p.Slug); err == nil {
		t.Error("an empty note was accepted")
	}
}

func TestNoPlantHasNotesToBeginWith(t *testing.T) {
	deps, p, ctx := toxicityDeps(t)

	out, err := runVerbCtx(t, ctx, deps, "notes", "--plant", p.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "no notes") {
		t.Errorf("expected an empty list to say so, got %q", out)
	}
}
