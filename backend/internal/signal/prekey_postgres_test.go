//go:build postgres_concurrency

// This file is excluded from the default `go test ./...` run (build tag).
// It spins up a real, ephemeral Postgres via embedded-postgres to prove
// GetPrekeyBundle's OPK claim is safe under actual concurrent connections —
// something SQLite's MaxOpenConns(1) can never exercise, since only one
// connection (and therefore only one in-flight transaction) can exist.
//
// Run explicitly:
//
//	go test -tags postgres_concurrency ./internal/signal/... -run TestGetPrekeyBundle_Postgres -v
package signal_test

import (
	"database/sql"
	"fmt"
	"sync"
	"testing"

	dbpkg "obscura.network/core/internal/db"
	"obscura.network/core/internal/dbi"
	"obscura.network/core/internal/dbtest"
	"obscura.network/core/internal/signal"
)

// openPostgresTestDB starts an ephemeral embedded Postgres instance (via
// internal/dbtest) and applies the minimal schema GetPrekeyBundle needs.
func openPostgresTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db := dbtest.StartPostgres(t)

	schema := `
		CREATE TABLE prekey_bundles (
			did               TEXT PRIMARY KEY,
			identity_key      TEXT NOT NULL,
			signed_prekey     TEXT NOT NULL,
			signed_prekey_sig TEXT NOT NULL,
			signed_prekey_id  INTEGER DEFAULT 0,
			updated_at        TEXT NOT NULL
		);
		CREATE TABLE one_time_prekeys (
			id         TEXT PRIMARY KEY,
			did        TEXT NOT NULL,
			opk_id     INTEGER NOT NULL,
			public_key TEXT NOT NULL,
			used       INTEGER DEFAULT 0,
			created_at TEXT NOT NULL,
			used_at    TEXT
		);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

func seedBundlePG(t *testing.T, db *sql.DB, did string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO prekey_bundles (did, identity_key, signed_prekey, signed_prekey_sig, signed_prekey_id, updated_at)
		VALUES ($1, 'ik', 'spk', 'sig', 1, 'now')`, did)
	if err != nil {
		t.Fatalf("seed bundle: %v", err)
	}
}

func seedOPKPG(t *testing.T, db *sql.DB, rowID, did string, opkID int) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO one_time_prekeys (id, did, opk_id, public_key, used, created_at)
		VALUES ($1, $2, $3, $4, 0, 'now')`,
		rowID, did, opkID, fmt.Sprintf("opk_pub_%d", opkID))
	if err != nil {
		t.Fatalf("seed opk %d: %v", opkID, err)
	}
}

// TestGetPrekeyBundle_Postgres_ConcurrentClaimsAreExclusive is the RED/GREEN
// test.
//
// A single race-condition test run is inherently probabilistic — one lucky
// (unlucky) goroutine schedule can pass even with the bug present. So this
// runs many independent rounds, each seeding exactly ONE fresh OPK and
// releasing `callers` goroutines at once (via a start barrier, to maximize
// the chance their SELECTs land before anyone's UPDATE commits) to race for
// it. Exactly one must win per round; the rest must gracefully get none.
// The test fails on the FIRST round that shows the OPK handed to more than
// one caller.
//
// Before the RowsAffected fix, GetPrekeyBundle reads the candidate OPK, then
// fires the claiming UPDATE without checking whether it actually changed a
// row — so a caller that loses the race (its UPDATE affects 0 rows because
// another connection already flipped used=1) still returns the OPK as if it
// had won. Postgres's row lock keeps the DATABASE consistent (only one UPDATE
// really takes effect), but the APPLICATION hands the same key to two callers.
func TestGetPrekeyBundle_Postgres_ConcurrentClaimsAreExclusive(t *testing.T) {
	db := openPostgresTestDB(t)
	// prekey.go writes its queries with `?` placeholders (shared source with
	// the sqlite path) — route through the same driver-aware rebind wrapper
	// production uses (internal/db.Conn), not the raw pgx *sql.DB, or every
	// query fails with a syntax error before the race is ever exercised.
	store := signal.NewSessionStore(dbpkg.NewConn(db, dbi.DriverPostgres))

	const did = "did:obs:race-target"
	seedBundlePG(t, db, did)

	const rounds = 60
	const callersPerRound = 8

	for round := 0; round < rounds; round++ {
		rowID := fmt.Sprintf("round-%d", round)
		opkID := 1000 + round // unique per round, easy to spot in failures
		seedOPKPG(t, db, rowID, did, opkID)

		var ready sync.WaitGroup     // all goroutines signal "about to call"
		start := make(chan struct{}) // closed once to release them together
		var wg sync.WaitGroup
		results := make([]int, callersPerRound)

		ready.Add(callersPerRound)
		for i := 0; i < callersPerRound; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				ready.Done()
				<-start
				bundle, err := store.GetPrekeyBundle(did)
				if err != nil {
					t.Errorf("round %d caller %d: GetPrekeyBundle: %v", round, i, err)
					return
				}
				results[i] = bundle.OneTimePreKeyID
			}(i)
		}
		ready.Wait() // all goroutines parked right before the call
		close(start)
		wg.Wait()

		winners := 0
		for _, id := range results {
			if id == opkID {
				winners++
			} else if id != 0 {
				t.Fatalf("round %d: caller reported OPK id %d, expected %d or 0", round, id, opkID)
			}
		}
		if winners != 1 {
			t.Fatalf("round %d: OPK %d was handed out to %d callers (of %d) — must be exactly 1",
				round, opkID, winners, callersPerRound)
		}
	}
}
