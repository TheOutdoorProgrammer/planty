package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

func openDatabase(ctx context.Context, command, dsn string, log *slog.Logger) (*store.Store, error) {
	if !waitForDatabase(command) {
		return store.Open(ctx, dsn)
	}
	return store.OpenWithRetry(ctx, dsn, func(ctx context.Context, attempt int, delay time.Duration) {
		log.WarnContext(ctx, "database unavailable; waiting before job starts", "attempt", attempt, "retry_delay", delay)
	})
}

func waitForDatabase(command string) bool {
	switch command {
	case "ingest", "verify-water", "reconcile-actuators", "prune-photos", "daily", "cold", "away", "chase", "remind", "thirst":
		return true
	default:
		return false
	}
}
