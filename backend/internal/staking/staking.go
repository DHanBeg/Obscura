// Package staking implements the OBS staking and slashing state layer
// described in ADR-0011 (Bölüm 8.5).
//
// This is the off-chain counterpart to the on-chain staking contracts. Like the
// token package it stores all amounts as TEXT decimal strings (18 decimals) and
// does arithmetic with math/big.Int. A zk-Rollup will later settle to this
// ledger.
//
// Mechanics (ADR-0011, Option A):
//   - User stakers: min 1,000 OBS, 30-day lock, 5-15% variable APY (10% mid is
//     the default — apy_bps = 1000).
//   - Node operators: min 10,000 OBS, 30-day lock. Operator fee share is handled
//     separately (not in this package).
//   - 7-day unstake cooldown after RequestUnstake before Withdraw is allowed.
//   - Slash tiers: 1% / 10% / 50% / 100% by severity. Slashes <= 10% apply
//     immediately; slashes > 10% become 'pending' and require 3/5 multisig
//     review (3 approvals) before they apply.
//   - Uptime < 95% over a recorded window auto-creates a pending slash event
//     proportional to the gap (1% per percentage point below 95%).
//
// Principal handling: staked principal is moved out of the staker's
// obs_accounts row into a well-known staking-pool account (StakePoolDID) inside
// a single DB transaction (no transfer fee — this is a lock, not a payment).
// Withdraw moves the principal back and mints the accrued reward. An applied
// slash burns the slashed amount from the staking pool via token.Burn.
package staking

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/token"
	"obscura.network/core/internal/dbi"
)

// StakePoolDID is the well-known account that custodies locked staked
// principal. token.Burn slashes from this account; token operations never
// charge a fee against it.
const StakePoolDID = "did:obs:stakepool"

// Staking parameters (ADR-0011).
const (
	StakeTypeUser         = "user"
	StakeTypeNodeOperator = "node_operator"

	// DefaultAPYBps is the 10% mid-point of the 5-15% variable user APY band.
	DefaultAPYBps = 1000

	lockDays     = 30
	cooldownDays = 7

	// UptimeThresholdPct: below this, an operator is slashable (ADR-0011).
	UptimeThresholdPct = 95.0

	// multisigApprovalsNeeded is the 3-of-5 threshold for reversing/applying a
	// >10% slash review.
	multisigApprovalsNeeded = 3

	// autoSlashMaxSeverity: slashes at or below this severity apply
	// immediately; above it they go to multisig review.
	autoSlashMaxSeverity = 10
)

var (
	tenPow18 = new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)

	// MinUserStake is 1,000 OBS in smallest units.
	MinUserStake = new(big.Int).Mul(big.NewInt(1000), tenPow18)
	// MinNodeOperatorStake is 10,000 OBS in smallest units.
	MinNodeOperatorStake = new(big.Int).Mul(big.NewInt(10000), tenPow18)
)

// Errors returned by this package.
var (
	ErrInvalidStakeType  = fmt.Errorf("invalid stake type")
	ErrBelowMinimum      = fmt.Errorf("stake below minimum")
	ErrInvalidAmount     = fmt.Errorf("invalid amount")
	ErrStakeNotFound     = fmt.Errorf("stake not found")
	ErrNotStakeOwner     = fmt.Errorf("stake does not belong to caller")
	ErrWrongStatus       = fmt.Errorf("stake is not in the required status")
	ErrCooldownActive    = fmt.Errorf("unstake cooldown has not elapsed")
	ErrInvalidSeverity   = fmt.Errorf("invalid slash severity")
	ErrSlashNotFound     = fmt.Errorf("slash event not found")
	ErrSlashNotPending   = fmt.Errorf("slash event is not pending review")
	ErrAlreadyReviewed   = fmt.Errorf("reviewer has already reviewed this slash")
	ErrInsufficientFunds = fmt.Errorf("insufficient obs balance")
)

// validSeverities are the four ADR-0011 slash tiers.
var validSeverities = map[int]bool{1: true, 10: true, 50: true, 100: true}

func nowStr() string { return time.Now().UTC().Format(time.RFC3339) }

// parseBig parses a decimal string into a *big.Int, erroring on corruption.
func parseBig(s string) (*big.Int, error) {
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return nil, fmt.Errorf("corrupt amount %q", s)
	}
	return v, nil
}

// Stake locks `amount` OBS from `did`'s transparent balance into a new stake.
// stakeType must be "user" (min 1,000 OBS) or "node_operator" (min 10,000 OBS).
// The stake is locked for 30 days. Returns the new stake id.
func Stake(ctx context.Context, did string, amount *big.Int, stakeType string) (string, error) {
	if did == "" {
		return "", fmt.Errorf("%w: did required", ErrInvalidAmount)
	}
	if amount == nil || amount.Sign() <= 0 {
		return "", fmt.Errorf("%w: must be positive", ErrInvalidAmount)
	}
	var minStake *big.Int
	switch stakeType {
	case StakeTypeUser:
		minStake = MinUserStake
	case StakeTypeNodeOperator:
		minStake = MinNodeOperatorStake
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidStakeType, stakeType)
	}
	if amount.Cmp(minStake) < 0 {
		return "", fmt.Errorf("%w: %s needs at least %s, got %s",
			ErrBelowMinimum, stakeType, minStake, amount)
	}

	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := nowStr()

	// Debit the staker's transparent balance (no fee — this is a lock).
	bal, err := txBalance(tx, did)
	if err != nil {
		return "", err
	}
	if bal.Cmp(amount) < 0 {
		return "", fmt.Errorf("%w: have %s need %s", ErrInsufficientFunds, bal, amount)
	}
	if err := setBalance(tx, did, new(big.Int).Sub(bal, amount), now); err != nil {
		return "", err
	}
	// Credit the staking pool — principal is custodied here until withdrawn.
	poolBal, err := txBalance(tx, StakePoolDID)
	if err != nil {
		return "", err
	}
	if err := setBalance(tx, StakePoolDID, new(big.Int).Add(poolBal, amount), now); err != nil {
		return "", err
	}

	stakeID := uuid.New().String()
	lockedUntil := time.Now().UTC().AddDate(0, 0, lockDays).Format(time.RFC3339)
	_, err = tx.Exec(`
		INSERT INTO stakes
			(id, user_did, amount, stake_type, locked_until, apy_bps, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, 'active', ?)`,
		stakeID, did, amount.String(), stakeType, lockedUntil, DefaultAPYBps, now)
	if err != nil {
		return "", fmt.Errorf("insert stake: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit tx: %w", err)
	}
	return stakeID, nil
}

// RequestUnstake marks an active stake as 'unstaking' and starts the 7-day
// cooldown. The stake's principal stays locked until Withdraw.
func RequestUnstake(ctx context.Context, did, stakeID string) error {
	st, err := getStake(ctx, stakeID)
	if err != nil {
		return err
	}
	if st.UserDID != did {
		return ErrNotStakeOwner
	}
	if st.Status != "active" {
		return fmt.Errorf("%w: have %q want active", ErrWrongStatus, st.Status)
	}
	now := nowStr()
	res, err := db.DB.ExecContext(ctx, `
		UPDATE stakes SET status = 'unstaking', unstake_requested_at = ?
		WHERE id = ? AND status = 'active'`, now, stakeID)
	if err != nil {
		return fmt.Errorf("request unstake: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrWrongStatus
	}
	return nil
}

// Withdraw returns principal + accrued reward to the staker's transparent
// balance and marks the stake 'withdrawn'. Only allowed once the 7-day cooldown
// after RequestUnstake has elapsed. The reward is freshly minted (it is new
// issuance — ADR-0010 inflation); the principal is moved back from the pool.
func Withdraw(ctx context.Context, did, stakeID string) error {
	st, err := getStake(ctx, stakeID)
	if err != nil {
		return err
	}
	if st.UserDID != did {
		return ErrNotStakeOwner
	}
	if st.Status != "unstaking" {
		return fmt.Errorf("%w: have %q want unstaking", ErrWrongStatus, st.Status)
	}
	if st.UnstakeRequestedAt == "" {
		return fmt.Errorf("%w: no unstake request timestamp", ErrWrongStatus)
	}
	reqAt, err := time.Parse(time.RFC3339, st.UnstakeRequestedAt)
	if err != nil {
		return fmt.Errorf("parse unstake_requested_at: %w", err)
	}
	if time.Now().UTC().Before(reqAt.AddDate(0, 0, cooldownDays)) {
		return fmt.Errorf("%w: available after %s", ErrCooldownActive,
			reqAt.AddDate(0, 0, cooldownDays).Format(time.RFC3339))
	}

	principal, err := parseBig(st.Amount)
	if err != nil {
		return err
	}
	reward := accruedReward(st, time.Now().UTC())

	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := nowStr()

	// Move principal back: pool -> staker.
	poolBal, err := txBalance(tx, StakePoolDID)
	if err != nil {
		return err
	}
	if poolBal.Cmp(principal) < 0 {
		return fmt.Errorf("invariant violation: stake pool below principal")
	}
	if err := setBalance(tx, StakePoolDID, new(big.Int).Sub(poolBal, principal), now); err != nil {
		return err
	}
	userBal, err := txBalance(tx, did)
	if err != nil {
		return err
	}
	if err := setBalance(tx, did, new(big.Int).Add(userBal, principal), now); err != nil {
		return err
	}

	res, err := tx.Exec(`
		UPDATE stakes SET status = 'withdrawn', withdrawn_at = ?
		WHERE id = ? AND status = 'unstaking'`, now, stakeID)
	if err != nil {
		return fmt.Errorf("mark withdrawn: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrWrongStatus
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	// Mint the accrued reward as new issuance. Done after commit so a mint
	// failure cannot roll back the principal return; reward is best-effort.
	if reward.Sign() > 0 {
		if _, err := token.Mint(ctx, did, reward, "staking reward "+stakeID); err != nil {
			return fmt.Errorf("withdraw succeeded but reward mint failed: %w", err)
		}
	}
	return nil
}

// AccruedReward returns the APY-based reward accrued by a stake so far,
// pro-rated by the time it has been staked (from created_at to now, or to
// withdrawn_at if already withdrawn).
func AccruedReward(stakeID string) (*big.Int, error) {
	st, err := getStake(context.Background(), stakeID)
	if err != nil {
		return nil, err
	}
	until := time.Now().UTC()
	if st.WithdrawnAt != "" {
		if w, err := time.Parse(time.RFC3339, st.WithdrawnAt); err == nil {
			until = w
		}
	}
	return accruedReward(st, until), nil
}

// accruedReward computes reward = principal * apy_bps/10000 * elapsed/year.
// Uses integer math throughout (numerator scaled by seconds, divided last).
func accruedReward(st *Position, until time.Time) *big.Int {
	principal, ok := new(big.Int).SetString(st.Amount, 10)
	if !ok {
		return big.NewInt(0)
	}
	createdAt, err := time.Parse(time.RFC3339, st.CreatedAt)
	if err != nil {
		return big.NewInt(0)
	}
	elapsed := until.Sub(createdAt)
	if elapsed <= 0 {
		return big.NewInt(0)
	}
	secondsStaked := big.NewInt(int64(elapsed.Seconds()))
	secondsPerYear := big.NewInt(365 * 24 * 60 * 60)

	// reward = principal * apyBps * secondsStaked / (10000 * secondsPerYear)
	reward := new(big.Int).Mul(principal, big.NewInt(int64(st.APYBps)))
	reward.Mul(reward, secondsStaked)
	denom := new(big.Int).Mul(big.NewInt(10000), secondsPerYear)
	reward.Div(reward, denom)
	return reward
}

// Slash creates a slash event against a stake. severityPct must be one of the
// ADR-0011 tiers: 1, 10, 50, 100. Slashes at or below 10% are applied
// immediately (status 'applied', amount burned from the staking pool). Slashes
// above 10% are recorded as 'pending' and require 3/5 multisig review before
// they apply. Returns the slash event id.
func Slash(ctx context.Context, did, stakeID, reason string, severityPct int) (string, error) {
	if !validSeverities[severityPct] {
		return "", fmt.Errorf("%w: %d (must be 1, 10, 50 or 100)", ErrInvalidSeverity, severityPct)
	}
	if reason == "" {
		return "", fmt.Errorf("slash reason required")
	}
	st, err := getStake(ctx, stakeID)
	if err != nil {
		return "", err
	}
	if st.UserDID != did {
		return "", ErrNotStakeOwner
	}
	return createSlashEvent(ctx, st, reason, severityPct)
}

// createSlashEvent is the shared path used by Slash and the uptime auto-slasher.
func createSlashEvent(ctx context.Context, st *Position, reason string, severityPct int) (string, error) {
	principal, err := parseBig(st.Amount)
	if err != nil {
		return "", err
	}
	// amountSlashed = principal * severityPct / 100
	amountSlashed := new(big.Int).Mul(principal, big.NewInt(int64(severityPct)))
	amountSlashed.Div(amountSlashed, big.NewInt(100))

	slashID := uuid.New().String()
	now := nowStr()

	if severityPct <= autoSlashMaxSeverity {
		// Auto-apply: burn from the staking pool and reduce the stake amount.
		if err := applySlash(ctx, slashID, st, amountSlashed, reason, severityPct, "", now); err != nil {
			return "", err
		}
		return slashID, nil
	}

	// Catastrophic (>10%): record pending, await 3/5 multisig review.
	_, err = db.DB.ExecContext(ctx, `
		INSERT INTO slash_events
			(id, user_did, stake_id, reason, severity_pct, amount_slashed, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, 'pending', ?)`,
		slashID, st.UserDID, st.ID, reason, severityPct, amountSlashed.String(), now)
	if err != nil {
		return "", fmt.Errorf("insert pending slash: %w", err)
	}
	return slashID, nil
}

// applySlash burns the slashed amount from the staking pool, reduces the
// stake's principal, and records/updates the slash event as 'applied'. Used
// both for immediate auto-slashes and for approved multisig slashes.
func applySlash(ctx context.Context, slashID string, st *Position, amountSlashed *big.Int, reason string, severityPct int, reviewedBy, now string) error {
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Reduce the stake's recorded principal so reward/withdraw use the post-
	// slash amount.
	cur, err := parseBig(st.Amount)
	if err != nil {
		return err
	}
	remaining := new(big.Int).Sub(cur, amountSlashed)
	if remaining.Sign() < 0 {
		remaining = big.NewInt(0)
	}
	if _, err := tx.Exec(`UPDATE stakes SET amount = ? WHERE id = ?`,
		remaining.String(), st.ID); err != nil {
		return fmt.Errorf("reduce stake principal: %w", err)
	}

	// Upsert the slash event as applied.
	_, err = tx.Exec(`
		INSERT INTO slash_events
			(id, user_did, stake_id, reason, severity_pct, amount_slashed, status, reviewed_by, created_at, applied_at)
		VALUES (?, ?, ?, ?, ?, ?, 'applied', ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			status = 'applied', reviewed_by = excluded.reviewed_by, applied_at = excluded.applied_at`,
		slashID, st.UserDID, st.ID, reason, severityPct, amountSlashed.String(), reviewedBy, now, now)
	if err != nil {
		return fmt.Errorf("upsert applied slash: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	// Burn the slashed principal from the staking pool (removes it from
	// circulation permanently — ADR-0011: slashed stake is burned).
	if amountSlashed.Sign() > 0 {
		if _, err := token.Burn(ctx, StakePoolDID, amountSlashed, "slash "+slashID); err != nil {
			return fmt.Errorf("slash recorded but burn failed: %w", err)
		}
	}
	return nil
}

// ReviewSlash records a reviewer's decision on a pending (>10%) slash event.
// Once 3 distinct reviewers approve, the slash is applied (amount burned). A
// reviewer may only review a given slash once. Rejection votes are recorded but
// do not by themselves resolve the event (ADR-0011: multisig is reverse-only;
// here a non-approved pending slash simply never applies).
func ReviewSlash(ctx context.Context, slashEventID, reviewerDID string, approve bool) error {
	if reviewerDID == "" {
		return fmt.Errorf("reviewer did required")
	}
	se, err := getSlashEvent(ctx, slashEventID)
	if err != nil {
		return err
	}
	if se.Status != "pending" {
		return fmt.Errorf("%w: status is %q", ErrSlashNotPending, se.Status)
	}

	approveInt := 0
	if approve {
		approveInt = 1
	}
	now := nowStr()
	_, err = db.DB.ExecContext(ctx, `
		INSERT INTO slash_reviews (slash_event_id, reviewer_did, approve, created_at)
		VALUES (?, ?, ?, ?)`, slashEventID, reviewerDID, approveInt, now)
	if err != nil {
		// PK conflict => reviewer already reviewed this event.
		return fmt.Errorf("%w: %v", ErrAlreadyReviewed, err)
	}

	// Count approvals.
	var approvals int
	if err := db.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM slash_reviews WHERE slash_event_id = ? AND approve = 1`,
		slashEventID).Scan(&approvals); err != nil {
		return fmt.Errorf("count approvals: %w", err)
	}

	if approvals >= multisigApprovalsNeeded {
		st, err := getStake(ctx, se.StakeID)
		if err != nil {
			return err
		}
		amountSlashed, err := parseBig(se.AmountSlashed)
		if err != nil {
			return err
		}
		if err := applySlash(ctx, se.ID, st, amountSlashed, se.Reason, se.SeverityPct,
			reviewerDID, nowStr()); err != nil {
			return err
		}
	}
	return nil
}

// RecordUptime stores a node uptime window. If the window's uptime is below the
// 95% threshold, a pending slash event is auto-created against the operator's
// most recent active stake, proportional to the gap (1% per percentage point
// below 95%, capped at the 100% tier). Returns the slash event id if one was
// created, otherwise "".
func RecordUptime(ctx context.Context, nodeID, did string, uptimePct float64) (string, error) {
	if nodeID == "" || did == "" {
		return "", fmt.Errorf("node_id and did required")
	}
	if uptimePct < 0 || uptimePct > 100 {
		return "", fmt.Errorf("uptime_pct must be between 0 and 100")
	}
	now := nowStr()
	windowEnd := time.Now().UTC()
	windowStart := windowEnd.AddDate(0, 0, -30) // 30-day rolling window (ADR-0011)

	_, err := db.DB.ExecContext(ctx, `
		INSERT INTO node_uptime
			(id, node_id, user_did, window_start, window_end, uptime_pct, recorded_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), nodeID, did,
		windowStart.Format(time.RFC3339), windowEnd.Format(time.RFC3339), uptimePct, now)
	if err != nil {
		return "", fmt.Errorf("insert uptime: %w", err)
	}

	if uptimePct >= UptimeThresholdPct {
		return "", nil
	}

	// Below threshold: find the operator's most recent active stake to slash.
	st, err := latestActiveStake(ctx, did)
	if err != nil {
		if err == ErrStakeNotFound {
			// Operator has no active stake — nothing to slash.
			return "", nil
		}
		return "", err
	}

	// Severity = ceil(gap) snapped to the nearest valid ADR-0011 tier.
	gap := UptimeThresholdPct - uptimePct
	severity := uptimeGapToSeverity(gap)
	reason := fmt.Sprintf("uptime %.2f%% below %.0f%% threshold (node %s)",
		uptimePct, UptimeThresholdPct, nodeID)
	return createSlashEvent(ctx, st, reason, severity)
}

// uptimeGapToSeverity maps an uptime gap (percentage points below 95%) to one
// of the four ADR-0011 slash tiers. The ADR specifies "1% per percentage point"
// but the slash tiers are discrete (1/10/50/100), so we snap up to the smallest
// tier that covers the gap.
func uptimeGapToSeverity(gap float64) int {
	switch {
	case gap <= 1:
		return 1
	case gap <= 10:
		return 10
	case gap <= 50:
		return 50
	default:
		return 100
	}
}

// ─── DB read helpers ─────────────────────────────────────────────────────────

// Position is a row of the stakes table (one staking position).
type Position struct {
	ID                 string `json:"id"`
	UserDID            string `json:"user_did"`
	Amount             string `json:"amount"`
	StakeType          string `json:"stake_type"`
	LockedUntil        string `json:"locked_until"`
	APYBps             int    `json:"apy_bps"`
	Status             string `json:"status"`
	CreatedAt          string `json:"created_at"`
	UnstakeRequestedAt string `json:"unstake_requested_at,omitempty"`
	WithdrawnAt        string `json:"withdrawn_at,omitempty"`
}

// SlashEvent is a row of the slash_events table.
type SlashEvent struct {
	ID            string `json:"id"`
	UserDID       string `json:"user_did"`
	StakeID       string `json:"stake_id"`
	Reason        string `json:"reason"`
	SeverityPct   int    `json:"severity_pct"`
	AmountSlashed string `json:"amount_slashed"`
	Status        string `json:"status"`
	ReviewedBy    string `json:"reviewed_by,omitempty"`
	CreatedAt     string `json:"created_at"`
	AppliedAt     string `json:"applied_at,omitempty"`
}

func scanStake(row interface{ Scan(...any) error }) (*Position, error) {
	var s Position
	var unstakeReq, withdrawn sql.NullString
	if err := row.Scan(&s.ID, &s.UserDID, &s.Amount, &s.StakeType, &s.LockedUntil,
		&s.APYBps, &s.Status, &s.CreatedAt, &unstakeReq, &withdrawn); err != nil {
		return nil, err
	}
	s.UnstakeRequestedAt = unstakeReq.String
	s.WithdrawnAt = withdrawn.String
	return &s, nil
}

const stakeCols = `id, user_did, amount, stake_type, locked_until, apy_bps, status,
	created_at, unstake_requested_at, withdrawn_at`

func getStake(ctx context.Context, stakeID string) (*Position, error) {
	row := db.DB.QueryRowContext(ctx,
		`SELECT `+stakeCols+` FROM stakes WHERE id = ?`, stakeID)
	s, err := scanStake(row)
	if err == sql.ErrNoRows {
		return nil, ErrStakeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get stake: %w", err)
	}
	return s, nil
}

func latestActiveStake(ctx context.Context, did string) (*Position, error) {
	row := db.DB.QueryRowContext(ctx,
		`SELECT `+stakeCols+` FROM stakes
		 WHERE user_did = ? AND status = 'active'
		 ORDER BY created_at DESC LIMIT 1`, did)
	s, err := scanStake(row)
	if err == sql.ErrNoRows {
		return nil, ErrStakeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("latest active stake: %w", err)
	}
	return s, nil
}

// Positions returns all of a user's stakes, newest first.
func Positions(ctx context.Context, did string) ([]*Position, error) {
	rows, err := db.DB.QueryContext(ctx,
		`SELECT `+stakeCols+` FROM stakes WHERE user_did = ? ORDER BY created_at DESC`, did)
	if err != nil {
		return nil, fmt.Errorf("positions query: %w", err)
	}
	defer rows.Close()
	out := []*Position{}
	for rows.Next() {
		s, err := scanStake(rows)
		if err != nil {
			return nil, fmt.Errorf("positions scan: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// NodeOperatorStakeOBS returns total active node_operator stake for did
// expressed in whole OBS units (not smallest units). Returns 0 if none found.
func NodeOperatorStakeOBS(ctx context.Context, did string) (float64, error) {
	rows, err := db.DB.QueryContext(ctx,
		`SELECT amount FROM stakes WHERE user_did = ? AND stake_type = 'node_operator' AND status = 'active'`,
		did)
	if err != nil {
		return 0, fmt.Errorf("node operator stake query: %w", err)
	}
	defer rows.Close()
	total := new(big.Int)
	for rows.Next() {
		var amtStr string
		if err := rows.Scan(&amtStr); err != nil {
			return 0, err
		}
		amt := new(big.Int)
		if _, ok := amt.SetString(amtStr, 10); !ok {
			continue
		}
		total.Add(total, amt)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if total.Sign() == 0 {
		return 0, nil
	}
	f, _ := new(big.Float).Quo(
		new(big.Float).SetInt(total),
		new(big.Float).SetInt(tenPow18),
	).Float64()
	return f, nil
}

func scanSlashEvent(row interface{ Scan(...any) error }) (*SlashEvent, error) {
	var e SlashEvent
	var reviewedBy, appliedAt sql.NullString
	if err := row.Scan(&e.ID, &e.UserDID, &e.StakeID, &e.Reason, &e.SeverityPct,
		&e.AmountSlashed, &e.Status, &reviewedBy, &e.CreatedAt, &appliedAt); err != nil {
		return nil, err
	}
	e.ReviewedBy = reviewedBy.String
	e.AppliedAt = appliedAt.String
	return &e, nil
}

const slashCols = `id, user_did, stake_id, reason, severity_pct, amount_slashed,
	status, reviewed_by, created_at, applied_at`

func getSlashEvent(ctx context.Context, id string) (*SlashEvent, error) {
	row := db.DB.QueryRowContext(ctx,
		`SELECT `+slashCols+` FROM slash_events WHERE id = ?`, id)
	e, err := scanSlashEvent(row)
	if err == sql.ErrNoRows {
		return nil, ErrSlashNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get slash event: %w", err)
	}
	return e, nil
}

// SlashEvents returns all slash events targeting a user, newest first.
func SlashEvents(ctx context.Context, did string) ([]*SlashEvent, error) {
	rows, err := db.DB.QueryContext(ctx,
		`SELECT `+slashCols+` FROM slash_events WHERE user_did = ? ORDER BY created_at DESC`, did)
	if err != nil {
		return nil, fmt.Errorf("slash events query: %w", err)
	}
	defer rows.Close()
	out := []*SlashEvent{}
	for rows.Next() {
		e, err := scanSlashEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("slash events scan: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ─── obs_accounts helpers (no-fee internal lock/unlock) ──────────────────────
//
// These mirror token's private txBalance/setBalance. Staking deliberately does
// not go through token.Transfer because locking principal is not a payment and
// must not incur the transfer fee.

func txBalance(tx dbi.Querier, did string) (*big.Int, error) {
	var s string
	err := tx.QueryRow(
		`SELECT transparent_balance FROM obs_accounts WHERE user_did = ?`, did).Scan(&s)
	if err == sql.ErrNoRows {
		return big.NewInt(0), nil
	}
	if err != nil {
		return nil, fmt.Errorf("balance lookup: %w", err)
	}
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return nil, fmt.Errorf("balance for %s is corrupt: %q", did, s)
	}
	return v, nil
}

func setBalance(tx dbi.Querier, did string, v *big.Int, now string) error {
	_, err := tx.Exec(`
		INSERT INTO obs_accounts (user_did, transparent_balance, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(user_did) DO UPDATE SET
			transparent_balance = excluded.transparent_balance,
			updated_at          = excluded.updated_at`,
		did, v.String(), now)
	if err != nil {
		return fmt.Errorf("set balance %s: %w", did, err)
	}
	return nil
}
