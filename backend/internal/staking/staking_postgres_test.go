//go:build postgres_concurrency

// This file is excluded from the default `go test ./...` run (build tag).
// It spins up a real, ephemeral Postgres via embedded-postgres to prove
// Stake's balance read-modify-write is safe under actual concurrent
// connections — something SQLite's MaxOpenConns(1) can never exercise,
// since only one connection (and therefore only one in-flight transaction)
// can exist.
//
// Run explicitly:
//
//	go test -tags postgres_concurrency ./internal/staking/... -run TestStake_Postgres -v
package staking_test

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"
	"os"
	"sync"
	"testing"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	_ "github.com/jackc/pgx/v5/stdlib"

	dbpkg "obscura.network/core/internal/db"
	"obscura.network/core/internal/dbi"
	"obscura.network/core/internal/staking"
)

// openPostgresStakingDB starts an ephemeral embedded Postgres instance with
// the minimal schema Stake needs, and returns the raw pooled connection
// (NOT MaxOpenConns(1) — the whole point of the test).
func openPostgresStakingDB(t *testing.T) *sql.DB {
	t.Helper()

	port := uint32(15800 + (os.Getpid() % 300))
	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Port(port).
		Username("obscura").
		Password("obscura").
		Database("obscura_staking_test").
		Locale("C"))
	if err := pg.Start(); err != nil {
		t.Fatalf("start embedded postgres: %v", err)
	}
	t.Cleanup(func() { _ = pg.Stop() })

	dsn := fmt.Sprintf("postgres://obscura:obscura@localhost:%d/obscura_staking_test?sslmode=disable", port)
	rawDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open pgx: %v", err)
	}
	t.Cleanup(func() { _ = rawDB.Close() })
	rawDB.SetMaxOpenConns(30)

	schema := `
		CREATE TABLE obs_accounts (
			user_did            TEXT PRIMARY KEY,
			transparent_balance TEXT NOT NULL,
			updated_at          TEXT NOT NULL
		);
		CREATE TABLE stakes (
			id                   TEXT PRIMARY KEY,
			user_did             TEXT NOT NULL,
			amount               TEXT NOT NULL,
			stake_type           TEXT NOT NULL DEFAULT 'user',
			locked_until         TEXT NOT NULL,
			apy_bps              INTEGER NOT NULL DEFAULT 1000,
			status               TEXT NOT NULL DEFAULT 'active',
			created_at           TEXT NOT NULL,
			unstake_requested_at TEXT,
			withdrawn_at         TEXT
		);
	`
	if _, err := rawDB.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return rawDB
}

func pgStakingBalance(t *testing.T, rawDB *sql.DB, did string) *big.Int {
	t.Helper()
	var s string
	err := rawDB.QueryRow(`SELECT transparent_balance FROM obs_accounts WHERE user_did = $1`, did).Scan(&s)
	if err == sql.ErrNoRows {
		return big.NewInt(0)
	}
	if err != nil {
		t.Fatalf("balance %s: %v", did, err)
	}
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		t.Fatalf("corrupt balance for %s: %q", did, s)
	}
	return v
}

// withPostgresStakingDB installs a Postgres-backed db.DB (and matching
// OBSCURA_DB_DRIVER, t.Setenv-scoped) for the duration of fn, then restores
// whatever db.DB pointed at before. See token_postgres_test.go's identical
// helper for why the env var has to be set alongside db.DB: staking.go's
// txBalance/setBalance would read dbi.DriverFromEnv() the same way
// token.go's do, once fixed.
func withPostgresStakingDB(t *testing.T, rawDB *sql.DB, fn func()) {
	t.Helper()
	t.Setenv("OBSCURA_DB_DRIVER", "postgres")
	prev := dbpkg.DB
	dbpkg.DB = dbpkg.NewConn(rawDB, dbi.DriverPostgres)
	defer func() { dbpkg.DB = prev }()
	fn()
}

// TestStake_Postgres_ConcurrentStakesDontLoseUpdates is the RED/GREEN test.
//
// Mirrors internal/token/token_postgres_test.go's
// TestTransfer_Postgres_ConcurrentSpendsDontLoseUpdates: many independent
// rounds, each funding a fresh staker with EXACTLY enough balance for K of N
// concurrent Stake attempts to succeed (K < N), releasing all N via a start
// barrier. Per round:
//   - exactly K attempts succeed;
//   - the staker's final balance is exactly 0;
//   - StakePoolDID's balance increased by exactly (cumulative successes
//     across rounds so far) * amount — value conservation, not just a
//     success count that happens to look right.
//
// Before the fix, staking.go's private txBalance/setBalance copy has the
// exact same unprotected read-modify-write token.go had: two concurrent
// Stake calls for the same staker can both read the same pre-debit balance
// and both pass the sufficiency check.
func TestStake_Postgres_ConcurrentStakesDontLoseUpdates(t *testing.T) {
	rawDB := openPostgresStakingDB(t)

	const rounds = 20
	const attemptsPerRound = 6
	const affordable = 3 // K: exactly this many of attemptsPerRound can succeed

	amount := staking.MinUserStake

	withPostgresStakingDB(t, rawDB, func() {
		cumulativeSuccesses := 0
		for round := 0; round < rounds; round++ {
			staker := fmt.Sprintf("did:obs:staker-%d", round)
			startBalance := new(big.Int).Mul(amount, big.NewInt(affordable))

			if _, err := rawDB.Exec(
				`INSERT INTO obs_accounts (user_did, transparent_balance, updated_at) VALUES ($1, $2, 'now')`,
				staker, startBalance.String(),
			); err != nil {
				t.Fatalf("round %d: seed staker: %v", round, err)
			}

			var ready sync.WaitGroup
			start := make(chan struct{})
			var wg sync.WaitGroup
			succeeded := make([]bool, attemptsPerRound)

			ready.Add(attemptsPerRound)
			for i := 0; i < attemptsPerRound; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					ready.Done()
					<-start
					_, err := staking.Stake(context.Background(), staker, amount, staking.StakeTypeUser)
					succeeded[i] = err == nil
					if err != nil {
						t.Logf("round %d attempt %d error: %v", round, i, err)
					}
				}(i)
			}
			ready.Wait()
			close(start)
			wg.Wait()

			successCount := 0
			for _, ok := range succeeded {
				if ok {
					successCount++
				}
			}
			if successCount != affordable {
				t.Fatalf("round %d: %d/%d stakes succeeded, want exactly %d",
					round, successCount, attemptsPerRound, affordable)
			}
			cumulativeSuccesses += successCount

			finalStakerBal := pgStakingBalance(t, rawDB, staker)
			if finalStakerBal.Sign() != 0 {
				t.Fatalf("round %d: staker balance = %s, want 0", round, finalStakerBal)
			}

			poolBal := pgStakingBalance(t, rawDB, staking.StakePoolDID)
			wantPool := new(big.Int).Mul(amount, big.NewInt(int64(cumulativeSuccesses)))
			if poolBal.Cmp(wantPool) != 0 {
				t.Fatalf("round %d: stake pool balance = %s, want %s (cumulative across rounds so far)",
					round, poolBal, wantPool)
			}
		}
	})
}
