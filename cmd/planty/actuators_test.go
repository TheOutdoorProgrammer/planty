package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strings"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type actuatorReconcileFunc func(context.Context, time.Time) (int, error)

func (f actuatorReconcileFunc) Reconcile(ctx context.Context, now time.Time) (int, error) {
	return f(ctx, now)
}

func refusedActuatorRequest() error {
	return &url.Error{Op: "Post", URL: "https://home.example.com/api/services/switch/turn_off", Err: &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}}
}

func TestActuatorJobRecoversAfterRestartAndRefreshesIntent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		var logs bytes.Buffer
		recorder := tracetest.NewSpanRecorder()
		provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
		previous := otel.GetTracerProvider()
		otel.SetTracerProvider(provider)
		t.Cleanup(func() { otel.SetTracerProvider(previous) })
		ctx, finish := telemetry.StartJob(t.Context(), "reconcile-actuators")
		attempts, stopped := 0, false
		control := actuatorReconcileFunc(func(ctx context.Context, now time.Time) (int, error) {
			attempts++
			if now != time.Now().UTC() {
				t.Fatal("reconciliation reused the pre-restart schedule time")
			}
			if now.Before(start.Add(45 * time.Second)) {
				return 0, errors.Join(fmt.Errorf("light: %w", refusedActuatorRequest()), fmt.Errorf("fan: %w", refusedActuatorRequest()))
			}
			if now.Before(start.Add(time.Minute)) {
				return 0, errors.New("home assistant POST /api/services/switch/turn_off: 400 Bad Request")
			}
			stopped = true
			return 2, nil
		})
		err := runActuatorReconciliation(ctx, control, slog.New(telemetry.LogHandler(slog.NewJSONHandler(&logs, nil))))
		finish(err)
		if err != nil || !stopped || attempts != 5 {
			t.Fatalf("recovery err=%v stopped=%t attempts=%d", err, stopped, attempts)
		}
		if got := strings.Count(logs.String(), `"level":"ERROR"`); got != 4 {
			t.Fatalf("failed attempts must remain visible, got %d error records: %s", got, &logs)
		}
		if strings.Contains(logs.String(), "home.example.com") {
			t.Fatal("attempt log exposed the dependency URL")
		}
		spans := recorder.Ended()
		if len(spans) != 6 {
			t.Fatalf("expected job and five attempt spans, got %d", len(spans))
		}
		for _, span := range spans[:4] {
			if span.Status().Code != codes.Error || !span.Parent().Equal(spans[5].SpanContext()) {
				t.Fatalf("failed attempt lost error status or job parent: %v", span)
			}
			if !strings.Contains(logs.String(), span.SpanContext().SpanID().String()) {
				t.Fatal("failed attempt log lost span correlation")
			}
		}
		if spans[4].Status().Code == codes.Error || spans[5].Status().Code == codes.Error {
			t.Fatal("completed reconciliation or recovered job marked failed")
		}
	})
}

func TestActuatorJobFailsWhenDependencyDoesNotRecover(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		attempts := 0
		failure := refusedActuatorRequest()
		control := actuatorReconcileFunc(func(ctx context.Context, now time.Time) (int, error) {
			attempts++
			return 0, failure
		})
		err := runActuatorReconciliation(t.Context(), control, slog.New(slog.DiscardHandler))
		if !errors.Is(err, failure) || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("must preserve dependency failure and exhausted deadline: %v", err)
		}
		if time.Since(start) != 2*time.Minute || attempts != 8 {
			t.Fatalf("unbounded or incomplete recovery: elapsed=%s attempts=%d", time.Since(start), attempts)
		}
	})
}

func TestActuatorJobCancellationDoesNotStartAnotherPass(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		attempts := 0
		failure := refusedActuatorRequest()
		control := actuatorReconcileFunc(func(ctx context.Context, now time.Time) (int, error) {
			attempts++
			cancel()
			return 0, failure
		})
		err := runActuatorReconciliation(ctx, control, slog.New(slog.DiscardHandler))
		if !errors.Is(err, context.Canceled) || !errors.Is(err, failure) || attempts != 1 {
			t.Fatalf("cancellation err=%v attempts=%d", err, attempts)
		}
	})
}

func TestActuatorJobHealthyPassReturnsImmediately(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		attempts := 0
		control := actuatorReconcileFunc(func(context.Context, time.Time) (int, error) {
			attempts++
			return 0, nil
		})
		if err := runActuatorReconciliation(t.Context(), control, slog.New(slog.DiscardHandler)); err != nil {
			t.Fatal(err)
		}
		if attempts != 1 || time.Since(start) != 0 {
			t.Fatalf("healthy pass retried: attempts=%d elapsed=%s", attempts, time.Since(start))
		}
	})
}

func TestActuatorJobDoesNotRunAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	control := actuatorReconcileFunc(func(context.Context, time.Time) (int, error) {
		t.Fatal("started reconciliation after cancellation")
		return 0, nil
	})
	if err := runActuatorReconciliation(ctx, control, slog.New(slog.DiscardHandler)); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %v", err)
	}
}

func TestActuatorJobHonorsShorterDeadlineDuringAPass(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		control := actuatorReconcileFunc(func(ctx context.Context, now time.Time) (int, error) {
			<-ctx.Done()
			return 0, nil
		})
		if err := runActuatorReconciliation(ctx, control, slog.New(slog.DiscardHandler)); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("canceled pass reported success: %v", err)
		}
	})
}
