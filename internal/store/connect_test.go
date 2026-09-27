package store

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/pgtest"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestDatabaseStartupRecoversAfterShutdown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		want := &Store{}
		attempts, warnings := 0, 0
		got, err := openWithRetry(context.Background(), func(ctx context.Context) (*Store, error) {
			attempts++
			deadline, ok := ctx.Deadline()
			if !ok || deadline.Sub(time.Now()) > 10*time.Second {
				t.Fatal("connection attempt is not bounded")
			}
			if time.Since(start) < time.Minute {
				return nil, fmt.Errorf("ping: %w", &pgconn.PgError{Code: "57P03"})
			}
			if time.Since(start) < 3*time.Minute {
				return nil, &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}
			}
			return want, nil
		}, func(context.Context, int, time.Duration) { warnings++ })
		if err != nil || got != want || attempts < 3 || warnings != attempts-1 {
			t.Fatalf("recovery: store=%p err=%v attempts=%d warnings=%d", got, err, attempts, warnings)
		}
		if elapsed := time.Since(start); elapsed < 3*time.Minute || elapsed > 3*time.Minute+15*time.Second {
			t.Fatalf("recovery delayed: %v", elapsed)
		}
	})
}

func TestDatabaseStartupDeadlineAndCancellation(t *testing.T) {
	for _, tc := range []struct {
		name        string
		cancelAfter time.Duration
		want        error
	}{
		{"deadline", 0, context.DeadlineExceeded},
		{"canceled", 2 * time.Second, context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if tc.cancelAfter > 0 {
					time.AfterFunc(tc.cancelAfter, cancel)
				}
				start := time.Now()
				db, err := openWithRetry(ctx, func(context.Context) (*Store, error) {
					return nil, syscall.ECONNREFUSED
				}, nil)
				if db != nil || !errors.Is(err, tc.want) || !errors.Is(err, syscall.ECONNREFUSED) {
					t.Fatalf("failure lost its cause: store=%p err=%v", db, err)
				}
				want := 5 * time.Minute
				if tc.cancelAfter > 0 {
					want = tc.cancelAfter
				}
				if elapsed := time.Since(start); elapsed != want {
					t.Fatalf("wait=%v, want %v", elapsed, want)
				}
			})
		})
	}
}

func TestDatabaseStartupBoundsHungAttempts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		attempts := 0
		_, err := openWithRetry(context.Background(), func(ctx context.Context) (*Store, error) {
			attempts++
			<-ctx.Done()
			return nil, ctx.Err()
		}, nil)
		if !errors.Is(err, context.DeadlineExceeded) || attempts < 2 || time.Since(start) != 5*time.Minute {
			t.Fatalf("hung attempts: err=%v attempts=%d elapsed=%v", err, attempts, time.Since(start))
		}
	})
}

func TestDatabaseStartupRejectsPermanentFailures(t *testing.T) {
	for _, failure := range []error{
		&pgconn.PgError{Code: "28P01"},
		&pgconn.PgError{Code: "3D000"},
		&pgconn.PgError{Code: "42501"},
		errors.New("invalid connection string"),
		&net.DNSError{IsNotFound: true},
	} {
		t.Run(failure.Error(), func(t *testing.T) {
			attempts := 0
			_, err := openWithRetry(context.Background(), func(context.Context) (*Store, error) {
				attempts++
				return nil, fmt.Errorf("connect: %w", failure)
			}, func(context.Context, int, time.Duration) { t.Fatal("permanent failure retried") })
			if attempts != 1 || !errors.Is(err, failure) {
				t.Fatalf("attempts=%d err=%v", attempts, err)
			}
		})
	}
}

func TestTransientConnectErrors(t *testing.T) {
	for _, err := range []error{
		&pgconn.PgError{Code: "57P01"}, &pgconn.PgError{Code: "57P02"},
		&pgconn.PgError{Code: "57P03"}, &pgconn.PgError{Code: "53300"},
		syscall.ECONNREFUSED, syscall.ECONNRESET, io.EOF, io.ErrUnexpectedEOF,
		&net.DNSError{IsTemporary: true}, context.DeadlineExceeded,
	} {
		if !transientConnectError(fmt.Errorf("ping: %w", err)) {
			t.Errorf("not retryable: %v", err)
		}
	}
	if transientConnectError(context.Canceled) {
		t.Fatal("cancellation must stop retrying")
	}
}

func TestDatabaseStartupWithRealPostgres(t *testing.T) {
	dsn := pgtest.DSN(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	unavailable := "postgres://postgres@" + listener.Addr().String() + "/planty?sslmode=disable"
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	attempts := 0
	db, err := openWithRetry(context.Background(), func(ctx context.Context) (*Store, error) {
		attempts++
		if attempts <= 2 {
			return Open(ctx, unavailable)
		}
		return Open(ctx, dsn)
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if attempts != 3 {
		t.Fatalf("attempts=%d, want 3", attempts)
	}
	if err := db.Healthy(context.Background()); err != nil {
		t.Fatalf("recovered pool unusable: %v", err)
	}
}
