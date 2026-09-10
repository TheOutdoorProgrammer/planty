package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

type actuatorReconciler interface {
	Reconcile(context.Context, time.Time) (int, error)
}

func runActuatorReconciliation(ctx context.Context, control actuatorReconciler, log *slog.Logger) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	deadline, _ := ctx.Deadline()

	var lastErr error
	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return errors.Join(lastErr, err)
		}
		if !time.Now().Before(deadline) {
			return errors.Join(lastErr, context.DeadlineExceeded)
		}
		attemptCtx, span := otel.Tracer("planty/actuators").Start(ctx, "actuators.reconcile")
		// Re-read durable intent on each pass: replaying a service call could
		// turn a device on after its schedule window or lease has expired.
		_, lastErr = control.Reconcile(attemptCtx, time.Now().UTC())
		if lastErr != nil {
			span.SetStatus(codes.Error, "actuator reconciliation failed")
			span.SetAttributes(attribute.String("error.type", fmt.Sprintf("%T", lastErr)))
			log.ErrorContext(attemptCtx, "actuator reconciliation failed", "attempt", attempt, "error_type", fmt.Sprintf("%T", lastErr))
		}
		span.End()
		if lastErr == nil {
			return ctx.Err()
		}

		timer := time.NewTimer(15 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return errors.Join(lastErr, ctx.Err())
		case <-timer.C:
		}
	}
}
