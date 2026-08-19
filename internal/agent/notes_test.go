package agent

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/TheOutdoorProgrammer/planty/internal/job"
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

// "There is a cat here" is not a fact about the pothos, and with nowhere to
// put it the model wrote it against whichever plant was under discussion.
func TestAHouseholdNoteBelongsToNoPlant(t *testing.T) {
	deps, p, ctx := toxicityDeps(t)

	if _, err := runVerbCtx(t, ctx, deps, "note", "--household",
		"--title", "Cat", "--text", "there is a cat indoors that chews leaves"); err != nil {
		t.Fatalf("writing a household note failed: %v", err)
	}

	house, err := runVerbCtx(t, ctx, deps, "notes", "--household")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(house, "chews leaves") {
		t.Errorf("the household note did not come back:\n%s", house)
	}

	onPlant, _ := runVerbCtx(t, ctx, deps, "notes", "--plant", p.Slug)
	if strings.Contains(onPlant, "chews leaves") {
		t.Error("a household note was filed against a plant")
	}
}

// A forgotten --plant must be refused rather than quietly becoming a note
// about the whole house.
func TestANoteMustSayWhatItIsAbout(t *testing.T) {
	deps, p, ctx := toxicityDeps(t)

	for _, args := range [][]string{
		{"note", "--text", "about what?"},
		{"note", "--plant", p.Slug, "--household", "--text", "both"},
		{"notes"},
		{"notes", "--plant", p.Slug, "--household"},
	} {
		if _, err := runVerbCtx(t, ctx, deps, args...); err == nil {
			t.Errorf("accepted an ambiguous subject: %v", args)
		}
	}
}

// The whole point: a household note reaches the model on every consultation.
func TestHouseholdNotesReachEveryPlant(t *testing.T) {
	deps, p, ctx := toxicityDeps(t)

	// Named after the test: the household is shared by everything, so a fixed
	// body would be found even when this test wrote nothing.
	mine := "a cat indoors, per " + t.Name()
	if _, err := runVerbCtx(t, ctx, deps, "note", "--household", "--text", mine); err != nil {
		t.Fatal(err)
	}

	history, err := job.Gather(ctx, deps.Store, p, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	var found bool
	for _, note := range history.Household {
		if note.Body == mine {
			found = true
			if note.PlantID != uuid.Nil {
				t.Error("a household note came back owned by a plant")
			}
		}
	}
	if !found {
		t.Errorf("a consultation about %s did not carry the household note", p.Slug)
	}
}

// The queue is for somebody who is not here. Asked of the person already
// reading the reply, it goes into a table nobody opens.
func TestQueueingAQuestionForWhoeverIsReadingIsRefused(t *testing.T) {
	deps, p, ctx := toxicityDeps(t)
	t.Setenv("PLANTY_CHAT", "1")

	_, err := runVerbCtx(t, ctx, deps, "ask", "--plant", p.Slug,
		"--question", "When did you last water this?")
	if err == nil {
		t.Fatal("a question was queued for somebody already reading the reply")
	}
	if !strings.Contains(err.Error(), "ask them in it") {
		t.Errorf("the refusal did not say what to do instead: %v", err)
	}
}

// A question for the friend whose plants these are is exactly what the queue
// is for, so it still works mid-conversation.
func TestAQuestionForSomebodyAbsentIsStillQueued(t *testing.T) {
	deps, p, ctx := toxicityDeps(t)
	t.Setenv("PLANTY_CHAT", "1")

	if _, err := runVerbCtx(t, ctx, deps, "ask", "--plant", p.Slug,
		"--of", "Aric", "--question", "Is the front porch covered?"); err != nil {
		t.Fatalf("a question for an absent steward was refused: %v", err)
	}
}

// The daily job has nobody to ask, so the queue is its only channel.
func TestTheScheduledJobMayStillQueue(t *testing.T) {
	deps, p, ctx := toxicityDeps(t)

	if _, err := runVerbCtx(t, ctx, deps, "ask", "--plant", p.Slug,
		"--question", "Has it ever been repotted?"); err != nil {
		t.Fatalf("a scheduled job could not queue a question: %v", err)
	}
}
