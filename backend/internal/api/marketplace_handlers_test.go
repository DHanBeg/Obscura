package api

// Tests for the marketplace HTTP handlers. Same approach as
// complaint_handlers_test.go: call handlers directly, injecting the
// authenticated user into the request context via withUser (bypassing the
// pre-existing, already-broken OTP/JWT integration fixture — see that
// file's comment). marketplace.CreateListing's tier gate reads the DID's
// tier from the users table (via sybil.CallerTier), not from the injected
// models.User struct, so tests seed a real users row too.

import (
	"bytes"
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/marketplace"
	"obscura.network/core/internal/models"
	"obscura.network/core/internal/token"
)

// seedMarketplaceUser inserts a users row with the given DID and tier —
// marketplace.CreateListing's access-level gate reads this via
// sybil.CallerTier, independent of the models.User injected via withUser.
func seedMarketplaceUser(t *testing.T, did string, tier int) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	suffix := uuid.New().String()[:8]
	_, err := db.DB.Exec(`
		INSERT INTO users (id, phone, username, display_name, did, identity_key, avatar_url,
		                   tier, credit_score, is_active, is_banned, node_id,
		                   created_at, updated_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, '', '', ?, 50, 1, 0, '', ?, ?, ?)`,
		did, "+9055501"+suffix, "mkt-"+suffix, "mkt-"+suffix, did, tier, now, now, now,
	)
	if err != nil {
		t.Fatalf("seedMarketplaceUser %s: %v", did, err)
	}
}

// obsAmount returns n whole OBS as a smallest-unit decimal string.
func obsAmount(n int64) string {
	return new(big.Int).Mul(big.NewInt(n), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)).String()
}

func fundMarketplaceUser(t *testing.T, did string, wholeOBS int64) {
	t.Helper()
	amount, _ := new(big.Int).SetString(obsAmount(wholeOBS), 10)
	if _, err := token.Mint(context.Background(), did, amount, "test funding"); err != nil {
		t.Fatalf("fund %s: %v", did, err)
	}
}

func TestHandleMarketplaceCreateListing_HappyPath(t *testing.T) {
	seller := "did:obs:mkth-create-seller"
	seedMarketplaceUser(t, seller, 5)

	body, _ := json.Marshal(map[string]string{
		"title": "Laptop", "description": "Used", "price": obsAmount(1), "category": "electronics",
	})
	req := withUser(httptest.NewRequest("POST", "/v1/marketplace/listings", bytes.NewReader(body)),
		&models.User{DID: seller})
	rec := httptest.NewRecorder()

	HandleMarketplaceCreateListing(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	resp := decodeResp(t, rec)
	if !resp.Success {
		t.Fatalf("Success = false, error = %q", resp.Error)
	}
}

func TestHandleMarketplaceCreateListing_AccessDenied(t *testing.T) {
	base := "did:obs:mkth-create-base"
	seedMarketplaceUser(t, base, 1) // access level 1, below SellerAccessLevel (3)

	body, _ := json.Marshal(map[string]string{
		"title": "Laptop", "description": "Used", "price": obsAmount(1), "category": "electronics",
	})
	req := withUser(httptest.NewRequest("POST", "/v1/marketplace/listings", bytes.NewReader(body)),
		&models.User{DID: base})
	rec := httptest.NewRecorder()

	HandleMarketplaceCreateListing(rec, req)

	if rec.Code != 403 {
		t.Fatalf("status = %d, want 403 (base-tier seller)", rec.Code)
	}
}

func TestHandleMarketplaceListListings_HappyPath(t *testing.T) {
	seller := "did:obs:mkth-list-seller"
	seedMarketplaceUser(t, seller, 5)
	if _, err := marketplace.CreateListing(context.Background(), seller, "Item", "d", obsAmount(1), "misc"); err != nil {
		t.Fatalf("CreateListing: %v", err)
	}

	req := withUser(httptest.NewRequest("GET", "/v1/marketplace/listings?status=active", nil),
		&models.User{DID: seller})
	rec := httptest.NewRecorder()

	HandleMarketplaceListListings(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	resp := decodeResp(t, rec)
	if !resp.Success {
		t.Fatalf("Success = false, error = %q", resp.Error)
	}
}

func TestHandleMarketplaceGetListing_HappyPath(t *testing.T) {
	seller := "did:obs:mkth-get-seller"
	seedMarketplaceUser(t, seller, 5)
	id, err := marketplace.CreateListing(context.Background(), seller, "Item", "d", obsAmount(1), "misc")
	if err != nil {
		t.Fatalf("CreateListing: %v", err)
	}

	req := withUser(httptest.NewRequest("GET", "/v1/marketplace/listings/"+id, nil), &models.User{DID: seller})
	req = mux.SetURLVars(req, map[string]string{"id": id})
	rec := httptest.NewRecorder()

	HandleMarketplaceGetListing(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestHandleMarketplaceGetListing_NotFound(t *testing.T) {
	req := withUser(httptest.NewRequest("GET", "/v1/marketplace/listings/no-such-id", nil), &models.User{DID: "did:obs:mkth-get-nf"})
	req = mux.SetURLVars(req, map[string]string{"id": "no-such-id"})
	rec := httptest.NewRecorder()

	HandleMarketplaceGetListing(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestHandleMarketplaceUpdateListing_HappyPath(t *testing.T) {
	seller := "did:obs:mkth-update-seller"
	seedMarketplaceUser(t, seller, 5)
	id, err := marketplace.CreateListing(context.Background(), seller, "Item", "d", obsAmount(1), "misc")
	if err != nil {
		t.Fatalf("CreateListing: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"title": "Updated Title"})
	req := withUser(httptest.NewRequest("PATCH", "/v1/marketplace/listings/"+id, bytes.NewReader(body)),
		&models.User{DID: seller})
	req = mux.SetURLVars(req, map[string]string{"id": id})
	rec := httptest.NewRecorder()

	HandleMarketplaceUpdateListing(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	listing, err := marketplace.GetListing(id)
	if err != nil {
		t.Fatalf("GetListing: %v", err)
	}
	if listing.Title != "Updated Title" {
		t.Fatalf("Title = %q, want %q", listing.Title, "Updated Title")
	}
}

func TestHandleMarketplaceUpdateListing_NotSeller(t *testing.T) {
	seller := "did:obs:mkth-update-owner"
	other := "did:obs:mkth-update-other"
	seedMarketplaceUser(t, seller, 5)
	seedMarketplaceUser(t, other, 5)
	id, err := marketplace.CreateListing(context.Background(), seller, "Item", "d", obsAmount(1), "misc")
	if err != nil {
		t.Fatalf("CreateListing: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"title": "Hijacked"})
	req := withUser(httptest.NewRequest("PATCH", "/v1/marketplace/listings/"+id, bytes.NewReader(body)),
		&models.User{DID: other})
	req = mux.SetURLVars(req, map[string]string{"id": id})
	rec := httptest.NewRecorder()

	HandleMarketplaceUpdateListing(rec, req)

	if rec.Code != 403 {
		t.Fatalf("status = %d, want 403 (non-seller update)", rec.Code)
	}
}

func TestHandleMarketplaceDeleteListing_HappyPath(t *testing.T) {
	seller := "did:obs:mkth-delete-seller"
	seedMarketplaceUser(t, seller, 5)
	id, err := marketplace.CreateListing(context.Background(), seller, "Item", "d", obsAmount(1), "misc")
	if err != nil {
		t.Fatalf("CreateListing: %v", err)
	}

	req := withUser(httptest.NewRequest("DELETE", "/v1/marketplace/listings/"+id, nil), &models.User{DID: seller})
	req = mux.SetURLVars(req, map[string]string{"id": id})
	rec := httptest.NewRecorder()

	HandleMarketplaceDeleteListing(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	listing, err := marketplace.GetListing(id)
	if err != nil {
		t.Fatalf("GetListing: %v", err)
	}
	if listing.Status != marketplace.StatusRemoved {
		t.Fatalf("status = %q, want %q", listing.Status, marketplace.StatusRemoved)
	}
}

func TestHandleMarketplacePurchase_HappyPath(t *testing.T) {
	seller := "did:obs:mkth-purchase-seller"
	buyer := "did:obs:mkth-purchase-buyer"
	seedMarketplaceUser(t, seller, 5)
	seedMarketplaceUser(t, buyer, 1)
	fundMarketplaceUser(t, buyer, 100)

	id, err := marketplace.CreateListing(context.Background(), seller, "Item", "d", obsAmount(5), "misc")
	if err != nil {
		t.Fatalf("CreateListing: %v", err)
	}

	req := withUser(httptest.NewRequest("POST", "/v1/marketplace/listings/"+id+"/purchase", nil), &models.User{DID: buyer})
	req = mux.SetURLVars(req, map[string]string{"id": id})
	rec := httptest.NewRecorder()

	HandleMarketplacePurchase(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	listing, err := marketplace.GetListing(id)
	if err != nil {
		t.Fatalf("GetListing: %v", err)
	}
	if listing.Status != marketplace.StatusSold {
		t.Fatalf("status = %q, want %q", listing.Status, marketplace.StatusSold)
	}
}

func TestHandleMarketplacePurchase_SelfPurchaseRejected(t *testing.T) {
	seller := "did:obs:mkth-purchase-self"
	seedMarketplaceUser(t, seller, 5)
	fundMarketplaceUser(t, seller, 100)

	id, err := marketplace.CreateListing(context.Background(), seller, "Item", "d", obsAmount(5), "misc")
	if err != nil {
		t.Fatalf("CreateListing: %v", err)
	}

	req := withUser(httptest.NewRequest("POST", "/v1/marketplace/listings/"+id+"/purchase", nil), &models.User{DID: seller})
	req = mux.SetURLVars(req, map[string]string{"id": id})
	rec := httptest.NewRecorder()

	HandleMarketplacePurchase(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 (self-purchase)", rec.Code)
	}
}

// TestHandleMarketplaceRelease_HappyPath — #31 Adım 4 HTTP surface: buyer
// releases a held purchase, seller gets paid.
func TestHandleMarketplaceRelease_HappyPath(t *testing.T) {
	seller := "did:obs:mkth-release-seller"
	buyer := "did:obs:mkth-release-buyer"
	seedMarketplaceUser(t, seller, 5)
	seedMarketplaceUser(t, buyer, 1)
	fundMarketplaceUser(t, buyer, 100)

	listingID, err := marketplace.CreateListing(context.Background(), seller, "Item", "d", obsAmount(5), "misc")
	if err != nil {
		t.Fatalf("CreateListing: %v", err)
	}
	purchase, err := marketplace.Purchase(context.Background(), listingID, buyer)
	if err != nil {
		t.Fatalf("Purchase: %v", err)
	}

	req := withUser(httptest.NewRequest("POST", "/v1/marketplace/transactions/"+purchase.TransactionID+"/release", nil), &models.User{DID: buyer})
	req = mux.SetURLVars(req, map[string]string{"id": purchase.TransactionID})
	rec := httptest.NewRecorder()

	HandleMarketplaceRelease(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	txn, err := marketplace.GetTransaction(purchase.TransactionID)
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}
	if txn.Status != marketplace.TransactionStatusReleased {
		t.Fatalf("status = %q, want %q", txn.Status, marketplace.TransactionStatusReleased)
	}
}

// TestHandleMarketplaceRelease_NotBuyer_Rejected proves the 403 mapping for
// marketplace.ErrNotBuyer end to end through the HTTP handler.
func TestHandleMarketplaceRelease_NotBuyer_Rejected(t *testing.T) {
	seller := "did:obs:mkth-release-nb-seller"
	buyer := "did:obs:mkth-release-nb-buyer"
	stranger := "did:obs:mkth-release-nb-stranger"
	seedMarketplaceUser(t, seller, 5)
	seedMarketplaceUser(t, buyer, 1)
	fundMarketplaceUser(t, buyer, 100)

	listingID, err := marketplace.CreateListing(context.Background(), seller, "Item", "d", obsAmount(5), "misc")
	if err != nil {
		t.Fatalf("CreateListing: %v", err)
	}
	purchase, err := marketplace.Purchase(context.Background(), listingID, buyer)
	if err != nil {
		t.Fatalf("Purchase: %v", err)
	}

	req := withUser(httptest.NewRequest("POST", "/v1/marketplace/transactions/"+purchase.TransactionID+"/release", nil), &models.User{DID: stranger})
	req = mux.SetURLVars(req, map[string]string{"id": purchase.TransactionID})
	rec := httptest.NewRecorder()

	HandleMarketplaceRelease(rec, req)

	if rec.Code != 403 {
		t.Fatalf("status = %d, want 403 (body: %s)", rec.Code, rec.Body.String())
	}
	txn, err := marketplace.GetTransaction(purchase.TransactionID)
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}
	if txn.Status != marketplace.TransactionStatusHeld {
		t.Fatalf("status = %q, want still %q (rejected release must not flip state)", txn.Status, marketplace.TransactionStatusHeld)
	}
}

// TestHandleMarketplaceRelease_AlreadyReleased_Conflict proves the 409
// mapping for marketplace.ErrAlreadyResolved.
func TestHandleMarketplaceRelease_AlreadyReleased_Conflict(t *testing.T) {
	seller := "did:obs:mkth-release-dup-seller"
	buyer := "did:obs:mkth-release-dup-buyer"
	seedMarketplaceUser(t, seller, 5)
	seedMarketplaceUser(t, buyer, 1)
	fundMarketplaceUser(t, buyer, 100)

	listingID, err := marketplace.CreateListing(context.Background(), seller, "Item", "d", obsAmount(5), "misc")
	if err != nil {
		t.Fatalf("CreateListing: %v", err)
	}
	purchase, err := marketplace.Purchase(context.Background(), listingID, buyer)
	if err != nil {
		t.Fatalf("Purchase: %v", err)
	}
	if _, err := marketplace.Release(context.Background(), purchase.TransactionID, buyer); err != nil {
		t.Fatalf("first Release: %v", err)
	}

	req := withUser(httptest.NewRequest("POST", "/v1/marketplace/transactions/"+purchase.TransactionID+"/release", nil), &models.User{DID: buyer})
	req = mux.SetURLVars(req, map[string]string{"id": purchase.TransactionID})
	rec := httptest.NewRecorder()

	HandleMarketplaceRelease(rec, req)

	if rec.Code != 409 {
		t.Fatalf("status = %d, want 409 (body: %s)", rec.Code, rec.Body.String())
	}
}

// TestHandleMarketplaceOpenDispute_HappyPath — #31 Adım 5 HTTP surface:
// buyer opens a dispute on a held transaction.
func TestHandleMarketplaceOpenDispute_HappyPath(t *testing.T) {
	seller := "did:obs:mkth-dispute-seller"
	buyer := "did:obs:mkth-dispute-buyer"
	seedMarketplaceUser(t, seller, 5)
	seedMarketplaceUser(t, buyer, 1)
	fundMarketplaceUser(t, buyer, 100)

	listingID, err := marketplace.CreateListing(context.Background(), seller, "Item", "d", obsAmount(5), "misc")
	if err != nil {
		t.Fatalf("CreateListing: %v", err)
	}
	purchase, err := marketplace.Purchase(context.Background(), listingID, buyer)
	if err != nil {
		t.Fatalf("Purchase: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"reason": "never arrived"})
	req := withUser(httptest.NewRequest("POST", "/v1/marketplace/transactions/"+purchase.TransactionID+"/dispute", bytes.NewReader(body)), &models.User{DID: buyer})
	req = mux.SetURLVars(req, map[string]string{"id": purchase.TransactionID})
	rec := httptest.NewRecorder()

	HandleMarketplaceOpenDispute(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
}

// TestHandleAdminResolveMarketplaceDispute_AdminHappyPath wires
// AdminMiddleware directly around the handler (same shape as
// admin_middleware_test.go's callAuthed, adapted to this package's
// withUser-based auth bypass) and proves both directions explicitly asked
// for: an admin DID resolves successfully (200), a non-admin DID is
// rejected (403) without moving money.
func TestHandleAdminResolveMarketplaceDispute_AdminHappyPath(t *testing.T) {
	seller := "did:obs:mkth-adminresolve-seller"
	buyer := "did:obs:mkth-adminresolve-buyer"
	admin := "did:obs:mkth-adminresolve-admin"
	seedMarketplaceUser(t, seller, 5)
	seedMarketplaceUser(t, buyer, 1)
	fundMarketplaceUser(t, buyer, 100)

	listingID, err := marketplace.CreateListing(context.Background(), seller, "Item", "d", obsAmount(5), "misc")
	if err != nil {
		t.Fatalf("CreateListing: %v", err)
	}
	purchase, err := marketplace.Purchase(context.Background(), listingID, buyer)
	if err != nil {
		t.Fatalf("Purchase: %v", err)
	}
	dispute, err := marketplace.OpenDispute(context.Background(), purchase.TransactionID, buyer, "wrong item")
	if err != nil {
		t.Fatalf("OpenDispute: %v", err)
	}

	prev, had := os.LookupEnv("OBSCURA_ADMIN_DIDS")
	os.Setenv("OBSCURA_ADMIN_DIDS", admin)
	t.Cleanup(func() {
		if had {
			os.Setenv("OBSCURA_ADMIN_DIDS", prev)
		} else {
			os.Unsetenv("OBSCURA_ADMIN_DIDS")
		}
	})

	handler := AdminMiddleware(http.HandlerFunc(HandleAdminResolveMarketplaceDispute))
	body, _ := json.Marshal(map[string]bool{"upheld": false})

	req := withUser(httptest.NewRequest("POST", "/v1/admin/marketplace-disputes/"+dispute.ID+"/resolve", bytes.NewReader(body)), &models.User{DID: admin})
	req = mux.SetURLVars(req, map[string]string{"id": dispute.ID})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("admin resolve status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	txn, err := marketplace.GetTransaction(purchase.TransactionID)
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}
	if txn.Status != marketplace.TransactionStatusReleased {
		t.Fatalf("tx status = %q, want %q", txn.Status, marketplace.TransactionStatusReleased)
	}
}

// TestHandleAdminResolveMarketplaceDispute_NotAdmin_Rejected proves
// AdminMiddleware actually gates this specific route: a caller not listed
// in OBSCURA_ADMIN_DIDS gets 403 and no money moves.
func TestHandleAdminResolveMarketplaceDispute_NotAdmin_Rejected(t *testing.T) {
	seller := "did:obs:mkth-notadmin-seller"
	buyer := "did:obs:mkth-notadmin-buyer"
	notAdmin := "did:obs:mkth-notadmin-caller"
	seedMarketplaceUser(t, seller, 5)
	seedMarketplaceUser(t, buyer, 1)
	fundMarketplaceUser(t, buyer, 100)

	listingID, err := marketplace.CreateListing(context.Background(), seller, "Item", "d", obsAmount(5), "misc")
	if err != nil {
		t.Fatalf("CreateListing: %v", err)
	}
	purchase, err := marketplace.Purchase(context.Background(), listingID, buyer)
	if err != nil {
		t.Fatalf("Purchase: %v", err)
	}
	dispute, err := marketplace.OpenDispute(context.Background(), purchase.TransactionID, buyer, "wrong item")
	if err != nil {
		t.Fatalf("OpenDispute: %v", err)
	}

	prev, had := os.LookupEnv("OBSCURA_ADMIN_DIDS")
	os.Setenv("OBSCURA_ADMIN_DIDS", "did:obs:someone-else-entirely")
	t.Cleanup(func() {
		if had {
			os.Setenv("OBSCURA_ADMIN_DIDS", prev)
		} else {
			os.Unsetenv("OBSCURA_ADMIN_DIDS")
		}
	})

	handler := AdminMiddleware(http.HandlerFunc(HandleAdminResolveMarketplaceDispute))
	body, _ := json.Marshal(map[string]bool{"upheld": false})

	req := withUser(httptest.NewRequest("POST", "/v1/admin/marketplace-disputes/"+dispute.ID+"/resolve", bytes.NewReader(body)), &models.User{DID: notAdmin})
	req = mux.SetURLVars(req, map[string]string{"id": dispute.ID})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 403 {
		t.Fatalf("status = %d, want 403 (body: %s)", rec.Code, rec.Body.String())
	}
	sellerBal, err := token.Balance(seller)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if sellerBal.Sign() != 0 {
		t.Fatalf("seller balance = %s, want 0 (rejected admin resolve must not pay anyone)", sellerBal)
	}
	txn, err := marketplace.GetTransaction(purchase.TransactionID)
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}
	if txn.Status != marketplace.TransactionStatusHeld {
		t.Fatalf("tx status = %q, want still %q", txn.Status, marketplace.TransactionStatusHeld)
	}
}

// ─── #30 — GET /v1/marketplace/transactions[/id], GET /v1/marketplace/disputes/{id} ──

func TestHandleMarketplaceGetTransaction_BuyerAndSeller_HappyPath(t *testing.T) {
	seller := "did:obs:mkth-gettx-seller"
	buyer := "did:obs:mkth-gettx-buyer"
	seedMarketplaceUser(t, seller, 5)
	seedMarketplaceUser(t, buyer, 1)
	fundMarketplaceUser(t, buyer, 100)

	listingID, err := marketplace.CreateListing(context.Background(), seller, "Item", "d", obsAmount(5), "misc")
	if err != nil {
		t.Fatalf("CreateListing: %v", err)
	}
	purchase, err := marketplace.Purchase(context.Background(), listingID, buyer)
	if err != nil {
		t.Fatalf("Purchase: %v", err)
	}

	for _, caller := range []string{buyer, seller} {
		req := withUser(httptest.NewRequest("GET", "/v1/marketplace/transactions/"+purchase.TransactionID, nil), &models.User{DID: caller})
		req = mux.SetURLVars(req, map[string]string{"id": purchase.TransactionID})
		rec := httptest.NewRecorder()

		HandleMarketplaceGetTransaction(rec, req)

		if rec.Code != 200 {
			t.Fatalf("caller=%s status = %d, want 200 (body: %s)", caller, rec.Code, rec.Body.String())
		}
	}
}

func TestHandleMarketplaceGetTransaction_Stranger_Rejected(t *testing.T) {
	seller := "did:obs:mkth-gettx-strg-seller"
	buyer := "did:obs:mkth-gettx-strg-buyer"
	stranger := "did:obs:mkth-gettx-strg-stranger"
	seedMarketplaceUser(t, seller, 5)
	seedMarketplaceUser(t, buyer, 1)
	fundMarketplaceUser(t, buyer, 100)

	listingID, err := marketplace.CreateListing(context.Background(), seller, "Item", "d", obsAmount(5), "misc")
	if err != nil {
		t.Fatalf("CreateListing: %v", err)
	}
	purchase, err := marketplace.Purchase(context.Background(), listingID, buyer)
	if err != nil {
		t.Fatalf("Purchase: %v", err)
	}

	req := withUser(httptest.NewRequest("GET", "/v1/marketplace/transactions/"+purchase.TransactionID, nil), &models.User{DID: stranger})
	req = mux.SetURLVars(req, map[string]string{"id": purchase.TransactionID})
	rec := httptest.NewRecorder()

	HandleMarketplaceGetTransaction(rec, req)

	if rec.Code != 403 {
		t.Fatalf("status = %d, want 403 (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestHandleMarketplaceGetTransaction_NotFound(t *testing.T) {
	req := withUser(httptest.NewRequest("GET", "/v1/marketplace/transactions/no-such-id", nil), &models.User{DID: "did:obs:mkth-gettx-nf"})
	req = mux.SetURLVars(req, map[string]string{"id": "no-such-id"})
	rec := httptest.NewRecorder()

	HandleMarketplaceGetTransaction(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// TestHandleMarketplaceListMyTransactions_CoversBuyerAndSellerRoles proves
// one caller sees transactions where they bought AND transactions where
// they sold, in one list (#30 doc comment: "buyer_did/seller_did ile
// ayırt eder").
func TestHandleMarketplaceListMyTransactions_CoversBuyerAndSellerRoles(t *testing.T) {
	alice := "did:obs:mkth-listtx-alice"
	bob := "did:obs:mkth-listtx-bob"
	seedMarketplaceUser(t, alice, 5)
	seedMarketplaceUser(t, bob, 5)
	fundMarketplaceUser(t, alice, 100)
	fundMarketplaceUser(t, bob, 100)

	// Alice sells to Bob.
	listing1, err := marketplace.CreateListing(context.Background(), alice, "Item1", "d", obsAmount(5), "misc")
	if err != nil {
		t.Fatalf("CreateListing 1: %v", err)
	}
	if _, err := marketplace.Purchase(context.Background(), listing1, bob); err != nil {
		t.Fatalf("Purchase 1: %v", err)
	}

	// Bob sells to Alice.
	listing2, err := marketplace.CreateListing(context.Background(), bob, "Item2", "d", obsAmount(3), "misc")
	if err != nil {
		t.Fatalf("CreateListing 2: %v", err)
	}
	if _, err := marketplace.Purchase(context.Background(), listing2, alice); err != nil {
		t.Fatalf("Purchase 2: %v", err)
	}

	req := withUser(httptest.NewRequest("GET", "/v1/marketplace/transactions", nil), &models.User{DID: alice})
	rec := httptest.NewRecorder()

	HandleMarketplaceListMyTransactions(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var body struct {
		Data struct {
			Transactions []marketplace.TransactionInfo `json:"transactions"`
			Count        int                            `json:"count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v (body: %s)", err, rec.Body.String())
	}
	if body.Data.Count != 2 {
		t.Fatalf("count = %d, want 2 (one bought, one sold)", body.Data.Count)
	}
	sawBought, sawSold := false, false
	for _, txn := range body.Data.Transactions {
		if txn.BuyerDID == alice {
			sawBought = true
		}
		if txn.SellerDID == alice {
			sawSold = true
		}
	}
	if !sawBought || !sawSold {
		t.Fatalf("expected both a bought and a sold transaction, sawBought=%v sawSold=%v", sawBought, sawSold)
	}
}

func TestHandleMarketplaceGetDispute_BuyerAndSeller_HappyPath(t *testing.T) {
	seller := "did:obs:mkth-getdisp-seller"
	buyer := "did:obs:mkth-getdisp-buyer"
	seedMarketplaceUser(t, seller, 5)
	seedMarketplaceUser(t, buyer, 1)
	fundMarketplaceUser(t, buyer, 100)

	listingID, err := marketplace.CreateListing(context.Background(), seller, "Item", "d", obsAmount(5), "misc")
	if err != nil {
		t.Fatalf("CreateListing: %v", err)
	}
	purchase, err := marketplace.Purchase(context.Background(), listingID, buyer)
	if err != nil {
		t.Fatalf("Purchase: %v", err)
	}
	dispute, err := marketplace.OpenDispute(context.Background(), purchase.TransactionID, buyer, "never arrived")
	if err != nil {
		t.Fatalf("OpenDispute: %v", err)
	}

	for _, caller := range []string{buyer, seller} {
		req := withUser(httptest.NewRequest("GET", "/v1/marketplace/disputes/"+dispute.ID, nil), &models.User{DID: caller})
		req = mux.SetURLVars(req, map[string]string{"id": dispute.ID})
		rec := httptest.NewRecorder()

		HandleMarketplaceGetDispute(rec, req)

		if rec.Code != 200 {
			t.Fatalf("caller=%s status = %d, want 200 (body: %s)", caller, rec.Code, rec.Body.String())
		}
	}
}

func TestHandleMarketplaceGetDispute_Stranger_Rejected(t *testing.T) {
	seller := "did:obs:mkth-getdisp-strg-seller"
	buyer := "did:obs:mkth-getdisp-strg-buyer"
	stranger := "did:obs:mkth-getdisp-strg-stranger"
	seedMarketplaceUser(t, seller, 5)
	seedMarketplaceUser(t, buyer, 1)
	fundMarketplaceUser(t, buyer, 100)

	listingID, err := marketplace.CreateListing(context.Background(), seller, "Item", "d", obsAmount(5), "misc")
	if err != nil {
		t.Fatalf("CreateListing: %v", err)
	}
	purchase, err := marketplace.Purchase(context.Background(), listingID, buyer)
	if err != nil {
		t.Fatalf("Purchase: %v", err)
	}
	dispute, err := marketplace.OpenDispute(context.Background(), purchase.TransactionID, buyer, "never arrived")
	if err != nil {
		t.Fatalf("OpenDispute: %v", err)
	}

	req := withUser(httptest.NewRequest("GET", "/v1/marketplace/disputes/"+dispute.ID, nil), &models.User{DID: stranger})
	req = mux.SetURLVars(req, map[string]string{"id": dispute.ID})
	rec := httptest.NewRecorder()

	HandleMarketplaceGetDispute(rec, req)

	if rec.Code != 403 {
		t.Fatalf("status = %d, want 403 (body: %s)", rec.Code, rec.Body.String())
	}
}
