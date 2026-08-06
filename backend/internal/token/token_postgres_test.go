//go:build postgres_concurrency

// This file is excluded from the default `go test ./...` run (build tag).
// It spins up a real, ephemeral Postgres via embedded-postgres to prove
// Transfer's balance/supply read-modify-write is safe under actual
// concurrent connections — something SQLite's MaxOpenConns(1) can never
// exercise, since only one connection (and therefore only one in-flight
// transaction) can exist.
//
// Run explicitly:
//
//	go test -tags postgres_concurrency ./internal/token/... -run TestTransfer_Postgres -v
package token_test

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
	"obscura.network/core/internal/token"
)

// openPostgresTokenDB starts an ephemeral embedded Postgres instance with the
// minimal schema Transfer/Mint/Burn need, and returns the raw connection
// (pooled — NOT MaxOpenConns(1)) alongside a driver-aware Conn wrapper
// suitable for installing as the package-level db.DB token.go reads.
func openPostgresTokenDB(t *testing.T) *sql.DB {
	t.Helper()

	port := uint32(15700 + (os.Getpid() % 300))
	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Port(port).
		Username("obscura").
		Password("obscura").
		Database("obscura_token_test").
		Locale("C"))
	if err := pg.Start(); err != nil {
		t.Fatalf("start embedded postgres: %v", err)
	}
	t.Cleanup(func() { _ = pg.Stop() })

	dsn := fmt.Sprintf("postgres://obscura:obscura@localhost:%d/obscura_token_test?sslmode=disable", port)
	rawDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open pgx: %v", err)
	}
	t.Cleanup(func() { _ = rawDB.Close() })
	rawDB.SetMaxOpenConns(30) // a real pool — the entire point of the test

	schema := `
		CREATE TABLE obs_accounts (
			user_did             TEXT PRIMARY KEY,
			transparent_balance  TEXT NOT NULL,
			updated_at           TEXT NOT NULL
		);
		CREATE TABLE obs_supply (
			id           INTEGER PRIMARY KEY,
			total_supply TEXT NOT NULL,
			circulating  TEXT NOT NULL DEFAULT '0',
			burned       TEXT NOT NULL DEFAULT '0',
			last_updated TEXT NOT NULL
		);
		CREATE TABLE obs_transactions (
			id         TEXT PRIMARY KEY,
			from_did   TEXT NOT NULL,
			to_did     TEXT NOT NULL,
			amount     TEXT NOT NULL,
			fee        TEXT NOT NULL DEFAULT '0',
			tx_type    TEXT NOT NULL,
			memo       TEXT DEFAULT '',
			status     TEXT NOT NULL DEFAULT 'confirmed',
			created_at TEXT NOT NULL
		);
	`
	if _, err := rawDB.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	// Huge headroom so nothing in this test ever hits the total-supply cap,
	// and circulating starts pre-funded (this test seeds sender balances
	// directly via raw SQL, bypassing Mint, so circulating must already
	// account for them or Transfer's burn-from-circulating step underflows).
	totalSupply := new(big.Int).Exp(big.NewInt(10), big.NewInt(40), nil).String()
	circulating := new(big.Int).Exp(big.NewInt(10), big.NewInt(30), nil).String()
	if _, err := rawDB.Exec(
		`INSERT INTO obs_supply (id, total_supply, circulating, burned, last_updated) VALUES (1, $1, $2, '0', 'now')`,
		totalSupply, circulating,
	); err != nil {
		t.Fatalf("seed supply: %v", err)
	}
	return rawDB
}

func pgBalance(t *testing.T, rawDB *sql.DB, did string) *big.Int {
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

// withPostgresDB installs a Postgres-backed db.DB for the duration of fn,
// then restores whatever db.DB pointed at before (the sqlite instance
// TestMain in token_test.go already set up for every other test in this
// package's binary). It also sets OBSCURA_DB_DRIVER=postgres (t.Setenv,
// auto-restored) — token.go's txBalance/readSupply branch on
// dbi.DriverFromEnv(), the same single-source-of-truth every other
// driver-aware DDL in this codebase uses (in production Init() sets db.DB
// and the env var together, so they're never out of sync the way a test
// swapping db.DB alone would leave them).
func withPostgresDB(t *testing.T, rawDB *sql.DB, fn func()) {
	t.Helper()
	t.Setenv("OBSCURA_DB_DRIVER", "postgres")
	prev := dbpkg.DB
	dbpkg.DB = dbpkg.NewConn(rawDB, dbi.DriverPostgres)
	defer func() { dbpkg.DB = prev }()
	fn()
}

// TestTransfer_Postgres_ConcurrentSpendsDontLoseUpdates is the RED/GREEN
// test.
//
// Race-condition tests are inherently probabilistic, so this runs many
// independent rounds. Each round funds a fresh sender with EXACTLY enough
// balance for K of N concurrent Transfer attempts to succeed (K < N), then
// releases all N goroutines at once via a start barrier. It then checks,
// per round:
//   - exactly K attempts succeeded (not more — a lost update on the balance
//     read lets an over-committed transfer pass its "insufficient balance"
//     check);
//   - the sender's final balance is exactly 0 (every successful debit
//     actually landed — a lost update on the balance WRITE leaves stale
//     money behind);
//   - total value is conserved: sum of recipient credits + fee pool credit
//   - burned amount equals exactly what left the sender.
//
// Before the SELECT...FOR UPDATE fix, txBalance's plain SELECT lets two
// concurrent transactions both read the same pre-debit balance and both
// pass the sufficiency check, so more than K attempts can "succeed" while
// the sender's balance reflects only some of the debits (or none, in the
// worst interleaving) — money is created or destroyed.
func TestTransfer_Postgres_ConcurrentSpendsDontLoseUpdates(t *testing.T) {
	rawDB := openPostgresTokenDB(t)

	const rounds = 20
	const attemptsPerRound = 6
	const affordable = 3 // K: exactly this many of attemptsPerRound can succeed

	amount := big.NewInt(1_000_000)
	fee := token.TransferFee()
	totalDebit := new(big.Int).Add(amount, fee)
	burnPart := new(big.Int).Div(fee, big.NewInt(2))
	poolPart := new(big.Int).Sub(fee, burnPart)

	withPostgresDB(t, rawDB, func() {
		for round := 0; round < rounds; round++ {
			sender := fmt.Sprintf("did:obs:sender-%d", round)
			startBalance := new(big.Int).Mul(totalDebit, big.NewInt(affordable))

			if _, err := rawDB.Exec(
				`INSERT INTO obs_accounts (user_did, transparent_balance, updated_at) VALUES ($1, $2, 'now')`,
				sender, startBalance.String(),
			); err != nil {
				t.Fatalf("round %d: seed sender: %v", round, err)
			}
			supplyBefore := pgSupplySnapshot(t, rawDB)

			var ready sync.WaitGroup
			start := make(chan struct{})
			var wg sync.WaitGroup
			succeeded := make([]bool, attemptsPerRound)
			recipients := make([]string, attemptsPerRound)

			ready.Add(attemptsPerRound)
			for i := 0; i < attemptsPerRound; i++ {
				recipients[i] = fmt.Sprintf("did:obs:recipient-%d-%d", round, i)
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					ready.Done()
					<-start
					_, err := token.Transfer(context.Background(), sender, recipients[i], amount, "race-test")
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
				t.Fatalf("round %d: %d/%d transfers succeeded, want exactly %d",
					round, successCount, attemptsPerRound, affordable)
			}

			finalSenderBal := pgBalance(t, rawDB, sender)
			if finalSenderBal.Sign() != 0 {
				t.Fatalf("round %d: sender balance = %s, want 0 (exactly %d debits of %s each from a balance of exactly %d*%s)",
					round, finalSenderBal, affordable, totalDebit, affordable, totalDebit)
			}

			// Value conservation: every recipient that "succeeded" must show
			// exactly `amount` credited; recipients that didn't must show 0.
			creditedRecipients := 0
			for i, r := range recipients {
				bal := pgBalance(t, rawDB, r)
				wantCredited := succeeded[i]
				gotCredited := bal.Cmp(amount) == 0
				if wantCredited != gotCredited {
					t.Fatalf("round %d recipient %d: succeeded=%v but balance=%s (want %s if succeeded, 0 otherwise)",
						round, i, succeeded[i], bal, amount)
				}
				if gotCredited {
					creditedRecipients++
				}
			}
			if creditedRecipients != affordable {
				t.Fatalf("round %d: %d recipients credited, want %d", round, creditedRecipients, affordable)
			}

			supplyAfter := pgSupplySnapshot(t, rawDB)
			wantBurnedDelta := new(big.Int).Mul(burnPart, big.NewInt(int64(affordable)))
			gotBurnedDelta := new(big.Int).Sub(supplyAfter.burned, supplyBefore.burned)
			if gotBurnedDelta.Cmp(wantBurnedDelta) != 0 {
				t.Fatalf("round %d: burned delta = %s, want %s", round, gotBurnedDelta, wantBurnedDelta)
			}

			feePoolBal := pgBalance(t, rawDB, token.FeePoolDID)
			wantFeePool := new(big.Int).Mul(poolPart, big.NewInt(int64(successAcrossRounds(round, affordable))))
			if feePoolBal.Cmp(wantFeePool) != 0 {
				t.Fatalf("round %d: fee pool balance = %s, want %s (cumulative across rounds so far)",
					round, feePoolBal, wantFeePool)
			}
		}
	})
}

// successAcrossRounds computes the expected cumulative successful-transfer
// count through and including `round` (rounds are 0-indexed), since
// FeePoolDID accumulates across the whole test rather than resetting per
// round.
func successAcrossRounds(round, affordable int) int {
	return (round + 1) * affordable
}

type supplySnapshot struct {
	burned *big.Int
}

func pgSupplySnapshot(t *testing.T, rawDB *sql.DB) supplySnapshot {
	t.Helper()
	var burned string
	if err := rawDB.QueryRow(`SELECT burned FROM obs_supply WHERE id = 1`).Scan(&burned); err != nil {
		t.Fatalf("read supply: %v", err)
	}
	b, ok := new(big.Int).SetString(burned, 10)
	if !ok {
		t.Fatalf("corrupt burned: %q", burned)
	}
	return supplySnapshot{burned: b}
}
