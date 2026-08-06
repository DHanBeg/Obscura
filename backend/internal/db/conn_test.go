package db

import "testing"

func TestRebind_SQLite_NoOp(t *testing.T) {
	query := "SELECT * FROM users WHERE did = ? AND status = ?"
	got := rebind(DriverSQLite, query)
	if got != query {
		t.Fatalf("sqlite rebind should be a no-op, got %q", got)
	}
}

func TestRebind_Postgres_ConvertsPlaceholders(t *testing.T) {
	query := "INSERT INTO users (did, status) VALUES (?, ?) ON CONFLICT(did) DO UPDATE SET status = ?"
	want := "INSERT INTO users (did, status) VALUES ($1, $2) ON CONFLICT(did) DO UPDATE SET status = $3"
	got := rebind(DriverPostgres, query)
	if got != want {
		t.Fatalf("postgres rebind mismatch:\n got:  %q\n want: %q", got, want)
	}
}

func TestDriverFromEnv_DefaultsToSQLite(t *testing.T) {
	t.Setenv("OBSCURA_DB_DRIVER", "")
	if got := driverFromEnv(); got != DriverSQLite {
		t.Fatalf("expected default driver %q, got %q", DriverSQLite, got)
	}
}

func TestDriverFromEnv_UnknownFallsBackToSQLite(t *testing.T) {
	t.Setenv("OBSCURA_DB_DRIVER", "mysql")
	if got := driverFromEnv(); got != DriverSQLite {
		t.Fatalf("unknown driver should fall back to sqlite, got %q", got)
	}
}

func TestDriverFromEnv_Postgres(t *testing.T) {
	t.Setenv("OBSCURA_DB_DRIVER", "postgres")
	if got := driverFromEnv(); got != DriverPostgres {
		t.Fatalf("expected %q, got %q", DriverPostgres, got)
	}
}

func TestInit_RejectsPostgresForNow(t *testing.T) {
	t.Setenv("OBSCURA_DB_DRIVER", "postgres")
	if err := Init(t.TempDir()); err == nil {
		t.Fatal("expected Init to reject postgres until schema/migrations are ported, got nil error")
	}
}
