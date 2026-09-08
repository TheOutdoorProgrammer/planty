package telemetry

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestStartWithoutEndpointLeavesProviderAlone(t *testing.T) {
	t.Setenv("OTEL_SDK_DISABLED", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	before := otel.GetTracerProvider()
	shutdown, err := Start(t.Context(), "planty", "test", slog.Default())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := shutdown(t.Context()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if otel.GetTracerProvider() != before {
		t.Fatal("Start changed the global provider without an OTLP endpoint")
	}
}

func TestLogHandlerAddsTraceContext(t *testing.T) {
	var output bytes.Buffer
	log := slog.New(LogHandler(slog.NewJSONHandler(&output, nil)))
	span := trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}})
	log.InfoContext(trace.ContextWithSpanContext(context.Background(), span), "correlated")
	for _, want := range []string{`"trace_id":"01000000000000000000000000000000"`, `"span_id":"0200000000000000"`} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("log output %q does not contain %q", output.String(), want)
		}
	}
}

func TestFailedJobRecordsSafeStatusAndCorrelatesLegacyLogs(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(previous) })
	ctx, finish := StartJob(context.Background(), "daily")
	var output bytes.Buffer
	logger := WithContext(ctx, slog.New(LogHandler(slog.NewJSONHandler(&output, nil))))
	logger.With("operation", "synthetic").Error("synthetic failure")
	finish(errors.New("private payload must not enter span"))
	spans := recorder.Ended()
	if len(spans) != 1 || spans[0].Name() != "planty.job.daily" || spans[0].Status().Code != codes.Error {
		t.Fatalf("job failure was not recorded: %v", spans)
	}
	if !strings.Contains(output.String(), spans[0].SpanContext().TraceID().String()) {
		t.Fatal("contextless job log lost its enclosing trace")
	}
	if len(spans[0].Events()) != 0 || strings.Contains(spans[0].Status().Description, "private") {
		t.Fatal("private exception detail entered the job span")
	}
	_, finish = StartJob(context.Background(), "private-unrecognized-command")
	finish(nil)
	if recorder.Ended()[1].Name() != "planty.job.unknown" {
		t.Fatal("unrecognized command created an unbounded span name")
	}
}
