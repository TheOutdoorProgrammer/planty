// Package store is the Postgres layer. It is the only thing that touches the
// database; the API and the Dusk plugin both reach it through the HTTP surface.
package store

import (
	"context"
	"embed"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Store holds the connection pool.
type Store struct {
	pool *pgxpool.Pool
}

// Open connects and verifies the database is reachable before returning.
func Open(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Close releases the pool.
func (s *Store) Close() { s.pool.Close() }

// migrationLock is an arbitrary but fixed key for the advisory lock. Any
// process migrating this database must use the same number to serialise.
const migrationLock = 8_150_413

// goose keeps its dialect and filesystem in package globals, so two goroutines
// migrating at once race inside the library. The advisory lock only serialises
// across processes; this serialises within one.
var migrating sync.Mutex

// SilenceMigrations stops goose narrating itself. Every command migrates on
// start, so a command whose output a model reads otherwise begins with
// "goose: no migrations to run" every single time.
func SilenceMigrations() { goose.SetLogger(quietLogger{}) }

// quietLogger drops what goose prints. Fatalf still ends the process, because
// swallowing a failed migration would be much worse than a noisy one.
type quietLogger struct{}

func (quietLogger) Printf(string, ...any) {}
func (quietLogger) Fatalf(format string, v ...any) {
	panic(fmt.Sprintf(format, v...))
}

// Migrate applies pending migrations, serialised: every command migrates on
// start, so a deployment and a CronJob landing together would otherwise race.
func (s *Store) Migrate(ctx context.Context) error {
	migrating.Lock()
	defer migrating.Unlock()

	goose.SetBaseFS(migrations)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("dialect: %w", err)
	}

	held, err := s.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer held.Release()

	if _, err := held.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLock); err != nil {
		return fmt.Errorf("take migration lock: %w", err)
	}
	defer func() {
		if _, err := held.Exec(ctx, "SELECT pg_advisory_unlock($1)", migrationLock); err != nil {
			// Not fatal: the lock dies with the connection either way.
			_ = err
		}
	}()

	db := stdlib.OpenDBFromPool(s.pool)
	defer func() { _ = db.Close() }()

	return goose.UpContext(ctx, db, "migrations")
}

// Healthy reports whether the database still answers.
func (s *Store) Healthy(ctx context.Context) error { return s.pool.Ping(ctx) }
