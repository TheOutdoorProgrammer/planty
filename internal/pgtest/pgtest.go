// Package pgtest hands each test package its own Postgres database.
package pgtest

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
)

// Env names the server to create test databases on, not the database to use.
const Env = "PLANTY_TEST_DATABASE_URL"

var (
	once      sync.Once
	dsn       string
	provision error
)

// DSN returns a connection string to a database owned by the calling package.
// Packages run in parallel and the cold watch reads every plant, so a shared
// database let one package's rows decide another's results.
func DSN(t *testing.T) string {
	t.Helper()

	admin := os.Getenv(Env)
	if admin == "" {
		t.Skip("set " + Env + " to run tests that need Postgres")
	}

	once.Do(func() { dsn, provision = create(admin, databaseName()) })
	if provision != nil {
		t.Fatalf("provision test database: %v", provision)
	}
	return dsn
}

// create drops and recreates the database, so a failed run leaves its rows on
// the server to look at while the next run still starts clean.
func create(admin, name string) (string, error) {
	target, err := swapDatabase(admin, name)
	if err != nil {
		return "", err
	}

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		return "", fmt.Errorf("connect: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	quoted := pgx.Identifier{name}.Sanitize()
	for _, q := range []string{
		"DROP DATABASE IF EXISTS " + quoted + " WITH (FORCE)",
		"CREATE DATABASE " + quoted,
	} {
		if _, err := conn.Exec(ctx, q); err != nil {
			return "", fmt.Errorf("%s: %w", q, err)
		}
	}
	return target, nil
}

// swapDatabase points the connection string at a different database.
func swapDatabase(dsn, name string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", Env, err)
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return "", fmt.Errorf("%s must be a postgres:// URL", Env)
	}
	u.Path = "/" + name
	return u.String(), nil
}

// databaseName derives a stable name from the test binary, which go names
// after the package under test.
func databaseName() string {
	name := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		}
		return '_'
	}, strings.TrimSuffix(filepath.Base(os.Args[0]), ".test"))

	if name == "" {
		name = "pkg"
	}
	// Postgres truncates identifiers at 63 bytes, which would silently merge
	// two long package names into one database.
	if len(name) > 40 {
		name = name[:40]
	}
	return "planty_test_" + name
}
