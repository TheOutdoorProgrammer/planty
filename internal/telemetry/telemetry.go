// Package telemetry configures Planty's OpenTelemetry runtime.
package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type Shutdown func(context.Context) error

func Start(ctx context.Context, serviceName, serviceVersion string, log *slog.Logger) (Shutdown, error) {
	if !configured() {
		return func(context.Context) error { return nil }, nil
	}
	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, err
	}
	res, err := resource.New(ctx,
		resource.WithTelemetrySDK(),
		resource.WithAttributes(attribute.String("service.name", serviceName), attribute.String("service.version", serviceVersion)),
		resource.WithFromEnv(),
	)
	if err != nil {
		_ = exporter.Shutdown(ctx)
		return nil, err
	}
	provider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter), sdktrace.WithResource(res))
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) { log.Error("telemetry pipeline", "error", err) }))
	return provider.Shutdown, nil
}

func HTTPHandler(next http.Handler, operation string) http.Handler {
	return otelhttp.NewHandler(next, operation)
}

func LogHandler(next slog.Handler) slog.Handler { return traceHandler{Handler: next} }

func StartJob(ctx context.Context, command string) (context.Context, func(error)) {
	switch command {
	case "ingest", "verify-water", "reconcile-actuators", "prune-photos", "daily", "retry", "cold", "away", "chase", "remind", "thirst", "water", "autopsy", "agent", "seed", "migrate":
	default:
		command = "unknown"
	}
	ctx, span := otel.Tracer("planty/jobs").Start(ctx, "planty.job."+command)
	return ctx, func(err error) {
		if err != nil {
			span.SetStatus(codes.Error, "job failed")
			span.SetAttributes(attribute.String("error.type", fmt.Sprintf("%T", err)))
		}
		span.End()
	}
}

func WithContext(ctx context.Context, logger *slog.Logger) *slog.Logger {
	return slog.New(contextHandler{Handler: logger.Handler(), context: ctx})
}

type contextHandler struct {
	slog.Handler
	context context.Context
}

func (handler contextHandler) Handle(ctx context.Context, record slog.Record) error {
	if !trace.SpanContextFromContext(ctx).IsValid() {
		ctx = handler.context
	}
	return handler.Handler.Handle(ctx, record)
}

func (handler contextHandler) WithAttrs(attributes []slog.Attr) slog.Handler {
	return contextHandler{Handler: handler.Handler.WithAttrs(attributes), context: handler.context}
}

func (handler contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{Handler: handler.Handler.WithGroup(name), context: handler.context}
}

func configured() bool {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("OTEL_SDK_DISABLED")), "true") {
		return false
	}
	return strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")) != "" || strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")) != ""
}

type traceHandler struct{ slog.Handler }

func (h traceHandler) Handle(ctx context.Context, record slog.Record) error {
	span := trace.SpanContextFromContext(ctx)
	if span.IsValid() {
		record.AddAttrs(slog.String("trace_id", span.TraceID().String()), slog.String("span_id", span.SpanID().String()))
	}
	return h.Handler.Handle(ctx, record)
}

func (h traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return traceHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h traceHandler) WithGroup(name string) slog.Handler {
	return traceHandler{Handler: h.Handler.WithGroup(name)}
}
