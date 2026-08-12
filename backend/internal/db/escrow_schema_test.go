package db_test

// Escrow (#31, vault Phase-Status.md 2026-08-11, plan commit 28b1527) —
// Adım 1 schema-only migrations: marketplace_transactions.resolved_at/
// resolved_by, marketplace_disputes table, and the did:obs:marketplace-escrow
// seed row in obs_accounts. No money movement happens in this step, so these
// tests only cover schema shape + migration idempotency, not balances.

import (
	"database/sql"
	"path/filepath"
	"testing"

	dbpkg "obscura.network/core/internal/db"
)

// openFreshSQLite mirrors db.Init's DSN so the migrations run against the
// exact same driver/pragma configuration as production.
func openFreshSQLite(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "obscura.db")
	sqlDB, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=10000&_synchronous=NORMAL")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	return sqlDB
}

// TestEscrowSchema_MigrationsIdempotent runs the full central migration list
// (createTables + runMigrations, via RunMigrationsForTest — the same code
// Init() runs) twice against a fresh on-disk SQLite DB. The second run must
// be a clean no-op: every migration is looked up in _migrations by id first,
// so this mainly guards against a duplicate id or a non-idempotent ALTER/
// CREATE slipping into the new escrow entries (164-169).
func TestEscrowSchema_MigrationsIdempotent(t *testing.T) {
	sqlDB := openFreshSQLite(t)

	if err := dbpkg.RunMigrationsForTest(sqlDB, dbpkg.DriverSQLite); err != nil {
		t.Fatalf("first migration run failed: %v", err)
	}
	if err := dbpkg.RunMigrationsForTest(sqlDB, dbpkg.DriverSQLite); err != nil {
		t.Fatalf("second migration run failed (not idempotent): %v", err)
	}
}

// TestEscrowSchema_ColumnsTablesAndSeedExist verifies the three schema
// artifacts Adım 1 promises: the two new marketplace_transactions columns,
// the marketplace_disputes table, and the escrow account seed row — all
// without ever calling Purchase() or moving a balance.
func TestEscrowSchema_ColumnsTablesAndSeedExist(t *testing.T) {
	sqlDB := openFreshSQLite(t)
	if err := dbpkg.RunMigrationsForTest(sqlDB, dbpkg.DriverSQLite); err != nil {
		t.Fatalf("migration run failed: %v", err)
	}

	t.Run("marketplace_transactions has resolved_at and resolved_by", func(t *testing.T) {
		cols := tableColumns(t, sqlDB, "marketplace_transactions")
		for _, want := range []string{"resolved_at", "resolved_by", "status"} {
			if !cols[want] {
				t.Errorf("marketplace_transactions missing column %q", want)
			}
		}
	})

	t.Run("marketplace_disputes table exists with expected columns", func(t *testing.T) {
		cols := tableColumns(t, sqlDB, "marketplace_disputes")
		for _, want := range []string{
			"id", "transaction_id", "opener_did", "reason", "status",
			"resolved_by", "resolved_at", "created_at",
		} {
			if !cols[want] {
				t.Errorf("marketplace_disputes missing column %q", want)
			}
		}
	})

	t.Run("escrow account seeded with zero balance", func(t *testing.T) {
		var balance string
		err := sqlDB.QueryRow(
			`SELECT transparent_balance FROM obs_accounts WHERE user_did = ?`,
			"did:obs:marketplace-escrow",
		).Scan(&balance)
		if err != nil {
			t.Fatalf("escrow account not seeded: %v", err)
		}
		if balance != "0" {
			t.Errorf("escrow account balance = %q, want \"0\" — Adım 1 must not move money", balance)
		}
	})
}

// tableColumns returns the set of column names for tbl via PRAGMA
// table_info, failing the test if the table doesn't exist.
func tableColumns(t *testing.T, sqlDB *sql.DB, tbl string) map[string]bool {
	t.Helper()
	rows, err := sqlDB.Query("PRAGMA table_info(" + tbl + ")")
	if err != nil {
		t.Fatalf("PRAGMA table_info(%s): %v", tbl, err)
	}
	defer rows.Close()

	cols := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info row: %v", err)
		}
		cols[name] = true
	}
	if len(cols) == 0 {
		t.Fatalf("table %q has no columns (does it exist?)", tbl)
	}
	return cols
}
