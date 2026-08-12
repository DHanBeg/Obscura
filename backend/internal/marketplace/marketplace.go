// Package marketplace implements native listing + purchase (spec Bölüm 5.2
// Katman 3 — "İşletme ekleme, satış"). Design plan approved before
// implementation; see the conversation record for the DB/handler split.
//
// Listing lifecycle: active -> pending_purchase (reserved mid-Purchase) ->
// sold, or active -> removed (seller closes it). pending_purchase exists to
// close a double-sell race: token.Transfer runs its own DB transaction
// (SQLite MaxOpenConns(1) — nesting deadlocks, same reasoning as
// internal/airdrop/airdrop.go's Claim/mint split), so Purchase cannot wrap
// the reservation and the transfer in one atomic transaction. Instead it
// reserves the listing first with a conditional UPDATE ... WHERE status =
// 'active' and checks RowsAffected — the same optimistic-concurrency pattern
// airdrop.go uses for claims_count.
//
// Sybil resistance (internal/sybil) is deliberately NOT wired into Purchase
// in this version: token.Transfer's balance check already prevents
// double-spending a normal paid purchase, and there is no free/reward path
// here yet. If a marketplace reward/campaign flow is added later, it should
// nullify with sybil.ComputeNullifier("marketplace_listing:"+listingID, did).
package marketplace

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math/big"
	"time"

	"github.com/google/uuid"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/models"
	"obscura.network/core/internal/sybil"
	"obscura.network/core/internal/token"
)

// Listing status values.
const (
	StatusActive          = "active"
	StatusPendingPurchase = "pending_purchase"
	StatusSold            = "sold"
	StatusRemoved         = "removed"
	StatusFlagged         = "flagged" // set by internal/umay, not by this package
)

// marketplace_transactions status. Only "completed" is produced today —
// Purchase() is unchanged by the escrow schema migration (#31, vault
// Phase-Status.md 2026-08-11, plan commit 28b1527, Adım 1). held/released/
// refunded are reserved for the escrow flow landing in later steps: Purchase
// will start writing "held" (Adım 3), then "released" (Adım 4, buyer
// confirms) or "refunded" (Adım 5, admin dispute resolve) — both terminal,
// no path back to "held".
const (
	TransactionStatusCompleted = "completed"
	TransactionStatusHeld      = "held"
	TransactionStatusReleased  = "released"
	TransactionStatusRefunded  = "refunded"
)

// MarketplaceEscrowDID is the well-known account that holds funds between
// Purchase() and release/refund — same pattern as token.FeePoolDID
// (token.go:39). Seeded as a normal obs_accounts row by migration
// 169_marketplace_escrow_account_seed; balance is 0 until Adım 3 wires
// token.internalMove into Purchase().
const MarketplaceEscrowDID = "did:obs:marketplace-escrow"

// SellerAccessLevel is the spec Bölüm 5.2 access level required to create a
// listing ("Katman 3: İşletme ekleme, satış"). Browsing and purchasing are
// NOT gated — spec 5.2's Katman 1 note says base-tier users can already see
// marketplace listings, only opening one as a seller is restricted.
const SellerAccessLevel = 3

// Sentinel errors — callers (HTTP handlers) map these to status codes.
var (
	ErrNotFound      = fmt.Errorf("marketplace: listing not found")
	ErrNotSeller     = fmt.Errorf("marketplace: caller is not the listing owner")
	ErrAccessDenied  = fmt.Errorf("marketplace: seller access level required")
	ErrListingClosed = fmt.Errorf("marketplace: listing not active")
	ErrInvalidInput  = fmt.Errorf("marketplace: invalid input")
	ErrSelfPurchase  = fmt.Errorf("marketplace: cannot purchase own listing")
)

// ListingInfo is a read-only view of a listing row.
type ListingInfo struct {
	ID          string `json:"id"`
	SellerDID   string `json:"seller_did"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Price       string `json:"price"`
	Currency    string `json:"currency"`
	Category    string `json:"category"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// ListingPatch is a partial update for UpdateListing — nil fields are left
// unchanged.
type ListingPatch struct {
	Title       *string
	Description *string
	Price       *string
	Category    *string
	Status      *string
}

// PurchaseResult is returned on a successful Purchase.
type PurchaseResult struct {
	TransactionID string `json:"transaction_id"`
	TokenTxID     string `json:"token_tx_id"`
	Amount        string `json:"amount"`
}

func validatePrice(price string) error {
	amount, ok := new(big.Int).SetString(price, 10)
	if !ok || amount.Sign() <= 0 {
		return fmt.Errorf("%w: price must be a positive decimal string", ErrInvalidInput)
	}
	return nil
}

// CreateListing creates a new listing. sellerDID must hold at least
// SellerAccessLevel (spec 5.2 Katman 3).
func CreateListing(ctx context.Context, sellerDID, title, description, price, category string) (string, error) {
	if sellerDID == "" || title == "" || price == "" || category == "" {
		return "", fmt.Errorf("%w: sellerDID, title, price and category required", ErrInvalidInput)
	}
	if err := validatePrice(price); err != nil {
		return "", err
	}

	tier, err := sybil.CallerTier(sellerDID)
	if err != nil {
		return "", fmt.Errorf("marketplace: seller tier lookup: %w", err)
	}
	if models.TierToAccessLevel(tier) < SellerAccessLevel {
		return "", ErrAccessDenied
	}

	id := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = db.DB.ExecContext(ctx, `
		INSERT INTO marketplace_listings
			(id, seller_did, title, description, price, currency, category, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 'OBS', ?, ?, ?, ?)`,
		id, sellerDID, title, description, price, category, StatusActive, now, now,
	)
	if err != nil {
		return "", fmt.Errorf("marketplace: insert listing: %w", err)
	}
	return id, nil
}

// loadListing reads one listing row by id.
func loadListing(id string) (*ListingInfo, error) {
	var l ListingInfo
	err := db.DB.QueryRow(`
		SELECT id, seller_did, title, description, price, currency, category, status, created_at, updated_at
		FROM marketplace_listings WHERE id = ?`, id).Scan(
		&l.ID, &l.SellerDID, &l.Title, &l.Description, &l.Price, &l.Currency, &l.Category, &l.Status, &l.CreatedAt, &l.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("marketplace: load listing: %w", err)
	}
	return &l, nil
}

// GetListing returns one listing by id.
func GetListing(id string) (*ListingInfo, error) {
	if id == "" {
		return nil, fmt.Errorf("%w: id required", ErrInvalidInput)
	}
	return loadListing(id)
}

// ListListings returns listings, newest first, optionally filtered by
// status. limit is clamped to (0, 200]; a non-positive limit defaults to 50.
func ListListings(status string, limit, offset int) ([]ListingInfo, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	query := `SELECT id, seller_did, title, description, price, currency, category, status, created_at, updated_at
		FROM marketplace_listings`
	args := []any{}
	if status != "" {
		query += ` WHERE status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := db.DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("marketplace: list listings: %w", err)
	}
	defer rows.Close()

	out := make([]ListingInfo, 0)
	for rows.Next() {
		var l ListingInfo
		if err := rows.Scan(&l.ID, &l.SellerDID, &l.Title, &l.Description, &l.Price, &l.Currency, &l.Category, &l.Status, &l.CreatedAt, &l.UpdatedAt); err != nil {
			return nil, fmt.Errorf("marketplace: scan listing: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// UpdateListing applies patch to a listing. Only the seller may update it.
func UpdateListing(ctx context.Context, id, callerDID string, patch ListingPatch) error {
	if id == "" || callerDID == "" {
		return fmt.Errorf("%w: id and callerDID required", ErrInvalidInput)
	}

	listing, err := loadListing(id)
	if err != nil {
		return err
	}
	if listing.SellerDID != callerDID {
		return ErrNotSeller
	}

	title, description, price, category, status := listing.Title, listing.Description, listing.Price, listing.Category, listing.Status
	if patch.Title != nil {
		if *patch.Title == "" {
			return fmt.Errorf("%w: title cannot be empty", ErrInvalidInput)
		}
		title = *patch.Title
	}
	if patch.Description != nil {
		description = *patch.Description
	}
	if patch.Price != nil {
		if err := validatePrice(*patch.Price); err != nil {
			return err
		}
		price = *patch.Price
	}
	if patch.Category != nil {
		if *patch.Category == "" {
			return fmt.Errorf("%w: category cannot be empty", ErrInvalidInput)
		}
		category = *patch.Category
	}
	if patch.Status != nil {
		status = *patch.Status
	}

	_, err = db.DB.ExecContext(ctx, `
		UPDATE marketplace_listings
		SET title = ?, description = ?, price = ?, category = ?, status = ?, updated_at = ?
		WHERE id = ?`,
		title, description, price, category, status, time.Now().UTC().Format(time.RFC3339), id,
	)
	if err != nil {
		return fmt.Errorf("marketplace: update listing: %w", err)
	}
	return nil
}

// Purchase buys listingID on behalf of buyerDID: reserves the listing,
// transfers price OBS seller<-buyer via token.Transfer, records the purchase,
// and marks the listing sold. See the package doc for the reservation
// rationale (avoiding a double-sell race without nesting DB transactions).
func Purchase(ctx context.Context, listingID, buyerDID string) (*PurchaseResult, error) {
	if listingID == "" || buyerDID == "" {
		return nil, fmt.Errorf("%w: listingID and buyerDID required", ErrInvalidInput)
	}

	listing, err := loadListing(listingID)
	if err != nil {
		return nil, err
	}
	if listing.SellerDID == buyerDID {
		return nil, ErrSelfPurchase
	}
	if listing.Status != StatusActive {
		return nil, ErrListingClosed
	}
	price, ok := new(big.Int).SetString(listing.Price, 10)
	if !ok || price.Sign() <= 0 {
		return nil, fmt.Errorf("marketplace: listing price corrupt: %q", listing.Price)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.DB.ExecContext(ctx,
		`UPDATE marketplace_listings SET status = ?, updated_at = ? WHERE id = ? AND status = ?`,
		StatusPendingPurchase, now, listingID, StatusActive,
	)
	if err != nil {
		return nil, fmt.Errorf("marketplace: reserve listing: %w", err)
	}
	if affected, _ := res.RowsAffected(); affected != 1 {
		// Lost a concurrent race, or the listing closed between loadListing and here.
		return nil, ErrListingClosed
	}

	tokenTxID, err := token.Transfer(ctx, buyerDID, listing.SellerDID, price, "marketplace:"+listingID)
	if err != nil {
		// Release the reservation — the sale did not happen.
		_, _ = db.DB.ExecContext(ctx,
			`UPDATE marketplace_listings SET status = ?, updated_at = ? WHERE id = ?`,
			StatusActive, time.Now().UTC().Format(time.RFC3339), listingID,
		)
		return nil, fmt.Errorf("marketplace: transfer failed: %w", err)
	}

	purchaseID := uuid.New().String()
	completedAt := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.DB.ExecContext(ctx, `
		INSERT INTO marketplace_transactions (id, listing_id, buyer_did, seller_did, amount, token_tx_id, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		purchaseID, listingID, buyerDID, listing.SellerDID, listing.Price, tokenTxID, TransactionStatusCompleted, completedAt,
	); err != nil {
		// Money already moved (token.Transfer committed). Mark the listing sold
		// anyway — the payment is real regardless of this bookkeeping row — and
		// surface the error so an operator reconciles the missing transaction
		// record (same failure-mode philosophy as airdrop.go's post-claim mint:
		// don't lose the fact that money already moved). The returned error
		// alone does not survive past the HTTP handler that logs it, so this
		// ALSO writes a server-side log line — an operator scanning logs for
		// "RECONCILIATION" must be able to find every money-moved-but-unrecorded
		// event even if nobody read the API response.
		log.Printf("MARKETPLACE RECONCILIATION NEEDED: transfer %s succeeded but transaction record failed for listing %s (buyer=%s seller=%s amount=%s): %v",
			tokenTxID, listingID, buyerDID, listing.SellerDID, listing.Price, err)
		_, _ = db.DB.ExecContext(ctx,
			`UPDATE marketplace_listings SET status = ?, updated_at = ? WHERE id = ?`,
			StatusSold, completedAt, listingID,
		)
		return nil, fmt.Errorf("marketplace: transfer succeeded (tx=%s) but recording purchase failed for listing %s (buyer=%s seller=%s amount=%s), needs reconciliation: %w",
			tokenTxID, listingID, buyerDID, listing.SellerDID, listing.Price, err)
	}

	if _, err := db.DB.ExecContext(ctx,
		`UPDATE marketplace_listings SET status = ?, updated_at = ? WHERE id = ?`,
		StatusSold, completedAt, listingID,
	); err != nil {
		return nil, fmt.Errorf("marketplace: purchase recorded (tx=%s) but listing status update failed: %w", purchaseID, err)
	}

	return &PurchaseResult{TransactionID: purchaseID, TokenTxID: tokenTxID, Amount: listing.Price}, nil
}
