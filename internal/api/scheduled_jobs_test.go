package api_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/TheOutdoorProgrammer/planty/internal/api"
	"github.com/TheOutdoorProgrammer/planty/internal/scheduledjob"
)

type fakeScheduledJobs struct {
	jobs    []scheduledjob.Scheduled
	run     scheduledjob.Run
	created bool
	starts  []scheduledjob.ID
}

func (f *fakeScheduledJobs) List(context.Context) ([]scheduledjob.Scheduled, error) {
	return f.jobs, nil
}

func (f *fakeScheduledJobs) Start(_ context.Context, id scheduledjob.ID) (scheduledjob.Run, bool, error) {
	f.starts = append(f.starts, id)
	return f.run, f.created, nil
}

func TestScheduledJobsAreUnavailableWithoutKubernetesAccess(t *testing.T) {
	h, _, _ := newServer(t)

	rec, body := do(t, h, http.MethodGet, "/v1/scheduled-jobs", nil)
	if rec.Code != http.StatusServiceUnavailable || body["code"] != "service_unavailable" {
		t.Fatalf("unavailable response = %d %#v", rec.Code, body)
	}
}

func TestScheduledJobsListAndStartThroughTheAllowlist(t *testing.T) {
	_, db, _ := newServer(t)
	fake := &fakeScheduledJobs{
		jobs: []scheduledjob.Scheduled{{
			ID: scheduledjob.Daily, Name: "Check every plant", Category: "Care",
			Purpose: "Create a fresh assessment.", Schedule: "0 8 * * *", TimeZone: "America/New_York",
		}},
		run:     scheduledjob.Run{ID: "planty-manual-daily-test", Job: scheduledjob.Daily, State: scheduledjob.Queued},
		created: true,
	}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := api.New(db, quiet).WithScheduledJobs(fake).Handler()

	listed, body := do(t, h, http.MethodGet, "/v1/scheduled-jobs", nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("list status = %d, body %s", listed.Code, listed.Body.String())
	}
	jobs, _ := body["jobs"].([]any)
	if len(jobs) != 1 {
		t.Fatalf("jobs = %#v", jobs)
	}

	started, run := do(t, h, http.MethodPost, "/v1/scheduled-jobs/daily/runs", nil)
	if started.Code != http.StatusAccepted || run["job"] != "daily" || run["state"] != "queued" {
		t.Fatalf("start response = %d %#v", started.Code, run)
	}
	if len(fake.starts) != 1 || fake.starts[0] != scheduledjob.Daily {
		t.Fatalf("started = %#v", fake.starts)
	}

	unknown, _ := do(t, h, http.MethodPost, "/v1/scheduled-jobs/kubectl/runs", nil)
	if unknown.Code != http.StatusNotFound || len(fake.starts) != 1 {
		t.Fatalf("unknown job = %d, starts %#v", unknown.Code, fake.starts)
	}
}

func TestRepeatedStartReturnsTheExistingRun(t *testing.T) {
	_, db, _ := newServer(t)
	fake := &fakeScheduledJobs{
		run:     scheduledjob.Run{ID: "planty-daily-active", Job: scheduledjob.Daily, State: scheduledjob.Running},
		created: false,
	}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := api.New(db, quiet).WithScheduledJobs(fake).Handler()

	rec, body := do(t, h, http.MethodPost, "/v1/scheduled-jobs/daily/runs", nil)
	if rec.Code != http.StatusOK || body["id"] != "planty-daily-active" {
		t.Fatalf("existing run response = %d %#v", rec.Code, body)
	}
}
