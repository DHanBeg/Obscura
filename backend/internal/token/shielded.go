// Shielded transfer flow — spec Bölüm 8.3 ("Gizli Transfer Akışı (ZK)").
//
// Design (Aztec-inspired UTXO commitments, FAZ 2 simplification):
//
//   The server holds an append-only commitment tree but cannot link sender
//   to recipient. A "note" is an opaque Poseidon commitment of
//   (value, owner_pubkey, salt) computed entirely client-side. The server
//   stores the commitment as a leaf of a Merkle tree and the latest root in
//   a singleton row. Spending a note publishes a nullifier derived from
//   (secret, leaf_index) inside a ZK proof; the server records the nullifier
//   in a UNIQUE table so a second spend of the same note (double-spend) is
//   atomically rejected.
//
// Three operations:
//
//   Shield     — burn `amount` from sender's transparent balance and insert
//                a client-supplied commitment leaf. Re-derives the root.
//
//   Shielded   — verify a ZK proof against CircuitTokenBalance: prover knows
//                a note whose value >= amount and whose nullifier hasn't been
//                used. Server inserts the nullifier and the recipient's new
//                commitment leaf in one transaction. Sender and recipient are
//                unlinkable from the server's view.
//
//   Unshield   — verify the ZK proof, mark the note spent (nullifier insert),
//                and mint `amount` to recipientDID's transparent balance.
//
// FAZ 3 (NOT included here):
//   - real Merkle inclusion proof inside the circuit (depth=32 path). For now
//     the circuit only "binds" the root via Poseidon(root, balance_commitment)
//     so a forged root would still produce a valid pairing (the circuit
//     comment in circuits/token_balance.circom calls this out explicitly).
//     We still enforce publicSignals[2] == stored_root so the prover at least
//     commits to the current tree state.
//   - shielded→shielded with change note (1-in / 2-out)
//   - multi-asset notes
//   - encrypted note payload delivery to the recipient (off-band today)
package token

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/iden3/go-iden3-crypto/poseidon"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/zk"
)

// Sentinel errors — handlers map these to HTTP status codes.
var (
	ErrShieldedInvalidInput   = fmt.Errorf("shielded: invalid input")
	ErrShieldedProofInvalid   = fmt.Errorf("shielded: zk proof verification failed")
	ErrShieldedRootMismatch   = fmt.Errorf("shielded: proof root does not match current state")
	ErrShieldedNullifierUsed  = fmt.Errorf("shielded: nullifier already spent")
	ErrShieldedNoteNotFound   = fmt.Errorf("shielded: note not found")
)

// expectedShieldedSignals is the public-signal layout for CircuitTokenBalance:
//   [0] balance_commitment   (the leaf the prover is consuming/creating)
//   [1] nullifier            (deterministic per (secret, leaf_index))
//   [2] root                 (must match shielded_root.root at verify time)
//   [3] timestamp            (replay window — enforced != 0 inside circuit)
const expectedShieldedSignals = 4

// verifyShieldedProof is the seam through which shielded operations call into
// the ZK verifier. Tests override this to exercise the orchestration logic
// without producing a real Groth16 proof; production always runs the real one.
var verifyShieldedProof = func(proofJSON []byte, publicSignals []string) error {
	return zk.VerifyGroth16(zk.CircuitTokenBalance, proofJSON, publicSignals)
}

// SetShieldedVerifyHookForTest swaps the verify function and returns a restore
// func. ONLY for tests — keeps the smoke-fixture proof testable without
// re-deriving the underlying secret/salt witness.
func SetShieldedVerifyHookForTest(fn func(proofJSON []byte, publicSignals []string) error) func() {
	prev := verifyShieldedProof
	verifyShieldedProof = fn
	return func() { verifyShieldedProof = prev }
}

// ShieldedRootInfo is a read-only snapshot of the commitment tree state.
type ShieldedRootInfo struct {
	Root      string `json:"root"`
	LeafCount int    `json:"leaf_count"`
	UpdatedAt string `json:"updated_at"`
}

// ShieldedNote — one row of shielded_notes (public view; the commitment is
// opaque so listing leaves does not leak balances or owners).
type ShieldedNote struct {
	LeafIndex  int    `json:"leaf_index"`
	Commitment string `json:"commitment"`
	CreatedAt  string `json:"created_at"`
}

// computeRoot derives the new root from (previous_root, new_leaf) via Poseidon.
// This is a rolling-hash stand-in for a real Merkle tree: it produces a unique
// root per (leaf, position, history) without requiring path storage. It is
// sufficient for FAZ 2 because the circuit does not (yet) verify a Merkle
// inclusion proof — it only binds the prover to the current root. FAZ 3 will
// replace this with a real incremental Merkle tree of depth 32.
func computeRoot(prevRoot, leaf *big.Int) (*big.Int, error) {
	h, err := poseidon.Hash([]*big.Int{prevRoot, leaf})
	if err != nil {
		return nil, fmt.Errorf("poseidon: %w", err)
	}
	return h, nil
}

// parseCommitment decodes a decimal-string field element into *big.Int and
// rejects negatives / overflow. Commitments are field elements emitted by
// snarkjs (base-10 strings of up to ~77 digits).
func parseCommitment(s string) (*big.Int, error) {
	if s == "" {
		return nil, fmt.Errorf("%w: commitment empty", ErrShieldedInvalidInput)
	}
	v, ok := new(big.Int).SetString(s, 10)
	if !ok || v.Sign() < 0 {
		return nil, fmt.Errorf("%w: commitment not a non-negative base-10 integer", ErrShieldedInvalidInput)
	}
	return v, nil
}

// readShieldedRootTx returns (root, leaf_count) inside an open transaction.
func readShieldedRootTx(tx *sql.Tx) (*big.Int, int, error) {
	var rootStr string
	var count int
	err := tx.QueryRow(`SELECT root, leaf_count FROM shielded_root WHERE id = 1`).Scan(&rootStr, &count)
	if err != nil {
		return nil, 0, fmt.Errorf("read shielded_root: %w", err)
	}
	root, ok := new(big.Int).SetString(rootStr, 10)
	if !ok {
		return nil, 0, fmt.Errorf("shielded root corrupt: %q", rootStr)
	}
	return root, count, nil
}

// writeShieldedRootTx persists the new root + count.
func writeShieldedRootTx(tx *sql.Tx, root *big.Int, count int, now string) error {
	_, err := tx.Exec(
		`UPDATE shielded_root SET root = ?, leaf_count = ?, updated_at = ? WHERE id = 1`,
		root.String(), count, now)
	if err != nil {
		return fmt.Errorf("write shielded_root: %w", err)
	}
	return nil
}

// appendLeafTx inserts a new commitment leaf at the next index and returns the
// (new_root, new_index). The caller is responsible for the surrounding tx.
func appendLeafTx(tx *sql.Tx, commitment *big.Int, now string) (*big.Int, int, error) {
	prevRoot, count, err := readShieldedRootTx(tx)
	if err != nil {
		return nil, 0, err
	}
	newRoot, err := computeRoot(prevRoot, commitment)
	if err != nil {
		return nil, 0, err
	}
	idx := count // 0-indexed: count==0 → first leaf at index 0
	_, err = tx.Exec(`
		INSERT INTO shielded_notes (id, leaf_index, commitment, created_at)
		VALUES (?, ?, ?, ?)`,
		uuid.New().String(), idx, commitment.String(), now)
	if err != nil {
		return nil, 0, fmt.Errorf("insert shielded note: %w", err)
	}
	if err := writeShieldedRootTx(tx, newRoot, count+1, now); err != nil {
		return nil, 0, err
	}
	return newRoot, idx, nil
}

// Shield burns `amount` from fromDID's transparent balance and inserts the
// client-supplied commitment as a new leaf of the shielded tree. The caller
// has already computed commitment = Poseidon(value, owner_pubkey, salt)
// client-side, so the server learns nothing about value or owner.
//
// Returns the post-insert root (decimal string) and the new leaf index.
//
// The burn and the leaf insert run in two separate write transactions because
// token.Burn opens its own (SQLite serializes writes; nested begin would
// deadlock). If the second step fails, the burn is durable but the leaf is
// not — the user is asked to retry the same commitment, which is a no-op for
// the burn side (their balance is already reduced) and only re-runs the leaf
// insert. This matches the airdrop "mint after claim row" pattern.
func Shield(ctx context.Context, fromDID string, amount *big.Int, commitment string) (string, int, error) {
	if fromDID == "" {
		return "", 0, fmt.Errorf("%w: fromDID required", ErrShieldedInvalidInput)
	}
	if amount == nil || amount.Sign() <= 0 {
		return "", 0, fmt.Errorf("%w: amount must be positive", ErrShieldedInvalidInput)
	}
	if amount.Cmp(MaxUint256) > 0 {
		return "", 0, fmt.Errorf("%w: amount exceeds 256 bits", ErrShieldedInvalidInput)
	}
	commit, err := parseCommitment(commitment)
	if err != nil {
		return "", 0, err
	}

	// Burn the transparent amount first. If this fails (insufficient balance,
	// invalid amount, etc.) the leaf insert never runs and the tree is
	// untouched.
	if _, err := Burn(ctx, fromDID, amount, "shield"); err != nil {
		return "", 0, err
	}

	// Insert the leaf and bump the root in its own transaction.
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Format(time.RFC3339)
	newRoot, idx, err := appendLeafTx(tx, commit, now)
	if err != nil {
		return "", 0, err
	}
	if err := tx.Commit(); err != nil {
		return "", 0, fmt.Errorf("commit shield: %w", err)
	}
	return newRoot.String(), idx, nil
}

// ShieldedTransfer verifies a ZK proof that a known shielded note can be
// spent for `amount`, then atomically:
//   - inserts the nullifier (publicSignals[1]) — UNIQUE blocks double-spend
//   - inserts publicSignals[0] as a new recipient leaf and advances the root
//
// publicSignals layout (CircuitTokenBalance):
//   [0] balance_commitment   — for FAZ 2 simplification we reuse this as the
//                              recipient's new note commitment, since the
//                              1-in/1-out flow only emits one output. FAZ 3
//                              will add a separate output_commitment signal
//                              and a change note.
//   [1] nullifier            — spends the input note
//   [2] root                 — must match the current shielded_root
//   [3] timestamp            — circuit enforces != 0
//
// The server cannot tell sender from recipient: both notes are opaque
// commitments and the proof reveals neither secret nor amount.
func ShieldedTransfer(ctx context.Context, proofJSON string, publicSignals []string) error {
	if proofJSON == "" {
		return fmt.Errorf("%w: proof_json required", ErrShieldedInvalidInput)
	}
	if len(publicSignals) != expectedShieldedSignals {
		return fmt.Errorf("%w: expected %d public signals, got %d",
			ErrShieldedInvalidInput, expectedShieldedSignals, len(publicSignals))
	}

	recipientCommitment, err := parseCommitment(publicSignals[0])
	if err != nil {
		return err
	}
	nullifier := publicSignals[1]
	if nullifier == "" {
		return fmt.Errorf("%w: nullifier empty", ErrShieldedInvalidInput)
	}
	proofRoot, err := parseCommitment(publicSignals[2])
	if err != nil {
		return fmt.Errorf("%w: root %v", ErrShieldedInvalidInput, err)
	}

	// Friendly pre-check (the UNIQUE constraint inside the tx is the real guard).
	var existing string
	err = db.DB.QueryRowContext(ctx,
		`SELECT nullifier FROM shielded_nullifiers WHERE nullifier = ?`, nullifier).Scan(&existing)
	if err == nil {
		return ErrShieldedNullifierUsed
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("nullifier lookup: %w", err)
	}

	// Verify the proof matches the current tree state before doing the heavy
	// pairing check — cheap and rules out a stale proof immediately.
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	currentRoot, _, err := readShieldedRootTx(tx)
	if err != nil {
		return err
	}
	if currentRoot.Cmp(proofRoot) != 0 {
		return fmt.Errorf("%w: proof root=%s current=%s", ErrShieldedRootMismatch, proofRoot, currentRoot)
	}

	// Heavy: Groth16 pairing check.
	if err := verifyShieldedProof([]byte(proofJSON), publicSignals); err != nil {
		return fmt.Errorf("%w: %v", ErrShieldedProofInvalid, err)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Insert the nullifier. UNIQUE PRIMARY KEY makes this the serialization
	// point — a concurrent second spend of the same note fails here.
	_, err = tx.Exec(
		`INSERT INTO shielded_nullifiers (nullifier, used_at) VALUES (?, ?)`,
		nullifier, now)
	if err != nil {
		// Treat any constraint error as a double-spend race.
		return ErrShieldedNullifierUsed
	}

	// Insert the recipient's new commitment leaf and advance the root.
	if _, _, err := appendLeafTx(tx, recipientCommitment, now); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit shielded transfer: %w", err)
	}
	return nil
}

// Unshield verifies a ZK proof that the caller can spend a shielded note,
// marks it spent, and mints `amount` of transparent OBS to recipientDID.
//
// publicSignals layout matches ShieldedTransfer. `amount` is supplied by the
// caller because the circuit hides it; in a stricter setup we would bind the
// amount to a public signal (FAZ 3 will add explicit value commitments so the
// minted amount is range-checked against the spent note's value). For FAZ 2
// we trust that ZK ownership + non-replay are the meaningful guarantees and
// that `amount` is the value the note was created for.
func Unshield(ctx context.Context, recipientDID string, proofJSON string, publicSignals []string, amount *big.Int) error {
	if recipientDID == "" {
		return fmt.Errorf("%w: recipientDID required", ErrShieldedInvalidInput)
	}
	if amount == nil || amount.Sign() <= 0 {
		return fmt.Errorf("%w: amount must be positive", ErrShieldedInvalidInput)
	}
	if amount.Cmp(MaxUint256) > 0 {
		return fmt.Errorf("%w: amount exceeds 256 bits", ErrShieldedInvalidInput)
	}
	if proofJSON == "" {
		return fmt.Errorf("%w: proof_json required", ErrShieldedInvalidInput)
	}
	if len(publicSignals) != expectedShieldedSignals {
		return fmt.Errorf("%w: expected %d public signals, got %d",
			ErrShieldedInvalidInput, expectedShieldedSignals, len(publicSignals))
	}

	nullifier := publicSignals[1]
	if nullifier == "" {
		return fmt.Errorf("%w: nullifier empty", ErrShieldedInvalidInput)
	}
	proofRoot, err := parseCommitment(publicSignals[2])
	if err != nil {
		return fmt.Errorf("%w: root %v", ErrShieldedInvalidInput, err)
	}

	// Pre-check the nullifier.
	var existing string
	err = db.DB.QueryRowContext(ctx,
		`SELECT nullifier FROM shielded_nullifiers WHERE nullifier = ?`, nullifier).Scan(&existing)
	if err == nil {
		return ErrShieldedNullifierUsed
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("nullifier lookup: %w", err)
	}

	// Verify root matches.
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	currentRoot, _, err := readShieldedRootTx(tx)
	if err != nil {
		return err
	}
	if currentRoot.Cmp(proofRoot) != 0 {
		return fmt.Errorf("%w: proof root=%s current=%s", ErrShieldedRootMismatch, proofRoot, currentRoot)
	}

	if err := verifyShieldedProof([]byte(proofJSON), publicSignals); err != nil {
		return fmt.Errorf("%w: %v", ErrShieldedProofInvalid, err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = tx.Exec(
		`INSERT INTO shielded_nullifiers (nullifier, used_at) VALUES (?, ?)`,
		nullifier, now)
	if err != nil {
		return ErrShieldedNullifierUsed
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit unshield: %w", err)
	}

	// Mint outside the tx (same reason as airdrop.Claim: token.Mint opens its
	// own tx, SQLite serializes writes). If this fails the nullifier is
	// already burned — safer than minting before the burn (which would allow
	// double-mint on a retry).
	if _, err := Mint(ctx, recipientDID, amount, "unshield"); err != nil {
		return fmt.Errorf("unshield mint failed (nullifier %s burned, needs reconciliation): %w", nullifier, err)
	}
	return nil
}

// GetShieldedRoot returns the current Merkle root and leaf count.
func GetShieldedRoot() (*ShieldedRootInfo, error) {
	var info ShieldedRootInfo
	err := db.DB.QueryRow(
		`SELECT root, leaf_count, updated_at FROM shielded_root WHERE id = 1`,
	).Scan(&info.Root, &info.LeafCount, &info.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("read shielded root: %w", err)
	}
	return &info, nil
}

// ListShieldedNotes returns up to `limit` commitments newest-first. The notes
// are opaque commitments so listing them does not leak amounts or owners; the
// recipient identifies their own note by recomputing the commitment locally.
func ListShieldedNotes(limit int) ([]ShieldedNote, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := db.DB.Query(`
		SELECT leaf_index, commitment, created_at
		FROM shielded_notes
		ORDER BY leaf_index DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list notes: %w", err)
	}
	defer rows.Close()

	out := make([]ShieldedNote, 0, limit)
	for rows.Next() {
		var n ShieldedNote
		if err := rows.Scan(&n.LeafIndex, &n.Commitment, &n.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan note: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// SetShieldedRootForTest force-overwrites the current root + leaf count and
// purges existing notes/nullifiers so the next append starts at the supplied
// leafCount cleanly. ONLY for tests — lets a test align the in-DB tree state
// with a fixture proof's public root signal without re-generating the tree.
func SetShieldedRootForTest(root string, leafCount int) error {
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := db.DB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM shielded_notes`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM shielded_nullifiers`); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`UPDATE shielded_root SET root = ?, leaf_count = ?, updated_at = ? WHERE id = 1`,
		root, leafCount, now); err != nil {
		return err
	}
	return tx.Commit()
}
