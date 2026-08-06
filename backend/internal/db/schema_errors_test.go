package db

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsBenignSchemaError_Nil(t *testing.T) {
	if IsBenignSchemaError(DriverSQLite, nil) {
		t.Fatal("nil error should never be benign")
	}
	if IsBenignSchemaError(DriverPostgres, nil) {
		t.Fatal("nil error should never be benign")
	}
}

func TestIsBenignSchemaError_SQLite_MatchesOldStringCheck(t *testing.T) {
	// Same two substrings runMigrations() tolerated before this change —
	// regression guard: SQLite behavior must stay byte-for-byte identical.
	cases := []struct {
		msg  string
		want bool
	}{
		{`duplicate column name: fcm_token`, true},
		{`table "mls_key_packages" already exists`, true},
		{`SQL logic error: no such table: foo`, false},
		{`UNIQUE constraint failed: users.did`, false},
	}
	for _, c := range cases {
		got := IsBenignSchemaError(DriverSQLite, errors.New(c.msg))
		if got != c.want {
			t.Errorf("IsBenignSchemaError(sqlite, %q) = %v, want %v", c.msg, got, c.want)
		}
	}
}

func TestIsBenignSchemaError_Postgres_ChecksSQLSTATE(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"duplicate_column 42701", &pgconn.PgError{Code: "42701"}, true},
		{"duplicate_table 42P07", &pgconn.PgError{Code: "42P07"}, true},
		{"duplicate_object 42710", &pgconn.PgError{Code: "42710"}, true},
		{"undefined_table 42P01 (not benign)", &pgconn.PgError{Code: "42P01"}, false},
		{"syntax_error 42601 (not benign)", &pgconn.PgError{Code: "42601"}, false},
		// A plain error whose TEXT happens to mention "duplicate column"
		// must NOT be treated as benign on postgres — only a real
		// *pgconn.PgError with a matching SQLSTATE counts. This is the
		// whole point of the driver-aware split: postgres never falls
		// back to sqlite's string-matching.
		{"plain error with matching text, wrong driver", errors.New("duplicate column name: x"), false},
	}
	for _, c := range cases {
		got := IsBenignSchemaError(DriverPostgres, c.err)
		if got != c.want {
			t.Errorf("%s: IsBenignSchemaError(postgres, err) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestIsBenignSchemaError_Postgres_WrappedPgError(t *testing.T) {
	// pgx errors are often wrapped (e.g. by database/sql); errors.As must
	// still unwrap to the underlying *pgconn.PgError.
	wrapped := errors.Join(errors.New("exec failed"), &pgconn.PgError{Code: "42701"})
	if !IsBenignSchemaError(DriverPostgres, wrapped) {
		t.Fatal("expected wrapped duplicate_column error to be tolerated")
	}
}
