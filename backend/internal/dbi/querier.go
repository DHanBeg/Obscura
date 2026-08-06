// Package dbi holds the minimal SQL query-execution interface shared across
// the backend. It has zero internal dependencies on purpose: any package
// that runs queries against a passed-in DB handle can depend on dbi.Querier
// without risking an import cycle back into internal/db (which itself
// imports internal/zk, internal/identity, ...).
package dbi

import (
	"context"
	"database/sql"
)

// Querier is the method set functions need to run parameterized SQL without
// caring whether they were handed a *sql.DB, a *sql.Tx, or a driver-
// abstracting wrapper (db.Conn / db.Tx in internal/db). *sql.DB and *sql.Tx
// already satisfy this interface as-is — no call site needs to change,
// only the declared parameter/field type.
type Querier interface {
	Exec(query string, args ...any) (sql.Result, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}
