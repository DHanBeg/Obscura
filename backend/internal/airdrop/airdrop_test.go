package airdrop_test

// Tests for the ZK-gated, Sybil-resistant airdrop distribution layer
// (spec Bölüm 12.2). Uses a real on-disk SQLite DB in a temp dir — the same
// pattern as internal/token/token_test.go. modernc.org/sqlite is pure Go and
// db.Init runs the migration system, so airdrop_* + obs_* + users tables all
// exist after setup.
//
// identity_proof ZK proof: no identity_proof smoke fixture ships in
// circuits/test/ (only credit_threshold, token_balance, vote_proof, storage do).
// Generating a real Groth16 proof in-test would require the .zkey + .wasm and a
// snarkjs/witness toolchain — too heavy and fragile for a unit test. Instead we
// mock at the zk.VerifyGroth16 boundary via airdrop.SetVerifyHookForTest: each
// test installs a stub that either accepts or rejects, so the claim *logic*
// (campaign state, eligibility, nullifier, mint) is exercised end-to-end while
// the cryptographic verification itself is covered by internal/zk's own tests.

import (
	"context"
	"math/big"
	"os"
	"testing"
	"time"

	"obscura.network/core/internal/airdrop"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/token"
)

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "obscura-airdrop-test-*")
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

// makeUser inserts a users row with the given DID, tier and account age, so
// eligibility checks have something to read. ageDays in the past.
func makeUser(t *testing.T, did string, tier, ageDays int) {
	t.Helper()
	createdAt := time.Now().UTC().Add(-time.Duration(ageDays) * 24 * time.Hour).Format(time.RFC3339)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.DB.Exec(`
		INSERT INTO users (id, phone, did, tier, created_at, updated_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		did, "+9055500"+did[len(did)-5:], did, tier, createdAt, now, now)
	if err != nil {
		t.Fatalf("makeUser %s: %v", did, err)
	}
}

// acceptProof installs a verify hook that accepts any proof, returns restore.
func acceptProof(t *testing.T) func() {
	t.Helper()
	return airdrop.SetVerifyHookForTest(func(_ []byte, _ []string) error { return nil })
}

// fakeProof / fakeSignals are placeholder claim inputs — content is irrelevant
// because the verify hook is mocked. publicSignals length is not checked when
// the hook is mocked (the real length check lives inside zk.VerifyGroth16).
const fakeProof = `{"pi_a":["1","2","3"],"pi_b":[["1","2"],["3","4"],["1","0"]],"pi_c":["1","2","3"],"protocol":"groth16","curve":"bn128"}`

var fakeSignals = []string{"1", "100", "200", "300"}

func mustCreateCampaign(t *testing.T, admin string, perClaim *big.Int, minTier, minAge, maxClaims int) string {
	t.Helper()
	id, err := airdrop.CreateCampaign(context.Background(), admin, "test-campaign",
		new(big.Int).Mul(perClaim, big.NewInt(int64(maxClaims)+10)), // pool covers max + slack
		perClaim, minTier, minAge, maxClaims, time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	return id
}

func TestCreateCampaign_AdminOnly(t *testing.T) {
	admin := "did:obs:airdrop-admin-1"
	nonAdmin := "did:obs:airdrop-user-1"
	makeUser(t, admin, 5, 30)    // Diamond → admin
	makeUser(t, nonAdmin, 3, 30) // Gold → not admin

	// Non-admin is rejected.
	_, err := airdrop.CreateCampaign(context.Background(), nonAdmin, "nope",
		obs(1000), obs(10), 1, 0, 10, time.Now().Add(time.Hour))
	if err == nil {
		t.Fatal("expected ErrNotAdmin for non-admin creator, got nil")
	}

	// Admin succeeds.
	id, err := airdrop.CreateCampaign(context.Background(), admin, "yep",
		obs(1000), obs(10), 1, 0, 10, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("admin create failed: %v", err)
	}
	if id == "" {
		t.Fatal("empty campaign id")
	}

	// total_pool too small for per_claim*max_claims is rejected.
	_, err = airdrop.CreateCampaign(context.Background(), admin, "underfunded",
		obs(50), obs(10), 1, 0, 10, time.Now().Add(time.Hour)) // 50 < 10*10
	if err == nil {
		t.Fatal("expected rejection for underfunded campaign")
	}
}

func TestClaim_HappyPath(t *testing.T) {
	defer acceptProof(t)()

	admin := "did:obs:airdrop-admin-2"
	claimer := "did:obs:airdrop-claim-2"
	makeUser(t, admin, 5, 30)
	makeUser(t, claimer, 2, 30)

	perClaim := obs(25)
	campID := mustCreateCampaign(t, admin, perClaim, 1, 0, 5)

	balBefore, _ := token.Balance(claimer)

	amount, err := airdrop.Claim(context.Background(), campID, claimer, fakeProof, fakeSignals)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if amount.Cmp(perClaim) != 0 {
		t.Errorf("claimed amount = %s, want %s", amount, perClaim)
	}

	// Tokens minted to claimer's transparent balance.
	balAfter, _ := token.Balance(claimer)
	wantBal := new(big.Int).Add(balBefore, perClaim)
	if balAfter.Cmp(wantBal) != 0 {
		t.Errorf("claimer balance = %s, want %s", balAfter, wantBal)
	}

	// claims_count incremented.
	info, err := airdrop.CampaignStatus(campID)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if info.ClaimsCount != 1 {
		t.Errorf("claims_count = %d, want 1", info.ClaimsCount)
	}
	if info.PoolDistributed != perClaim.String() {
		t.Errorf("pool_distributed = %s, want %s", info.PoolDistributed, perClaim)
	}
}

func TestClaim_DoubleClaimRejected(t *testing.T) {
	defer acceptProof(t)()

	admin := "did:obs:airdrop-admin-3"
	claimer := "did:obs:airdrop-claim-3"
	makeUser(t, admin, 5, 30)
	makeUser(t, claimer, 2, 30)

	campID := mustCreateCampaign(t, admin, obs(10), 1, 0, 10)

	// First claim succeeds.
	if _, err := airdrop.Claim(context.Background(), campID, claimer, fakeProof, fakeSignals); err != nil {
		t.Fatalf("first claim: %v", err)
	}

	// Second claim by the SAME identity is rejected by the nullifier.
	_, err := airdrop.Claim(context.Background(), campID, claimer, fakeProof, fakeSignals)
	if err == nil {
		t.Fatal("expected ErrAlreadyClaimed on second claim, got nil")
	}

	// claims_count must still be 1 (no double increment).
	info, _ := airdrop.CampaignStatus(campID)
	if info.ClaimsCount != 1 {
		t.Errorf("claims_count = %d after double-claim attempt, want 1", info.ClaimsCount)
	}
}

func TestClaim_IneligibleTierRejected(t *testing.T) {
	defer acceptProof(t)()

	admin := "did:obs:airdrop-admin-4"
	claimer := "did:obs:airdrop-claim-4"
	makeUser(t, admin, 5, 30)
	makeUser(t, claimer, 2, 30) // tier 2

	// Campaign requires tier >= 4.
	campID := mustCreateCampaign(t, admin, obs(10), 4, 0, 10)

	_, err := airdrop.Claim(context.Background(), campID, claimer, fakeProof, fakeSignals)
	if err == nil {
		t.Fatal("expected ErrIneligible for under-tier claimer, got nil")
	}

	// Nothing minted, no claim recorded.
	bal, _ := token.Balance(claimer)
	if bal.Sign() != 0 {
		t.Errorf("ineligible claimer balance = %s, want 0", bal)
	}
	info, _ := airdrop.CampaignStatus(campID)
	if info.ClaimsCount != 0 {
		t.Errorf("claims_count = %d after rejected claim, want 0", info.ClaimsCount)
	}
}

func TestClaim_CampaignEndedRejected(t *testing.T) {
	defer acceptProof(t)()

	admin := "did:obs:airdrop-admin-5"
	claimer := "did:obs:airdrop-claim-5"
	makeUser(t, admin, 5, 30)
	makeUser(t, claimer, 3, 30)

	campID := mustCreateCampaign(t, admin, obs(10), 1, 0, 10)

	// Admin ends the campaign early.
	if err := airdrop.EndCampaign(context.Background(), campID, admin); err != nil {
		t.Fatalf("end campaign: %v", err)
	}

	_, err := airdrop.Claim(context.Background(), campID, claimer, fakeProof, fakeSignals)
	if err == nil {
		t.Fatal("expected ErrCampaignClosed claiming from ended campaign, got nil")
	}

	// A non-admin cannot end a campaign.
	admin2 := "did:obs:airdrop-admin-5b"
	makeUser(t, admin2, 5, 30)
	campID2 := mustCreateCampaign(t, admin2, obs(10), 1, 0, 10)
	if err := airdrop.EndCampaign(context.Background(), campID2, claimer); err == nil {
		t.Fatal("expected ErrNotAdmin when non-admin ends campaign")
	}
}

func TestClaim_MaxClaimsReached(t *testing.T) {
	defer acceptProof(t)()

	admin := "did:obs:airdrop-admin-6"
	makeUser(t, admin, 5, 30)

	// Campaign allows exactly 2 claims.
	campID := mustCreateCampaign(t, admin, obs(10), 1, 0, 2)

	// Two distinct identities claim — both succeed.
	for i, did := range []string{"did:obs:airdrop-claim-6a", "did:obs:airdrop-claim-6b"} {
		makeUser(t, did, 2, 30)
		if _, err := airdrop.Claim(context.Background(), campID, did, fakeProof, fakeSignals); err != nil {
			t.Fatalf("claim %d (%s): %v", i, did, err)
		}
	}

	// Third identity is rejected — pool exhausted.
	third := "did:obs:airdrop-claim-6c"
	makeUser(t, third, 2, 30)
	_, err := airdrop.Claim(context.Background(), campID, third, fakeProof, fakeSignals)
	if err == nil {
		t.Fatal("expected ErrPoolExhausted when max_claims reached, got nil")
	}

	info, _ := airdrop.CampaignStatus(campID)
	if info.ClaimsCount != 2 {
		t.Errorf("claims_count = %d, want 2", info.ClaimsCount)
	}
	if info.PoolRemaining != "0" {
		t.Errorf("pool_remaining = %s, want 0", info.PoolRemaining)
	}
}

// TestClaim_InvalidProofRejected confirms a failing ZK verification blocks the
// claim (Sybil resistance: no valid identity proof → no claim).
func TestClaim_InvalidProofRejected(t *testing.T) {
	restore := airdrop.SetVerifyHookForTest(func(_ []byte, _ []string) error {
		return context.DeadlineExceeded // any non-nil error
	})
	defer restore()

	admin := "did:obs:airdrop-admin-7"
	claimer := "did:obs:airdrop-claim-7"
	makeUser(t, admin, 5, 30)
	makeUser(t, claimer, 3, 30)

	campID := mustCreateCampaign(t, admin, obs(10), 1, 0, 10)

	_, err := airdrop.Claim(context.Background(), campID, claimer, fakeProof, fakeSignals)
	if err == nil {
		t.Fatal("expected ErrInvalidProof for failing ZK verification, got nil")
	}
	info, _ := airdrop.CampaignStatus(campID)
	if info.ClaimsCount != 0 {
		t.Errorf("claims_count = %d after invalid-proof claim, want 0", info.ClaimsCount)
	}
}
