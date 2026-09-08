package nativetelemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	collectorlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	common "go.opentelemetry.io/proto/otlp/common/v1"
	logs "go.opentelemetry.io/proto/otlp/logs/v1"
	resource "go.opentelemetry.io/proto/otlp/resource/v1"
	trace "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

const MaxBodyBytes = 64 << 10

type Envelope struct {
	SchemaVersion int     `json:"schema_version"`
	Release       string  `json:"release"`
	Build         string  `json:"build"`
	Events        []Event `json:"events"`
}

type Event struct {
	ID         string    `json:"id"`
	Timestamp  time.Time `json:"timestamp"`
	Operation  string    `json:"operation"`
	Outcome    string    `json:"outcome"`
	DurationMS int64     `json:"duration_ms"`
	ErrorClass string    `json:"error_class,omitempty"`
	Crash      *Crash    `json:"crash,omitempty"`
}

type Crash struct {
	ImageUUID    string  `json:"image_uuid"`
	Architecture string  `json:"architecture"`
	Frames       []Frame `json:"frames"`
}

type Frame struct {
	Offset uint64 `json:"offset"`
}

type ResolvedFrame struct {
	Function string
	File     string
	Line     int64
}

type SymbolResolver interface {
	Resolve(context.Context, Crash) ([]ResolvedFrame, error)
}

type Relay struct {
	endpoint string
	client   *http.Client
	symbols  SymbolResolver
	active   chan struct{}
	mu       sync.Mutex
	tokens   float64
	updated  time.Time
}

func New(endpoint string, symbols SymbolResolver) (*Relay, error) {
	if endpoint != "" {
		u, err := url.Parse(endpoint)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return nil, errors.New("invalid native telemetry collector endpoint")
		}
	}
	return &Relay{
		endpoint: strings.TrimRight(endpoint, "/"), symbols: symbols,
		client: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		active: make(chan struct{}, 2), tokens: 60, updated: time.Now(),
	}, nil
}

var releasePattern = regexp.MustCompile(`^[0-9]{1,4}(\.[0-9]{1,4}){0,3}$`)
var buildPattern = regexp.MustCompile(`^[0-9]{1,12}$`)

func (e Envelope) Validate(now time.Time) error {
	invalid := errors.New("invalid native telemetry envelope")
	if e.SchemaVersion != 1 || !releasePattern.MatchString(e.Release) || !buildPattern.MatchString(e.Build) || len(e.Events) == 0 || len(e.Events) > 16 {
		return invalid
	}
	seen := make(map[uuid.UUID]bool, len(e.Events))
	for _, event := range e.Events {
		id, err := uuid.Parse(event.ID)
		if err != nil || len(event.ID) != 36 || id.Version() != 4 || id.Variant() != uuid.RFC4122 || seen[id] || event.Timestamp.Before(now.Add(-30*24*time.Hour)) || event.Timestamp.After(now.Add(5*time.Minute)) || event.DurationMS < 0 || event.DurationMS > 3600000 {
			return invalid
		}
		seen[id] = true
		switch event.Operation {
		case "app.start", "notification.open", "api.request", "app.crash", "app.hang", "telemetry.delivery":
		default:
			return invalid
		}
		switch event.Outcome {
		case "success", "failure", "cancelled":
		default:
			return invalid
		}
		switch event.ErrorClass {
		case "", "transport", "timeout", "unauthorized", "server", "decoding", "crash", "hang", "other", "queue_full", "queue_expired":
		default:
			return invalid
		}
		if (event.Outcome == "failure") != (event.ErrorClass != "") {
			return invalid
		}
		queueLoss := event.ErrorClass == "queue_full" || event.ErrorClass == "queue_expired"
		if (event.Operation == "telemetry.delivery") != queueLoss {
			return invalid
		}
		if event.Operation == "app.crash" || event.Operation == "app.hang" {
			if event.Crash == nil || event.Outcome != "failure" || event.ErrorClass != strings.TrimPrefix(event.Operation, "app.") {
				return invalid
			}
		} else if event.Crash != nil {
			return invalid
		}
		if event.Crash != nil {
			id, err := uuid.Parse(event.Crash.ImageUUID)
			if err != nil || len(event.Crash.ImageUUID) != 36 || id == uuid.Nil || len(event.Crash.Frames) == 0 || len(event.Crash.Frames) > 64 {
				return invalid
			}
			switch event.Crash.Architecture {
			case "arm64", "arm64e", "x86_64":
			default:
				return invalid
			}
			for _, frame := range event.Crash.Frames {
				if frame.Offset >= 1<<40 {
					return invalid
				}
			}
		}
	}
	return nil
}

func (r *Relay) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		w.WriteHeader(http.StatusUnsupportedMediaType)
		return
	}
	if r.endpoint == "" {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	select {
	case r.active <- struct{}{}:
		defer func() { <-r.active }()
	default:
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	request.Body = http.MaxBytesReader(w, request.Body, MaxBodyBytes)
	defer func() { _ = request.Body.Close() }()
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Now().Add(10 * time.Second))
	body, err := io.ReadAll(request.Body)
	_ = controller.SetReadDeadline(time.Time{})
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var envelope Envelope
	if decoder.Decode(&envelope) != nil || decoder.Decode(new(any)) != io.EOF || envelope.Validate(time.Now()) != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if !r.allow(len(envelope.Events)) {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 30*time.Second)
	defer cancel()
	logRequest, traceRequest := r.records(ctx, envelope)
	if r.forward(ctx, "traces", traceRequest, new(collectortrace.ExportTraceServiceResponse)) != nil || r.forward(ctx, "logs", logRequest, new(collectorlogs.ExportLogsServiceResponse)) != nil {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (r *Relay) allow(count int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	r.tokens = min(60, r.tokens+now.Sub(r.updated).Seconds())
	r.updated = now
	if r.tokens < float64(count) {
		return false
	}
	r.tokens -= float64(count)
	return true
}

func (r *Relay) forward(ctx context.Context, signal string, payload, responseMessage proto.Message) error {
	body, err := proto.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint+"/v1/"+signal, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-protobuf")
	response, err := r.client.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	body, err = io.ReadAll(io.LimitReader(response.Body, 64<<10+1))
	if err != nil || len(body) > 64<<10 || response.StatusCode != http.StatusOK || proto.Unmarshal(body, responseMessage) != nil {
		return errors.New("collector did not acknowledge native telemetry")
	}
	switch message := responseMessage.(type) {
	case *collectortrace.ExportTraceServiceResponse:
		if message.GetPartialSuccess().GetRejectedSpans() != 0 || message.GetPartialSuccess().GetErrorMessage() != "" {
			return errors.New("collector partially rejected native traces")
		}
	case *collectorlogs.ExportLogsServiceResponse:
		if message.GetPartialSuccess().GetRejectedLogRecords() != 0 || message.GetPartialSuccess().GetErrorMessage() != "" {
			return errors.New("collector partially rejected native logs")
		}
	}
	return nil
}

func textAttribute(key, value string) *common.KeyValue {
	return &common.KeyValue{Key: key, Value: &common.AnyValue{Value: &common.AnyValue_StringValue{StringValue: value}}}
}

func (r *Relay) records(ctx context.Context, envelope Envelope) (*collectorlogs.ExportLogsServiceRequest, *collectortrace.ExportTraceServiceRequest) {
	res := &resource.Resource{Attributes: []*common.KeyValue{
		textAttribute("service.name", "planty-ios"), textAttribute("service.version", envelope.Release), textAttribute("service.build", envelope.Build),
	}}
	logScope := &logs.ScopeLogs{Scope: &common.InstrumentationScope{Name: "planty/native"}}
	traceScope := &trace.ScopeSpans{Scope: &common.InstrumentationScope{Name: "planty/native"}}
	for _, event := range envelope.Events {
		id := uuid.MustParse(event.ID)
		spanID := id[8:]
		attributes := []*common.KeyValue{textAttribute("operation", event.Operation), textAttribute("outcome", event.Outcome)}
		status := &trace.Status{Code: trace.Status_STATUS_CODE_OK}
		severity := logs.SeverityNumber_SEVERITY_NUMBER_INFO
		if event.Outcome == "failure" {
			status.Code = trace.Status_STATUS_CODE_ERROR
			severity = logs.SeverityNumber_SEVERITY_NUMBER_ERROR
			attributes = append(attributes, textAttribute("error.type", event.ErrorClass))
		}
		if event.Crash != nil {
			attributes = append(attributes, r.crashAttributes(ctx, *event.Crash)...)
		}
		end := uint64(event.Timestamp.UnixNano())
		start := uint64(event.Timestamp.Add(-time.Duration(event.DurationMS) * time.Millisecond).UnixNano())
		logScope.LogRecords = append(logScope.LogRecords, &logs.LogRecord{
			TimeUnixNano: end, ObservedTimeUnixNano: uint64(time.Now().UnixNano()), SeverityNumber: severity,
			Body: &common.AnyValue{Value: &common.AnyValue_StringValue{StringValue: event.Operation}}, Attributes: attributes, TraceId: id[:], SpanId: spanID, Flags: 1,
		})
		traceScope.Spans = append(traceScope.Spans, &trace.Span{
			TraceId: id[:], SpanId: spanID, Flags: 1, Name: event.Operation, Kind: trace.Span_SPAN_KIND_INTERNAL,
			StartTimeUnixNano: start, EndTimeUnixNano: end, Attributes: attributes, Status: status,
		})
	}
	return &collectorlogs.ExportLogsServiceRequest{ResourceLogs: []*logs.ResourceLogs{{Resource: res, ScopeLogs: []*logs.ScopeLogs{logScope}}}},
		&collectortrace.ExportTraceServiceRequest{ResourceSpans: []*trace.ResourceSpans{{Resource: res, ScopeSpans: []*trace.ScopeSpans{traceScope}}}}
}
