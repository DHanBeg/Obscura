package token_test

// Shielded transfer tests (spec Bölüm 8.3). Same setup pattern as
// token_test.go — temp on-disk SQLite, db.Init runs all migrations so the
// shielded_* tables exist.
//
// ZK verification strategy:
//   - TestShieldedTransfer_HappyPath drives the real verifier against the
//     fixture proof at circuits/test/token_balance_smoke_proof.json. The
//     fixture's public root signal (778899) is force-loaded into shielded_root
//     via SetShieldedRootForTest so the proof's root matches current state.
//   - Other tests use SetShieldedVerifyHookForTest to stub verification and
//     focus on orchestration (root mismatch, double-spend, mint side-effect).

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"obscura.network/core/internal/token"
	"obscura.network/core/internal/zk"
)

// loadSmokeFixture pulls the prebuilt proof+public_inputs from
// circuits/test/token_balance_smoke_proof.json.
func loadSmokeFixture(t *testing.T) (proofJSON string, publicSignals []string) {
	t.Helper()
	// Resolve repo root from this test file's working dir
	// (E:\obscura\backend\internal\token). circuits is at ../../../circuits.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repo := filepath.Clean(filepath.Join(cwd, "..", "..", ".."))
	fxPath := filepath.Join(repo, "circuits", "test", "token_balance_smoke_proof.json")
	raw, err := os.ReadFile(fxPath)
	if err != nil {
		t.Fatalf("read fixture %s: %v", fxPath, err)
	}
	var fx struct {
		ProofJSON    string   `json:"proof_json"`
		PublicInputs []string `json:"public_inputs"`
	}
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return fx.ProofJSON, fx.PublicInputs
}

// ensureTokenBalanceVKey loads the on-disk verification keys (or embedded ones)
// for the token_balance circuit. The token package tests don't otherwise need
// the ZK subsystem, so we wire it up here on demand.
func ensureTokenBalanceVKey(t *testing.T) {
	t.Helper()
	if zk.IsCircuitKnown(zk.CircuitTokenBalance) {
		return
	}
	// Try embedded first via empty dir; LoadVerificationKeys falls back to
	// the embed.FS baked into the binary.
	if err := zk.LoadVerificationKeys(""); err != nil {
		t.Fatalf("load zk keys: %v", err)
	}
}

func TestShield_HappyPath(t *testing.T) {
	ctx := context.Background()
	alice := "did:obs:shield-alice"
	fund(t, alice, obs(100))

	balBefore := mustBalance(t, alice)
	rootBefore, err := token.GetShieldedRoot()
	if err != nil {
		t.Fatalf("get root: %v", err)
	}

	commitment := "111222333" // arbitrary opaque commitment
	rootAfter, idx, err := token.Shield(ctx, alice, obs(10), commitment)
	if err != nil {
		t.Fatalf("shield: %v", err)
	}
	if rootAfter == "" {
		t.Fatal("empty root_after")
	}
	if rootAfter == rootBefore.Root {
		t.Error("root did not advance after shield")
	}
	if idx != rootBefore.LeafCount {
		t.Errorf("leaf index = %d, want %d", idx, rootBefore.LeafCount)
	}

	// Transparent balance was burned by the amount.
	wantBal := new(big.Int).Sub(balBefore, obs(10))
	if got := mustBalance(t, alice); got.Cmp(wantBal) != 0 {
		t.Errorf("alice balance = %s, want %s (burn not applied)", got, wantBal)
	}

	// Leaf count incremented by 1.
	after, err := token.GetShieldedRoot()
	if err != nil {
		t.Fatalf("get root after: %v", err)
	}
	if after.LeafCount != rootBefore.LeafCount+1 {
		t.Errorf("leaf_count = %d, want %d", after.LeafCount, rootBefore.LeafCount+1)
	}
	if after.Root != rootAfter {
		t.Errorf("stored root = %s, returned %s", after.Root, rootAfter)
	}

	// Leaf appears in the listing.
	notes, err := token.ListShieldedNotes(10)
	if err != nil {
		t.Fatalf("list notes: %v", err)
	}
	found := false
	for _, n := range notes {
		if n.Commitment == commitment && n.LeafIndex == idx {
			found = true
		}
	}
	if !found {
		t.Errorf("commitment %q not in listed notes", commitment)
	}
}

func TestShield_InsufficientBalanceRejected(t *testing.T) {
	ctx := context.Background()
	alice := "did:obs:shield-poor"
	fund(t, alice, obs(1))

	rootBefore, _ := token.GetShieldedRoot()

	_, _, err := token.Shield(ctx, alice, obs(10), "999")
	if err == nil {
		t.Fatal("expected insufficient-balance error")
	}

	// Tree state must be unchanged.
	rootAfter, _ := token.GetShieldedRoot()
	if rootAfter.LeafCount != rootBefore.LeafCount {
		t.Errorf("leaf_count mutated on failed shield: %d -> %d",
			rootBefore.LeafCount, rootAfter.LeafCount)
	}
	if rootAfter.Root != rootBefore.Root {
		t.Errorf("root mutated on failed shield: %s -> %s", rootBefore.Root, rootAfter.Root)
	}

	// Balance untouched.
	if got := mustBalance(t, alice); got.Cmp(obs(1)) != 0 {
		t.Errorf("alice balance changed on failed shield: %s", got)
	}
}

func TestShieldedTransfer_HappyPath(t *testing.T) {
	ctx := context.Background()
	ensureTokenBalanceVKey(t)

	proofJSON, publicSignals := loadSmokeFixture(t)
	if len(publicSignals) != 4 {
		t.Fatalf("fixture has %d public signals, want 4", len(publicSignals))
	}

	// Align the in-DB root with the fixture proof's public root signal.
	fixtureRoot := publicSignals[2]
	if err := token.SetShieldedRootForTest(fixtureRoot, 1); err != nil {
		t.Fatalf("set root: %v", err)
	}

	if err := token.ShieldedTransfer(ctx, proofJSON, publicSignals); err != nil {
		t.Fatalf("shielded transfer: %v", err)
	}

	// Nullifier is now recorded — a second identical transfer must be rejected.
	err := token.ShieldedTransfer(ctx, proofJSON, publicSignals)
	if err == nil {
		t.Error("expected double-spend rejection on replay")
	}

	// Recipient leaf was inserted: leaf_count advanced past the seed.
	after, _ := token.GetShieldedRoot()
	if after.LeafCount < 2 {
		t.Errorf("leaf_count = %d after transfer, want >= 2", after.LeafCount)
	}
}

func TestShieldedTransfer_DoubleSpendRejected(t *testing.T) {
	ctx := context.Background()

	restore := token.SetShieldedVerifyHookForTest(func(_ []byte, _ []string) error {
		return nil // accept any proof
	})
	defer restore()

	// Seed a known root so the proof passes the root check.
	if err := token.SetShieldedRootForTest("424242", 0); err != nil {
		t.Fatalf("set root: %v", err)
	}
	pub := []string{"100", "doublespend-null-1", "424242", "1700000000"}

	if err := token.ShieldedTransfer(ctx, "{}", pub); err != nil {
		t.Fatalf("first transfer: %v", err)
	}

	// Re-align root since the first transfer advanced it.
	after, _ := token.GetShieldedRoot()
	pub[2] = after.Root
	err := token.ShieldedTransfer(ctx, "{}", pub)
	if err == nil {
		t.Fatal("expected nullifier-already-used rejection")
	}
}

func TestShieldedTransfer_InvalidProofRejected(t *testing.T) {
	ctx := context.Background()

	restore := token.SetShieldedVerifyHookForTest(func(_ []byte, _ []string) error {
		return fmt.Errorf("simulated pairing failure")
	})
	defer restore()

	if err := token.SetShieldedRootForTest("555", 0); err != nil {
		t.Fatalf("set root: %v", err)
	}
	pub := []string{"100", "bad-proof-null", "555", "1700000000"}

	beforeNotes, _ := token.GetShieldedRoot()
	err := token.ShieldedTransfer(ctx, "{}", pub)
	if err == nil {
		t.Fatal("expected invalid-proof rejection")
	}
	afterNotes, _ := token.GetShieldedRoot()
	if afterNotes.LeafCount != beforeNotes.LeafCount {
		t.Errorf("leaf_count mutated on rejected proof: %d -> %d",
			beforeNotes.LeafCount, afterNotes.LeafCount)
	}
}

func TestUnshield_HappyPath(t *testing.T) {
	ctx := context.Background()
	restore := token.SetShieldedVerifyHookForTest(func(_ []byte, _ []string) error {
		return nil
	})
	defer restore()

	if err := token.SetShieldedRootForTest("777", 0); err != nil {
		t.Fatalf("set root: %v", err)
	}
	recipient := "did:obs:unshield-recipient"
	balBefore := mustBalance(t, recipient)

	pub := []string{"100", "unshield-null-1", "777", "1700000000"}
	amount := obs(5)
	if err := token.Unshield(ctx, recipient, "{}", pub, amount); err != nil {
		t.Fatalf("unshield: %v", err)
	}

	want := new(big.Int).Add(balBefore, amount)
	if got := mustBalance(t, recipient); got.Cmp(want) != 0 {
		t.Errorf("recipient balance = %s, want %s (mint not applied)", got, want)
	}
}

func TestUnshield_NullifierAlreadyUsed(t *testing.T) {
	ctx := context.Background()
	restore := token.SetShieldedVerifyHookForTest(func(_ []byte, _ []string) error {
		return nil
	})
	defer restore()

	if err := token.SetShieldedRootForTest("888", 0); err != nil {
		t.Fatalf("set root: %v", err)
	}
	recipient := "did:obs:unshield-dup"
	pub := []string{"100", "unshield-dup-null", "888", "1700000000"}

	if err := token.Unshield(ctx, recipient, "{}", pub, obs(2)); err != nil {
		t.Fatalf("first unshield: %v", err)
	}

	// Second unshield with the same nullifier must be rejected and must NOT
	// mint a second time.
	balAfterFirst := mustBalance(t, recipient)
	err := token.Unshield(ctx, recipient, "{}", pub, obs(2))
	if err == nil {
		t.Fatal("expected nullifier-already-used rejection")
	}
	if got := mustBalance(t, recipient); got.Cmp(balAfterFirst) != 0 {
		t.Errorf("balance changed on rejected unshield: %s -> %s", balAfterFirst, got)
	}
}
