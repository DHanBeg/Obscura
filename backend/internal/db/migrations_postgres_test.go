//go:build postgres_concurrency

// This file is excluded from the default `go test ./...` run (build tag).
// It is the permanent, committed form of the one-off diagnostic used to
// find every SQLite-only construct in the schema (see the commit history
// around ADR "Adım 3"): it runs the REAL, current migration list —
// createTables() + runMigrations(), exactly as Init() runs them — plus the
// schema-creation entry points of every package that manages its own
// tables outside the central _migrations tracking, against a real,
// ephemeral Postgres instance. Because it calls the actual production
// functions (not a hand-copied extraction of their SQL text), it can't
// silently drift out of sync with the schema the way a one-off script can.
//
// Run explicitly:
//
//	go test -tags postgres_concurrency ./internal/db/... -run TestSchema_Postgres -v
package db_test

import (
	"testing"

	"obscura.network/core/internal/bots"
	"obscura.network/core/internal/dao"
	dbpkg "obscura.network/core/internal/db"
	"obscura.network/core/internal/db/migrations"
	"obscura.network/core/internal/dbi"
	"obscura.network/core/internal/dbtest"
	"obscura.network/core/internal/federation"
	"obscura.network/core/internal/miniapp"
	"obscura.network/core/internal/p2p"
	"obscura.network/core/internal/storage"
)

// TestSchema_Postgres_CentralMigrationsApply is Part A: createTables() plus
// every entry in runMigrations()'s list (161 migrations as of this test's
// writing — see db.RunMigrationsForTest's doc comment for why this calls
// the real functions instead of re-extracting their SQL). A migration
// failing here means either its SQLite-only syntax needs a pgSQL override
// (see the {id, sql, pgSQL} struct in database.go) or IsBenignSchemaError
// needs a new SQLSTATE case.
func TestSchema_Postgres_CentralMigrationsApply(t *testing.T) {
	rawDB := dbtest.StartPostgres(t)
	if err := dbpkg.RunMigrationsForTest(rawDB, dbi.DriverPostgres); err != nil {
		t.Fatalf("central migrations failed against Postgres: %v", err)
	}
}

// TestSchema_Postgres_ScatteredSchemaApplies is Part B: the packages that
// manage their own tables outside the central migrations list (none of
// them use _migrations tracking, each just runs idempotent
// CREATE TABLE IF NOT EXISTS on its own Init/InitSchema call). Each gets
// its own fresh Postgres instance and calls its real, exported entry point
// with a driver-aware Conn — no hand-copied DDL.
//
// Two of the original twelve scattered files from the Adım 3 audit are not
// covered here:
//   - internal/zk/anomaly.go and internal/zk/proof_log.go don't have their
//     own Init entry points — their DDL (ProofLogMigrationSQL,
//     ProofLogChainMigrationSQL/Postgres) is referenced directly from the
//     central migrations list (084_zk_proof_log, 086_zk_proof_log_chain),
//     so Part A above already exercises them.
//   - internal/subscriber (both migrate.go and schema.go) is intentionally
//     excluded, same as every prior step: it always opens its own raw
//     SQLite file (OpenSubscriberDB never consults OBSCURA_DB_DRIVER) and
//     migrate.go's rebuild SQL is generated at runtime from
//     sqlite_master introspection, not static text. Already scoped as
//     separate full-rewrite work, not test-infrastructure work.
func TestSchema_Postgres_ScatteredSchemaApplies(t *testing.T) {
	cases := []struct {
		name string
		init func(dbi.Querier) error
	}{
		{"dao", dao.Init},
		{"federation", federation.Init},
		{"storage", storage.Init},
		{"bots", bots.Init},
		{"miniapp", miniapp.InitSchema},
		{"p2p_peer_cache", p2p.AddPeerCacheMigration},
		{"mls_schema", func(q dbi.Querier) error {
			for _, ddl := range migrations.MLSMigrations() {
				if _, err := q.Exec(ddl); err != nil {
					return err
				}
			}
			return nil
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rawDB := dbtest.StartPostgres(t)
			t.Setenv("OBSCURA_DB_DRIVER", "postgres") // dao/federation branch on dbi.DriverFromEnv()
			conn := dbpkg.NewConn(rawDB, dbi.DriverPostgres)
			if err := c.init(conn); err != nil {
				t.Fatalf("%s schema failed against Postgres: %v", c.name, err)
			}
		})
	}
}

// Not covered by any test in this file: internal/api's node_shards and
// internal/subscriber's subscribers/audit_log tables would round out the
// picture, but neither package exposes a schema-only entry point narrow
// enough to call here without side effects: api.InitStorage()
// starts a background TTL-pruning goroutine and mutates a package-level
// singleton, and subscriber.OpenSubscriberDB always opens its own SQLite
// file regardless of OBSCURA_DB_DRIVER. Both were verified against
// Postgres manually during the Adım 3 audit (node_shards: BLOB -> BYTEA;
// subscribers/audit_log: BLOB -> BYTEA) and their DDL already carries the
// driver branch internally — this permanent test just doesn't have a way
// to reach it without either changing those packages' public surface or
// hand-copying their SQL text (which is exactly the drift risk this file
// exists to avoid). Left as a known gap rather than worked around.
