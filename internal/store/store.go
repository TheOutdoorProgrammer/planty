// Package store is the Postgres layer. It is the only thing that touches the
// database; the API and the Dusk plugin both reach it through the HTTP surface.
package store

import (
	"context"
	"embed"
	"fmt"

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

// Migrate applies every pending migration.
func (s *Store) Migrate(ctx context.Context) error {
	goose.SetBaseFS(migrations)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("dialect: %w", err)
	}

	db := stdlib.OpenDBFromPool(s.pool)
	defer db.Close()

	return goose.UpContext(ctx, db, "migrations")
}

// Healthy reports whether the database still answers.
func (s *Store) Healthy(ctx context.Context) error { return s.pool.Ping(ctx) }
