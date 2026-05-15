package governance_test

// Tests for the ZK-voting governance layer (ADR-0012). The same TestMain DB
// bootstrap pattern as internal/airdrop and internal/staking: pure-Go SQLite
// in a temp dir, migrations run via db.Init.
//
// ZK proof verification approach:
//   - Most lifecycle tests stub the vote_proof verification via
//     governance.SetVerifyHookForTest. Generating a fresh Groth16 proof per
//     test would require snarkjs + .zkey + .wasm — too heavy and fragile here,
//     and the real proof verification is already covered by
//     internal/zk/verifier_test.go (TestVerifyGroth16_VoteProof against the
//     circuits/test/vote_proof_smoke_proof.json fixture).
//   - The tampered-proof test runs the REAL verifier against a corrupted
//     proof to confirm the wire-up actually rejects bad proofs.
//   - Public-signal binding (poll_id, voter_root) is enforced by SubmitVote
//     BEFORE the proof is verified, so we can exercise those rejection paths
//     with the accept-everything hook installed.

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"obscura.network/core/internal/db"
	"obscura.network/core/internal/governance"
	"obscura.network/core/internal/token"
	"obscura.network/core/internal/zk"
)

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "obscura-governance-test-*")
	if err != nil {
		panic("temp dir: " + err.Error())
	}
	if err := db.Init(tmpDir); err != nil {
		panic("test DB init: " + err.Error())
	}
	// Load vkeys so the real verifier path works when used.
	keysDir := filepath.Join("..", "zk", "keys")
	_ = zk.LoadVerificationKeys(keysDir)

	code := m.Run()
	db.Close()
	os.RemoveAll(tmpDir)
	os.Exit(code)
}

// obs returns n whole OBS as a smallest-unit *big.Int (n * 10^18).
func obs(n int64) *big.Int {
	return new(big.Int).Mul(big.NewInt(n), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
}

// uniquePhone returns a strictly-unique phone for the duration of the process
// — DIDs in different tests can share their last-5 chars, and users.phone has
// a UNIQUE constraint.
var phoneCounter int64

func uniquePhone() string {
	phoneCounter++
	return fmt.Sprintf("+9055%010d", phoneCounter)
}

// makeUser inserts a users row.
func makeUser(t *testing.T, did string, tier int) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.DB.Exec(`
		INSERT INTO users (id, phone, did, tier, created_at, updated_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		did, uniquePhone(), did, tier, now, now, now)
	if err != nil {
		t.Fatalf("makeUser %s: %v", did, err)
	}
}

// credit gives a DID a transparent OBS balance directly (bypasses transfer fee).
// Also bumps obs_supply.circulating so token.Burn's invariant check passes.
// All arithmetic is in math/big.Int — SQLite's TEXT-as-INTEGER coercion turns
// large numbers into scientific notation, which corrupts our decimal strings.
func credit(t *testing.T, did string, amount *big.Int) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)

	// Read current balance, add amount, write back.
	var curBal string
	err := db.DB.QueryRow(
		`SELECT transparent_balance FROM obs_accounts WHERE user_did = ?`, did).Scan(&curBal)
	cur := big.NewInt(0)
	if err == nil {
		if v, ok := new(big.Int).SetString(curBal, 10); ok {
			cur = v
		}
	}
	newBal := new(big.Int).Add(cur, amount)
	if _, err := db.DB.Exec(`
		INSERT INTO obs_accounts (user_did, transparent_balance, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(user_did) DO UPDATE SET
			transparent_balance = excluded.transparent_balance,
			updated_at = excluded.updated_at`,
		did, newBal.String(), now); err != nil {
		t.Fatalf("credit %s: %v", did, err)
	}

	// Bump circulating using math/big (NOT SQLite arithmetic).
	var supCirc string
	if err := db.DB.QueryRow(
		`SELECT circulating FROM obs_supply WHERE id = 1`).Scan(&supCirc); err != nil {
		// Seed if absent.
		_, _ = db.DB.Exec(`INSERT OR IGNORE INTO obs_supply (id, total_supply, circulating, burned, last_updated)
			 VALUES (1, '1000000000000000000000000000', '0', '0', ?)`, now)
		supCirc = "0"
	}
	circ, _ := new(big.Int).SetString(supCirc, 10)
	if circ == nil {
		circ = big.NewInt(0)
	}
	newCirc := new(big.Int).Add(circ, amount)
	if _, err := db.DB.Exec(
		`UPDATE obs_supply SET circulating = ?, last_updated = ? WHERE id = 1`,
		newCirc.String(), now); err != nil {
		t.Fatalf("bump supply: %v", err)
	}
}

// stakeFor inserts an active stake row so StakedBalanceFn sees the right amount.
func stakeFor(t *testing.T, did string, amount *big.Int) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	lockedUntil := time.Now().UTC().Add(30 * 24 * time.Hour).Format(time.RFC3339)
	_, err := db.DB.Exec(`
		INSERT INTO stakes
			(id, user_did, amount, stake_type, locked_until, apy_bps, status, created_at)
		VALUES (?, ?, ?, 'user', ?, 1000, 'active', ?)`,
		fmt.Sprintf("stake-%s", did), did, amount.String(), lockedUntil, now)
	if err != nil {
		t.Fatalf("stakeFor %s: %v", did, err)
	}
}

// makeEligible sets up a Platinum-tier user with 5000+ OBS staked.
func makeEligible(t *testing.T, did string) {
	t.Helper()
	makeUser(t, did, governance.MinTierToParticipate)
	stakeFor(t, did, obs(5000))
}

// makeDiamondEligible: Diamond tier + enough stake.
func makeDiamondEligible(t *testing.T, did string) {
	t.Helper()
	makeUser(t, did, governance.DiamondTier)
	stakeFor(t, did, obs(5000))
}

// proposalRoot/PollID fetches the live merkle_root and poll_id for a proposal.
func proposalRoot(t *testing.T, id string) (root, pollID string) {
	t.Helper()
	if err := db.DB.QueryRow(
		`SELECT merkle_root FROM governance_eligibility_snapshots WHERE proposal_id = ?`, id,
	).Scan(&root); err != nil {
		t.Fatalf("read root: %v", err)
	}
	if err := db.DB.QueryRow(
		`SELECT poll_id FROM proposals WHERE id = ?`, id,
	).Scan(&pollID); err != nil {
		t.Fatalf("read poll_id: %v", err)
	}
	return root, pollID
}

// makeSignals builds a 5-element public-signals slice with the given poll_id +
// root and a fresh nullifier. Used with the accept-everything verify hook.
func makeSignals(pollID, root, nullifier string) []string {
	return []string{
		pollID,
		"99999999",   // vote_commitment (opaque)
		root,         // voter_root
		nullifier,    // nullifier
		"1700000000", // timestamp
	}
}

// acceptHook installs an accept-everything verify hook, returns restore.
func acceptHook(t *testing.T) func() {
	t.Helper()
	return governance.SetVerifyHookForTest(func(_ []byte, _ []string) error { return nil })
}

// fakeProofJSON: structurally valid JSON; ignored by the accept hook.
const fakeProofJSON = `{"pi_a":["1","2","3"],"pi_b":[["1","2"],["3","4"],["1","0"]],"pi_c":["1","2","3"],"protocol":"groth16","curve":"bn128"}`

// ─── CreateProposal ──────────────────────────────────────────────────────────

func TestCreateProposal_HappyPath(t *testing.T) {
	defer acceptHook(t)()
	proposer := "did:obs:gov-prop-1"
	makeEligible(t, proposer)
	credit(t, proposer, obs(1000)) // > 100 OBS for the burn

	balBefore, _ := token.Balance(proposer)

	id, err := governance.CreateProposal(context.Background(), proposer,
		"Test proposal", "Some body", "param", `{"k":"v"}`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if id == "" {
		t.Fatal("empty proposal id")
	}

	// 100 OBS burned.
	balAfter, _ := token.Balance(proposer)
	wantBal := new(big.Int).Sub(balBefore, obs(100))
	if balAfter.Cmp(wantBal) != 0 {
		t.Errorf("balance after burn = %s, want %s", balAfter, wantBal)
	}

	// Proposal row exists, status active, eligibility snapshot frozen.
	p, err := governance.GetProposal(context.Background(), id)
	if err != nil {
		t.Fatalf("get proposal: %v", err)
	}
	if p.Status != "active" {
		t.Errorf("status = %q, want active", p.Status)
	}
	root, _ := proposalRoot(t, id)
	if root == "" {
		t.Error("merkle_root empty")
	}
}

func TestCreateProposal_RejectsNonPlatinum(t *testing.T) {
	proposer := "did:obs:gov-prop-2"
	makeUser(t, proposer, 3) // Gold, not Platinum
	stakeFor(t, proposer, obs(10000))
	credit(t, proposer, obs(1000))

	_, err := governance.CreateProposal(context.Background(), proposer,
		"Nope", "", "param", "")
	if err == nil {
		t.Fatal("expected ErrNotEligible, got nil")
	}
}

func TestCreateProposal_BurnAtomicityOnFailure(t *testing.T) {
	proposer := "did:obs:gov-prop-3"
	makeEligible(t, proposer)
	// No credit → not enough OBS to burn 100. CreateProposal should fail
	// at the burn step and NOT have inserted a proposal row.
	credit(t, proposer, obs(10)) // < 100 OBS

	_, err := governance.CreateProposal(context.Background(), proposer,
		"Underfunded", "", "param", "")
	if err == nil {
		t.Fatal("expected burn failure, got nil")
	}

	// No proposal row should exist for this proposer.
	var n int
	_ = db.DB.QueryRow(
		`SELECT COUNT(*) FROM proposals WHERE proposer_did = ?`, proposer,
	).Scan(&n)
	if n != 0 {
		t.Errorf("expected 0 proposals after burn failure, got %d", n)
	}
}

// ─── SubmitVote ──────────────────────────────────────────────────────────────

func TestSubmitVote_HappyPath(t *testing.T) {
	defer acceptHook(t)()
	proposer := "did:obs:gov-vote-1"
	makeEligible(t, proposer)
	credit(t, proposer, obs(1000))

	id, err := governance.CreateProposal(context.Background(), proposer,
		"Vote test", "", "param", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	root, pollID := proposalRoot(t, id)

	signals := makeSignals(pollID, root, "nullifier-vote-1")
	if err := governance.SubmitVote(context.Background(), id, fakeProofJSON, signals,
		governance.EncodeChoice(governance.ChoiceYes)); err != nil {
		t.Fatalf("submit vote: %v", err)
	}

	var n int
	_ = db.DB.QueryRow(`SELECT COUNT(*) FROM proposal_votes WHERE proposal_id = ?`, id).Scan(&n)
	if n != 1 {
		t.Errorf("vote count = %d, want 1", n)
	}
}

func TestSubmitVote_DoubleVoteRejectedByNullifier(t *testing.T) {
	defer acceptHook(t)()
	proposer := "did:obs:gov-vote-2"
	makeEligible(t, proposer)
	credit(t, proposer, obs(1000))

	id, _ := governance.CreateProposal(context.Background(), proposer,
		"Double", "", "param", "")
	root, pollID := proposalRoot(t, id)

	signals := makeSignals(pollID, root, "shared-nullifier")
	if err := governance.SubmitVote(context.Background(), id, fakeProofJSON, signals,
		governance.EncodeChoice(governance.ChoiceYes)); err != nil {
		t.Fatalf("first vote: %v", err)
	}
	err := governance.SubmitVote(context.Background(), id, fakeProofJSON, signals,
		governance.EncodeChoice(governance.ChoiceNo))
	if err == nil {
		t.Fatal("expected double-vote rejection, got nil")
	}
}

func TestSubmitVote_RejectsOutsideWindow(t *testing.T) {
	defer acceptHook(t)()
	proposer := "did:obs:gov-vote-3"
	makeEligible(t, proposer)
	credit(t, proposer, obs(1000))

	id, _ := governance.CreateProposal(context.Background(), proposer,
		"Window", "", "param", "")
	// Force voting_ends_at into the past.
	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	if _, err := db.DB.Exec(
		`UPDATE proposals SET voting_ends_at = ? WHERE id = ?`, past, id); err != nil {
		t.Fatalf("force ends: %v", err)
	}
	root, pollID := proposalRoot(t, id)

	signals := makeSignals(pollID, root, "n-window")
	err := governance.SubmitVote(context.Background(), id, fakeProofJSON, signals,
		governance.EncodeChoice(governance.ChoiceYes))
	if err == nil {
		t.Fatal("expected ErrVotingClosed, got nil")
	}
}

// TestSubmitVote_RejectsInvalidProof exercises the REAL verifier with a
// tampered proof: the smoke fixture is loaded, public_inputs are corrupted so
// the pairing check fails, and SubmitVote must reject. This confirms the
// vote_proof verifier is actually wired in (not just stubbed).
func TestSubmitVote_RejectsInvalidProof(t *testing.T) {
	proposer := "did:obs:gov-vote-4"
	makeEligible(t, proposer)
	credit(t, proposer, obs(1000))

	id, _ := governance.CreateProposal(context.Background(), proposer,
		"Tamper", "", "param", "")

	// Re-point the proposal's poll_id and merkle_root to the fixture's values
	// so the binding checks pass and we actually reach the verifier.
	fixturePath := filepath.Join("..", "..", "..", "circuits", "test", "vote_proof_smoke_proof.json")
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Skipf("vote_proof_smoke_proof.json missing: %v", err)
	}
	var payload struct {
		ProofJSON    string   `json:"proof_json"`
		PublicInputs []string `json:"public_inputs"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	// public_inputs: [poll_id, vote_commitment, voter_root, nullifier, timestamp]
	if _, err := db.DB.Exec(
		`UPDATE proposals SET poll_id = ? WHERE id = ?`, payload.PublicInputs[0], id); err != nil {
		t.Fatalf("patch poll_id: %v", err)
	}
	if _, err := db.DB.Exec(
		`UPDATE governance_eligibility_snapshots SET merkle_root = ? WHERE proposal_id = ?`,
		payload.PublicInputs[2], id); err != nil {
		t.Fatalf("patch root: %v", err)
	}

	// Tamper: change the nullifier so the pairing check fails. Use a fresh
	// nullifier so we don't trip the UNIQUE constraint from another test.
	tampered := make([]string, len(payload.PublicInputs))
	copy(tampered, payload.PublicInputs)
	tampered[3] = "11111111111111111111111111111111111111111111111111111111111111111111111111"

	err = governance.SubmitVote(context.Background(), id, payload.ProofJSON, tampered,
		governance.EncodeChoice(governance.ChoiceYes))
	if err == nil {
		t.Fatal("expected ErrProofInvalid for tampered proof, got nil")
	}
}

// ─── Tally / Finalize ────────────────────────────────────────────────────────

// finalizeReady forces a proposal's voting_ends_at to the past so Tally runs.
func finalizeReady(t *testing.T, id string) {
	t.Helper()
	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	if _, err := db.DB.Exec(
		`UPDATE proposals SET voting_ends_at = ? WHERE id = ?`, past, id); err != nil {
		t.Fatalf("force ends: %v", err)
	}
}

// seedExtraEligible adds N extra eligible voters (Platinum + 5000 staked) so
// the eligibility snapshot has a known size, and quorum math is predictable.
func seedExtraEligible(t *testing.T, prefix string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		did := fmt.Sprintf("%s-%02d", prefix, i)
		makeEligible(t, did)
	}
}

func TestFinalize_PassedWithQuorumAndMajority(t *testing.T) {
	defer acceptHook(t)()
	proposer := "did:obs:gov-fin-pass"
	makeEligible(t, proposer)
	credit(t, proposer, obs(1000))
	// 9 additional eligibles → snapshot size = 10 → quorum = 1 vote.
	seedExtraEligible(t, "did:obs:gov-fin-pass-extra", 9)

	id, _ := governance.CreateProposal(context.Background(), proposer,
		"Pass me", "", "param", "")
	root, pollID := proposalRoot(t, id)

	// Other tests leave eligible users behind in the shared DB; cast enough
	// YES votes to clear quorum (ceil(voter_count/10)) with margin, plus a few
	// NO votes so the decisive-ratio path is exercised.
	var voterCount int
	_ = db.DB.QueryRow(
		`SELECT voter_count FROM governance_eligibility_snapshots WHERE proposal_id = ?`, id,
	).Scan(&voterCount)
	yesNeeded := (voterCount+9)/10 + 3
	noVotes := 1
	nullCtr := 0
	for i := 0; i < yesNeeded; i++ {
		signals := makeSignals(pollID, root, fmt.Sprintf("null-pass-%d", nullCtr))
		nullCtr++
		if err := governance.SubmitVote(context.Background(), id, fakeProofJSON, signals,
			governance.EncodeChoice(governance.ChoiceYes)); err != nil {
			t.Fatalf("yes vote %d: %v", i, err)
		}
	}
	for i := 0; i < noVotes; i++ {
		signals := makeSignals(pollID, root, fmt.Sprintf("null-pass-%d", nullCtr))
		nullCtr++
		if err := governance.SubmitVote(context.Background(), id, fakeProofJSON, signals,
			governance.EncodeChoice(governance.ChoiceNo)); err != nil {
			t.Fatalf("no vote %d: %v", i, err)
		}
	}

	finalizeReady(t, id)
	tally, err := governance.Finalize(context.Background(), id)
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if !tally.QuorumMet {
		t.Error("quorum should be met (4 votes / 10 eligible)")
	}
	if !tally.Passed {
		t.Errorf("expected passed=true, tally=%+v", tally)
	}
	if tally.Status != "passed" {
		t.Errorf("status = %q, want passed", tally.Status)
	}

	// timelock_ends_at should be set ~48h out.
	var tl string
	_ = db.DB.QueryRow(`SELECT timelock_ends_at FROM proposals WHERE id = ?`, id).Scan(&tl)
	if tl == "" {
		t.Error("timelock_ends_at not set on passed proposal")
	}
}

func TestFinalize_FailedNoQuorum(t *testing.T) {
	defer acceptHook(t)()
	proposer := "did:obs:gov-fin-noq"
	makeEligible(t, proposer)
	credit(t, proposer, obs(1000))
	// 99 extra eligibles → snapshot size = 100 → quorum = 10 votes.
	seedExtraEligible(t, "did:obs:gov-fin-noq-extra", 99)

	id, _ := governance.CreateProposal(context.Background(), proposer,
		"No quorum", "", "param", "")
	root, pollID := proposalRoot(t, id)

	// Only 2 YES — well below the 10-vote quorum.
	for i := 0; i < 2; i++ {
		signals := makeSignals(pollID, root, fmt.Sprintf("null-noq-%d", i))
		if err := governance.SubmitVote(context.Background(), id, fakeProofJSON, signals,
			governance.EncodeChoice(governance.ChoiceYes)); err != nil {
			t.Fatalf("vote: %v", err)
		}
	}

	finalizeReady(t, id)
	tally, err := governance.Finalize(context.Background(), id)
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if tally.QuorumMet {
		t.Error("quorum should NOT be met")
	}
	if tally.Passed {
		t.Error("should not pass without quorum")
	}
	if tally.Status != "rejected" {
		t.Errorf("status = %q, want rejected", tally.Status)
	}
}

// ─── Veto ────────────────────────────────────────────────────────────────────

func TestVeto_DiamondCanVeto(t *testing.T) {
	defer acceptHook(t)()
	proposer := "did:obs:gov-veto-prop"
	makeEligible(t, proposer)
	credit(t, proposer, obs(1000))
	// 4 extra eligibles → snapshot size 5 → quorum = 1.
	seedExtraEligible(t, "did:obs:gov-veto-extra", 4)

	id, _ := governance.CreateProposal(context.Background(), proposer,
		"Vetoable", "", "param", "")
	root, pollID := proposalRoot(t, id)

	// 4 YES + 1 VETO → veto*5 = 5 >= total(5) → vetoed.
	for i := 0; i < 4; i++ {
		signals := makeSignals(pollID, root, fmt.Sprintf("null-veto-yes-%d", i))
		if err := governance.SubmitVote(context.Background(), id, fakeProofJSON, signals,
			governance.EncodeChoice(governance.ChoiceYes)); err != nil {
			t.Fatalf("yes %d: %v", i, err)
		}
	}
	signals := makeSignals(pollID, root, "null-veto-veto")
	if err := governance.SubmitVote(context.Background(), id, fakeProofJSON, signals,
		governance.EncodeChoice(governance.ChoiceVeto)); err != nil {
		t.Fatalf("veto vote: %v", err)
	}

	finalizeReady(t, id)
	tally, err := governance.Finalize(context.Background(), id)
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if !tally.Vetoed {
		t.Errorf("expected vetoed=true, tally=%+v", tally)
	}
	if tally.Passed {
		t.Error("vetoed proposal must not also be passed")
	}
	if tally.Status != "vetoed" {
		t.Errorf("status = %q, want vetoed", tally.Status)
	}
}

// ─── Execute ────────────────────────────────────────────────────────────────

func TestExecute_RejectsBeforeTimelock(t *testing.T) {
	defer acceptHook(t)()
	proposer := "did:obs:gov-exec-1"
	makeEligible(t, proposer)
	credit(t, proposer, obs(1000))
	// 4 extra eligibles → snapshot 5, quorum 1.
	seedExtraEligible(t, "did:obs:gov-exec-extra", 4)

	id, _ := governance.CreateProposal(context.Background(), proposer,
		"Exec timelock", "", "param", "")
	root, pollID := proposalRoot(t, id)
	// Quorum = ceil(voter_count / 10). Other tests in this package leave
	// eligible users behind in the shared DB, so the snapshot may be much
	// larger than this test's own seed. Read the live voter_count and submit
	// enough YES votes to clear quorum unambiguously.
	var voterCount int
	_ = db.DB.QueryRow(
		`SELECT voter_count FROM governance_eligibility_snapshots WHERE proposal_id = ?`, id,
	).Scan(&voterCount)
	needed := (voterCount+9)/10 + 1
	for i := 0; i < needed; i++ {
		signals := makeSignals(pollID, root, fmt.Sprintf("null-exec-%d", i))
		if err := governance.SubmitVote(context.Background(), id, fakeProofJSON, signals,
			governance.EncodeChoice(governance.ChoiceYes)); err != nil {
			t.Fatalf("vote %d: %v", i, err)
		}
	}

	finalizeReady(t, id)
	if _, err := governance.Finalize(context.Background(), id); err != nil {
		t.Fatalf("finalize: %v", err)
	}

	// Timelock is 48h out — execution must be refused.
	if err := governance.Execute(context.Background(), id); err == nil {
		t.Fatal("expected ErrTimelockActive, got nil")
	}

	// Force the timelock into the past — execution succeeds.
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	if _, err := db.DB.Exec(
		`UPDATE proposals SET timelock_ends_at = ? WHERE id = ?`, past, id); err != nil {
		t.Fatalf("force timelock: %v", err)
	}
	if err := governance.Execute(context.Background(), id); err != nil {
		t.Fatalf("execute after timelock: %v", err)
	}
	var status string
	_ = db.DB.QueryRow(`SELECT status FROM proposals WHERE id = ?`, id).Scan(&status)
	if status != "executed" {
		t.Errorf("status = %q, want executed", status)
	}
}
