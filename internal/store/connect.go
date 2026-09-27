package store

import (
	"context"
	"errors"
	"io"
	"net"
	"syscall"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// OpenWithRetry waits up to five minutes for a transient database outage.
// Only opening and pinging the pool are retried, before migrations or job work.
func OpenWithRetry(ctx context.Context, dsn string, retrying func(context.Context, int, time.Duration)) (*Store, error) {
	return openWithRetry(ctx, func(ctx context.Context) (*Store, error) {
		return Open(ctx, dsn)
	}, retrying)
}

func openWithRetry(ctx context.Context, open func(context.Context) (*Store, error), retrying func(context.Context, int, time.Duration)) (*Store, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	ctx, span := otel.Tracer("planty/store").Start(ctx, "database.connect")
	defer span.End()

	delay := backoff.NewExponentialBackOff()
	delay.InitialInterval = time.Second
	delay.MaxInterval = 10 * time.Second
	var lastErr error
	attempts := 0
	db, err := backoff.Retry(ctx, func() (*Store, error) {
		if err := ctx.Err(); err != nil {
			return nil, backoff.Permanent(err)
		}
		attempts++
		attemptCtx, stop := context.WithTimeout(ctx, 10*time.Second)
		defer stop()
		db, err := open(attemptCtx)
		lastErr = err
		if err != nil && !transientConnectError(err) {
			return nil, backoff.Permanent(err)
		}
		return db, err
	}, backoff.WithBackOff(delay), backoff.WithMaxElapsedTime(0), backoff.WithNotify(func(_ error, next time.Duration) {
		if retrying != nil {
			retrying(ctx, attempts, next)
		}
	}))
	span.SetAttributes(attribute.Int("db.connection.attempts", attempts))
	if err != nil {
		span.SetStatus(codes.Error, "database connection unavailable")
		return nil, errors.Join(lastErr, err)
	}
	return db, nil
}

func transientConnectError(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "57P01", "57P02", "57P03", "53300":
			return true
		default:
			return false
		}
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return dnsErr.IsTimeout || dnsErr.IsTemporary
	}
	var netErr net.Error
	return errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout())
}
