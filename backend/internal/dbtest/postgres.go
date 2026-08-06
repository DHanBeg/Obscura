// Package dbtest provides shared test-support helpers for exercising code
// against a real, ephemeral Postgres instance (embedded-postgres) instead
// of SQLite. Used by the *_postgres_test.go files gated behind the
// postgres_concurrency build tag across the backend (internal/signal,
// internal/token, internal/staking, internal/db) — kept in its own
// importable package so those files don't each duplicate the same
// embedded-postgres bootstrap/port-selection boilerplate.
package dbtest

import (
	"database/sql"
	"fmt"
	"net"
	"testing"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// freePort asks the OS for an unused TCP port by binding to :0 and reading
// back the assigned port, then releasing it immediately. There's a narrow
// window between releasing it here and embedded-postgres binding it where
// another process could grab it first, but that's true of any "find a free
// port" approach without an OS-level reservation API; in practice this is
// not a practical flake source for these tests. This replaces the earlier
// per-file pattern of a hardcoded port range offset by PID%N, which could
// collide between packages sharing a PID-derived offset.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("dbtest: find free port: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// StartPostgres starts an ephemeral embedded Postgres instance and returns
// a *sql.DB connected to it via the pgx driver, with a real connection pool
// (NOT MaxOpenConns(1) — the whole reason these tests exist is to exercise
// genuine concurrent connections, which SQLite's single-connection model
// can never do). The instance and connection are torn down automatically
// via t.Cleanup, in LIFO order (connection closed before the server stops).
//
// The caller is responsible for its own schema (CREATE TABLE ...) — this
// only manages the Postgres process and the connection to it.
func StartPostgres(t *testing.T) *sql.DB {
	t.Helper()

	port := freePort(t)
	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Port(uint32(port)).
		Username("obscura").
		Password("obscura").
		Database("obscura_test").
		// RuntimePath MUST be unique per instance: embedded-postgres does
		// `os.RemoveAll(runtimePath)` at the start of every Start() call
		// (see its embedded_postgres.go), and defaults to a fixed shared
		// path (~/.embedded-postgres-go/extracted) if not set. `go test
		// ./...` runs different packages' test binaries in parallel by
		// default, so two of these tests starting at once against the
		// shared default path would race — one's RemoveAll can delete the
		// other's freshly-extracted binaries/data mid-startup. t.TempDir()
		// is unique per test and already auto-cleaned. The binary archive
		// itself (CachePath, left at its default) stays shared/reused —
		// it's only ever read after being downloaded once, no race there.
		RuntimePath(t.TempDir()).
		Locale("C")) // avoids initdb choking on non-ASCII system locale names (observed on Windows with a Turkish locale)
	if err := pg.Start(); err != nil {
		t.Fatalf("dbtest: start embedded postgres: %v", err)
	}
	t.Cleanup(func() { _ = pg.Stop() })

	dsn := fmt.Sprintf("postgres://obscura:obscura@localhost:%d/obscura_test?sslmode=disable", port)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("dbtest: open pgx: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(30)

	return db
}
