package store

import (
	"errors"
	"strings"
	"testing"

	"github.com/TheOutdoorProgrammer/planty/internal/judge"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

func TestAnAssignmentRoundTripsAndIsReadBackAsAModel(t *testing.T) {
	s, ctx := testStore(t)

	if _, ok := s.For(ctx, judge.JobAssess); ok {
		t.Fatal("a job had an assignment before one was made")
	}

	err := s.SetModelAssignment(ctx, ModelAssignment{
		Job: judge.JobAssess, Provider: "opencode-go", Model: "qwen3.8-max",
	})
	if err != nil {
		t.Fatalf("SetModelAssignment: %v", err)
	}

	got, ok := s.For(ctx, judge.JobAssess)
	if !ok {
		t.Fatal("the assignment did not come back")
	}
	if got.Ref() != "opencode-go/qwen3.8-max" {
		t.Errorf("got %s", got.Ref())
	}
	if !got.Skills.Schema {
		t.Error("the model came back without its capabilities")
	}

	listed, err := s.ModelAssignments(ctx)
	if err != nil {
		t.Fatalf("ModelAssignments: %v", err)
	}
	if len(listed) != 1 || listed[0].Job != judge.JobAssess {
		t.Errorf("the listing does not show the assignment: %+v", listed)
	}

	if err := s.ClearModelAssignment(ctx, judge.JobAssess); err != nil {
		t.Fatalf("ClearModelAssignment: %v", err)
	}
	if _, ok := s.For(ctx, judge.JobAssess); ok {
		t.Error("the assignment survived being cleared")
	}
}

func TestReassigningAJobReplacesRatherThanDuplicates(t *testing.T) {
	s, ctx := testStore(t)
	t.Cleanup(func() { _ = s.ClearModelAssignment(ctx, judge.JobIdentify) })

	for _, model := range []string{"qwen3.8-max", "mimo-v2.5"} {
		if err := s.SetModelAssignment(ctx, ModelAssignment{
			Job: judge.JobIdentify, Provider: "opencode-go", Model: model,
		}); err != nil {
			t.Fatalf("SetModelAssignment(%s): %v", model, err)
		}
	}

	got, ok := s.For(ctx, judge.JobIdentify)
	if !ok || got.ID != "mimo-v2.5" {
		t.Errorf("the second assignment did not win: %+v", got)
	}
}

// The store refuses an incapable pairing as well as the handler, so a direct
// write cannot leave the service holding something it will only fail on later.
func TestTheStoreRefusesAModelThatCannotDoTheJob(t *testing.T) {
	s, ctx := testStore(t)

	err := s.SetModelAssignment(ctx, ModelAssignment{
		Job: judge.JobIdentify, Provider: "opencode-go", Model: "deepseek-v4-flash",
	})
	if err == nil {
		t.Fatal("a blind model was assigned to identification")
	}
	if !errors.Is(err, plant.ErrInvalid) {
		t.Errorf("the refusal is not reported as invalid input: %v", err)
	}
	if !strings.Contains(err.Error(), "images") {
		t.Errorf("the refusal does not say why: %v", err)
	}

	if err := s.SetModelAssignment(ctx, ModelAssignment{
		Job: judge.JobAssess, Provider: "opencode-go", Model: "invented-model",
	}); !errors.Is(err, plant.ErrInvalid) {
		t.Errorf("an unknown model was accepted: %v", err)
	}
}

// A row naming something the catalogue stopped offering must not strand the
// service; the job quietly falls back to its default instead.
func TestARowNamingAnUnknownModelFallsBack(t *testing.T) {
	s, ctx := testStore(t)

	if _, err := s.pool.Exec(ctx,
		`INSERT INTO model_assignments (job, provider, model) VALUES ($1, $2, $3)
		 ON CONFLICT (job) DO UPDATE SET provider = EXCLUDED.provider, model = EXCLUDED.model`,
		string(judge.JobPostmortem), "opencode-go", "retired-model"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	t.Cleanup(func() { _ = s.ClearModelAssignment(ctx, judge.JobPostmortem) })

	if _, ok := s.For(ctx, judge.JobPostmortem); ok {
		t.Error("a row naming an unknown model was honoured")
	}
}
