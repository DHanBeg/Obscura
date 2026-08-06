package db

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"

	"obscura.network/core/internal/dbi"
)

// pgBenignSchemaCodes are the Postgres SQLSTATE codes that mean "this
// column/table/index/constraint already exists" — the Postgres analogue of
// SQLite's "duplicate column"/"already exists" error text. See
// https://www.postgresql.org/docs/current/errcodes-appendix.html.
var pgBenignSchemaCodes = map[string]bool{
	"42701": true, // duplicate_column
	"42P07": true, // duplicate_table
	"42710": true, // duplicate_object (index/constraint)
}

// IsBenignSchemaError reports whether err is a "this already exists"
// failure that idempotent ALTER TABLE/CREATE statements should tolerate —
// driver-aware. On sqlite it string-matches the driver's error text (the
// only signal modernc.org/sqlite exposes); on Postgres it checks the real
// SQLSTATE code via pgconn.PgError, which is exact and locale-independent.
// Exported so callers outside this package (e.g. internal/federation, which
// runs its own idempotent ALTER TABLE ADD COLUMN and can't use
// runMigrations()'s _migrations tracking) can share the same logic instead
// of keeping their own copy of the sqlite-only string match.
func IsBenignSchemaError(driver string, err error) bool {
	if err == nil {
		return false
	}
	if driver == dbi.DriverPostgres {
		var pgErr *pgconn.PgError
		return errors.As(err, &pgErr) && pgBenignSchemaCodes[pgErr.Code]
	}
	msg := err.Error()
	return contains(msg, "duplicate column") || contains(msg, "already exists")
}
