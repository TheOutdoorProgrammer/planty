package job

import (
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/TheOutdoorProgrammer/planty/internal/judge"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

func TestDailyMissingProviderDefersAndRecoversOnlyFailures(t *testing.T) {
	for _, partial := range []bool{false, true} {
		t.Run(map[bool]string{false: "all unavailable", true: "partial success"}[partial], func(t *testing.T) {
			s, ctx := quotaTestStore(t)
			good := tender(t, s, ctx, "A fern", plant.StewardSelf, 55)
			tender(t, s, ctx, "B fern", plant.StewardSelf, 55)
			tender(t, s, ctx, "C fern", plant.StewardSelf, 55)
			unavailable := store.ModelAssignment{Job: judge.JobAssess, Provider: "codex", Model: judge.AstraModel}
			var calls atomic.Int32
			seat := quotaTestJudge(t, s, func(w http.ResponseWriter, _ *http.Request) {
				if calls.Add(1) == 1 && partial {
					if err := s.SetModelAssignment(ctx, unavailable); err != nil {
						t.Error(err)
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
				}
				successfulAssessmentResponse(w)
			})
			if !partial {
				if err := s.SetModelAssignment(ctx, unavailable); err != nil {
					t.Fatal(err)
				}
			}
			f := newFakeHA(t, weatherEntity)
			daily := Daily{Store: s, Judge: seat, Log: quietLog(), Notifications: f}
			if err := daily.Run(ctx); !errors.Is(err, judge.ErrProviderUnavailable) {
				t.Fatalf("lost provider classification: %v", err)
			}
			wantSucceeded := 0
			var prior plant.Verdict
			if partial {
				wantSucceeded = 1
				var err error
				prior, err = s.LatestVerdict(ctx, good.ID)
				if err != nil {
					t.Fatal(err)
				}
			}
			if calls.Load() != int32(wantSucceeded) || len(f.notified) != 1 {
				t.Fatalf("calls=%d notifications=%d", calls.Load(), len(f.notified))
			}
			run, err := s.LatestJudgmentRun(ctx)
			if err != nil || run.Expected != 3 || run.Succeeded != wantSucceeded || run.Failed != 3-wantSucceeded || run.CompletedAt == nil {
				t.Fatalf("inaccurate coverage: %+v, %v", run, err)
			}
			failures, err := s.FailedJudgments(ctx, run.ID)
			if err != nil || len(failures) != 3-wantSucceeded {
				t.Fatalf("failures=%+v, %v", failures, err)
			}
			for i, failure := range failures {
				wantAttempts := 0
				if i == 0 {
					wantAttempts = 1
				}
				if failure.Attempts != wantAttempts || !strings.Contains(failure.FinalError, "not configured") {
					t.Errorf("failure=%+v, want attempts=%d and configuration reason", failure, wantAttempts)
				}
			}
			if err := daily.RetryFailed(ctx); !errors.Is(err, judge.ErrProviderUnavailable) {
				t.Fatalf("retry lost provider classification: %v", err)
			}
			after, err := s.FailedJudgments(ctx, run.ID)
			if err != nil || len(after) != len(failures) {
				t.Fatalf("failures after retry=%+v, %v", after, err)
			}
			for i, failure := range after {
				wantAttempts := failures[i].Attempts
				if i == 0 {
					wantAttempts++
				}
				if failure.Attempts != wantAttempts {
					t.Errorf("retry changed deferred attempts: %+v", failure)
				}
			}
			if err := s.SetModelAssignment(ctx, store.ModelAssignment{
				Job: judge.JobAssess, Provider: "opencode-go", Model: "qwen3.8-max",
			}); err != nil {
				t.Fatal(err)
			}
			if err := daily.RetryFailed(ctx); err != nil {
				t.Fatal(err)
			}
			finished, err := s.LatestJudgmentRun(ctx)
			if err != nil || finished.ID != run.ID || finished.Succeeded != 3 || finished.Failed != 0 || finished.CompletedAt == nil || calls.Load() != 3 {
				t.Fatalf("failed-only recovery=%+v, calls=%d, error=%v", finished, calls.Load(), err)
			}
			if partial {
				unchanged, err := s.LatestVerdict(ctx, good.ID)
				if err != nil || unchanged.ID != prior.ID {
					t.Fatalf("recovery replaced a successful verdict: %+v, %v", unchanged, err)
				}
			}
		})
	}
}
