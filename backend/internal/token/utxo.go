// Package token — UTXO note model + nullifier set (spec Bölüm 8.3).
//
// UTXONote is the in-memory representation of a shielded note. Notes are
// never stored in cleartext on the server — only the commitment (a Poseidon
// hash) lands in `shielded_notes`. Senders / recipients hold the witness
// (value + owner_secret + salt) client-side and reconstruct the commitment
// when they want to prove ownership.
//
// SpendNote is a server-side helper that drives the spend lifecycle:
//   1. Compute the nullifier from the witness (Poseidon(secret, salt)).
//   2. Check it against NullifierSet (double-spend protection).
//   3. Compute the recipient's new commitment from caller-supplied params.
//   4. Return a witness layout that the client can plug into
//      circuits/token_balance.circom to produce a Groth16 proof.
//
// The actual ZK proof generation stays on the client (snarkjs in browser,
// Rust prover in mobile/desktop). This file only orchestrates the data
// shapes so backend handlers, tests, and tooling agree on field names.
//
// FAZ 3 (NOT in this file):
//   - 2-out spends (recipient + change note)
//   - encrypted note payload delivery (off-band today)
//   - cross-chain note bridging (see internal/bridge)

package token

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"
	"time"

	"github.com/iden3/go-iden3-crypto/poseidon"
	"obscura.network/core/internal/db"
)

// UTXONote — single shielded note. Mirrors the circuit's witness layout.
//
// Field semantics:
//   - Commitment  = Poseidon(secret, value, salt)        — public
//   - Nullifier   = Poseidon(secret, salt)               — public on spend
//   - Value       = atomic-unit balance held by this note — private
//   - OwnerSecret = owner's spending key                  — private
//   - Salt        = blinding factor (32 random bytes)     — private
//
// LeafIndex is filled by the server when the commitment is inserted into the
// shielded tree (see appendLeafTx). For a brand-new note that hasn't been
// inserted yet, LeafIndex is -1.
type UTXONote struct {
	Commitment  string   `json:"commitment"`
	Nullifier   string   `json:"nullifier"`
	Value       *big.Int `json:"-"`            // never marshalled to JSON
	OwnerSecret *big.Int `json:"-"`            // ditto
	Salt        *big.Int `json:"-"`            // ditto
	LeafIndex   int      `json:"leaf_index"`   // -1 if not yet in tree
}

// NewUTXONote constructs a note from its witness components and computes
// commitment + nullifier deterministically. All three inputs must be
// non-nil positive field elements (value may be zero for change notes).
//
// Returns an error if Poseidon hashing fails (only possible for inputs
// outside the BN254 scalar field, which we range-check).
func NewUTXONote(value, ownerSecret, salt *big.Int) (*UTXONote, error) {
	if value == nil || ownerSecret == nil || salt == nil {
		return nil, fmt.Errorf("%w: value/ownerSecret/salt required", ErrShieldedInvalidInput)
	}
	if value.Sign() < 0 {
		return nil, fmt.Errorf("%w: note value must be non-negative", ErrShieldedInvalidInput)
	}
	if value.Cmp(MaxUint256) > 0 {
		return nil, fmt.Errorf("%w: note value exceeds 256 bits", ErrShieldedInvalidInput)
	}

	// commitment = Poseidon(secret, value, salt) — same order as token_balance.circom.
	cmt, err := poseidon.Hash([]*big.Int{ownerSecret, value, salt})
	if err != nil {
		return nil, fmt.Errorf("utxo: poseidon(commitment): %w", err)
	}
	// nullifier = Poseidon(secret, salt)
	nul, err := poseidon.Hash([]*big.Int{ownerSecret, salt})
	if err != nil {
		return nil, fmt.Errorf("utxo: poseidon(nullifier): %w", err)
	}

	return &UTXONote{
		Commitment:  cmt.String(),
		Nullifier:   nul.String(),
		Value:       new(big.Int).Set(value),
		OwnerSecret: new(big.Int).Set(ownerSecret),
		Salt:        new(big.Int).Set(salt),
		LeafIndex:   -1,
	}, nil
}

// SpendNoteResult is the data a client needs to assemble a ZK spend proof.
// It carries the freshly-computed recipient commitment + the spent note's
// public nullifier. The Groth16 proof itself is generated client-side.
type SpendNoteResult struct {
	InputNullifier      string `json:"input_nullifier"`
	RecipientCommitment string `json:"recipient_commitment"`
	SpentLeafIndex      int    `json:"spent_leaf_index"`
}

// SpendNote validates that `note` can be spent (nullifier unused) and
// computes the recipient's new commitment from (recipientSecret, value,
// recipientSalt). The caller is responsible for then producing a Groth16
// proof against CircuitTokenBalance and calling ShieldedTransfer.
//
// This helper does NOT mutate the DB — it's a pure orchestration step
// that catches double-spends early (before the expensive proof gen) and
// produces the recipient_commitment that the circuit will publish.
func SpendNote(
	ctx context.Context,
	note *UTXONote,
	value *big.Int,
	recipientSecret, recipientSalt *big.Int,
) (*SpendNoteResult, *UTXONote, error) {
	if note == nil {
		return nil, nil, fmt.Errorf("%w: note required", ErrShieldedInvalidInput)
	}
	if note.LeafIndex < 0 {
		return nil, nil, fmt.Errorf("%w: note not yet in tree (leaf_index = -1)", ErrShieldedInvalidInput)
	}
	if note.Value == nil || value == nil {
		return nil, nil, fmt.Errorf("%w: value required", ErrShieldedInvalidInput)
	}
	if value.Sign() <= 0 {
		return nil, nil, fmt.Errorf("%w: spend value must be positive", ErrShieldedInvalidInput)
	}
	if value.Cmp(note.Value) > 0 {
		return nil, nil, fmt.Errorf("%w: spend value %s > note value %s",
			ErrShieldedInvalidInput, value, note.Value)
	}

	// Early double-spend check — the actual atomic guard is the UNIQUE
	// constraint inside ShieldedTransfer's transaction.
	used, err := NullifierUsed(ctx, note.Nullifier)
	if err != nil {
		return nil, nil, err
	}
	if used {
		return nil, nil, ErrShieldedNullifierUsed
	}

	// Build the recipient's new note. FAZ 2 simplification: 1-in/1-out, no
	// change note (the spec circuit only emits one balance_commitment public
	// signal). Change-note support arrives with the 1-in/2-out circuit.
	recipientNote, err := NewUTXONote(value, recipientSecret, recipientSalt)
	if err != nil {
		return nil, nil, fmt.Errorf("utxo: recipient note: %w", err)
	}

	return &SpendNoteResult{
		InputNullifier:      note.Nullifier,
		RecipientCommitment: recipientNote.Commitment,
		SpentLeafIndex:      note.LeafIndex,
	}, recipientNote, nil
}

// ─── NullifierSet ────────────────────────────────────────────────────────────
//
// Thin wrapper around the `shielded_nullifiers` table. The table itself is
// declared in db migrations 053; this layer just gives callers a typed API
// that doesn't expose raw SQL.
//
// NOTE: The underlying table doubles as the atomic double-spend guard
// (UNIQUE PRIMARY KEY on `nullifier`). MarkSpent here is therefore an
// inverse of "check then write" — callers should still wrap the
// check-and-mark inside a transaction for full atomicity (see
// ShieldedTransfer for the production pattern).

// NullifierUsed reports whether `nullifier` is already recorded as spent.
// Returns (false, nil) if the row doesn't exist; (true, nil) if it does;
// (false, err) on any other error.
func NullifierUsed(ctx context.Context, nullifier string) (bool, error) {
	if nullifier == "" {
		return false, fmt.Errorf("%w: nullifier empty", ErrShieldedInvalidInput)
	}
	var existing string
	err := db.DB.QueryRowContext(ctx,
		`SELECT nullifier FROM shielded_nullifiers WHERE nullifier = ?`, nullifier,
	).Scan(&existing)
	if err == nil {
		return true, nil
	}
	if err == sql.ErrNoRows {
		return false, nil
	}
	return false, fmt.Errorf("nullifier lookup: %w", err)
}

// MarkNullifierSpent records `nullifier` as spent. Returns
// ErrShieldedNullifierUsed if the row already existed (double-spend race).
//
// Production code should prefer the in-transaction insert inside
// ShieldedTransfer / Unshield so the nullifier write and the leaf insert
// commit together. This standalone helper exists for tooling, tests, and
// one-off admin scripts.
func MarkNullifierSpent(ctx context.Context, nullifier string) error {
	if nullifier == "" {
		return fmt.Errorf("%w: nullifier empty", ErrShieldedInvalidInput)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.DB.ExecContext(ctx,
		`INSERT INTO shielded_nullifiers (nullifier, used_at) VALUES (?, ?)`,
		nullifier, now)
	if err != nil {
		// modernc.org/sqlite surfaces constraint failures with the substring
		// "UNIQUE constraint failed"; treat any insert error on the unique
		// primary key as a double-spend.
		return ErrShieldedNullifierUsed
	}
	return nil
}

// CountNullifiers returns the total number of spent nullifiers (for ops
// dashboards). Cheap — uses the table's primary key index.
func CountNullifiers() (int, error) {
	var n int
	err := db.DB.QueryRow(`SELECT COUNT(*) FROM shielded_nullifiers`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count nullifiers: %w", err)
	}
	return n, nil
}
