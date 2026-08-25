package scheduledjob

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
)

func TestDefinitionsCoverEveryCronJobManifest(t *testing.T) {
	manifest, err := os.ReadFile("../../deploy/cronjobs.yaml")
	if err != nil {
		t.Fatal(err)
	}
	matches := regexp.MustCompile(`(?m)^  name: (planty-[a-z0-9-]+)$`).FindAllSubmatch(manifest, -1)
	manifestNames := make([]string, 0, len(matches))
	for _, match := range matches {
		manifestNames = append(manifestNames, string(match[1]))
	}
	definitionNames := make([]string, 0, len(Definitions()))
	for _, definition := range Definitions() {
		definitionNames = append(definitionNames, definition.CronJob)
	}
	sort.Strings(manifestNames)
	sort.Strings(definitionNames)
	if strings.Join(definitionNames, "\n") != strings.Join(manifestNames, "\n") {
		t.Fatalf("definition CronJobs = %v, manifest CronJobs = %v", definitionNames, manifestNames)
	}
}

func TestUnknownJobNeverReachesKubernetes(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests++
	}))
	defer server.Close()
	client := newTestClient(t, server)

	if _, _, err := client.Start(context.Background(), ID("shell-anything")); err == nil {
		t.Fatal("an arbitrary job was accepted")
	}
	if requests != 0 {
		t.Fatalf("unknown job made %d Kubernetes requests", requests)
	}
}

func TestStartCopiesTheCronJobTemplateAndBoundsItsLifetime(t *testing.T) {
	var mu sync.Mutex
	var created jobRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("authorization header = %q", r.Header.Get("Authorization"))
		}
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/cronjobs/planty-daily"):
			writeJSON(t, w, map[string]any{
				"metadata": map[string]any{"name": "planty-daily", "namespace": "planty"},
				"spec": map[string]any{
					"schedule": "0 8 * * *",
					"jobTemplate": map[string]any{
						"metadata": map[string]any{"labels": map[string]string{"template": "daily"}},
						"spec":     map[string]any{"backoffLimit": 1, "template": map[string]any{"spec": map[string]any{"restartPolicy": "Never"}}},
					},
				},
				"status": map[string]any{"active": []any{}},
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/jobs"):
			writeJSON(t, w, map[string]any{"items": []any{}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/jobs"):
			mu.Lock()
			defer mu.Unlock()
			if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
				t.Fatalf("decode created job: %v", err)
			}
			writeJSON(t, w, map[string]any{
				"metadata": map[string]any{
					"name":   "planty-manual-daily-abcde",
					"labels": map[string]string{managedByLabel: managedByValue, jobIDLabel: "daily"},
				},
				"status": map[string]any{"active": 1},
			})
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := newTestClient(t, server)

	run, wasCreated, err := client.Start(context.Background(), Daily)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if !wasCreated || run.Job != Daily || run.State != Running {
		t.Fatalf("created run = %#v, created=%v", run, wasCreated)
	}
	mu.Lock()
	defer mu.Unlock()
	if created.Metadata.GenerateName != "planty-manual-daily-" {
		t.Errorf("generateName = %q", created.Metadata.GenerateName)
	}
	if created.Metadata.Labels[managedByLabel] != managedByValue ||
		created.Metadata.Labels[jobIDLabel] != "daily" || created.Metadata.Labels["template"] != "daily" {
		t.Errorf("labels = %#v", created.Metadata.Labels)
	}
	if created.Spec["ttlSecondsAfterFinished"] != float64(manualRunTTLSeconds) {
		t.Errorf("TTL = %#v", created.Spec["ttlSecondsAfterFinished"])
	}
	if created.Spec["backoffLimit"] != float64(1) {
		t.Errorf("CronJob spec was not copied: %#v", created.Spec)
	}
}

func TestScheduledActiveRunPreventsADuplicate(t *testing.T) {
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/cronjobs/planty-daily"):
			writeJSON(t, w, map[string]any{
				"metadata": map[string]any{"name": "planty-daily", "namespace": "planty"},
				"spec":     map[string]any{"jobTemplate": map[string]any{"spec": map[string]any{"template": map[string]any{}}}},
				"status":   map[string]any{"active": []map[string]string{{"name": "planty-daily-123"}}},
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/jobs/planty-daily-123"):
			writeJSON(t, w, map[string]any{
				"metadata": map[string]any{"name": "planty-daily-123"},
				"status":   map[string]any{"active": 1},
			})
		case r.Method == http.MethodPost:
			posts++
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := newTestClient(t, server)

	run, created, err := client.Start(context.Background(), Daily)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if created || posts != 0 || run.ID != "planty-daily-123" || run.State != Running {
		t.Fatalf("duplicate guard = %#v, created=%v, posts=%d", run, created, posts)
	}
}

func newTestClient(t *testing.T, server *httptest.Server) *Kubernetes {
	t.Helper()
	client, err := NewKubernetes(KubernetesConfig{
		BaseURL: server.URL, Namespace: "planty", Token: "test-token", Client: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func writeJSON(t *testing.T, w http.ResponseWriter, body any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(body); err != nil {
		t.Fatal(err)
	}
}
