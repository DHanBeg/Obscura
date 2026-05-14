package token_test

// Tests for the OBS token off-chain state layer (ADR-0010).
//
// Uses a real on-disk SQLite DB in a temp dir (the same pattern as
// internal/api/integration_test.go) — modernc.org/sqlite is pure Go and the
// migration system runs in db.Init, so obs_* tables exist after setup.

import (
	"context"
	"math/big"
	"os"
	"testing"

	"obscura.network/core/internal/db"
	"obscura.network/core/internal/token"
)

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "obscura-token-test-*")
	if err != nil {
		panic("temp dir: " + err.Error())
	}
	if err := db.Init(tmpDir); err != nil {
		panic("test DB init: " + err.Error())
	}
	code := m.Run()
	db.Close()
	os.RemoveAll(tmpDir)
	os.Exit(code)
}

// obs returns n whole OBS as a smallest-unit *big.Int (n * 10^18).
func obs(n int64) *big.Int {
	return new(big.Int).Mul(big.NewInt(n), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
}

// fund mints `amount` to `did` and fails the test on error.
func fund(t *testing.T, did string, amount *big.Int) {
	t.Helper()
	if _, err := token.Mint(context.Background(), did, amount, "test funding"); err != nil {
		t.Fatalf("fund %s: %v", did, err)
	}
}

func mustBalance(t *testing.T, did string) *big.Int {
	t.Helper()
	b, err := token.Balance(did)
	if err != nil {
		t.Fatalf("balance %s: %v", did, err)
	}
	return b
}

func mustSupply(t *testing.T) *token.SupplyInfo {
	t.Helper()
	s, err := token.Supply()
	if err != nil {
		t.Fatalf("supply: %v", err)
	}
	return s
}

func TestTransfer_HappyPath(t *testing.T) {
	alice := "did:obs:happy-alice"
	bob := "did:obs:happy-bob"
	fund(t, alice, obs(100))

	supBefore := mustSupply(t)
	circBefore, _ := new(big.Int).SetString(supBefore.Circulating, 10)
	burnedBefore, _ := new(big.Int).SetString(supBefore.Burned, 10)
	poolBefore := mustBalance(t, token.FeePoolDID)

	amount := obs(10)
	fee := token.TransferFee()
	burnPart := new(big.Int).Div(fee, big.NewInt(2))
	poolPart := new(big.Int).Sub(fee, burnPart)

	txID, err := token.Transfer(context.Background(), alice, bob, amount, "lunch")
	if err != nil {
		t.Fatalf("transfer: %v", err)
	}
	if txID == "" {
		t.Fatal("empty tx id")
	}

	// Alice: -amount -fee
	wantAlice := new(big.Int).Sub(obs(100), new(big.Int).Add(amount, fee))
	if got := mustBalance(t, alice); got.Cmp(wantAlice) != 0 {
		t.Errorf("alice balance = %s, want %s", got, wantAlice)
	}
	// Bob: +amount
	if got := mustBalance(t, bob); got.Cmp(amount) != 0 {
		t.Errorf("bob balance = %s, want %s", got, amount)
	}
	// Fee pool: +poolPart
	wantPool := new(big.Int).Add(poolBefore, poolPart)
	if got := mustBalance(t, token.FeePoolDID); got.Cmp(wantPool) != 0 {
		t.Errorf("feepool balance = %s, want %s", got, wantPool)
	}

	// Supply: circulating -burnPart, burned +burnPart.
	supAfter := mustSupply(t)
	circAfter, _ := new(big.Int).SetString(supAfter.Circulating, 10)
	burnedAfter, _ := new(big.Int).SetString(supAfter.Burned, 10)
	wantCirc := new(big.Int).Sub(circBefore, burnPart)
	wantBurned := new(big.Int).Add(burnedBefore, burnPart)
	if circAfter.Cmp(wantCirc) != 0 {
		t.Errorf("circulating = %s, want %s", circAfter, wantCirc)
	}
	if burnedAfter.Cmp(wantBurned) != 0 {
		t.Errorf("burned = %s, want %s", burnedAfter, wantBurned)
	}

	// History records the transfer for both parties.
	hist, err := token.History(alice, 50)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	found := false
	for _, h := range hist {
		if h.ID == txID {
			found = true
			if h.TxType != "transfer" || h.Amount != amount.String() || h.Fee != fee.String() {
				t.Errorf("history row mismatch: %+v", h)
			}
		}
	}
	if !found {
		t.Error("transfer not found in alice history")
	}
}

func TestTransfer_InsufficientBalance(t *testing.T) {
	alice := "did:obs:poor-alice"
	bob := "did:obs:poor-bob"
	fund(t, alice, obs(5)) // not enough to cover 10 + fee

	_, err := token.Transfer(context.Background(), alice, bob, obs(10), "too much")
	if err == nil {
		t.Fatal("expected insufficient balance error, got nil")
	}

	// Balances must be untouched.
	if got := mustBalance(t, alice); got.Cmp(obs(5)) != 0 {
		t.Errorf("alice balance changed on failed transfer: %s", got)
	}
	if got := mustBalance(t, bob); got.Sign() != 0 {
		t.Errorf("bob balance changed on failed transfer: %s", got)
	}

	// Edge case: balance exactly == amount but no room for fee → also rejected.
	carol := "did:obs:exact-carol"
	dave := "did:obs:exact-dave"
	fund(t, carol, obs(10))
	if _, err := token.Transfer(context.Background(), carol, dave, obs(10), ""); err == nil {
		t.Error("expected rejection when balance cannot cover amount+fee")
	}
	if got := mustBalance(t, carol); got.Cmp(obs(10)) != 0 {
		t.Errorf("carol balance changed: %s", got)
	}
}

// TestTransfer_Atomicity verifies a failed transfer leaves zero partial state:
// balances, fee pool, and supply are all exactly as they were before.
func TestTransfer_Atomicity(t *testing.T) {
	alice := "did:obs:atomic-alice"
	bob := "did:obs:atomic-bob"
	fund(t, alice, obs(1)) // far short of the 50 OBS transfer below

	aliceBefore := mustBalance(t, alice)
	bobBefore := mustBalance(t, bob)
	poolBefore := mustBalance(t, token.FeePoolDID)
	supBefore := mustSupply(t)

	if _, err := token.Transfer(context.Background(), alice, bob, obs(50), "fail"); err == nil {
		t.Fatal("expected failure")
	}

	if mustBalance(t, alice).Cmp(aliceBefore) != 0 {
		t.Error("alice balance mutated by failed transfer")
	}
	if mustBalance(t, bob).Cmp(bobBefore) != 0 {
		t.Error("bob balance mutated by failed transfer")
	}
	if mustBalance(t, token.FeePoolDID).Cmp(poolBefore) != 0 {
		t.Error("fee pool mutated by failed transfer")
	}
	supAfter := mustSupply(t)
	if supAfter.Circulating != supBefore.Circulating || supAfter.Burned != supBefore.Burned {
		t.Errorf("supply mutated by failed transfer: before=%+v after=%+v", supBefore, supAfter)
	}

	// And no transaction row was recorded for the failed attempt.
	hist, _ := token.History(alice, 50)
	for _, h := range hist {
		if h.TxType == "transfer" {
			t.Errorf("failed transfer left a tx row: %+v", h)
		}
	}
}

func TestMintBurn_SupplyTracking(t *testing.T) {
	acct := "did:obs:supply-acct"

	supStart := mustSupply(t)
	circStart, _ := new(big.Int).SetString(supStart.Circulating, 10)
	burnedStart, _ := new(big.Int).SetString(supStart.Burned, 10)

	// Mint 1000 OBS → circulating += 1000, balance += 1000.
	mintAmt := obs(1000)
	if _, err := token.Mint(context.Background(), acct, mintAmt, "genesis"); err != nil {
		t.Fatalf("mint: %v", err)
	}
	if got := mustBalance(t, acct); got.Cmp(mintAmt) != 0 {
		t.Errorf("balance after mint = %s, want %s", got, mintAmt)
	}
	supAfterMint := mustSupply(t)
	circAfterMint, _ := new(big.Int).SetString(supAfterMint.Circulating, 10)
	if want := new(big.Int).Add(circStart, mintAmt); circAfterMint.Cmp(want) != 0 {
		t.Errorf("circulating after mint = %s, want %s", circAfterMint, want)
	}

	// Burn 400 OBS → circulating -= 400, burned += 400, balance -= 400.
	burnAmt := obs(400)
	if _, err := token.Burn(context.Background(), acct, burnAmt, "penalty"); err != nil {
		t.Fatalf("burn: %v", err)
	}
	if got, want := mustBalance(t, acct), obs(600); got.Cmp(want) != 0 {
		t.Errorf("balance after burn = %s, want %s", got, want)
	}
	supAfterBurn := mustSupply(t)
	circAfterBurn, _ := new(big.Int).SetString(supAfterBurn.Circulating, 10)
	burnedAfterBurn, _ := new(big.Int).SetString(supAfterBurn.Burned, 10)
	if want := new(big.Int).Add(circStart, obs(600)); circAfterBurn.Cmp(want) != 0 {
		t.Errorf("circulating after burn = %s, want %s", circAfterBurn, want)
	}
	if want := new(big.Int).Add(burnedStart, burnAmt); burnedAfterBurn.Cmp(want) != 0 {
		t.Errorf("burned after burn = %s, want %s", burnedAfterBurn, want)
	}

	// Burning more than the balance is rejected and changes nothing.
	balBefore := mustBalance(t, acct)
	if _, err := token.Burn(context.Background(), acct, obs(10000), "overburn"); err == nil {
		t.Error("expected burn rejection for over-balance amount")
	}
	if mustBalance(t, acct).Cmp(balBefore) != 0 {
		t.Error("failed burn mutated balance")
	}

	// Total supply is the 1B constant and never changes via mint/burn.
	if supAfterBurn.Total != supStart.Total {
		t.Errorf("total supply changed: %s -> %s", supStart.Total, supAfterBurn.Total)
	}
}
