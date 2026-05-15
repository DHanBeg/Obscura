package staking_test

// Tests for the OBS staking + slashing state layer (ADR-0011).
//
// Uses a real on-disk SQLite DB in a temp dir — same pattern as
// internal/token/token_test.go. db.Init runs migrations 029-036 so the
// stakes / slash_events / slash_reviews / node_uptime tables exist.

import (
	"context"
	"math/big"
	"os"
	"testing"
	"time"

	"obscura.network/core/internal/db"
	"obscura.network/core/internal/staking"
	"obscura.network/core/internal/token"
)

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "obscura-staking-test-*")
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

// backdateStake rewrites a stake's created_at / locked_until / unstake_requested_at
// so cooldown/lock logic can be exercised without real waiting.
func backdateStake(t *testing.T, stakeID string, createdDaysAgo, unstakeReqDaysAgo int) {
	t.Helper()
	created := time.Now().UTC().AddDate(0, 0, -createdDaysAgo).Format(time.RFC3339)
	locked := time.Now().UTC().AddDate(0, 0, -createdDaysAgo+30).Format(time.RFC3339)
	if unstakeReqDaysAgo >= 0 {
		req := time.Now().UTC().AddDate(0, 0, -unstakeReqDaysAgo).Format(time.RFC3339)
		if _, err := db.DB.Exec(
			`UPDATE stakes SET created_at = ?, locked_until = ?, unstake_requested_at = ? WHERE id = ?`,
			created, locked, req, stakeID); err != nil {
			t.Fatalf("backdate stake: %v", err)
		}
		return
	}
	if _, err := db.DB.Exec(
		`UPDATE stakes SET created_at = ?, locked_until = ? WHERE id = ?`,
		created, locked, stakeID); err != nil {
		t.Fatalf("backdate stake: %v", err)
	}
}

func TestStake_MinimumEnforced(t *testing.T) {
	ctx := context.Background()
	did := "did:obs:stake-min"
	fund(t, did, obs(20000))

	// User: below 1,000 OBS rejected.
	if _, err := staking.Stake(ctx, did, obs(999), staking.StakeTypeUser); err == nil {
		t.Error("expected rejection for 999 OBS user stake")
	}
	// User: exactly 1,000 OBS accepted.
	if _, err := staking.Stake(ctx, did, obs(1000), staking.StakeTypeUser); err != nil {
		t.Errorf("1000 OBS user stake should succeed: %v", err)
	}
	// Node operator: below 10,000 OBS rejected.
	if _, err := staking.Stake(ctx, did, obs(9999), staking.StakeTypeNodeOperator); err == nil {
		t.Error("expected rejection for 9999 OBS node_operator stake")
	}
	// Node operator: exactly 10,000 OBS accepted.
	if _, err := staking.Stake(ctx, did, obs(10000), staking.StakeTypeNodeOperator); err != nil {
		t.Errorf("10000 OBS node_operator stake should succeed: %v", err)
	}
	// Invalid stake type rejected.
	if _, err := staking.Stake(ctx, did, obs(5000), "validator"); err == nil {
		t.Error("expected rejection for invalid stake type")
	}
}

func TestStake_LocksBalance(t *testing.T) {
	ctx := context.Background()
	did := "did:obs:stake-locks"
	fund(t, did, obs(5000))

	poolBefore := mustBalance(t, staking.StakePoolDID)

	stakeID, err := staking.Stake(ctx, did, obs(3000), staking.StakeTypeUser)
	if err != nil {
		t.Fatalf("stake: %v", err)
	}
	if stakeID == "" {
		t.Fatal("empty stake id")
	}

	// Staker's transparent balance dropped by exactly the staked amount (no fee).
	if got, want := mustBalance(t, did), obs(2000); got.Cmp(want) != 0 {
		t.Errorf("staker balance = %s, want %s", got, want)
	}
	// Staking pool gained the principal.
	wantPool := new(big.Int).Add(poolBefore, obs(3000))
	if got := mustBalance(t, staking.StakePoolDID); got.Cmp(wantPool) != 0 {
		t.Errorf("pool balance = %s, want %s", got, wantPool)
	}

	// Cannot stake more than remaining balance.
	if _, err := staking.Stake(ctx, did, obs(3000), staking.StakeTypeUser); err == nil {
		t.Error("expected insufficient-funds rejection")
	}
}

func TestUnstakeWithdraw_CooldownEnforced(t *testing.T) {
	ctx := context.Background()
	did := "did:obs:cooldown"
	fund(t, did, obs(2000))

	stakeID, err := staking.Stake(ctx, did, obs(2000), staking.StakeTypeUser)
	if err != nil {
		t.Fatalf("stake: %v", err)
	}

	// Withdraw before requesting unstake → rejected.
	if err := staking.Withdraw(ctx, did, stakeID); err == nil {
		t.Error("expected rejection: withdraw before unstake request")
	}

	if err := staking.RequestUnstake(ctx, did, stakeID); err != nil {
		t.Fatalf("request unstake: %v", err)
	}

	// Withdraw immediately after unstake request → cooldown not elapsed.
	if err := staking.Withdraw(ctx, did, stakeID); err == nil {
		t.Error("expected cooldown rejection")
	}

	// Backdate the unstake request 8 days → cooldown (7d) elapsed.
	backdateStake(t, stakeID, 40, 8)
	if err := staking.Withdraw(ctx, did, stakeID); err != nil {
		t.Fatalf("withdraw after cooldown should succeed: %v", err)
	}

	// Principal returned (>= staked; reward minted on top).
	if got := mustBalance(t, did); got.Cmp(obs(2000)) < 0 {
		t.Errorf("balance after withdraw = %s, want >= 2000 OBS", got)
	}

	// Double withdraw rejected.
	if err := staking.Withdraw(ctx, did, stakeID); err == nil {
		t.Error("expected rejection on double withdraw")
	}
}

func TestAccruedReward_ProRated(t *testing.T) {
	ctx := context.Background()
	did := "did:obs:reward"
	fund(t, did, obs(10000))

	stakeID, err := staking.Stake(ctx, did, obs(10000), staking.StakeTypeUser)
	if err != nil {
		t.Fatalf("stake: %v", err)
	}

	// Fresh stake → ~zero reward.
	r0, err := staking.AccruedReward(stakeID)
	if err != nil {
		t.Fatalf("accrued reward: %v", err)
	}
	if r0.Sign() > 0 && r0.Cmp(obs(1)) > 0 {
		t.Errorf("fresh stake reward unexpectedly large: %s", r0)
	}

	// Backdate 365 days → reward ≈ 10% of 10,000 = 1,000 OBS (apy_bps=1000).
	backdateStake(t, stakeID, 365, -1)
	r1, err := staking.AccruedReward(stakeID)
	if err != nil {
		t.Fatalf("accrued reward: %v", err)
	}
	want := obs(1000)
	// Allow small rounding drift (±1 OBS).
	diff := new(big.Int).Abs(new(big.Int).Sub(r1, want))
	if diff.Cmp(obs(1)) > 0 {
		t.Errorf("1-year reward = %s, want ~%s", r1, want)
	}

	// Half a year → roughly half the reward (pro-rated).
	backdateStake(t, stakeID, 182, -1)
	rHalf, err := staking.AccruedReward(stakeID)
	if err != nil {
		t.Fatalf("accrued reward: %v", err)
	}
	if rHalf.Cmp(r1) >= 0 {
		t.Errorf("half-year reward %s should be less than full-year %s", rHalf, r1)
	}
	if rHalf.Cmp(obs(400)) < 0 || rHalf.Cmp(obs(600)) > 0 {
		t.Errorf("half-year reward = %s, want ~500 OBS", rHalf)
	}
}

func TestSlash_SmallAppliedImmediately(t *testing.T) {
	ctx := context.Background()
	did := "did:obs:slash-small"
	fund(t, did, obs(10000))

	stakeID, err := staking.Stake(ctx, did, obs(10000), staking.StakeTypeNodeOperator)
	if err != nil {
		t.Fatalf("stake: %v", err)
	}

	// 10% slash → applied immediately.
	slashID, err := staking.Slash(ctx, did, stakeID, "missed proof", 10)
	if err != nil {
		t.Fatalf("slash: %v", err)
	}

	events, err := staking.SlashEvents(ctx, did)
	if err != nil {
		t.Fatalf("slash events: %v", err)
	}
	var found *staking.SlashEvent
	for _, e := range events {
		if e.ID == slashID {
			found = e
		}
	}
	if found == nil {
		t.Fatal("slash event not found")
	}
	if found.Status != "applied" {
		t.Errorf("status = %q, want applied", found.Status)
	}
	// amount_slashed = 10% of 10,000 = 1,000 OBS.
	if found.AmountSlashed != obs(1000).String() {
		t.Errorf("amount_slashed = %s, want %s", found.AmountSlashed, obs(1000))
	}

	// Stake principal reduced to 9,000 OBS.
	positions, _ := staking.Positions(ctx, did)
	for _, p := range positions {
		if p.ID == stakeID && p.Amount != obs(9000).String() {
			t.Errorf("stake amount after slash = %s, want %s", p.Amount, obs(9000))
		}
	}
}

func TestSlash_LargeNeedsMultisig(t *testing.T) {
	ctx := context.Background()
	did := "did:obs:slash-large"
	fund(t, did, obs(10000))

	stakeID, err := staking.Stake(ctx, did, obs(10000), staking.StakeTypeNodeOperator)
	if err != nil {
		t.Fatalf("stake: %v", err)
	}

	// 50% slash → pending, NOT applied.
	slashID, err := staking.Slash(ctx, did, stakeID, "invalid proof", 50)
	if err != nil {
		t.Fatalf("slash: %v", err)
	}

	getEvent := func() *staking.SlashEvent {
		events, _ := staking.SlashEvents(ctx, did)
		for _, e := range events {
			if e.ID == slashID {
				return e
			}
		}
		return nil
	}

	if e := getEvent(); e == nil || e.Status != "pending" {
		t.Fatalf("expected pending slash, got %+v", e)
	}

	// 2 approvals — still pending.
	if err := staking.ReviewSlash(ctx, slashID, "did:obs:reviewer-1", true); err != nil {
		t.Fatalf("review 1: %v", err)
	}
	if err := staking.ReviewSlash(ctx, slashID, "did:obs:reviewer-2", true); err != nil {
		t.Fatalf("review 2: %v", err)
	}
	if e := getEvent(); e.Status != "pending" {
		t.Errorf("after 2 approvals status = %q, want still pending", e.Status)
	}

	// Same reviewer cannot review twice.
	if err := staking.ReviewSlash(ctx, slashID, "did:obs:reviewer-1", true); err == nil {
		t.Error("expected rejection: duplicate reviewer")
	}

	// 3rd distinct approval → applied.
	if err := staking.ReviewSlash(ctx, slashID, "did:obs:reviewer-3", true); err != nil {
		t.Fatalf("review 3: %v", err)
	}
	if e := getEvent(); e.Status != "applied" {
		t.Errorf("after 3 approvals status = %q, want applied", e.Status)
	}

	// Reviewing an already-applied slash is rejected.
	if err := staking.ReviewSlash(ctx, slashID, "did:obs:reviewer-4", true); err == nil {
		t.Error("expected rejection: review of non-pending slash")
	}
}

func TestSlash_BurnsTokens(t *testing.T) {
	ctx := context.Background()
	did := "did:obs:slash-burn"
	fund(t, did, obs(10000))

	stakeID, err := staking.Stake(ctx, did, obs(10000), staking.StakeTypeNodeOperator)
	if err != nil {
		t.Fatalf("stake: %v", err)
	}

	supBefore, _ := token.Supply()
	burnedBefore, _ := new(big.Int).SetString(supBefore.Burned, 10)
	circBefore, _ := new(big.Int).SetString(supBefore.Circulating, 10)
	poolBefore := mustBalance(t, staking.StakePoolDID)

	// 1% slash → applied immediately, 100 OBS burned.
	if _, err := staking.Slash(ctx, did, stakeID, "downtime", 1); err != nil {
		t.Fatalf("slash: %v", err)
	}

	slashed := obs(100) // 1% of 10,000

	// Burned counter increased by the slashed amount.
	supAfter, _ := token.Supply()
	burnedAfter, _ := new(big.Int).SetString(supAfter.Burned, 10)
	circAfter, _ := new(big.Int).SetString(supAfter.Circulating, 10)
	if want := new(big.Int).Add(burnedBefore, slashed); burnedAfter.Cmp(want) != 0 {
		t.Errorf("burned = %s, want %s", burnedAfter, want)
	}
	if want := new(big.Int).Sub(circBefore, slashed); circAfter.Cmp(want) != 0 {
		t.Errorf("circulating = %s, want %s", circAfter, want)
	}
	// Staking pool drained by the slashed amount.
	if want := new(big.Int).Sub(poolBefore, slashed); mustBalance(t, staking.StakePoolDID).Cmp(want) != 0 {
		t.Errorf("pool balance = %s, want %s", mustBalance(t, staking.StakePoolDID), want)
	}
}

// TestRecordUptime_AutoSlash verifies the uptime-based auto-slash path:
// uptime below 95% creates a pending slash; at/above 95% creates none.
func TestRecordUptime_AutoSlash(t *testing.T) {
	ctx := context.Background()
	did := "did:obs:uptime"
	fund(t, did, obs(10000))
	if _, err := staking.Stake(ctx, did, obs(10000), staking.StakeTypeNodeOperator); err != nil {
		t.Fatalf("stake: %v", err)
	}

	// 98% uptime → no slash.
	if id, err := staking.RecordUptime(ctx, "node-A", did, 98.0); err != nil || id != "" {
		t.Errorf("98%% uptime should not slash: id=%q err=%v", id, err)
	}
	// 90% uptime (5pp gap) → 10% tier → auto-applied (ADR-0011: ≤10% applies immediately).
	id, err := staking.RecordUptime(ctx, "node-A", did, 90.0)
	if err != nil {
		t.Fatalf("record uptime: %v", err)
	}
	if id == "" {
		t.Fatal("expected auto-slash event for 90% uptime")
	}
	events, _ := staking.SlashEvents(ctx, did)
	var found *staking.SlashEvent
	for _, e := range events {
		if e.ID == id {
			found = e
		}
	}
	if found == nil || found.Status != "applied" {
		t.Errorf("expected applied uptime slash (10%% tier), got %+v", found)
	}

	// 40% uptime (55pp gap) → 100% tier → catastrophic, pending multisig.
	bigDID := "did:obs:uptime-catastrophic"
	fund(t, bigDID, obs(10000))
	if _, err := staking.Stake(ctx, bigDID, obs(10000), staking.StakeTypeNodeOperator); err != nil {
		t.Fatalf("stake: %v", err)
	}
	id2, err := staking.RecordUptime(ctx, "node-B", bigDID, 40.0)
	if err != nil {
		t.Fatalf("record uptime: %v", err)
	}
	events2, _ := staking.SlashEvents(ctx, bigDID)
	var found2 *staking.SlashEvent
	for _, e := range events2 {
		if e.ID == id2 {
			found2 = e
		}
	}
	if found2 == nil || found2.Status != "pending" {
		t.Errorf("expected pending catastrophic uptime slash, got %+v", found2)
	}
}
