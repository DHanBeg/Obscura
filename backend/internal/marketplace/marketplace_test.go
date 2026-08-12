package marketplace_test

// Tests for internal/marketplace. Uses a real on-disk SQLite DB in a temp
// dir (same pattern as internal/airdrop/airdrop_test.go and
// internal/token/token_test.go) — token.Transfer and the tier lookup both
// depend on the package-level db.DB, so a hand-rolled in-memory schema isn't
// enough here; db.Init runs the full migration set.

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"obscura.network/core/internal/db"
	"obscura.network/core/internal/marketplace"
	"obscura.network/core/internal/token"
)

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "obscura-marketplace-test-*")
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

var phoneCounter int64

// makeUser inserts a users row with the given DID and tier (mirrors
// airdrop_test.go's helper — sybil.CallerTier reads this row). Phone numbers
// are counter-derived, not DID-derived — several test DIDs in this file
// share a suffix (e.g. "...-seller"), which collided against users.phone's
// UNIQUE constraint when phone was built from the DID's tail.
func makeUser(t *testing.T, did string, tier int) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	n := atomic.AddInt64(&phoneCounter, 1)
	phone := fmt.Sprintf("+90555%07d", n)
	_, err := db.DB.Exec(`
		INSERT INTO users (id, phone, did, tier, created_at, updated_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		did, phone, did, tier, now, now, now)
	if err != nil {
		t.Fatalf("makeUser %s: %v", did, err)
	}
}

// fund mints amount to did so it can afford a purchase (Transfer + fee).
func fund(t *testing.T, did string, amount *big.Int) {
	t.Helper()
	if _, err := token.Mint(context.Background(), did, amount, "test funding"); err != nil {
		t.Fatalf("fund %s: %v", did, err)
	}
}

func TestCreateListing_RequiresSellerAccessLevel(t *testing.T) {
	base := "did:obs:mkt-seller-base"
	seller := "did:obs:mkt-seller-tier5"
	makeUser(t, base, 1)   // access level 1 — below SellerAccessLevel (3)
	makeUser(t, seller, 5) // access level 3

	if _, err := marketplace.CreateListing(context.Background(), base, "t", "d", "1000000000000000000", "electronics"); err != marketplace.ErrAccessDenied {
		t.Fatalf("CreateListing(base tier) err = %v, want ErrAccessDenied", err)
	}

	id, err := marketplace.CreateListing(context.Background(), seller, "Laptop", "Used, good condition", "1000000000000000000", "electronics")
	if err != nil {
		t.Fatalf("CreateListing(seller tier): %v", err)
	}
	if id == "" {
		t.Fatal("CreateListing returned empty id")
	}

	listing, err := marketplace.GetListing(id)
	if err != nil {
		t.Fatalf("GetListing: %v", err)
	}
	if listing.Status != marketplace.StatusActive {
		t.Fatalf("new listing status = %q, want %q", listing.Status, marketplace.StatusActive)
	}
	if listing.SellerDID != seller {
		t.Fatalf("SellerDID = %q, want %q", listing.SellerDID, seller)
	}
}

func TestCreateListing_InvalidInput(t *testing.T) {
	seller := "did:obs:mkt-seller-invalid"
	makeUser(t, seller, 4)

	cases := []struct {
		name, title, price, category string
	}{
		{"empty title", "", "100", "misc"},
		{"empty price", "t", "", "misc"},
		{"non-numeric price", "t", "not-a-number", "misc"},
		{"zero price", "t", "0", "misc"},
		{"negative price", "t", "-5", "misc"},
		{"empty category", "t", "100", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := marketplace.CreateListing(context.Background(), seller, c.title, "d", c.price, c.category); err == nil {
				t.Fatalf("CreateListing(%s) = nil error, want ErrInvalidInput", c.name)
			}
		})
	}
}

func TestUpdateListing_OnlySellerCanUpdate(t *testing.T) {
	seller := "did:obs:mkt-upd-seller"
	other := "did:obs:mkt-upd-other"
	makeUser(t, seller, 5)
	makeUser(t, other, 5)

	id, err := marketplace.CreateListing(context.Background(), seller, "Bike", "Road bike", "500000000000000000", "sports")
	if err != nil {
		t.Fatalf("CreateListing: %v", err)
	}

	newTitle := "Road Bike (Updated)"
	if err := marketplace.UpdateListing(context.Background(), id, other, marketplace.ListingPatch{Title: &newTitle}); err != marketplace.ErrNotSeller {
		t.Fatalf("UpdateListing(non-seller) err = %v, want ErrNotSeller", err)
	}

	if err := marketplace.UpdateListing(context.Background(), id, seller, marketplace.ListingPatch{Title: &newTitle}); err != nil {
		t.Fatalf("UpdateListing(seller): %v", err)
	}
	listing, err := marketplace.GetListing(id)
	if err != nil {
		t.Fatalf("GetListing: %v", err)
	}
	if listing.Title != newTitle {
		t.Fatalf("Title = %q, want %q", listing.Title, newTitle)
	}
}

// TestPurchase_HappyPath — CHANGED by escrow Adım 3 (#31, plan 28b1527):
// before this step it asserted "seller balance += price" (money paid out
// immediately). That's no longer true — Purchase() now holds funds in
// MarketplaceEscrowDID instead of paying the seller, so this asserts the
// opposite: seller balance UNCHANGED, escrow balance += price, and the
// marketplace_transactions row is "held" not "completed". It also pins down
// the buyer's total debit (price+fee) so a future change can't silently
// alter what the buyer pays.
func TestPurchase_HappyPath(t *testing.T) {
	seller := "did:obs:mkt-buy-seller"
	buyer := "did:obs:mkt-buy-buyer"
	makeUser(t, seller, 5)
	makeUser(t, buyer, 1) // buying is not tier-gated

	price := obs(10)
	fund(t, buyer, obs(100)) // covers price + transfer fee

	id, err := marketplace.CreateListing(context.Background(), seller, "Desk", "Standing desk", price.String(), "furniture")
	if err != nil {
		t.Fatalf("CreateListing: %v", err)
	}

	sellerBalBefore := mustBalance(t, seller)
	buyerBalBefore := mustBalance(t, buyer)
	escrowBalBefore := mustBalance(t, marketplace.MarketplaceEscrowDID)

	result, err := marketplace.Purchase(context.Background(), id, buyer)
	if err != nil {
		t.Fatalf("Purchase: %v", err)
	}
	if result.TokenTxID == "" || result.TransactionID == "" {
		t.Fatalf("Purchase result missing ids: %+v", result)
	}
	if result.Amount != price.String() {
		t.Fatalf("Amount = %q, want %q", result.Amount, price.String())
	}

	// Seller is NOT paid at purchase time anymore — funds sit in escrow.
	sellerBalAfter := mustBalance(t, seller)
	if sellerBalAfter.Cmp(sellerBalBefore) != 0 {
		t.Fatalf("seller balance = %s, want unchanged %s (escrow holds the payment now, not the seller)", sellerBalAfter, sellerBalBefore)
	}

	// Escrow holds exactly `price` — the full amount, no fee skimmed off it
	// (Transfer always credits the recipient the full amount; the fee is an
	// extra debit from the sender).
	escrowBalAfter := mustBalance(t, marketplace.MarketplaceEscrowDID)
	wantEscrow := new(big.Int).Add(escrowBalBefore, price)
	if escrowBalAfter.Cmp(wantEscrow) != 0 {
		t.Fatalf("escrow balance = %s, want %s", escrowBalAfter, wantEscrow)
	}

	// Buyer pays exactly price+fee — IDENTICAL to what a plain token.Transfer
	// would have debited before the escrow change. This is the "buyer's total
	// payment did not change" guarantee.
	buyerBalAfter := mustBalance(t, buyer)
	wantBuyerDebit := new(big.Int).Add(price, token.TransferFee())
	wantBuyer := new(big.Int).Sub(buyerBalBefore, wantBuyerDebit)
	if buyerBalAfter.Cmp(wantBuyer) != 0 {
		t.Fatalf("buyer balance = %s, want %s (debited price+fee = %s, same as pre-escrow Transfer)", buyerBalAfter, wantBuyer, wantBuyerDebit)
	}

	listing, err := marketplace.GetListing(id)
	if err != nil {
		t.Fatalf("GetListing: %v", err)
	}
	if listing.Status != marketplace.StatusSold {
		t.Fatalf("listing status = %q, want %q", listing.Status, marketplace.StatusSold)
	}

	// The marketplace_transactions row must record "held" (funds in escrow,
	// seller not yet paid), not "completed" — the pre-escrow terminal status.
	gotStatus := mustTxStatus(t, result.TransactionID)
	if gotStatus != marketplace.TransactionStatusHeld {
		t.Fatalf("marketplace_transactions.status = %q, want %q", gotStatus, marketplace.TransactionStatusHeld)
	}

	// The underlying obs_transactions row must be self-describing as an
	// escrow hold: to_did is the escrow account and memo is tagged
	// "marketplace-escrow-hold:" — so an operator reading the ledger doesn't
	// have to cross-reference marketplace_transactions to tell this apart
	// from some other kind of transfer.
	hist, err := token.History(buyer, 50)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	var holdTx *token.TxRecord
	for i := range hist {
		if hist[i].ID == result.TokenTxID {
			holdTx = &hist[i]
		}
	}
	if holdTx == nil {
		t.Fatalf("token tx %s not found in buyer history", result.TokenTxID)
	}
	if holdTx.ToDID != marketplace.MarketplaceEscrowDID {
		t.Fatalf("hold tx ToDID = %q, want %q", holdTx.ToDID, marketplace.MarketplaceEscrowDID)
	}
	wantMemo := "marketplace-escrow-hold:" + id
	if holdTx.Memo != wantMemo {
		t.Fatalf("hold tx Memo = %q, want %q", holdTx.Memo, wantMemo)
	}
}

// TestPurchase_BuyerTotalPaymentUnchangedByEscrow is the explicit
// side-by-side proof the plan calls for: a buyer purchasing through the
// escrow-routed Purchase() must be debited exactly what an equally-funded
// buyer moving the same amount through a plain token.Transfer would be
// debited (price+fee) — escrow only changes WHERE the price leg lands
// (escrow instead of the seller), never what the buyer pays.
func TestPurchase_BuyerTotalPaymentUnchangedByEscrow(t *testing.T) {
	seller := "did:obs:mkt-feecmp-seller"
	escrowBuyer := "did:obs:mkt-feecmp-escrow-buyer"
	plainBuyer := "did:obs:mkt-feecmp-plain-buyer"
	plainRecipient := "did:obs:mkt-feecmp-plain-recipient"
	makeUser(t, seller, 5)
	makeUser(t, escrowBuyer, 1)
	makeUser(t, plainBuyer, 1)

	price := obs(20)
	fund(t, escrowBuyer, obs(100))
	fund(t, plainBuyer, obs(100))

	id, err := marketplace.CreateListing(context.Background(), seller, "Camera", "Old but works", price.String(), "electronics")
	if err != nil {
		t.Fatalf("CreateListing: %v", err)
	}

	escrowBuyerBefore := mustBalance(t, escrowBuyer)
	plainBuyerBefore := mustBalance(t, plainBuyer)

	if _, err := marketplace.Purchase(context.Background(), id, escrowBuyer); err != nil {
		t.Fatalf("Purchase: %v", err)
	}
	if _, err := token.Transfer(context.Background(), plainBuyer, plainRecipient, price, "comparison"); err != nil {
		t.Fatalf("Transfer: %v", err)
	}

	escrowBuyerDebit := new(big.Int).Sub(escrowBuyerBefore, mustBalance(t, escrowBuyer))
	plainBuyerDebit := new(big.Int).Sub(plainBuyerBefore, mustBalance(t, plainBuyer))
	if escrowBuyerDebit.Cmp(plainBuyerDebit) != 0 {
		t.Fatalf("escrow-purchase buyer debit = %s, plain-Transfer buyer debit = %s — must be identical (no silent fee change)",
			escrowBuyerDebit, plainBuyerDebit)
	}
}

// mustTxStatus reads marketplace_transactions.status for a purchase id.
func mustTxStatus(t *testing.T, purchaseID string) string {
	t.Helper()
	var status string
	if err := db.DB.QueryRow(`SELECT status FROM marketplace_transactions WHERE id = ?`, purchaseID).Scan(&status); err != nil {
		t.Fatalf("read marketplace_transactions status for %s: %v", purchaseID, err)
	}
	return status
}

func TestPurchase_SelfPurchaseRejected(t *testing.T) {
	seller := "did:obs:mkt-self-seller"
	makeUser(t, seller, 5)
	fund(t, seller, obs(100))

	id, err := marketplace.CreateListing(context.Background(), seller, "Chair", "Office chair", obs(5).String(), "furniture")
	if err != nil {
		t.Fatalf("CreateListing: %v", err)
	}

	if _, err := marketplace.Purchase(context.Background(), id, seller); err != marketplace.ErrSelfPurchase {
		t.Fatalf("Purchase(self) err = %v, want ErrSelfPurchase", err)
	}
}

func TestPurchase_ClosedListingRejected(t *testing.T) {
	seller := "did:obs:mkt-closed-seller"
	buyer1 := "did:obs:mkt-closed-buyer1"
	buyer2 := "did:obs:mkt-closed-buyer2"
	makeUser(t, seller, 5)
	makeUser(t, buyer1, 1)
	makeUser(t, buyer2, 1)
	fund(t, buyer1, obs(100))
	fund(t, buyer2, obs(100))

	id, err := marketplace.CreateListing(context.Background(), seller, "Lamp", "Desk lamp", obs(1).String(), "furniture")
	if err != nil {
		t.Fatalf("CreateListing: %v", err)
	}

	if _, err := marketplace.Purchase(context.Background(), id, buyer1); err != nil {
		t.Fatalf("first Purchase: %v", err)
	}

	// Listing is now 'sold' — a second purchase attempt must be rejected,
	// which also proves the reservation step actually closes the listing
	// (no double-sell).
	if _, err := marketplace.Purchase(context.Background(), id, buyer2); err != marketplace.ErrListingClosed {
		t.Fatalf("second Purchase err = %v, want ErrListingClosed", err)
	}
}

func TestPurchase_NotFound(t *testing.T) {
	buyer := "did:obs:mkt-notfound-buyer"
	makeUser(t, buyer, 1)
	if _, err := marketplace.Purchase(context.Background(), "no-such-listing", buyer); err != marketplace.ErrNotFound {
		t.Fatalf("Purchase(unknown listing) err = %v, want ErrNotFound", err)
	}
}

func TestListListings_FiltersByStatus(t *testing.T) {
	seller := "did:obs:mkt-list-seller"
	makeUser(t, seller, 5)

	id1, err := marketplace.CreateListing(context.Background(), seller, "Item A", "d", "100", "misc")
	if err != nil {
		t.Fatalf("CreateListing A: %v", err)
	}
	if _, err := marketplace.CreateListing(context.Background(), seller, "Item B", "d", "100", "misc"); err != nil {
		t.Fatalf("CreateListing B: %v", err)
	}

	removed := marketplace.StatusRemoved
	if err := marketplace.UpdateListing(context.Background(), id1, seller, marketplace.ListingPatch{Status: &removed}); err != nil {
		t.Fatalf("UpdateListing: %v", err)
	}

	active, err := marketplace.ListListings(marketplace.StatusActive, 50, 0)
	if err != nil {
		t.Fatalf("ListListings(active): %v", err)
	}
	for _, l := range active {
		if l.ID == id1 {
			t.Fatalf("removed listing %s still present in active list", id1)
		}
	}

	removedList, err := marketplace.ListListings(marketplace.StatusRemoved, 50, 0)
	if err != nil {
		t.Fatalf("ListListings(removed): %v", err)
	}
	found := false
	for _, l := range removedList {
		if l.ID == id1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("removed listing %s not found in removed list", id1)
	}
}

// TestPurchase_RecordInsertFails_LogsReconciliation forces the
// marketplace_transactions INSERT to fail AFTER token.Transfer has already
// committed, by dropping the table out from under Purchase. This proves two
// things: (1) the returned error carries every field an operator needs to
// manually reconcile (token tx id, listing id, buyer, seller, amount), and
// (2) a server-side log line is written too — the error alone doesn't
// survive past whatever HTTP handler logs it, so a money-moved-but-
// unrecorded event must also be independently greppable in server logs.
func TestPurchase_RecordInsertFails_LogsReconciliation(t *testing.T) {
	seller := "did:obs:mkt-reconcile-seller"
	buyer := "did:obs:mkt-reconcile-buyer"
	makeUser(t, seller, 5)
	makeUser(t, buyer, 1)
	fund(t, buyer, obs(100))

	price := obs(7)
	id, err := marketplace.CreateListing(context.Background(), seller, "Monitor", "27-inch", price.String(), "electronics")
	if err != nil {
		t.Fatalf("CreateListing: %v", err)
	}

	if _, err := db.DB.Exec(`DROP TABLE marketplace_transactions`); err != nil {
		t.Fatalf("drop marketplace_transactions: %v", err)
	}
	t.Cleanup(func() {
		// Restore the table so later tests in this binary still have it —
		// TestMain runs migrations once for the whole package, not per-test.
		// Must match the CURRENT full schema (migration 151 + 164/165's
		// resolved_at/resolved_by), not just migration 151's original shape —
		// this bit release_test.go's tests once already: they all run after
		// this one in the same binary and need those columns to exist.
		schema := `CREATE TABLE IF NOT EXISTS marketplace_transactions (
			id          TEXT PRIMARY KEY,
			listing_id  TEXT NOT NULL,
			buyer_did   TEXT NOT NULL,
			seller_did  TEXT NOT NULL,
			amount      TEXT NOT NULL,
			token_tx_id TEXT NOT NULL,
			status      TEXT NOT NULL DEFAULT 'completed',
			created_at  TEXT NOT NULL,
			resolved_at TEXT,
			resolved_by TEXT
		)`
		if _, err := db.DB.Exec(schema); err != nil {
			t.Fatalf("restore marketplace_transactions: %v", err)
		}
	})

	var logBuf bytes.Buffer
	prevOutput := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&logBuf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(prevOutput)
		log.SetFlags(prevFlags)
	}()

	_, err = marketplace.Purchase(context.Background(), id, buyer)
	if err == nil {
		t.Fatal("Purchase: want error when transaction record insert fails, got nil")
	}

	for _, want := range []string{id, buyer, seller, price.String()} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Purchase error %q missing %q", err.Error(), want)
		}
	}

	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "RECONCILIATION") {
		t.Errorf("log output missing RECONCILIATION marker, got: %q", logOutput)
	}
	for _, want := range []string{id, buyer, seller, price.String()} {
		if !strings.Contains(logOutput, want) {
			t.Errorf("log output %q missing %q", logOutput, want)
		}
	}

	// The listing must still be marked sold — the payment happened even
	// though the bookkeeping row didn't.
	listing, err := marketplace.GetListing(id)
	if err != nil {
		t.Fatalf("GetListing: %v", err)
	}
	if listing.Status != marketplace.StatusSold {
		t.Fatalf("listing status = %q, want %q (payment succeeded despite record failure)", listing.Status, marketplace.StatusSold)
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
