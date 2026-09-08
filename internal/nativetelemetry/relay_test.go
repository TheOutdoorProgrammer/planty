package nativetelemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	collectorlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

func fixtureEnvelope() Envelope {
	return Envelope{SchemaVersion: 1, Release: "1.1", Build: "165", Events: []Event{{
		ID: uuid.NewString(), Timestamp: time.Now().UTC(), Operation: "notification.open", Outcome: "success", DurationMS: 12,
	}}}
}

func requestEnvelope(t *testing.T, relay http.Handler, envelope Envelope) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return requestBody(relay, body)
}

func requestBody(relay http.Handler, body []byte) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/v1/native-telemetry", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	relay.ServeHTTP(response, request)
	return response
}

func TestRelayWaitsForCollectorAndPreservesOriginalIdentity(t *testing.T) {
	var mu sync.Mutex
	var tracePayload collectortrace.ExportTraceServiceRequest
	var logPayload collectorlogs.ExportLogsServiceRequest
	allowLogs := make(chan struct{})
	defer func() {
		select {
		case <-allowLogs:
		default:
			close(allowLogs)
		}
	}()
	logsArrived := make(chan struct{})
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/x-protobuf" || r.Header.Get("Authorization") != "" {
			t.Error("collector request must be protobuf without mobile credentials")
		}
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		switch r.URL.Path {
		case "/v1/traces":
			if err := proto.Unmarshal(body, &tracePayload); err != nil {
				t.Error(err)
			}
		case "/v1/logs":
			if err := proto.Unmarshal(body, &logPayload); err != nil {
				t.Error(err)
			}
		default:
			t.Errorf("unexpected signal %s", r.URL.Path)
		}
		mu.Unlock()
		if r.URL.Path == "/v1/logs" {
			close(logsArrived)
			<-allowLogs
		}
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(http.StatusOK)
	}))
	defer collector.Close()
	relay, err := New(collector.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	envelope := fixtureEnvelope()
	envelope.Events[0].Timestamp = time.Now().UTC().Add(-24 * time.Hour)
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- requestEnvelope(t, relay, envelope) }()
	select {
	case <-logsArrived:
	case <-time.After(5 * time.Second):
		close(allowLogs)
		t.Fatal("collector logs did not arrive")
	}
	select {
	case <-done:
		t.Fatal("relay acknowledged before the collector accepted logs")
	default:
	}
	close(allowLogs)
	if response := <-done; response.Code != http.StatusNoContent {
		t.Fatalf("status=%d", response.Code)
	}
	mu.Lock()
	defer mu.Unlock()
	span := tracePayload.ResourceSpans[0].ScopeSpans[0].Spans[0]
	log := logPayload.ResourceLogs[0].ScopeLogs[0].LogRecords[0]
	if span.Name != "notification.open" || log.Body.GetStringValue() != span.Name || !bytes.Equal(span.TraceId, log.TraceId) || !bytes.Equal(span.SpanId, log.SpanId) {
		t.Fatal("native trace and log lost their correlation")
	}
	if span.EndTimeUnixNano != uint64(envelope.Events[0].Timestamp.UnixNano()) || log.TimeUnixNano != span.EndTimeUnixNano {
		t.Fatal("replay timestamp was replaced with receipt time")
	}
	attrs := logPayload.ResourceLogs[0].Resource.Attributes
	if attrs[0].Value.GetStringValue() != "planty-ios" || attrs[1].Value.GetStringValue() != "1.1" || attrs[2].Value.GetStringValue() != "165" {
		t.Fatal("originating app identity was not preserved")
	}
}

func TestCollectorFailureNeverEarnsSuccess(t *testing.T) {
	for _, mode := range []string{"unavailable", "partial-traces", "partial-logs", "redirect", "malformed", "oversize"} {
		t.Run(mode, func(t *testing.T) {
			collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch mode {
				case "unavailable":
					w.WriteHeader(http.StatusServiceUnavailable)
				case "redirect":
					w.Header().Set("Location", "/different")
					w.WriteHeader(http.StatusTemporaryRedirect)
				case "malformed":
					_, _ = w.Write([]byte("invalid protobuf"))
				case "oversize":
					_, _ = w.Write(bytes.Repeat([]byte{0}, 65537))
				case "partial-traces":
					body, _ := proto.Marshal(&collectortrace.ExportTraceServiceResponse{PartialSuccess: &collectortrace.ExportTracePartialSuccess{RejectedSpans: 1}})
					_, _ = w.Write(body)
				case "partial-logs":
					if r.URL.Path == "/v1/logs" {
						body, _ := proto.Marshal(&collectorlogs.ExportLogsServiceResponse{PartialSuccess: &collectorlogs.ExportLogsPartialSuccess{RejectedLogRecords: 1}})
						_, _ = w.Write(body)
					}
				}
			}))
			defer collector.Close()
			relay, _ := New(collector.URL, nil)
			response := requestEnvelope(t, relay, fixtureEnvelope())
			if response.Code != http.StatusServiceUnavailable || response.Header().Get("Retry-After") != "60" {
				t.Fatalf("failed export status=%d", response.Code)
			}
		})
	}
}

func TestNativeEnvelopeRejectsPrivateAndUnboundedInput(t *testing.T) {
	relay, _ := New("http://127.0.0.1:1", nil)
	for name, mutate := range map[string]func(*Envelope){
		"arbitrary release":     func(e *Envelope) { e.Release = "private@example.invalid" },
		"arbitrary build":       func(e *Envelope) { e.Build = "private" },
		"arbitrary operation":   func(e *Envelope) { e.Events[0].Operation = "/v1/plants/private" },
		"raw error":             func(e *Envelope) { e.Events[0].ErrorClass = "private message" },
		"wrong schema":          func(e *Envelope) { e.SchemaVersion = 2 },
		"many events":           func(e *Envelope) { e.Events = make([]Event, 17) },
		"future":                func(e *Envelope) { e.Events[0].Timestamp = time.Now().Add(time.Hour) },
		"stale":                 func(e *Envelope) { e.Events[0].Timestamp = time.Now().Add(-31 * 24 * time.Hour) },
		"persistent identifier": func(e *Envelope) { e.Events[0].ID = "device-id" },
		"missing crash":         func(e *Envelope) { e.Events[0].Operation = "app.crash" },
		"huge duration":         func(e *Envelope) { e.Events[0].DurationMS = 3600001 },
		"same event twice":      func(e *Envelope) { e.Events = append(e.Events, e.Events[0]) },
	} {
		t.Run(name, func(t *testing.T) {
			envelope := fixtureEnvelope()
			mutate(&envelope)
			if response := requestEnvelope(t, relay, envelope); response.Code != http.StatusBadRequest {
				t.Fatalf("invalid envelope status=%d", response.Code)
			}
		})
	}
	body, _ := json.Marshal(fixtureEnvelope())
	for _, payload := range [][]byte{
		append(body, body...),
		bytes.Replace(body, []byte(`"schema_version":1`), []byte(`"schema_version":1,"private":"secret"`), 1),
		bytes.Replace(body, []byte(`"operation":"notification.open"`), []byte(`"operation":"notification.open","notification":{"body":"private"}`), 1),
	} {
		if response := requestBody(relay, payload); response.Code != http.StatusBadRequest {
			t.Fatalf("private or extra JSON accepted: %d", response.Code)
		}
	}
	if response := requestBody(relay, bytes.Repeat([]byte(" "), MaxBodyBytes+1)); response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize status=%d", response.Code)
	}
}

func TestCrashRetainsOnlyBoundedUnresolvedFrames(t *testing.T) {
	envelope := fixtureEnvelope()
	envelope.Events[0].Operation = "app.crash"
	envelope.Events[0].Outcome = "failure"
	envelope.Events[0].ErrorClass = "crash"
	envelope.Events[0].Crash = &Crash{ImageUUID: "a185c3af-a04b-37e6-89cc-f1c53f078adf", Architecture: "arm64", Frames: []Frame{{Offset: 0x328}}}
	if err := envelope.Validate(time.Now()); err != nil {
		t.Fatal(err)
	}
	relay, _ := New("", nil)
	logs, traces := relay.records(context.Background(), envelope)
	wire, _ := proto.Marshal(logs)
	for _, want := range []string{"app.crash", "crash.image.uuid", "0x328", "unresolved"} {
		if !bytes.Contains(wire, []byte(want)) {
			t.Errorf("missing safe field %s", want)
		}
	}
	if bytes.Contains(wire, []byte("exception.stacktrace")) || traces.ResourceSpans[0].ScopeSpans[0].Spans[0].Status.Code != 2 {
		t.Fatal("unresolved crash invented frames or lost error status")
	}
	for _, mutate := range []func(*Crash){
		func(c *Crash) { c.ImageUUID = "../../private" },
		func(c *Crash) { c.ImageUUID = uuid.Nil.String() },
		func(c *Crash) { c.Architecture = "../arm64" },
		func(c *Crash) { c.Frames = make([]Frame, 65) },
		func(c *Crash) { c.Frames[0].Offset = 1 << 40 },
	} {
		crash := *envelope.Events[0].Crash
		crash.Frames = append([]Frame(nil), crash.Frames...)
		mutate(&crash)
		copy := envelope
		copy.Events = append([]Event(nil), envelope.Events...)
		copy.Events[0].Crash = &crash
		if copy.Validate(time.Now()) == nil {
			t.Fatal("invalid crash accepted")
		}
	}
}

func TestRelayBoundsAuthenticatedTrafficAndDisablesCleanly(t *testing.T) {
	relay, _ := New("http://127.0.0.1:1", nil)
	relay.active <- struct{}{}
	relay.active <- struct{}{}
	if response := requestEnvelope(t, relay, fixtureEnvelope()); response.Code != http.StatusTooManyRequests {
		t.Fatal("concurrent request limit did not reject batch")
	}
	<-relay.active
	<-relay.active
	relay.tokens = 0
	if response := requestEnvelope(t, relay, fixtureEnvelope()); response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") != "60" {
		t.Fatal("global event rate limit did not reject batch")
	}
	relay, _ = New("", nil)
	if response := requestEnvelope(t, relay, fixtureEnvelope()); response.Code != http.StatusServiceUnavailable {
		t.Fatal("disabled collector acknowledged events")
	}
	credentialEndpoint := &url.URL{Scheme: "https", Host: "example.invalid", User: url.UserPassword("fixture", "fixture")}
	for _, endpoint := range []string{credentialEndpoint.String(), "https://example.invalid?token=private", "file:///tmp/test"} {
		if _, err := New(endpoint, nil); err == nil {
			t.Fatalf("unsafe endpoint accepted: %s", strings.Split(endpoint, ":")[0])
		}
	}
}
