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
	"errors"
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
	StatusFlagged         = "flagged" // planned: set by an external monitor, not by this package (umay, archive/umay/ — never wired in, C11)
)

// marketplace_transactions status (#31, vault Phase-Status.md 2026-08-11,
// plan commit 28b1527). As of Adım 3, Purchase() writes "held" — funds sit
// in MarketplaceEscrowDID, the seller is not paid yet. "released" (Adım 4,
// buyer confirms) or "refunded" (Adım 5, admin dispute resolve) come next,
// both terminal, no path back to "held". TransactionStatusCompleted is no
// longer written by Purchase() going forward — kept as a constant only
// because rows written before this step already carry that value.
const (
	TransactionStatusCompleted = "completed"
	TransactionStatusHeld      = "held"
	TransactionStatusReleased  = "released"
	TransactionStatusRefunded  = "refunded"
)

// MarketplaceEscrowDID is the well-known account that holds funds between
// Purchase() and release/refund — same pattern as token.FeePoolDID
// (token.go:39). Seeded as a normal obs_accounts row by migration
// 169_marketplace_escrow_account_seed. As of Adım 3, Purchase() sends the
// buyer's payment here (via token.Transfer, see Purchase's doc comment for
// why); token.InternalMove moves it onward to the seller (release, Adım 4)
// or back to the buyer (refund, Adım 5).
const MarketplaceEscrowDID = "did:obs:marketplace-escrow"

// SellerAccessLevel is the spec Bölüm 5.2 access level required to create a
// listing ("Katman 3: İşletme ekleme, satış"). Browsing and purchasing are
// NOT gated — spec 5.2's Katman 1 note says base-tier users can already see
// marketplace listings, only opening one as a seller is restricted.
const SellerAccessLevel = 3

// Sentinel errors — callers (HTTP handlers) map these to status codes.
var (
	ErrNotFound            = fmt.Errorf("marketplace: listing not found")
	ErrNotSeller           = fmt.Errorf("marketplace: caller is not the listing owner")
	ErrAccessDenied        = fmt.Errorf("marketplace: seller access level required")
	ErrListingClosed       = fmt.Errorf("marketplace: listing not active")
	ErrInvalidInput        = fmt.Errorf("marketplace: invalid input")
	ErrSelfPurchase        = fmt.Errorf("marketplace: cannot purchase own listing")
	ErrTransactionNotFound = fmt.Errorf("marketplace: transaction not found")
	ErrNotBuyer            = fmt.Errorf("marketplace: caller is not the buyer of this transaction")
	ErrAlreadyResolved     = fmt.Errorf("marketplace: transaction already resolved (not held)")
	ErrDisputeNotFound     = fmt.Errorf("marketplace: dispute not found")
	ErrDisputeAlreadyOpen  = fmt.Errorf("marketplace: an open dispute already exists for this transaction")
	// #30 — GetTransactionForCaller/GetDisputeForCaller'ın döndürdüğü hata:
	// ErrNotBuyer/ErrNotSeller'dan AYRI çünkü burada "sadece belirli bir rol
	// yasak" değil, "bu işlemle hiç ilgisi yok" anlamı taşıyor (görüntüleme,
	// aksiyon değil — buyer VEYA seller görebilir).
	ErrNotParticipant = fmt.Errorf("marketplace: caller is not a participant in this transaction")
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
// moves price OBS from buyer into escrow (MarketplaceEscrowDID) via
// token.Transfer, records the purchase as "held", and marks the listing
// sold. See the package doc for the reservation rationale (avoiding a
// double-sell race without nesting DB transactions).
//
// Escrow (#31, vault Phase-Status.md 2026-08-11, plan commit 28b1527, Adım
// 3): the seller is NOT paid at purchase time anymore — funds sit in escrow
// until a later release (Adım 4, buyer confirms) or refund (Adım 5, admin
// dispute resolve). This deliberately still calls token.Transfer (not
// InternalMove) for the hold leg, with MarketplaceEscrowDID as the
// recipient instead of listing.SellerDID: Transfer's fee mechanics
// (TransferFee(), 50% burn / 50% FeePoolDID) are completely unchanged by
// this step — the buyer pays exactly what they paid before (price+fee), and
// Transfer always credits the recipient the FULL `amount` regardless of the
// fee it debits from the sender, so escrow ends up holding exactly `price`.
// The release/refund legs (Adım 4/5) use token.InternalMove instead — a
// second Transfer(escrow, seller, price) would try to debit escrow
// price+fee, but escrow only ever holds exactly price, so InternalMove's
// fee-free, exact-amount move is what avoids ErrInsufficientBalance there.
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

	// memo prefix "marketplace-escrow-hold:" (not the old "marketplace:")
	// so this Transfer is self-describing in obs_transactions.memo without
	// having to cross-reference to_did == MarketplaceEscrowDID. Transfer()
	// doesn't take a separate txType param the way InternalMove does (its
	// obs_transactions.tx_type is always the hardcoded "transfer"), so memo
	// is the only lever available here for tagging this specific Transfer
	// call as an escrow hold rather than some other kind of transfer.
	tokenTxID, err := token.Transfer(ctx, buyerDID, MarketplaceEscrowDID, price, "marketplace-escrow-hold:"+listingID)
	if err != nil {
		// Release the reservation — the sale did not happen.
		_, _ = db.DB.ExecContext(ctx,
			`UPDATE marketplace_listings SET status = ?, updated_at = ? WHERE id = ?`,
			StatusActive, time.Now().UTC().Format(time.RFC3339), listingID,
		)
		return nil, fmt.Errorf("marketplace: transfer to escrow failed: %w", err)
	}

	purchaseID := uuid.New().String()
	completedAt := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.DB.ExecContext(ctx, `
		INSERT INTO marketplace_transactions (id, listing_id, buyer_did, seller_did, amount, token_tx_id, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		purchaseID, listingID, buyerDID, listing.SellerDID, listing.Price, tokenTxID, TransactionStatusHeld, completedAt,
	); err != nil {
		// Money already moved into escrow (token.Transfer committed) but the
		// marketplace_transactions row that tracks held/released/refunded state
		// didn't get written — this is now MORE urgent than before the escrow
		// change: without that row there is no held-funds record to release or
		// refund later, so this buyer's payment would sit in
		// MarketplaceEscrowDID indefinitely with no path back out. Mark the
		// listing sold anyway (the payment is real regardless of this
		// bookkeeping row) and surface the error so an operator reconciles the
		// missing transaction record (same failure-mode philosophy as
		// airdrop.go's post-claim mint: don't lose the fact that money already
		// moved). The returned error alone does not survive past the HTTP
		// handler that logs it, so this ALSO writes a server-side log line — an
		// operator scanning logs for "RECONCILIATION" must be able to find every
		// money-moved-but-unrecorded event even if nobody read the API response.
		log.Printf("MARKETPLACE RECONCILIATION NEEDED: transfer %s to escrow succeeded but transaction record failed for listing %s (buyer=%s seller=%s amount=%s) — funds stuck in %s with no tracking row: %v",
			tokenTxID, listingID, buyerDID, listing.SellerDID, listing.Price, MarketplaceEscrowDID, err)
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

// TransactionInfo is a read-only view of a marketplace_transactions row.
type TransactionInfo struct {
	ID         string `json:"id"`
	ListingID  string `json:"listing_id"`
	BuyerDID   string `json:"buyer_did"`
	SellerDID  string `json:"seller_did"`
	Amount     string `json:"amount"`
	TokenTxID  string `json:"token_tx_id"`
	Status     string `json:"status"`
	ResolvedAt string `json:"resolved_at,omitempty"`
	ResolvedBy string `json:"resolved_by,omitempty"`
	CreatedAt  string `json:"created_at"`
	// DisputeID is this transaction's marketplace_disputes.id, if one was ever
	// opened (#30 B9 parça 1 — was client-local-only via localStorage, lost on
	// device change; now query-time, no migration/backfill: OpenDispute only
	// accepts a "held" transaction and a disputed transaction can never return
	// to "held" (resolveHeld only reaches "released"/"refunded"), so at most
	// one dispute row per transaction ever exists — a plain subquery is exact).
	DisputeID string `json:"dispute_id,omitempty"`
}

// transactionDisputeIDSubquery finds transactionColumns' one-and-only
// dispute row, if any — see TransactionInfo.DisputeID's comment for why
// "at most one" holds. Interpolated into loadTransaction/ListTransactionsForUser's
// SELECT list; transactionColumns.ID must be in scope as `t.id` at the splice site.
const transactionDisputeIDSubquery = `(SELECT id FROM marketplace_disputes WHERE transaction_id = t.id LIMIT 1)`

// loadTransaction reads one marketplace_transactions row by id.
func loadTransaction(id string) (*TransactionInfo, error) {
	var t TransactionInfo
	var resolvedAt, resolvedBy, disputeID sql.NullString
	err := db.DB.QueryRow(`
		SELECT t.id, t.listing_id, t.buyer_did, t.seller_did, t.amount, t.token_tx_id, t.status, t.resolved_at, t.resolved_by, t.created_at,
		       `+transactionDisputeIDSubquery+`
		FROM marketplace_transactions t WHERE t.id = ?`, id).Scan(
		&t.ID, &t.ListingID, &t.BuyerDID, &t.SellerDID, &t.Amount, &t.TokenTxID, &t.Status, &resolvedAt, &resolvedBy, &t.CreatedAt, &disputeID,
	)
	if err == sql.ErrNoRows {
		return nil, ErrTransactionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("marketplace: load transaction: %w", err)
	}
	t.ResolvedAt = resolvedAt.String
	t.ResolvedBy = resolvedBy.String
	t.DisputeID = disputeID.String
	return &t, nil
}

// GetTransaction returns one marketplace_transactions row by id.
func GetTransaction(id string) (*TransactionInfo, error) {
	if id == "" {
		return nil, fmt.Errorf("%w: id required", ErrInvalidInput)
	}
	return loadTransaction(id)
}

// GetTransactionForCaller is GetTransaction gated to callerDID being the
// transaction's buyer or seller (#30 — mobile "Siparişlerim" screen; the
// row contains the other party's DID and the amount, not public data).
func GetTransactionForCaller(id, callerDID string) (*TransactionInfo, error) {
	txn, err := GetTransaction(id)
	if err != nil {
		return nil, err
	}
	if txn.BuyerDID != callerDID && txn.SellerDID != callerDID {
		return nil, ErrNotParticipant
	}
	return txn, nil
}

// ListTransactionsForUser returns every transaction where did is the buyer
// or the seller, newest first (#30 — "Siparişlerim": one list covers both
// purchases and sales, the caller tells them apart via buyer_did/seller_did).
func ListTransactionsForUser(did string) ([]TransactionInfo, error) {
	rows, err := db.DB.Query(`
		SELECT t.id, t.listing_id, t.buyer_did, t.seller_did, t.amount, t.token_tx_id, t.status, t.resolved_at, t.resolved_by, t.created_at,
		       `+transactionDisputeIDSubquery+`
		FROM marketplace_transactions t WHERE t.buyer_did = ? OR t.seller_did = ? ORDER BY t.created_at DESC`,
		did, did)
	if err != nil {
		return nil, fmt.Errorf("marketplace: list transactions: %w", err)
	}
	defer rows.Close()

	out := []TransactionInfo{}
	for rows.Next() {
		var t TransactionInfo
		var resolvedAt, resolvedBy, disputeID sql.NullString
		if err := rows.Scan(&t.ID, &t.ListingID, &t.BuyerDID, &t.SellerDID, &t.Amount, &t.TokenTxID, &t.Status, &resolvedAt, &resolvedBy, &t.CreatedAt, &disputeID); err != nil {
			return nil, fmt.Errorf("marketplace: scan transaction: %w", err)
		}
		t.ResolvedAt = resolvedAt.String
		t.ResolvedBy = resolvedBy.String
		t.DisputeID = disputeID.String
		out = append(out, t)
	}
	return out, rows.Err()
}

// escrowMove is the money-movement call Release and ResolveDispute use for
// the escrow-out leg (via resolveHeld). Defaults to token.InternalMove;
// overridden only by tests (see SetEscrowMoveForTest) that need to
// deterministically force token.ErrCommitUncertain — a real tx.Commit()
// failure isn't reliably reproducible against SQLite in a unit test, but
// resolveHeld's handling of that specific failure mode is exactly the
// "para donması" behavior this layer must prove, so a seam is the only way
// to test it for real rather than by inspection.
var escrowMove = token.InternalMove

// SetEscrowMoveForTest overrides escrowMove and returns the previous value
// so a test can defer-restore it. Not for production use.
func SetEscrowMoveForTest(fn func(ctx context.Context, from, to string, amount *big.Int, txType string) (string, error)) (prev func(ctx context.Context, from, to string, amount *big.Int, txType string) (string, error)) {
	prev = escrowMove
	escrowMove = fn
	return prev
}

// resolveHeld is the shared atomic core of Release (buyer confirms delivery,
// always escrow->seller) and ResolveDispute (admin decides escrow->seller or
// escrow->buyer) — #31, vault Phase-Status.md 2026-08-11, plan commit
// 28b1527, Adım 4/5. It performs the state-flip on marketplace_transactions
// (held -> targetStatus) BEFORE any money moves, as a single conditional
// UPDATE ... WHERE status = 'held' (same optimistic-concurrency shape as
// Purchase's listing reservation). Only the caller whose UPDATE actually
// changed a row (RowsAffected == 1) is allowed to move money; every other
// concurrent caller — a second release, a second dispute-resolve, or a
// release racing a dispute-resolve on the SAME transaction — sees
// RowsAffected == 0 and returns ErrAlreadyResolved without touching a
// balance. token.InternalMove can't nest inside this same DB transaction
// (SQLite MaxOpenConns(1) — nesting deadlocks, same reasoning as Purchase's
// separate reservation/Transfer statements), so the flip and the money move
// are unavoidably two statements — the ordering (flip first) is what makes
// a lost update impossible instead of merely unlikely.
//
// Money-stuck handling if the money move fails AFTER a successful flip:
//   - CERTAIN failure (anything but token.ErrCommitUncertain) — the money
//     move's own transaction never committed, escrow still holds the
//     money — the flip is reverted back to "held" so this is retryable.
//   - UNCERTAIN failure (token.ErrCommitUncertain — commit itself errored,
//     so whether the money actually moved is unknown) — the flip is left as
//     targetStatus and NOT reverted: reverting here would let a retry
//     succeed while the original attempt might *also* have actually
//     landed, double-paying recipient. An operator reconciles from the
//     "RECONCILIATION" log line instead (same philosophy as Purchase's own
//     transfer-succeeded-but-record-failed handling).
func resolveHeld(ctx context.Context, txn *TransactionInfo, recipient, resolvedBy, targetStatus, txType string, amount *big.Int) (tokenTxID string, err error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.DB.ExecContext(ctx,
		`UPDATE marketplace_transactions SET status = ?, resolved_at = ?, resolved_by = ? WHERE id = ? AND status = ?`,
		targetStatus, now, resolvedBy, txn.ID, TransactionStatusHeld,
	)
	if err != nil {
		return "", fmt.Errorf("marketplace: %s state-flip: %w", txType, err)
	}
	if affected, _ := res.RowsAffected(); affected != 1 {
		// Lost a concurrent resolve (release or dispute-resolve), or the
		// transaction wasn't "held" anymore by the time this ran — no money
		// moved.
		return "", ErrAlreadyResolved
	}

	tokenTxID, err = escrowMove(ctx, MarketplaceEscrowDID, recipient, amount, txType)
	if err != nil {
		if errors.Is(err, token.ErrCommitUncertain) {
			log.Printf("MARKETPLACE RECONCILIATION NEEDED: %s for transaction %s (listing=%s buyer=%s seller=%s amount=%s recipient=%s) has an UNCERTAIN outcome — commit error, funds may or may not have left escrow, status left as %q (NOT reverted, to avoid a double-pay on retry): %v",
				txType, txn.ID, txn.ListingID, txn.BuyerDID, txn.SellerDID, txn.Amount, recipient, targetStatus, err)
			return "", fmt.Errorf("marketplace: %s outcome uncertain for transaction %s, needs reconciliation: %w", txType, txn.ID, err)
		}

		// Certain failure — money never moved (InternalMove's tx rolled back
		// before/without committing). Revert the flip so this is retryable.
		revertAt := time.Now().UTC().Format(time.RFC3339)
		revertRes, revertErr := db.DB.ExecContext(ctx,
			`UPDATE marketplace_transactions SET status = ?, resolved_at = NULL, resolved_by = NULL WHERE id = ? AND status = ?`,
			TransactionStatusHeld, txn.ID, targetStatus,
		)
		revertAffected := int64(0)
		if revertRes != nil {
			revertAffected, _ = revertRes.RowsAffected()
		}
		if revertErr != nil || revertAffected != 1 {
			log.Printf("MARKETPLACE RECONCILIATION NEEDED: %s failed for transaction %s AND revert-to-held also failed (buyer=%s seller=%s amount=%s recipient=%s) at %s: move_err=%v revert_err=%v revert_affected=%d",
				txType, txn.ID, txn.BuyerDID, txn.SellerDID, txn.Amount, recipient, revertAt, err, revertErr, revertAffected)
		}
		return "", fmt.Errorf("marketplace: %s failed for transaction %s, reverted to held: %w", txType, txn.ID, err)
	}

	return tokenTxID, nil
}

// ReleaseResult is returned on a successful Release.
type ReleaseResult struct {
	TransactionID string `json:"transaction_id"`
	TokenTxID     string `json:"token_tx_id"`
	Amount        string `json:"amount"`
}

// Release marks a held purchase as delivered/accepted by buyerDID and pays
// the seller out of escrow (#31, vault Phase-Status.md 2026-08-11, plan
// commit 28b1527, Adım 4). Only the transaction's own buyer may release it
// (ErrNotBuyer otherwise); only a "held" transaction can be released
// (ErrAlreadyResolved for anything else — already released, already
// refunded, or already released by a concurrent call). See resolveHeld for
// the double-release defense and money-stuck handling — both shared with
// ResolveDispute's "seller wins" branch below.
func Release(ctx context.Context, transactionID, buyerDID string) (*ReleaseResult, error) {
	if transactionID == "" || buyerDID == "" {
		return nil, fmt.Errorf("%w: transactionID and buyerDID required", ErrInvalidInput)
	}

	txn, err := loadTransaction(transactionID)
	if err != nil {
		return nil, err
	}
	if txn.BuyerDID != buyerDID {
		return nil, ErrNotBuyer
	}

	amount, ok := new(big.Int).SetString(txn.Amount, 10)
	if !ok || amount.Sign() <= 0 {
		return nil, fmt.Errorf("marketplace: transaction amount corrupt: %q", txn.Amount)
	}

	tokenTxID, err := resolveHeld(ctx, txn, txn.SellerDID, buyerDID, TransactionStatusReleased, "escrow_release", amount)
	if err != nil {
		return nil, err
	}
	return &ReleaseResult{TransactionID: transactionID, TokenTxID: tokenTxID, Amount: txn.Amount}, nil
}

// Dispute status values.
const (
	DisputeStatusOpen     = "open"
	DisputeStatusResolved = "resolved"
)

// DisputeInfo is a read-only view of a marketplace_disputes row.
type DisputeInfo struct {
	ID            string `json:"id"`
	TransactionID string `json:"transaction_id"`
	OpenerDID     string `json:"opener_did"`
	Reason        string `json:"reason"`
	Status        string `json:"status"`
	ResolvedBy    string `json:"resolved_by,omitempty"`
	ResolvedAt    string `json:"resolved_at,omitempty"`
	CreatedAt     string `json:"created_at"`
}

// loadDispute reads one marketplace_disputes row by id.
func loadDispute(id string) (*DisputeInfo, error) {
	var d DisputeInfo
	var resolvedBy, resolvedAt sql.NullString
	err := db.DB.QueryRow(`
		SELECT id, transaction_id, opener_did, reason, status, resolved_by, resolved_at, created_at
		FROM marketplace_disputes WHERE id = ?`, id).Scan(
		&d.ID, &d.TransactionID, &d.OpenerDID, &d.Reason, &d.Status, &resolvedBy, &resolvedAt, &d.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrDisputeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("marketplace: load dispute: %w", err)
	}
	d.ResolvedBy = resolvedBy.String
	d.ResolvedAt = resolvedAt.String
	return &d, nil
}

// GetDispute returns one marketplace_disputes row by id.
func GetDispute(id string) (*DisputeInfo, error) {
	if id == "" {
		return nil, fmt.Errorf("%w: id required", ErrInvalidInput)
	}
	return loadDispute(id)
}

// GetDisputeForCaller is GetDispute gated to callerDID being the buyer or
// seller of the dispute's underlying transaction (#30 — the dispute opener
// is always the buyer, per OpenDispute's ErrNotBuyer check, but the seller
// should be able to see an open dispute against their own sale too).
func GetDisputeForCaller(id, callerDID string) (*DisputeInfo, error) {
	dispute, err := GetDispute(id)
	if err != nil {
		return nil, err
	}
	txn, err := loadTransaction(dispute.TransactionID)
	if err != nil {
		return nil, err
	}
	if txn.BuyerDID != callerDID && txn.SellerDID != callerDID {
		return nil, ErrNotParticipant
	}
	return dispute, nil
}

// OpenDispute lets transactionID's own buyer flag it for admin review (#31,
// vault Phase-Status.md 2026-08-11, plan commit 28b1527, Adım 5) — a
// SEPARATE table from review_queue, deliberately: review_queue's semantics
// are "remove/warn over content", this is "who gets paid over money" (see
// migration 166_marketplace_disputes's comment in database.go).
//
// Only "held" transactions can be disputed (ErrAlreadyResolved for
// released/refunded — both terminal, nothing left to dispute once money
// already moved) and only by that transaction's own buyer (ErrNotBuyer). At
// most one OPEN dispute per transaction (ErrDisputeAlreadyOpen) — this is
// data hygiene, not a money-safety guard: ResolveDispute's own atomic
// state-flip (resolveHeld) is what actually prevents double-payment, the
// same as Release.
//
// Deliberately does NOT block Release from also succeeding on a disputed
// transaction while the dispute is still open — not asked for by this
// step's scope, and money-safety doesn't need it: if a buyer opens a
// dispute and then also calls Release, resolveHeld's atomic flip means
// whichever happens first wins and the other gets ErrAlreadyResolved. A UI
// would typically hide the "release" action once a dispute is open, but
// that's a client concern, not a balance-safety one.
func OpenDispute(ctx context.Context, transactionID, buyerDID, reason string) (*DisputeInfo, error) {
	if transactionID == "" || buyerDID == "" || reason == "" {
		return nil, fmt.Errorf("%w: transactionID, buyerDID and reason required", ErrInvalidInput)
	}

	txn, err := loadTransaction(transactionID)
	if err != nil {
		return nil, err
	}
	if txn.BuyerDID != buyerDID {
		return nil, ErrNotBuyer
	}
	if txn.Status != TransactionStatusHeld {
		return nil, ErrAlreadyResolved
	}

	var existing int
	if err := db.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM marketplace_disputes WHERE transaction_id = ? AND status = ?`,
		transactionID, DisputeStatusOpen,
	).Scan(&existing); err != nil {
		return nil, fmt.Errorf("marketplace: check existing dispute: %w", err)
	}
	if existing > 0 {
		return nil, ErrDisputeAlreadyOpen
	}

	id := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.DB.ExecContext(ctx, `
		INSERT INTO marketplace_disputes (id, transaction_id, opener_did, reason, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		id, transactionID, buyerDID, reason, DisputeStatusOpen, now,
	); err != nil {
		return nil, fmt.Errorf("marketplace: insert dispute: %w", err)
	}

	return &DisputeInfo{ID: id, TransactionID: transactionID, OpenerDID: buyerDID, Reason: reason, Status: DisputeStatusOpen, CreatedAt: now}, nil
}

// ResolveDisputeResult is returned on a successful ResolveDispute.
type ResolveDisputeResult struct {
	DisputeID     string `json:"dispute_id"`
	TransactionID string `json:"transaction_id"`
	TokenTxID     string `json:"token_tx_id"`
	Amount        string `json:"amount"`
	Upheld        bool   `json:"upheld"`
	PaidTo        string `json:"paid_to"`
}

// ResolveDispute is the admin decision on an open dispute (#31, vault
// Phase-Status.md 2026-08-11, plan commit 28b1527, Adım 5). Caller
// authorization (admin-only, OBSCURA_ADMIN_DIDS) is enforced by the HTTP
// layer's AdminMiddleware before this is ever called — this function trusts
// adminDID and only records it as resolved_by, same as
// HandleAdminResolveReviewQueue trusts its own admin.DID.
//
//   - upheld=false (seller was right, sale stands): escrow pays the SELLER —
//     identical mechanics to Release, just admin-initiated instead of
//     buyer-initiated (see resolveHeld, shared by both).
//   - upheld=true (buyer was right): escrow REFUNDS the buyer.
//
// Refund amount is exactly `price`, never price+fee: the fee was already
// burned/pooled at Purchase() time (Adım 3, via token.Transfer) and escrow
// never held it in the first place — there is nothing to give back. This is
// a deliberate product decision, not an oversight: the platform's fee is
// earned the moment a sale is accepted into escrow; a refund reverses the
// goods payment, not the service fee. Refunding price+fee would require
// pulling money back out of FeePoolDID and un-burning supply, which
// token.InternalMove is deliberately incapable of (Adım 2: no supply
// changes, ever) and would reopen exactly the double-fee-charge risk the
// whole hold/release/refund design exists to avoid.
//
// Double-resolve defense and money-stuck handling are IDENTICAL to
// Release's — both go through resolveHeld's atomic marketplace_transactions
// state-flip. A second resolve of the same dispute — or a resolve racing a
// concurrent Release on the same transaction — sees RowsAffected == 0 on
// that flip and returns ErrAlreadyResolved without moving money twice.
func ResolveDispute(ctx context.Context, disputeID, adminDID string, upheld bool) (*ResolveDisputeResult, error) {
	if disputeID == "" || adminDID == "" {
		return nil, fmt.Errorf("%w: disputeID and adminDID required", ErrInvalidInput)
	}

	dispute, err := loadDispute(disputeID)
	if err != nil {
		return nil, err
	}
	if dispute.Status != DisputeStatusOpen {
		return nil, ErrAlreadyResolved
	}

	txn, err := loadTransaction(dispute.TransactionID)
	if err != nil {
		return nil, err
	}
	amount, ok := new(big.Int).SetString(txn.Amount, 10)
	if !ok || amount.Sign() <= 0 {
		return nil, fmt.Errorf("marketplace: transaction amount corrupt: %q", txn.Amount)
	}

	recipient := txn.SellerDID
	targetStatus := TransactionStatusReleased
	txType := "escrow_release"
	if upheld {
		recipient = txn.BuyerDID
		targetStatus = TransactionStatusRefunded
		txType = "escrow_refund"
	}

	tokenTxID, err := resolveHeld(ctx, txn, recipient, adminDID, targetStatus, txType, amount)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.DB.ExecContext(ctx,
		`UPDATE marketplace_disputes SET status = ?, resolved_by = ?, resolved_at = ? WHERE id = ? AND status = ?`,
		DisputeStatusResolved, adminDID, now, disputeID, DisputeStatusOpen,
	); err != nil {
		// Money already moved (resolveHeld committed) — marketplace_transactions
		// is the source of truth for money state and is already correctly
		// released/refunded. This is a bookkeeping gap only (the dispute row
		// itself didn't close), but still worth a reconciliation log so an
		// operator notices.
		log.Printf("MARKETPLACE RECONCILIATION NEEDED: dispute %s resolved (transaction %s, upheld=%v, paid %s to %s) but the dispute row itself failed to update: %v",
			disputeID, txn.ID, upheld, txn.Amount, recipient, err)
	}

	return &ResolveDisputeResult{
		DisputeID:     disputeID,
		TransactionID: txn.ID,
		TokenTxID:     tokenTxID,
		Amount:        txn.Amount,
		Upheld:        upheld,
		PaidTo:        recipient,
	}, nil
}
