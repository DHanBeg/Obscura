// Package airdrop implements ZK-gated, Sybil-resistant OBS airdrop distribution
// (spec Bölüm 12.2 — "Airdrop dağılımı (ZK ile)").
//
// Design — Sybil resistance:
//
// An airdrop is trivially gamed if one person can claim many times. Obscura
// resists this with three layered controls:
//
//  1. identity_proof ZK proof — the claimer proves ownership of an Obscura DID
//     without revealing the underlying secret/phone. Because a DID is derived
//     from a phone-verified identity (Poseidon(identity_secret, phone_hash)),
//     one phone == one identity == one claim. The proof is verified with
//     zk.VerifyGroth16 against the identity_proof circuit's verification key.
//
//  2. Per-(campaign, identity) nullifier — nullifier = sha256(campaign_id||did).
//     It is stored UNIQUE in airdrop_claims, so a second claim attempt (even
//     from another device, another JWT session, or a concurrent race) is
//     rejected by the UNIQUE constraint inside the claim transaction.
//
//  3. Eligibility snapshot — min_tier and min_account_age_days are frozen at
//     campaign-creation time. A user who does not meet the frozen criteria at
//     claim time is rejected, so eligibility cannot be farmed after the fact.
//
// The claim is NOT anonymous: we deliberately store user_did alongside the
// nullifier for audit and duplicate-by-account detection. The privacy property
// we keep is "the identity_secret is never revealed"; the property we drop is
// "claims are unlinkable". For an airdrop, auditability outweighs unlinkability.
//
// Amounts are TEXT decimal strings (18 decimals — exceeds int64), arithmetic
// via math/big.Int. Same convention as internal/token.
package airdrop

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/sybil"
	"obscura.network/core/internal/token"
	"obscura.network/core/internal/dbi"
)

// AdminMinTier is the tier a DID must hold to create or end a campaign.
//
// PLACEHOLDER: until a proper role/admin system exists, "admin" == Diamond tier
// (tier 5, the highest tier). This mirrors how internal/api/miniapp_handlers.go
// gates privileged mini-app actions behind a tier check. Replace with a real
// RBAC check when one lands.
const AdminMinTier = 5

// Campaign status values.
const (
	StatusActive = "active"
	StatusEnded  = "ended"
)

// Sentinel errors — callers (HTTP handlers) map these to status codes.
var (
	ErrNotFound       = fmt.Errorf("airdrop campaign not found")
	ErrNotAdmin       = fmt.Errorf("caller is not authorized (admin tier required)")
	ErrCampaignClosed = fmt.Errorf("campaign is not active")
	ErrDeadlinePassed = fmt.Errorf("campaign claim deadline has passed")
	ErrPoolExhausted  = fmt.Errorf("campaign claim pool is exhausted")
	ErrIneligible     = fmt.Errorf("claimer does not meet eligibility criteria")
	ErrAlreadyClaimed = fmt.Errorf("identity has already claimed from this campaign")
	ErrInvalidProof   = fmt.Errorf("identity proof verification failed")
	ErrInvalidInput   = fmt.Errorf("invalid input")
)

// SetVerifyHookForTest forwards to internal/sybil.SetVerifyHookForTest — the
// identity-proof verification seam (Sybil resistance control #1, see this
// file's package doc) now lives in internal/sybil, shared with any future
// Sybil-resistant flow. Kept here so existing airdrop.SetVerifyHookForTest
// call sites don't need to change. Production code never calls this.
func SetVerifyHookForTest(fn func(proofJSON []byte, publicSignals []string) error) func() {
	return sybil.SetVerifyHookForTest(fn)
}

// CampaignInfo is a read-only view of a campaign plus derived status figures.
type CampaignInfo struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	TotalPool         string `json:"total_pool"`
	PerClaim          string `json:"per_claim"`
	MinTier           int    `json:"min_tier"`
	MinAccountAgeDays int    `json:"min_account_age_days"`
	ClaimsCount       int    `json:"claims_count"`
	MaxClaims         int    `json:"max_claims"`
	Status            string `json:"status"`
	CreatedBy         string `json:"created_by"`
	CreatedAt         string `json:"created_at"`
	EndsAt            string `json:"ends_at"`
	// Derived:
	PoolRemaining string `json:"pool_remaining"` // per_claim * (max_claims - claims_count)
	PoolDistributed string `json:"pool_distributed"` // per_claim * claims_count
}

// callerTier forwards to internal/sybil.CallerTier (Sybil resistance control
// #3 — eligibility snapshot — see this file's package doc and the
// internal/sybil package doc).
func callerTier(did string) (int, error) {
	return sybil.CallerTier(did)
}

// CreateCampaign creates a new airdrop campaign. The creator must hold admin
// tier (see AdminMinTier). total_pool must be mintable within the OBS supply
// cap *and* large enough to cover per_claim * max_claims, so the campaign can
// never promise more than it can pay.
func CreateCampaign(ctx context.Context, creatorDID, name string, totalPool, perClaim *big.Int, minTier, minAgeDays, maxClaims int, endsAt time.Time) (string, error) {
	if creatorDID == "" || name == "" {
		return "", fmt.Errorf("%w: creatorDID and name required", ErrInvalidInput)
	}
	if totalPool == nil || totalPool.Sign() <= 0 {
		return "", fmt.Errorf("%w: total_pool must be positive", ErrInvalidInput)
	}
	if perClaim == nil || perClaim.Sign() <= 0 {
		return "", fmt.Errorf("%w: per_claim must be positive", ErrInvalidInput)
	}
	if maxClaims <= 0 {
		return "", fmt.Errorf("%w: max_claims must be positive", ErrInvalidInput)
	}
	if minTier < 0 || minTier > 5 {
		return "", fmt.Errorf("%w: min_tier must be 0..5", ErrInvalidInput)
	}
	if minAgeDays < 0 {
		return "", fmt.Errorf("%w: min_account_age_days must be >= 0", ErrInvalidInput)
	}
	if !endsAt.After(time.Now()) {
		return "", fmt.Errorf("%w: ends_at must be in the future", ErrInvalidInput)
	}

	// total_pool must cover the maximum possible payout.
	maxPayout := new(big.Int).Mul(perClaim, big.NewInt(int64(maxClaims)))
	if totalPool.Cmp(maxPayout) < 0 {
		return "", fmt.Errorf("%w: total_pool (%s) < per_claim*max_claims (%s)",
			ErrInvalidInput, totalPool, maxPayout)
	}

	// Authorization: creator must be admin tier.
	tier, err := callerTier(creatorDID)
	if err != nil {
		return "", err
	}
	if tier < AdminMinTier {
		return "", ErrNotAdmin
	}

	// total_pool must be mintable: positive, within 256 bits, and not exceeding
	// the remaining headroom under the 1B supply cap.
	if totalPool.Cmp(token.MaxUint256) > 0 {
		return "", fmt.Errorf("%w: total_pool exceeds 256 bits", ErrInvalidInput)
	}
	sup, err := token.Supply()
	if err != nil {
		return "", fmt.Errorf("supply lookup: %w", err)
	}
	totalCap, _ := new(big.Int).SetString(sup.Total, 10)
	circulating, _ := new(big.Int).SetString(sup.Circulating, 10)
	if totalCap == nil || circulating == nil {
		return "", fmt.Errorf("supply row corrupt")
	}
	headroom := new(big.Int).Sub(totalCap, circulating)
	if totalPool.Cmp(headroom) > 0 {
		return "", fmt.Errorf("%w: total_pool (%s) exceeds mintable supply headroom (%s)",
			ErrInvalidInput, totalPool, headroom)
	}

	id := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = db.DB.ExecContext(ctx, `
		INSERT INTO airdrop_campaigns
			(id, name, total_pool, per_claim, min_tier, min_account_age_days,
			 claims_count, max_claims, status, created_by, created_at, ends_at)
		VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?, ?)`,
		id, name, totalPool.String(), perClaim.String(), minTier, minAgeDays,
		maxClaims, StatusActive, creatorDID, now, endsAt.UTC().Format(time.RFC3339))
	if err != nil {
		return "", fmt.Errorf("insert campaign: %w", err)
	}
	return id, nil
}

// campaignRow is the raw DB shape of a campaign.
type campaignRow struct {
	id          string
	name        string
	totalPool   string
	perClaim    string
	minTier     int
	minAgeDays  int
	claimsCount int
	maxClaims   int
	status      string
	createdBy   string
	createdAt   string
	endsAt      string
}

// loadCampaignTx reads a campaign row inside a transaction.
func loadCampaignTx(tx dbi.Querier, id string) (*campaignRow, error) {
	var c campaignRow
	err := tx.QueryRow(`
		SELECT id, name, total_pool, per_claim, min_tier, min_account_age_days,
		       claims_count, max_claims, status, created_by, created_at, ends_at
		FROM airdrop_campaigns WHERE id = ?`, id).Scan(
		&c.id, &c.name, &c.totalPool, &c.perClaim, &c.minTier, &c.minAgeDays,
		&c.claimsCount, &c.maxClaims, &c.status, &c.createdBy, &c.createdAt, &c.endsAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load campaign: %w", err)
	}
	return &c, nil
}

// computeNullifier forwards to internal/sybil.ComputeNullifier — campaignID
// is the nullifier namespace here (Sybil resistance control #2, see this
// file's package doc and the internal/sybil package doc).
func computeNullifier(campaignID, claimerDID string) string {
	return sybil.ComputeNullifier(campaignID, claimerDID)
}

// Claim executes an airdrop claim for claimerDID against campaignID.
//
// Steps:
//  1. Load campaign; reject unless active, before deadline, and pool not full.
//  2. Verify the identity_proof ZK proof (Sybil resistance — proves DID
//     ownership without revealing the secret).
//  3. Check eligibility: claimer tier >= min_tier and account age >= min days.
//  4. Compute the per-(campaign, identity) nullifier; reject if it already
//     exists. The UNIQUE constraint on airdrop_claims.nullifier additionally
//     catches concurrent races inside the transaction.
//  5. In one DB transaction: insert the claim row, increment claims_count,
//     and mint per_claim OBS to the claimer's transparent balance.
//
// Returns the claimed amount on success.
func Claim(ctx context.Context, campaignID, claimerDID, identityProofJSON string, publicSignals []string) (*big.Int, error) {
	if campaignID == "" || claimerDID == "" {
		return nil, fmt.Errorf("%w: campaignID and claimerDID required", ErrInvalidInput)
	}
	if identityProofJSON == "" {
		return nil, fmt.Errorf("%w: identity proof required", ErrInvalidInput)
	}

	// Read-only pre-checks first (campaign state + eligibility + proof) so we
	// only open the write transaction once a claim is plausible.
	var c campaignRow
	err := db.DB.QueryRowContext(ctx, `
		SELECT id, name, total_pool, per_claim, min_tier, min_account_age_days,
		       claims_count, max_claims, status, created_by, created_at, ends_at
		FROM airdrop_campaigns WHERE id = ?`, campaignID).Scan(
		&c.id, &c.name, &c.totalPool, &c.perClaim, &c.minTier, &c.minAgeDays,
		&c.claimsCount, &c.maxClaims, &c.status, &c.createdBy, &c.createdAt, &c.endsAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load campaign: %w", err)
	}

	if c.status != StatusActive {
		return nil, ErrCampaignClosed
	}
	endsAt, err := time.Parse(time.RFC3339, c.endsAt)
	if err != nil {
		return nil, fmt.Errorf("campaign ends_at corrupt: %w", err)
	}
	if time.Now().After(endsAt) {
		return nil, ErrDeadlinePassed
	}
	if c.claimsCount >= c.maxClaims {
		return nil, ErrPoolExhausted
	}

	perClaim, ok := new(big.Int).SetString(c.perClaim, 10)
	if !ok {
		return nil, fmt.Errorf("campaign per_claim corrupt: %q", c.perClaim)
	}

	// 2. ZK proof — Sybil resistance. The proof attests the claimer owns a
	// phone-verified DID. publicSignals length is validated inside
	// zk.VerifyGroth16 (identity_proof expects 4).
	if err := sybil.VerifyIdentityProof([]byte(identityProofJSON), publicSignals); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidProof, err)
	}

	// 3. Eligibility: tier + account age. An unknown DID is tier 0 / no row →
	// ineligible for any campaign with a positive minimum.
	var tier int
	var createdAtStr string
	err = db.DB.QueryRowContext(ctx,
		`SELECT tier, created_at FROM users WHERE did = ?`, claimerDID).Scan(&tier, &createdAtStr)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: claimer has no account", ErrIneligible)
	}
	if err != nil {
		return nil, fmt.Errorf("claimer lookup: %w", err)
	}
	if tier < c.minTier {
		return nil, fmt.Errorf("%w: tier %d < required %d", ErrIneligible, tier, c.minTier)
	}
	if c.minAgeDays > 0 {
		createdAt, perr := time.Parse(time.RFC3339, createdAtStr)
		if perr != nil {
			return nil, fmt.Errorf("claimer created_at corrupt: %w", perr)
		}
		ageDays := int(time.Since(createdAt).Hours() / 24)
		if ageDays < c.minAgeDays {
			return nil, fmt.Errorf("%w: account age %dd < required %dd", ErrIneligible, ageDays, c.minAgeDays)
		}
	}

	// 4. Nullifier — per (campaign, identity). Pre-check for a friendly error;
	// the UNIQUE constraint below is the real race-safe guard.
	nullifier := computeNullifier(campaignID, claimerDID)
	var existing string
	err = db.DB.QueryRowContext(ctx,
		`SELECT id FROM airdrop_claims WHERE nullifier = ?`, nullifier).Scan(&existing)
	if err == nil {
		return nil, ErrAlreadyClaimed
	}
	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("nullifier lookup: %w", err)
	}

	// 5. Atomic claim: insert claim row + bump claims_count. The nullifier
	// UNIQUE constraint makes the INSERT the serialization point — a concurrent
	// second claim for the same identity fails here.
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Re-load under the transaction and re-check the dynamic guards so a
	// campaign that ended (or filled) between the pre-check and here is caught.
	fresh, err := loadCampaignTx(tx, campaignID)
	if err != nil {
		return nil, err
	}
	if fresh.status != StatusActive {
		return nil, ErrCampaignClosed
	}
	if fresh.claimsCount >= fresh.maxClaims {
		return nil, ErrPoolExhausted
	}

	now := time.Now().UTC().Format(time.RFC3339)
	claimID := uuid.New().String()
	_, err = tx.Exec(`
		INSERT INTO airdrop_claims (id, campaign_id, user_did, nullifier, amount, claimed_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		claimID, campaignID, claimerDID, nullifier, perClaim.String(), now)
	if err != nil {
		// UNIQUE(nullifier) violation → identity already claimed (race lost).
		return nil, ErrAlreadyClaimed
	}

	res, err := tx.Exec(`
		UPDATE airdrop_campaigns
		SET claims_count = claims_count + 1
		WHERE id = ? AND claims_count < max_claims AND status = ?`,
		campaignID, StatusActive)
	if err != nil {
		return nil, fmt.Errorf("increment claims_count: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected != 1 {
		// Campaign filled or ended concurrently — abort the whole claim.
		return nil, ErrPoolExhausted
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit claim: %w", err)
	}

	// Mint the reward. token.Mint runs its own transaction; it is intentionally
	// outside the claim tx because SQLite serializes writes (MaxOpenConns(1)) so
	// nested transactions would deadlock. If the mint fails the claim row still
	// stands — which is the safe failure mode: the identity is marked claimed
	// and an operator can reconcile, rather than the pool being double-spendable.
	if _, err := token.Mint(ctx, claimerDID, perClaim,
		fmt.Sprintf("airdrop claim: campaign %s", campaignID)); err != nil {
		return nil, fmt.Errorf("airdrop mint failed (claim %s recorded, needs reconciliation): %w", claimID, err)
	}

	return perClaim, nil
}

// CampaignStatus returns a read-only snapshot of a campaign including the
// distributed and remaining pool figures.
func CampaignStatus(campaignID string) (*CampaignInfo, error) {
	if campaignID == "" {
		return nil, fmt.Errorf("%w: campaignID required", ErrInvalidInput)
	}
	var c campaignRow
	err := db.DB.QueryRow(`
		SELECT id, name, total_pool, per_claim, min_tier, min_account_age_days,
		       claims_count, max_claims, status, created_by, created_at, ends_at
		FROM airdrop_campaigns WHERE id = ?`, campaignID).Scan(
		&c.id, &c.name, &c.totalPool, &c.perClaim, &c.minTier, &c.minAgeDays,
		&c.claimsCount, &c.maxClaims, &c.status, &c.createdBy, &c.createdAt, &c.endsAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load campaign: %w", err)
	}
	return campaignRowToInfo(&c)
}

// campaignRowToInfo converts a raw row to a CampaignInfo with derived figures.
func campaignRowToInfo(c *campaignRow) (*CampaignInfo, error) {
	perClaim, ok := new(big.Int).SetString(c.perClaim, 10)
	if !ok {
		return nil, fmt.Errorf("per_claim corrupt: %q", c.perClaim)
	}
	distributed := new(big.Int).Mul(perClaim, big.NewInt(int64(c.claimsCount)))
	remainingClaims := c.maxClaims - c.claimsCount
	if remainingClaims < 0 {
		remainingClaims = 0
	}
	remaining := new(big.Int).Mul(perClaim, big.NewInt(int64(remainingClaims)))
	return &CampaignInfo{
		ID:                c.id,
		Name:              c.name,
		TotalPool:         c.totalPool,
		PerClaim:          c.perClaim,
		MinTier:           c.minTier,
		MinAccountAgeDays: c.minAgeDays,
		ClaimsCount:       c.claimsCount,
		MaxClaims:         c.maxClaims,
		Status:            c.status,
		CreatedBy:         c.createdBy,
		CreatedAt:         c.createdAt,
		EndsAt:            c.endsAt,
		PoolRemaining:     remaining.String(),
		PoolDistributed:   distributed.String(),
	}, nil
}

// ListActiveCampaigns returns all campaigns currently in 'active' status,
// newest first. Campaigns past their deadline are still 'active' in the DB
// until EndCampaign or a sweep runs; callers should check ends_at.
func ListActiveCampaigns() ([]CampaignInfo, error) {
	rows, err := db.DB.Query(`
		SELECT id, name, total_pool, per_claim, min_tier, min_account_age_days,
		       claims_count, max_claims, status, created_by, created_at, ends_at
		FROM airdrop_campaigns WHERE status = ?
		ORDER BY created_at DESC`, StatusActive)
	if err != nil {
		return nil, fmt.Errorf("list campaigns: %w", err)
	}
	defer rows.Close()

	out := make([]CampaignInfo, 0)
	for rows.Next() {
		var c campaignRow
		if err := rows.Scan(&c.id, &c.name, &c.totalPool, &c.perClaim, &c.minTier,
			&c.minAgeDays, &c.claimsCount, &c.maxClaims, &c.status, &c.createdBy,
			&c.createdAt, &c.endsAt); err != nil {
			return nil, fmt.Errorf("scan campaign: %w", err)
		}
		info, err := campaignRowToInfo(&c)
		if err != nil {
			return nil, err
		}
		out = append(out, *info)
	}
	return out, rows.Err()
}

// EndCampaign marks a campaign 'ended' early. Caller must hold admin tier. Any
// unclaimed pool simply stays unminted — there is no clawback because tokens
// for unclaimed slots were never minted in the first place.
func EndCampaign(ctx context.Context, campaignID, callerDID string) error {
	if campaignID == "" || callerDID == "" {
		return fmt.Errorf("%w: campaignID and callerDID required", ErrInvalidInput)
	}
	tier, err := callerTier(callerDID)
	if err != nil {
		return err
	}
	if tier < AdminMinTier {
		return ErrNotAdmin
	}

	res, err := db.DB.ExecContext(ctx,
		`UPDATE airdrop_campaigns SET status = ? WHERE id = ? AND status = ?`,
		StatusEnded, campaignID, StatusActive)
	if err != nil {
		return fmt.Errorf("end campaign: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		// Either the campaign does not exist or it was already ended.
		var exists string
		qerr := db.DB.QueryRow(`SELECT id FROM airdrop_campaigns WHERE id = ?`, campaignID).Scan(&exists)
		if qerr == sql.ErrNoRows {
			return ErrNotFound
		}
		return ErrCampaignClosed
	}
	return nil
}

// marshalPublicInputs is a small helper kept for symmetry with other packages
// that persist public inputs; unused by airdrop today but handy for handlers.
func marshalPublicInputs(sig []string) string {
	b, _ := json.Marshal(sig)
	return string(b)
}

var _ = marshalPublicInputs
