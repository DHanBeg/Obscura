// Package governance implements Obscura's on-chain governance with ZK voting,
// per ADR-0012 (docs/adr/0012-governance.md).
//
// Lifecycle: CreateProposal → SubmitVote (ZK proofs) → Tally → Execute.
//
//	Eligibility : 5,000+ OBS staked AND Platinum tier (tier >= 4)
//	Proposal cost : 100 OBS burned (anti-spam)
//	Voting period : 7 days
//	Timelock      : 48 hours after a proposal passes
//	Quorum        : 10% of the eligible voter set
//	Pass          : 51% (param) / 67% (protocol)
//	Veto          : 1/5 of the votes cast from Diamond-tier voters
//
// Vote privacy: votes are submitted as Groth16 proofs of the vote_proof
// circuit. The proposal_votes table stores NO voter identity — only the
// nullifier (double-vote guard), the vote commitment, and an encrypted choice.
// The nullifier is derived inside the circuit from the voter secret + poll_id,
// so a voter cannot construct a second proof for the same poll, and cannot
// construct a *different* proof for an already-cast vote: receipts are not
// provable (ADR-0012 "Receipt non-provability").
//
// VOTING-WEIGHT DESIGN DECISION (FAZ 2):
// ADR-0012 envisions stake-weighted anonymous votes where the weight is proven
// inside the circuit. The shipped vote_proof.circom (circuits/vote_proof.circom)
// does NOT expose a stake-weight public signal — its public signals are only
// [poll_id, vote_commitment, voter_root, nullifier, timestamp]. Carrying a
// per-voter stake weight anonymously requires a more complex circuit (range
// proof over the staked amount, committed in the eligibility leaf). That is
// scoped as a FAZ 3 improvement.
//
// For FAZ 2 we therefore use ONE-PERSON-ONE-VOTE among the eligible set:
// every vote counts as weight 1. Quorum is 10% of the eligible voter COUNT
// (frozen in the eligibility snapshot at proposal creation). This still
// delivers the two ADR-0012 non-negotiables — vote privacy and tier-gated
// eligibility — and only defers the stake-weighting refinement.
package governance

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/token"
	"obscura.network/core/internal/zk"
)

// ─── ADR-0012 parameters ─────────────────────────────────────────────────────

const (
	// MinTierToParticipate is the Platinum tier gate (ADR-0012).
	MinTierToParticipate = 4
	// DiamondTier identifies Diamond-tier voters for the veto check.
	DiamondTier = 5

	// VotingPeriod is the 7-day vote window.
	VotingPeriod = 7 * 24 * time.Hour
	// ExecutionTimelock is the 48-hour delay between "passed" and "executed".
	ExecutionTimelock = 48 * time.Hour

	// QuorumNumerator/QuorumDenominator express the 10% quorum.
	QuorumNumerator   = 1
	QuorumDenominator = 10

	// Vote choices — must match circuits/vote_proof.circom (0..3).
	ChoiceYes     = 0
	ChoiceNo      = 1
	ChoiceAbstain = 2
	ChoiceVeto    = 3
)

// MinStakeToParticipate is 5,000 OBS in smallest units (5000 * 10^18).
var MinStakeToParticipate = new(big.Int).Mul(
	big.NewInt(5000), new(big.Int).Exp(big.NewInt(10), big.NewInt(token.Decimals), nil))

// ProposalCost is 100 OBS burned to create a proposal (anti-spam).
var ProposalCost = new(big.Int).Mul(
	big.NewInt(100), new(big.Int).Exp(big.NewInt(10), big.NewInt(token.Decimals), nil))

// ─── errors ──────────────────────────────────────────────────────────────────

var (
	ErrNotEligible      = fmt.Errorf("not eligible: requires 5000+ OBS staked and Platinum tier")
	ErrProposalNotFound = fmt.Errorf("proposal not found")
	ErrInvalidType      = fmt.Errorf("proposal_type must be 'param' or 'protocol'")
	ErrVotingClosed     = fmt.Errorf("voting period has ended")
	ErrVotingOpen       = fmt.Errorf("voting period is still open")
	ErrDoubleVote       = fmt.Errorf("nullifier already used: double vote rejected")
	ErrProofInvalid     = fmt.Errorf("vote proof verification failed")
	ErrPollMismatch     = fmt.Errorf("proof poll_id does not match proposal")
	ErrRootMismatch     = fmt.Errorf("proof voter_root does not match eligibility snapshot")
	ErrAlreadyTallied   = fmt.Errorf("proposal already tallied")
	ErrNotPassed        = fmt.Errorf("proposal is not in 'passed' state")
	ErrTimelockActive   = fmt.Errorf("execution timelock has not elapsed")
)

// ─── zk verification seam ────────────────────────────────────────────────────

// verifyVoteProof is the function SubmitVote calls to verify a vote_proof
// Groth16 proof. It is a package-level seam so tests that can't ship a freshly
// generated proof for every scenario can stub the verification while still
// exercising the lifecycle (proposal state, nullifier uniqueness, poll-id /
// voter-root binding). Production code never overrides it.
var verifyVoteProof = func(proofJSON []byte, publicSignals []string) error {
	return zk.VerifyGroth16(zk.CircuitVoteProof, proofJSON, publicSignals)
}

// SetVerifyHookForTest swaps the vote-proof verification function and returns
// a restore func. Tests use this to (a) accept a known-good fixture without
// running the heavy verifier and (b) simulate tamper rejection deterministically.
func SetVerifyHookForTest(fn func(proofJSON []byte, publicSignals []string) error) func() {
	prev := verifyVoteProof
	verifyVoteProof = fn
	return func() { verifyVoteProof = prev }
}

// ─── staking lookup seam ─────────────────────────────────────────────────────

// StakedBalanceFn returns the total active staked OBS (smallest units) for a
// DID. It is a package-level seam so the eligibility check can be unit-tested
// without the staking package, and so it can be repointed at the staking
// package's own accessor once that lands. The default sums the `stakes` table
// (migrations 029-030, owned by the staking agent / ADR-0011).
var StakedBalanceFn = defaultStakedBalance

func defaultStakedBalance(ctx context.Context, did string) (*big.Int, error) {
	rows, err := db.DB.QueryContext(ctx,
		`SELECT amount FROM stakes WHERE user_did = ? AND status = 'active'`, did)
	if err != nil {
		return nil, fmt.Errorf("staked balance lookup: %w", err)
	}
	defer rows.Close()
	total := big.NewInt(0)
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, fmt.Errorf("staked balance scan: %w", err)
		}
		v, ok := new(big.Int).SetString(s, 10)
		if !ok {
			return nil, fmt.Errorf("corrupt stake amount %q for %s", s, did)
		}
		total.Add(total, v)
	}
	return total, rows.Err()
}

// userTier returns the tier of a DID from the users table.
func userTier(ctx context.Context, did string) (int, error) {
	var tier int
	err := db.DB.QueryRowContext(ctx, `SELECT tier FROM users WHERE did = ?`, did).Scan(&tier)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("user %s not found", did)
	}
	if err != nil {
		return 0, fmt.Errorf("tier lookup: %w", err)
	}
	return tier, nil
}

// IsEligible reports whether a DID may propose or vote: 5000+ OBS staked AND
// Platinum tier (>= 4). Exposed so handlers can pre-flight before doing work.
func IsEligible(ctx context.Context, did string) (bool, error) {
	tier, err := userTier(ctx, did)
	if err != nil {
		return false, err
	}
	if tier < MinTierToParticipate {
		return false, nil
	}
	staked, err := StakedBalanceFn(ctx, did)
	if err != nil {
		return false, err
	}
	return staked.Cmp(MinStakeToParticipate) >= 0, nil
}

// eligibleVoterDIDs returns every DID currently meeting the eligibility bar.
// The result set is frozen into governance_eligibility_snapshots at proposal
// creation; quorum is later computed against its size.
func eligibleVoterDIDs(ctx context.Context) ([]string, error) {
	rows, err := db.DB.QueryContext(ctx,
		`SELECT did FROM users WHERE tier >= ?`, MinTierToParticipate)
	if err != nil {
		return nil, fmt.Errorf("eligible voter scan: %w", err)
	}
	defer rows.Close()
	var dids []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, fmt.Errorf("eligible voter scan: %w", err)
		}
		dids = append(dids, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Filter by stake. Done in Go (not SQL) because staked balance lives in a
	// separate table and is summed via the StakedBalanceFn seam.
	out := make([]string, 0, len(dids))
	for _, d := range dids {
		staked, err := StakedBalanceFn(ctx, d)
		if err != nil {
			return nil, err
		}
		if staked.Cmp(MinStakeToParticipate) >= 0 {
			out = append(out, d)
		}
	}
	return out, nil
}

// ─── types ───────────────────────────────────────────────────────────────────

// Proposal is a governance proposal row.
type Proposal struct {
	ID               string  `json:"id"`
	PollID           string  `json:"poll_id"`
	ProposerDID      string  `json:"proposer_did"`
	Title            string  `json:"title"`
	Description      string  `json:"description"`
	ProposalType     string  `json:"proposal_type"`
	ExecutionPayload string  `json:"execution_payload"`
	Status           string  `json:"status"`
	VotingEndsAt     string  `json:"voting_ends_at"`
	TimelockEndsAt   *string `json:"timelock_ends_at,omitempty"`
	QuorumRequired   string  `json:"quorum_required"`
	CreatedAt        string  `json:"created_at"`
}

// Tally is the finalized outcome of a proposal vote.
type Tally struct {
	ProposalID    string `json:"proposal_id"`
	YesWeight     string `json:"yes_weight"`
	NoWeight      string `json:"no_weight"`
	AbstainWeight string `json:"abstain_weight"`
	VetoCount     int    `json:"veto_count"`
	TotalVoters   int    `json:"total_voters"`
	QuorumMet     bool   `json:"quorum_met"`
	Passed        bool   `json:"passed"`
	Vetoed        bool   `json:"vetoed"`
	Status        string `json:"status"`
	FinalizedAt   string `json:"finalized_at"`
}

// ─── CreateProposal ──────────────────────────────────────────────────────────

// CreateProposal validates eligibility, burns the 100 OBS proposal cost,
// freezes the eligible-voter merkle root into a snapshot, and inserts the
// proposal with voting_ends_at = now + 7d. Returns the proposal id.
//
// NOTE: the burn happens before the insert. If the insert fails the burn is
// not refunded — proposals are cheap relative to the anti-spam intent and the
// token layer has no "unburn". Validation is exhaustive before the burn so the
// only post-burn failure is an unexpected DB error.
func CreateProposal(ctx context.Context, proposerDID, title, desc, ptype, payload string) (string, error) {
	if proposerDID == "" {
		return "", fmt.Errorf("proposer_did required")
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return "", fmt.Errorf("title required")
	}
	if len(title) > 200 {
		return "", fmt.Errorf("title too long (max 200)")
	}
	if len(desc) > 20000 {
		return "", fmt.Errorf("description too long (max 20000)")
	}
	if ptype != "param" && ptype != "protocol" {
		return "", ErrInvalidType
	}
	if len(payload) > 100000 {
		return "", fmt.Errorf("execution_payload too long (max 100000)")
	}

	eligible, err := IsEligible(ctx, proposerDID)
	if err != nil {
		return "", err
	}
	if !eligible {
		return "", ErrNotEligible
	}

	// Freeze the eligible voter set. The merkle root here is a simple
	// placeholder digest over the sorted DID list — the production root will
	// be a Poseidon merkle tree built by the circuit tooling. What matters for
	// FAZ 2 is that the set size is frozen for quorum and the root is recorded.
	voters, err := eligibleVoterDIDs(ctx)
	if err != nil {
		return "", err
	}
	merkleRoot := computeVoterRoot(voters)

	// Quorum required = 10% of the eligible voter count (one-person-one-vote).
	quorum := (len(voters)*QuorumNumerator + QuorumDenominator - 1) / QuorumDenominator // ceil

	// Burn the proposal cost. Eligibility (incl. 5000 staked) is already
	// checked; this verifies the proposer also holds 100 spendable OBS.
	if _, err := token.Burn(ctx, proposerDID, new(big.Int).Set(ProposalCost),
		"governance proposal cost"); err != nil {
		return "", fmt.Errorf("burn proposal cost: %w", err)
	}

	id := uuid.New().String()
	pollID := derivePollID(id)
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)
	votingEnds := now.Add(VotingPeriod).Format(time.RFC3339)

	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO proposals
			(id, poll_id, proposer_did, title, description, proposal_type,
			 execution_payload, status, voting_ends_at, quorum_required, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'active', ?, ?, ?)`,
		id, pollID, proposerDID, title, desc, ptype, payload, votingEnds,
		fmt.Sprintf("%d", quorum), nowStr)
	if err != nil {
		return "", fmt.Errorf("insert proposal: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO governance_eligibility_snapshots
			(proposal_id, merkle_root, voter_count, snapshot_at)
		VALUES (?, ?, ?, ?)`,
		id, merkleRoot, len(voters), nowStr)
	if err != nil {
		return "", fmt.Errorf("insert eligibility snapshot: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}
	return id, nil
}

// ─── SubmitVote ──────────────────────────────────────────────────────────────

// SubmitVote verifies a vote_proof Groth16 proof and records an anonymous vote.
//
// publicSignals layout (vote_proof.circom, 5 signals):
//
//	[0] poll_id          must match the proposal's poll_id
//	[1] vote_commitment  stored opaque (Poseidon(choice, secret))
//	[2] voter_root       must match the eligibility snapshot's merkle_root
//	[3] nullifier        UNIQUE — second submission with same nullifier rejected
//	[4] timestamp        circuit-enforced non-zero
//
// The encrypted choice is supplied separately by the caller (the choice itself
// is never derivable from the public signals). No voter identity is stored.
func SubmitVote(ctx context.Context, proposalID, proofJSON string, publicSignals []string, choiceEncrypted string) error {
	if proposalID == "" {
		return fmt.Errorf("proposal_id required")
	}
	if len(publicSignals) != 5 {
		return fmt.Errorf("%w: expected 5 public signals, got %d", ErrProofInvalid, len(publicSignals))
	}
	if choiceEncrypted == "" {
		return fmt.Errorf("choice_encrypted required")
	}

	// Load proposal + its eligibility snapshot.
	var pollID, status, votingEndsAt string
	err := db.DB.QueryRowContext(ctx,
		`SELECT poll_id, status, voting_ends_at FROM proposals WHERE id = ?`, proposalID).
		Scan(&pollID, &status, &votingEndsAt)
	if err == sql.ErrNoRows {
		return ErrProposalNotFound
	}
	if err != nil {
		return fmt.Errorf("load proposal: %w", err)
	}
	if status != "active" {
		return ErrVotingClosed
	}
	ends, err := time.Parse(time.RFC3339, votingEndsAt)
	if err != nil {
		return fmt.Errorf("parse voting_ends_at: %w", err)
	}
	if time.Now().UTC().After(ends) {
		return ErrVotingClosed
	}

	var snapshotRoot string
	err = db.DB.QueryRowContext(ctx,
		`SELECT merkle_root FROM governance_eligibility_snapshots WHERE proposal_id = ?`, proposalID).
		Scan(&snapshotRoot)
	if err != nil {
		return fmt.Errorf("load eligibility snapshot: %w", err)
	}

	// poll_id binding: the proof must be for THIS proposal.
	if publicSignals[0] != pollID {
		return ErrPollMismatch
	}
	// voter_root binding: the proof must be against the frozen voter set.
	if publicSignals[2] != snapshotRoot {
		return ErrRootMismatch
	}

	// Verify the Groth16 proof. zk.VerifyGroth16 also re-checks the signal
	// count against the registered circuit.
	if err := verifyVoteProof([]byte(proofJSON), publicSignals); err != nil {
		return fmt.Errorf("%w: %v", ErrProofInvalid, err)
	}

	nullifier := publicSignals[3]
	voteCommitment := publicSignals[1]
	voterRoot := publicSignals[2]

	// Insert. The UNIQUE constraint on nullifier is the double-vote guard:
	// a second submission with the same nullifier fails here.
	_, err = db.DB.ExecContext(ctx, `
		INSERT INTO proposal_votes
			(id, proposal_id, nullifier, vote_commitment, choice_encrypted, voter_root, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), proposalID, nullifier, voteCommitment, choiceEncrypted, voterRoot,
		time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		if isUniqueViolation(err) {
			return ErrDoubleVote
		}
		return fmt.Errorf("insert vote: %w", err)
	}
	return nil
}

// ─── Tally ───────────────────────────────────────────────────────────────────

// Tally finalizes a proposal after its voting period ends. It counts the
// recorded votes (decrypting the choice via decodeChoice), checks quorum
// (10% of the eligible voter count), determines pass (51% param / 67% protocol
// of the non-abstain votes), and checks the Diamond veto (1/5 of votes cast
// are veto choices). It writes proposal_tallies and updates proposal.status;
// if passed, timelock_ends_at is set to now + 48h.
//
// Finalize is idempotent-guarded: a proposal already moved out of 'active' is
// rejected with ErrAlreadyTallied. (The previous internal name was Tally —
// renamed because the package also exports a Tally *type*; the lifecycle verb
// is "finalize" per ADR-0012, and the persisted snapshot read lives in GetTally.)
func Finalize(ctx context.Context, proposalID string) (*Tally, error) {
	var ptype, status, votingEndsAt, quorumStr string
	err := db.DB.QueryRowContext(ctx,
		`SELECT proposal_type, status, voting_ends_at, quorum_required FROM proposals WHERE id = ?`,
		proposalID).Scan(&ptype, &status, &votingEndsAt, &quorumStr)
	if err == sql.ErrNoRows {
		return nil, ErrProposalNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load proposal: %w", err)
	}
	if status != "active" {
		return nil, ErrAlreadyTallied
	}
	ends, err := time.Parse(time.RFC3339, votingEndsAt)
	if err != nil {
		return nil, fmt.Errorf("parse voting_ends_at: %w", err)
	}
	if time.Now().UTC().Before(ends) {
		return nil, ErrVotingOpen
	}

	quorumRequired := new(big.Int)
	if _, ok := quorumRequired.SetString(quorumStr, 10); !ok {
		quorumRequired.SetInt64(0)
	}

	// Count votes. FAZ 2: weight = 1 per vote (one-person-one-vote).
	rows, err := db.DB.QueryContext(ctx,
		`SELECT choice_encrypted FROM proposal_votes WHERE proposal_id = ?`, proposalID)
	if err != nil {
		return nil, fmt.Errorf("load votes: %w", err)
	}
	defer rows.Close()

	var yes, no, abstain, veto, total int
	for rows.Next() {
		var enc string
		if err := rows.Scan(&enc); err != nil {
			return nil, fmt.Errorf("scan vote: %w", err)
		}
		total++
		switch decodeChoice(enc) {
		case ChoiceYes:
			yes++
		case ChoiceNo:
			no++
		case ChoiceAbstain:
			abstain++
		case ChoiceVeto:
			veto++
		default:
			// Unknown encoding: count toward turnout but not toward any side.
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Quorum: total votes cast >= 10% of eligible voter count.
	quorumMet := big.NewInt(int64(total)).Cmp(quorumRequired) >= 0

	// Pass threshold over the decisive votes (yes + no; abstain & veto excluded
	// from the ratio). 51% for param, 67% for protocol.
	decisive := yes + no
	passed := false
	if quorumMet && decisive > 0 {
		if ptype == "protocol" {
			// yes/decisive >= 2/3  ⇔  yes*3 >= decisive*2
			passed = yes*3 >= decisive*2
		} else {
			// yes/decisive >= 51%  ⇔  yes*100 >= decisive*51
			passed = yes*100 >= decisive*51
		}
	}

	// Diamond veto: 1/5 of the votes cast are veto choices.
	// ADR-0012 reserves veto for Diamond tier; because votes are anonymous the
	// tier cannot be re-derived post-hoc, so the veto choice in the circuit is
	// only meaningful when cast by Diamond voters. FAZ 2 enforces the 1/5
	// threshold on veto-choice votes; a Diamond-only circuit gate is FAZ 3.
	vetoed := total > 0 && veto*5 >= total
	if vetoed {
		passed = false
	}

	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	finalStatus := "rejected"
	var timelockEnds *string
	switch {
	case vetoed:
		finalStatus = "vetoed"
	case passed:
		finalStatus = "passed"
		t := now.Add(ExecutionTimelock).Format(time.RFC3339)
		timelockEnds = &t
	}

	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO proposal_tallies
			(proposal_id, yes_weight, no_weight, abstain_weight, veto_count, total_voters, finalized_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(proposal_id) DO UPDATE SET
			yes_weight=excluded.yes_weight, no_weight=excluded.no_weight,
			abstain_weight=excluded.abstain_weight, veto_count=excluded.veto_count,
			total_voters=excluded.total_voters, finalized_at=excluded.finalized_at`,
		proposalID, fmt.Sprintf("%d", yes), fmt.Sprintf("%d", no),
		fmt.Sprintf("%d", abstain), veto, total, nowStr)
	if err != nil {
		return nil, fmt.Errorf("write tally: %w", err)
	}

	if timelockEnds != nil {
		_, err = tx.ExecContext(ctx,
			`UPDATE proposals SET status = ?, timelock_ends_at = ? WHERE id = ?`,
			finalStatus, *timelockEnds, proposalID)
	} else {
		_, err = tx.ExecContext(ctx,
			`UPDATE proposals SET status = ? WHERE id = ?`, finalStatus, proposalID)
	}
	if err != nil {
		return nil, fmt.Errorf("update proposal status: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &Tally{
		ProposalID:    proposalID,
		YesWeight:     fmt.Sprintf("%d", yes),
		NoWeight:      fmt.Sprintf("%d", no),
		AbstainWeight: fmt.Sprintf("%d", abstain),
		VetoCount:     veto,
		TotalVoters:   total,
		QuorumMet:     quorumMet,
		Passed:        passed,
		Vetoed:        vetoed,
		Status:        finalStatus,
		FinalizedAt:   nowStr,
	}, nil
}

// GetTally returns a persisted tally for a proposal, or nil if it has not been
// tallied yet.
func GetTally(ctx context.Context, proposalID string) (*Tally, error) {
	var t Tally
	var status string
	err := db.DB.QueryRowContext(ctx, `
		SELECT t.proposal_id, t.yes_weight, t.no_weight, t.abstain_weight,
		       t.veto_count, t.total_voters, t.finalized_at, p.status
		FROM proposal_tallies t JOIN proposals p ON p.id = t.proposal_id
		WHERE t.proposal_id = ?`, proposalID).
		Scan(&t.ProposalID, &t.YesWeight, &t.NoWeight, &t.AbstainWeight,
			&t.VetoCount, &t.TotalVoters, &t.FinalizedAt, &status)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load tally: %w", err)
	}
	t.Status = status
	t.Vetoed = status == "vetoed"
	t.Passed = status == "passed" || status == "executed"
	return &t, nil
}

// ─── Execute ─────────────────────────────────────────────────────────────────

// Execute applies a passed proposal after its 48h timelock has elapsed. It only
// transitions status 'passed' → 'executed'.
//
// The actual application of execution_payload is proposal-type-specific (a
// param change touches a config table; a protocol change swaps a verifier key,
// etc.) and is intentionally left as a stub here — see the TODO below.
func Execute(ctx context.Context, proposalID string) error {
	var status, ptype, payload string
	var timelockEndsAt sql.NullString
	err := db.DB.QueryRowContext(ctx,
		`SELECT status, proposal_type, execution_payload, timelock_ends_at
		 FROM proposals WHERE id = ?`, proposalID).
		Scan(&status, &ptype, &payload, &timelockEndsAt)
	if err == sql.ErrNoRows {
		return ErrProposalNotFound
	}
	if err != nil {
		return fmt.Errorf("load proposal: %w", err)
	}
	if status != "passed" {
		return ErrNotPassed
	}
	if !timelockEndsAt.Valid {
		return fmt.Errorf("passed proposal has no timelock_ends_at — inconsistent state")
	}
	tl, err := time.Parse(time.RFC3339, timelockEndsAt.String)
	if err != nil {
		return fmt.Errorf("parse timelock_ends_at: %w", err)
	}
	if time.Now().UTC().Before(tl) {
		return ErrTimelockActive
	}

	// TODO(FAZ 3): apply execution_payload. Payload application is
	// proposal-type-specific:
	//   - "param"    → decode payload as a key/value config patch and write it
	//                  to the governance-controlled parameter store.
	//   - "protocol" → decode payload as an upgrade descriptor (new verifier
	//                  key id, contract address, etc.) and stage it for the
	//                  node operators / rollup settlement layer.
	// For FAZ 2 we record the state transition only; no payload is applied.
	_ = ptype
	_ = payload

	res, err := db.DB.ExecContext(ctx,
		`UPDATE proposals SET status = 'executed' WHERE id = ? AND status = 'passed'`,
		proposalID)
	if err != nil {
		return fmt.Errorf("mark executed: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotPassed
	}
	return nil
}

// ─── reads ───────────────────────────────────────────────────────────────────

// GetProposal loads a single proposal by id.
func GetProposal(ctx context.Context, proposalID string) (*Proposal, error) {
	var p Proposal
	var timelock sql.NullString
	err := db.DB.QueryRowContext(ctx, `
		SELECT id, poll_id, proposer_did, title, description, proposal_type,
		       execution_payload, status, voting_ends_at, timelock_ends_at,
		       quorum_required, created_at
		FROM proposals WHERE id = ?`, proposalID).
		Scan(&p.ID, &p.PollID, &p.ProposerDID, &p.Title, &p.Description, &p.ProposalType,
			&p.ExecutionPayload, &p.Status, &p.VotingEndsAt, &timelock,
			&p.QuorumRequired, &p.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrProposalNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load proposal: %w", err)
	}
	if timelock.Valid {
		p.TimelockEndsAt = &timelock.String
	}
	return &p, nil
}

// ListProposals returns the most recent proposals (active first, then recent),
// capped at limit.
func ListProposals(ctx context.Context, limit int) ([]Proposal, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := db.DB.QueryContext(ctx, `
		SELECT id, poll_id, proposer_did, title, description, proposal_type,
		       execution_payload, status, voting_ends_at, timelock_ends_at,
		       quorum_required, created_at
		FROM proposals
		ORDER BY (status = 'active') DESC, created_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list proposals: %w", err)
	}
	defer rows.Close()

	out := make([]Proposal, 0, limit)
	for rows.Next() {
		var p Proposal
		var timelock sql.NullString
		if err := rows.Scan(&p.ID, &p.PollID, &p.ProposerDID, &p.Title, &p.Description,
			&p.ProposalType, &p.ExecutionPayload, &p.Status, &p.VotingEndsAt, &timelock,
			&p.QuorumRequired, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan proposal: %w", err)
		}
		if timelock.Valid {
			p.TimelockEndsAt = &timelock.String
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// derivePollID maps a proposal UUID to the numeric poll_id the ZK circuit
// consumes (vote_proof.circom treats poll_id as a field element). We take the
// FNV-1a 64-bit hash of the UUID and render it as a decimal string. It is
// deterministic, collision-resistant enough for an id space this small, and
// the UNIQUE constraint on proposals.poll_id catches the astronomically
// unlikely clash.
func derivePollID(uuidStr string) string {
	const (
		offset64 = 14695981039346656037
		prime64  = 1099511628211
	)
	var h uint64 = offset64
	for i := 0; i < len(uuidStr); i++ {
		h ^= uint64(uuidStr[i])
		h *= prime64
	}
	return fmt.Sprintf("%d", h)
}

// computeVoterRoot builds a placeholder merkle root over the eligible voter
// set: FNV-1a over the sorted, newline-joined DID list. The production circuit
// uses a Poseidon merkle tree; for FAZ 2 what matters is that the value is
// deterministic and frozen so SubmitVote can bind a proof's voter_root to it.
func computeVoterRoot(dids []string) string {
	sorted := make([]string, len(dids))
	copy(sorted, dids)
	// simple insertion sort — voter sets are small in FAZ 2
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j-1] > sorted[j]; j-- {
			sorted[j-1], sorted[j] = sorted[j], sorted[j-1]
		}
	}
	joined := strings.Join(sorted, "\n")
	const (
		offset64 = 14695981039346656037
		prime64  = 1099511628211
	)
	var h uint64 = offset64
	for i := 0; i < len(joined); i++ {
		h ^= uint64(joined[i])
		h *= prime64
	}
	return fmt.Sprintf("%d", h)
}

// decodeChoice extracts the plaintext vote choice from the stored
// choice_encrypted blob.
//
// FAZ 2 transparency note: ADR-0012's end-state has choices homomorphically
// encrypted and decrypted only in aggregate by a threshold-decryptor service
// (TallyOracle, listed as FAZ 3 tech debt in the ADR). That service does not
// exist yet. For FAZ 2, choice_encrypted carries a JSON object
// `{"choice": <0..3>}` — opaque to the schema, but decodable here so Tally can
// produce real counts. The vote is still anonymous (no voter identity is
// stored anywhere); only the *aggregate* is meaningful. Swapping in real
// threshold decryption is a drop-in replacement of this function.
func decodeChoice(enc string) int {
	var obj struct {
		Choice int `json:"choice"`
	}
	if err := json.Unmarshal([]byte(enc), &obj); err != nil {
		return -1
	}
	return obj.Choice
}

// EncodeChoice is the FAZ 2 counterpart to decodeChoice: it produces the
// choice_encrypted blob a client submits alongside its proof. Exposed so the
// API layer and tests build the blob the same way.
func EncodeChoice(choice int) string {
	b, _ := json.Marshal(struct {
		Choice int `json:"choice"`
	}{Choice: choice})
	return string(b)
}

// isUniqueViolation reports whether err is a SQLite UNIQUE constraint failure.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "constraint failed")
}
