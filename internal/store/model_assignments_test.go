package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/TheOutdoorProgrammer/planty/internal/judge"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

func TestAnAssignmentRoundTripsAndIsReadBackAsAModel(t *testing.T) {
	s, ctx := testStore(t)

	if _, ok, err := s.For(ctx, judge.JobAssess); ok || err != nil {
		t.Fatalf("unassigned job: found=%v err=%v", ok, err)
	}

	err := s.SetModelAssignment(ctx, ModelAssignment{
		Job: judge.JobAssess, Provider: "opencode-go", Model: "qwen3.8-max",
	})
	if err != nil {
		t.Fatalf("SetModelAssignment: %v", err)
	}

	got, ok, err := s.For(ctx, judge.JobAssess)
	if !ok || err != nil {
		t.Fatalf("the assignment did not come back: found=%v err=%v", ok, err)
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
	if _, ok, err := s.For(ctx, judge.JobAssess); ok || err != nil {
		t.Errorf("cleared assignment: found=%v err=%v", ok, err)
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

	got, ok, err := s.For(ctx, judge.JobIdentify)
	if !ok || err != nil || got.ID != "mimo-v2.5" {
		t.Errorf("the second assignment did not win: %+v, %v", got, err)
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

func TestInvalidStoredAssignmentsReturnAnError(t *testing.T) {
	for _, tc := range []struct {
		name, provider, model, reason string
	}{
		{"retired model", "opencode-go", "retired-model", "there is no model"},
		{"unknown provider", "retired-provider", "qwen3.8-max", "there is no model"},
		{"missing vision", "opencode-go", "deepseek-v4-flash", "images"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, ctx := testStore(t)
			if _, err := s.pool.Exec(ctx,
				`INSERT INTO model_assignments (job, provider, model) VALUES ($1, $2, $3)
				 ON CONFLICT (job) DO UPDATE SET provider = EXCLUDED.provider, model = EXCLUDED.model`,
				string(judge.JobAssess), tc.provider, tc.model); err != nil {
				t.Fatalf("seed: %v", err)
			}
			t.Cleanup(func() { _ = s.ClearModelAssignment(ctx, judge.JobAssess) })

			_, ok, err := s.For(ctx, judge.JobAssess)
			if ok || !errors.Is(err, plant.ErrInvalid) {
				t.Fatalf("invalid assignment: found=%v err=%v", ok, err)
			}
			if !strings.Contains(err.Error(), tc.reason) {
				t.Errorf("error does not explain invalid assignment: %v", err)
			}
		})
	}
}

func TestAssignmentReadFailureDoesNotLookUnassigned(t *testing.T) {
	t.Run("canceled context", func(t *testing.T) {
		s, ctx := testStore(t)
		ctx, cancel := context.WithCancel(ctx)
		cancel()
		if _, ok, err := s.For(ctx, judge.JobAssess); ok || !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled lookup: found=%v err=%v", ok, err)
		}
	})
	t.Run("closed database", func(t *testing.T) {
		s, ctx := testStore(t)
		s.Close()
		if _, ok, err := s.For(ctx, judge.JobAssess); ok || err == nil {
			t.Fatalf("unavailable lookup: found=%v err=%v", ok, err)
		}
	})
}
