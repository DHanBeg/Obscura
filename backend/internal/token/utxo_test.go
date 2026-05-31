package token_test

// Tests for the UTXONote / NullifierSet / SpendNote helpers (utxo.go).
//
// These tests share the same package-level temp-DB harness defined in
// token_test.go (TestMain initializes db.Init on a temp directory).

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"obscura.network/core/internal/token"
)

// fld is a tiny helper that produces a *big.Int from a decimal string.
func fld(t *testing.T, s string) *big.Int {
	t.Helper()
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		t.Fatalf("parse field elem %q", s)
	}
	return v
}

func TestNewUTXONote_DeterministicCommitment(t *testing.T) {
	value := fld(t, "1000000000000000000") // 1 OBS
	secret := fld(t, "111")
	salt := fld(t, "222")

	n1, err := token.NewUTXONote(value, secret, salt)
	if err != nil {
		t.Fatalf("note 1: %v", err)
	}
	n2, err := token.NewUTXONote(value, secret, salt)
	if err != nil {
		t.Fatalf("note 2: %v", err)
	}

	if n1.Commitment != n2.Commitment {
		t.Errorf("commitment not deterministic:\n  n1 = %s\n  n2 = %s", n1.Commitment, n2.Commitment)
	}
	if n1.Nullifier != n2.Nullifier {
		t.Errorf("nullifier not deterministic:\n  n1 = %s\n  n2 = %s", n1.Nullifier, n2.Nullifier)
	}
	if n1.LeafIndex != -1 {
		t.Errorf("leaf_index = %d, want -1 for fresh note", n1.LeafIndex)
	}
}

func TestNewUTXONote_DifferentSaltDifferentCommitment(t *testing.T) {
	value := fld(t, "42")
	secret := fld(t, "111")

	n1, err := token.NewUTXONote(value, secret, fld(t, "1"))
	if err != nil {
		t.Fatalf("n1: %v", err)
	}
	n2, err := token.NewUTXONote(value, secret, fld(t, "2"))
	if err != nil {
		t.Fatalf("n2: %v", err)
	}
	if n1.Commitment == n2.Commitment {
		t.Error("commitment must change when salt changes")
	}
	if n1.Nullifier == n2.Nullifier {
		t.Error("nullifier must change when salt changes")
	}
}

func TestNewUTXONote_RejectsNil(t *testing.T) {
	if _, err := token.NewUTXONote(nil, fld(t, "1"), fld(t, "1")); err == nil {
		t.Error("expected error on nil value")
	}
	if _, err := token.NewUTXONote(fld(t, "1"), nil, fld(t, "1")); err == nil {
		t.Error("expected error on nil secret")
	}
	if _, err := token.NewUTXONote(fld(t, "1"), fld(t, "1"), nil); err == nil {
		t.Error("expected error on nil salt")
	}
}

func TestNewUTXONote_RejectsNegativeValue(t *testing.T) {
	neg := new(big.Int).Neg(fld(t, "1"))
	if _, err := token.NewUTXONote(neg, fld(t, "1"), fld(t, "1")); err == nil {
		t.Error("expected error on negative value")
	}
}

func TestNullifierSet_MarkAndCheck(t *testing.T) {
	ctx := context.Background()
	null := "test-nullifier-mark-and-check-001"

	used, err := token.NullifierUsed(ctx, null)
	if err != nil {
		t.Fatalf("initial check: %v", err)
	}
	if used {
		t.Fatal("fresh nullifier should not be marked used")
	}

	if err := token.MarkNullifierSpent(ctx, null); err != nil {
		t.Fatalf("mark: %v", err)
	}

	used, err = token.NullifierUsed(ctx, null)
	if err != nil {
		t.Fatalf("post-mark check: %v", err)
	}
	if !used {
		t.Fatal("nullifier should be marked used after MarkNullifierSpent")
	}

	// Second mark must fail with ErrShieldedNullifierUsed.
	err = token.MarkNullifierSpent(ctx, null)
	if !errors.Is(err, token.ErrShieldedNullifierUsed) {
		t.Errorf("double-mark error = %v, want ErrShieldedNullifierUsed", err)
	}
}

func TestSpendNote_HappyPath(t *testing.T) {
	ctx := context.Background()

	value := fld(t, "5000000000000000000") // 5 OBS
	sender, err := token.NewUTXONote(value, fld(t, "999"), fld(t, "11100"))
	if err != nil {
		t.Fatalf("sender note: %v", err)
	}
	sender.LeafIndex = 7 // pretend it was inserted at leaf 7

	res, recipientNote, err := token.SpendNote(ctx, sender,
		fld(t, "3000000000000000000"), // spend 3 OBS
		fld(t, "777"),                 // recipient secret
		fld(t, "88800"),               // recipient salt
	)
	if err != nil {
		t.Fatalf("spend: %v", err)
	}
	if res.InputNullifier != sender.Nullifier {
		t.Errorf("input_nullifier = %s, want %s", res.InputNullifier, sender.Nullifier)
	}
	if res.SpentLeafIndex != 7 {
		t.Errorf("spent_leaf_index = %d, want 7", res.SpentLeafIndex)
	}
	if res.RecipientCommitment != recipientNote.Commitment {
		t.Errorf("recipient_commitment mismatch:\n  result = %s\n  note   = %s",
			res.RecipientCommitment, recipientNote.Commitment)
	}
	if recipientNote.Value.Cmp(fld(t, "3000000000000000000")) != 0 {
		t.Errorf("recipient note value = %s, want 3e18", recipientNote.Value)
	}
}

func TestSpendNote_RejectsOverdraft(t *testing.T) {
	ctx := context.Background()
	value := fld(t, "100")
	sender, err := token.NewUTXONote(value, fld(t, "1"), fld(t, "54321"))
	if err != nil {
		t.Fatalf("sender: %v", err)
	}
	sender.LeafIndex = 1

	_, _, err = token.SpendNote(ctx, sender, fld(t, "101"), fld(t, "2"), fld(t, "3"))
	if err == nil {
		t.Fatal("expected overdraft rejection")
	}
}

func TestSpendNote_RejectsUnsettled(t *testing.T) {
	ctx := context.Background()
	sender, err := token.NewUTXONote(fld(t, "100"), fld(t, "1"), fld(t, "99999"))
	if err != nil {
		t.Fatalf("sender: %v", err)
	}
	// LeafIndex stays at -1 (note not inserted).

	_, _, err = token.SpendNote(ctx, sender, fld(t, "10"), fld(t, "2"), fld(t, "3"))
	if err == nil {
		t.Fatal("expected unsettled-note rejection")
	}
}

func TestSpendNote_RejectsAlreadySpent(t *testing.T) {
	ctx := context.Background()
	sender, err := token.NewUTXONote(fld(t, "100"), fld(t, "1"), fld(t, "11111"))
	if err != nil {
		t.Fatalf("sender: %v", err)
	}
	sender.LeafIndex = 0

	if err := token.MarkNullifierSpent(ctx, sender.Nullifier); err != nil {
		t.Fatalf("mark: %v", err)
	}

	_, _, err = token.SpendNote(ctx, sender, fld(t, "10"), fld(t, "2"), fld(t, "3"))
	if !errors.Is(err, token.ErrShieldedNullifierUsed) {
		t.Errorf("err = %v, want ErrShieldedNullifierUsed", err)
	}
}
