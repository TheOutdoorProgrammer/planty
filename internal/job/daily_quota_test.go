package job

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/TheOutdoorProgrammer/planty/internal/judge"
	"github.com/TheOutdoorProgrammer/planty/internal/pgtest"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

func quotaTestStore(t *testing.T) (*store.Store, context.Context) {
	t.Helper()
	ctx := context.Background()
	dsn := pgtest.DSN(t)
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := pgx.Identifier{"quota_" + strings.ReplaceAll(uuid.NewString(), "-", "")}.Sanitize()
	if _, err := conn.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		_ = conn.Close(ctx)
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		_ = conn.Close(ctx)
	})
	address, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := address.Query()
	query.Set("search_path", schema)
	address.RawQuery = query.Encode()
	db, err := store.Open(ctx, address.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return db, ctx
}

func quotaTestJudge(t *testing.T, s *store.Store, handler http.HandlerFunc) *judge.Judge {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("PLANTY_JUDGE", "api")
	t.Setenv("PLANTY_TEST_QUOTA_KEY", "test-key")
	providers, err := json.Marshal([]judge.Provider{{
		ID: "opencode-go", Kind: judge.KindOpenAI, BaseURL: server.URL,
		APIKeyEnv: "PLANTY_TEST_QUOTA_KEY",
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PLANTY_PROVIDERS", string(providers))
	for _, job := range []judge.Job{judge.JobAssess, judge.JobPostmortem} {
		if err := s.SetModelAssignment(context.Background(), store.ModelAssignment{
			Job: job, Provider: "opencode-go", Model: "qwen3.8-max",
		}); err != nil {
			t.Fatal(err)
		}
	}
	seat := judge.New().Assigned(s)
	if seat == nil {
		t.Fatal("test provider was not configured")
	}
	return seat
}

func quotaResponse(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	_, _ = w.Write([]byte(`{"error":{"message":"Go usage limit exceeded"}}`))
}

func successfulAssessmentResponse(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"choices": []any{map[string]any{"message": map[string]any{
			"role":    "assistant",
			"content": `{"action":"none","reasoning":"Wait today.","confidence":0.8,"sensor_summary":"No new concerns.","health_mode":"unchanged","health_value":0,"health_reasoning":"No new evidence."}`,
		}}},
	})
}

func TestDailyQuotaExhaustionDefersPlantsAndNotifies(t *testing.T) {
	s, ctx := quotaTestStore(t)
	plants := []plant.Plant{
		tender(t, s, ctx, "A fern", plant.StewardSelf, 55),
		tender(t, s, ctx, "B fern", plant.StewardSelf, 55),
		tender(t, s, ctx, "C fern", plant.StewardSelf, 55),
	}
	dead := tender(t, s, ctx, "Dead fern", plant.StewardSelf, 55)
	if err := s.ArchivePlant(ctx, dead.Slug, plant.StatusDead); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	seat := quotaTestJudge(t, s, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		quotaResponse(w)
	})
	f := newFakeHA(t, weatherEntity)
	err := (Daily{Store: s, Judge: seat, Log: quietLog(), Notifications: f}).Run(ctx)
	if !errors.Is(err, judge.ErrQuotaExhausted) {
		t.Fatalf("complete assessment failure lost quota classification: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("upstream calls = %d, want one including postmortems", calls.Load())
	}
	run, err := s.LatestJudgmentRun(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if run.Expected != 3 || run.Succeeded != 0 || run.Failed != 3 || run.CompletedAt == nil {
		t.Fatalf("inaccurate quota coverage: %+v", run)
	}
	failures, err := s.FailedJudgments(ctx, run.ID)
	if err != nil || len(failures) != 3 {
		t.Fatalf("failed plants = %+v, error = %v", failures, err)
	}
	for _, failure := range failures {
		wantAttempts := 0
		if failure.Plant.ID == plants[0].ID {
			wantAttempts = 1
		}
		if failure.Attempts != wantAttempts || failure.FinalError == "" {
			t.Errorf("failure = %+v, want %d actual requests and an explanation", failure, wantAttempts)
		}
	}
	for _, subject := range plants {
		if _, err := s.LatestVerdict(ctx, subject.ID); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("quota failure wrote a verdict: %v", err)
		}
		if _, err := s.LatestHealth(ctx, subject.ID); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("quota failure wrote health: %v", err)
		}
	}
	if len(f.notified) != 1 || !f.said("checked 0 of 3 plants; 3 failed") {
		t.Fatalf("incomplete-check push = %+v", f.notified)
	}
	digest, err := s.ReliableDigest(ctx, plant.StaleAfter)
	if err != nil || digest.AllClear() {
		t.Fatalf("quota outage became all clear: %+v, %v", digest, err)
	}
}

func TestDailyQuotaRetryPreservesSuccessAndRecovers(t *testing.T) {
	s, ctx := quotaTestStore(t)
	good := tender(t, s, ctx, "A healthy fern", plant.StewardSelf, 55)
	tender(t, s, ctx, "B deferred fern", plant.StewardSelf, 55)
	tender(t, s, ctx, "C deferred fern", plant.StewardSelf, 55)
	var calls atomic.Int32
	var recovered atomic.Bool
	seat := quotaTestJudge(t, s, func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 || recovered.Load() {
			successfulAssessmentResponse(w)
			return
		}
		quotaResponse(w)
	})
	f := newFakeHA(t, weatherEntity)
	daily := Daily{Store: s, Judge: seat, Log: quietLog(), Notifications: f}
	if err := daily.Run(ctx); !errors.Is(err, judge.ErrQuotaExhausted) {
		t.Fatalf("partial assessment failure lost quota classification: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("partial run calls = %d, want 2", calls.Load())
	}
	before, err := s.LatestVerdict(ctx, good.ID)
	if err != nil {
		t.Fatal(err)
	}
	run, err := s.LatestJudgmentRun(ctx)
	if err != nil || run.Succeeded != 1 || run.Failed != 2 || run.CompletedAt == nil {
		t.Fatalf("partial run = %+v, %v", run, err)
	}
	prior, err := s.FailedJudgments(ctx, run.ID)
	if err != nil || len(prior) != 2 {
		t.Fatalf("prior failures = %+v, %v", prior, err)
	}
	if _, _, err := s.BeginLatestJudgmentRetry(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordJudgmentPlantResult(ctx, run.ID, store.JudgmentResultInput{
		PlantID: prior[1].Plant.ID, Attempts: 2, Model: "prior-model",
		OriginalError: "invalid JSON", OriginalOutput: "{", FinalError: "repair failed",
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteJudgmentRun(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	prior, err = s.FailedJudgments(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := daily.RetryFailed(ctx); !errors.Is(err, judge.ErrQuotaExhausted) {
		t.Fatalf("exhausted quota retry lost classification: %v", err)
	}
	if calls.Load() != 3 {
		t.Fatalf("retry calls = %d, want one additional attempt", calls.Load())
	}
	after, err := s.FailedJudgments(ctx, run.ID)
	if err != nil || len(after) != 2 {
		t.Fatalf("failures after retry = %+v, %v", after, err)
	}
	for i, failure := range after {
		wantAttempts := prior[i].Attempts
		if i == 0 {
			wantAttempts++
		}
		if failure.Plant.ID != prior[i].Plant.ID || failure.Attempts != wantAttempts {
			t.Errorf("retry changed unattempted work: before=%+v after=%+v", prior[i], failure)
		}
		if i == 1 && (failure.Model != prior[i].Model || failure.OriginalError != prior[i].OriginalError ||
			failure.OriginalOutput != prior[i].OriginalOutput) {
			t.Errorf("deferral erased prior diagnostic evidence: before=%+v after=%+v", prior[i], failure)
		}
	}
	recovered.Store(true)
	if err := daily.RetryFailed(ctx); err != nil {
		t.Fatalf("recovered provider could not retry: %v", err)
	}
	if calls.Load() != 5 {
		t.Fatalf("recovery calls = %d, want exactly the two failed plants", calls.Load())
	}
	finished, err := s.LatestJudgmentRun(ctx)
	if err != nil || finished.ID != run.ID || finished.Succeeded != 3 || finished.Failed != 0 || finished.CompletedAt == nil {
		t.Fatalf("recovered run = %+v, %v", finished, err)
	}
	unchanged, err := s.LatestVerdict(ctx, good.ID)
	if err != nil || unchanged.ID != before.ID {
		t.Fatalf("successful judgment was rewritten: %+v, %v", unchanged, err)
	}
}

func TestDailyTransientProviderFailuresContinueToOtherPlants(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusBadGateway} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			s, ctx := quotaTestStore(t)
			tender(t, s, ctx, "A unavailable fern", plant.StewardSelf, 55)
			tender(t, s, ctx, "B healthy fern", plant.StewardSelf, 55)
			var calls atomic.Int32
			seat := quotaTestJudge(t, s, func(w http.ResponseWriter, _ *http.Request) {
				if calls.Add(1) == 1 {
					w.WriteHeader(status)
					_, _ = w.Write([]byte(`{"error":{"message":"temporarily unavailable"}}`))
					return
				}
				successfulAssessmentResponse(w)
			})
			f := newFakeHA(t, weatherEntity)
			_ = (Daily{Store: s, Judge: seat, Log: quietLog(), Notifications: f}).Run(ctx)
			if calls.Load() != 2 {
				t.Fatalf("ordinary provider failure stopped remaining plants: %d calls", calls.Load())
			}
			run, err := s.LatestJudgmentRun(ctx)
			if err != nil || run.Succeeded != 1 || run.Failed != 1 || run.CompletedAt == nil {
				t.Fatalf("transient failure coverage = %+v, %v", run, err)
			}
		})
	}
}

type quotaFailingNotifier struct{ err error }

func (n quotaFailingNotifier) Send(context.Context, string, string, map[string]any) error {
	return n.err
}

func TestDailyQuotaFailureRetainsNotificationFailure(t *testing.T) {
	s, ctx := quotaTestStore(t)
	tender(t, s, ctx, "Unavailable fern", plant.StewardSelf, 55)
	seat := quotaTestJudge(t, s, func(w http.ResponseWriter, _ *http.Request) { quotaResponse(w) })
	pushErr := errors.New("push unavailable")
	err := (Daily{
		Store: s, Judge: seat, Log: quietLog(), Notifications: quotaFailingNotifier{err: pushErr},
	}).Run(ctx)
	if !errors.Is(err, pushErr) || !errors.Is(err, judge.ErrQuotaExhausted) || !strings.Contains(err.Error(), "failed judgment") {
		t.Fatalf("assessment or notification error was lost: %v", err)
	}
}
