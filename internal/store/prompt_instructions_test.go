package store

import (
	"strings"
	"testing"

	"github.com/TheOutdoorProgrammer/planty/internal/judge"
)

func TestPromptInstructionsCanBeSetReplacedListedAndCleared(t *testing.T) {
	s, ctx := testStore(t)
	_ = s.ClearPromptInstruction(ctx, judge.JobAssess)
	t.Cleanup(func() { _ = s.ClearPromptInstruction(ctx, judge.JobAssess) })

	if _, ok := s.PromptInstructionsFor(ctx, judge.JobAssess); ok {
		t.Fatal("assess had an overlay before one was set")
	}

	first, err := s.SetPromptInstruction(ctx, PromptInstruction{
		Job: judge.JobAssess, Instructions: "  Mention whether another photo would resolve uncertainty.  ",
	})
	if err != nil {
		t.Fatalf("set prompt instructions: %v", err)
	}
	if first.Instructions != "Mention whether another photo would resolve uncertainty." || first.UpdatedAt.IsZero() {
		t.Fatalf("saved instruction was not normalized or timestamped: %+v", first)
	}

	second, err := s.SetPromptInstruction(ctx, PromptInstruction{
		Job: judge.JobAssess, Instructions: "Prefer observation over intervention.",
	})
	if err != nil {
		t.Fatalf("replace prompt instructions: %v", err)
	}
	if second.Instructions == first.Instructions {
		t.Fatal("replacement kept the old instructions")
	}

	listed, err := s.PromptInstructions(ctx)
	if err != nil {
		t.Fatalf("list prompt instructions: %v", err)
	}
	found := false
	for _, instruction := range listed {
		if instruction.Job == judge.JobAssess {
			found = instruction.Instructions == second.Instructions
		}
	}
	if !found {
		t.Fatalf("listing did not contain the replacement: %+v", listed)
	}

	if err := s.ClearPromptInstruction(ctx, judge.JobAssess); err != nil {
		t.Fatalf("clear prompt instructions: %v", err)
	}
	if _, ok := s.PromptInstructionsFor(ctx, judge.JobAssess); ok {
		t.Fatal("cleared overlay was still returned")
	}
}

func TestPromptInstructionsRejectUnknownBlankAndOversizedWrites(t *testing.T) {
	s, ctx := testStore(t)

	for name, instruction := range map[string]PromptInstruction{
		"unknown job": {Job: "invented", Instructions: "Do something."},
		"blank":       {Job: judge.JobAssess, Instructions: "  \n\t"},
		"oversized":   {Job: judge.JobAssess, Instructions: strings.Repeat("x", MaxPromptInstructionsLength+1)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := s.SetPromptInstruction(ctx, instruction); err == nil {
				t.Fatal("invalid prompt instructions were accepted")
			}
		})
	}
}
