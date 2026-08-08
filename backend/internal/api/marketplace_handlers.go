package api

// Marketplace API — HTTP surface over internal/marketplace (spec Bölüm 5.2
// Katman 3). See internal/marketplace/marketplace.go.
//
// All routes require authentication (registered under the `priv` subrouter).
// Creating a listing is additionally gated on seller access level inside the
// marketplace package (returns marketplace.ErrAccessDenied → 403 here).
//
//   POST   /v1/marketplace/listings              → create a listing (seller access level required)
//   GET    /v1/marketplace/listings               → list listings (?status=&limit=&offset=)
//   GET    /v1/marketplace/listings/{id}          → listing detail
//   PATCH  /v1/marketplace/listings/{id}          → update a listing (seller only)
//   DELETE /v1/marketplace/listings/{id}          → soft-remove a listing (seller only)
//   POST   /v1/marketplace/listings/{id}/purchase → purchase a listing
//   POST   /v1/marketplace/listings/{id}/report   → report a listing (#36, moderation pipeline)

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/marketplace"
	"obscura.network/core/internal/moderation"
)

// marketplaceErrCode maps a marketplace sentinel error to an HTTP status
// code — same pattern as airdropErrCode in airdrop_handlers.go.
func marketplaceErrCode(err error) int {
	switch {
	case errors.Is(err, marketplace.ErrNotFound):
		return 404
	case errors.Is(err, marketplace.ErrAccessDenied),
		errors.Is(err, marketplace.ErrNotSeller):
		return 403
	case errors.Is(err, marketplace.ErrListingClosed),
		errors.Is(err, marketplace.ErrInvalidInput),
		errors.Is(err, marketplace.ErrSelfPurchase):
		return 400
	default:
		return 500
	}
}

// POST /v1/marketplace/listings
// Body: { title, description, price, category }
// Seller access level (>=3, spec Bölüm 5.2 Katman 3) enforced inside
// marketplace.CreateListing.
func HandleMarketplaceCreateListing(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Price       string `json:"price"`
		Category    string `json:"category"`
	}
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz JSON")
		return
	}

	id, err := marketplace.CreateListing(r.Context(), user.DID, req.Title, req.Description, req.Price, req.Category)
	if err != nil {
		respond(w, marketplaceErrCode(err), nil, err.Error())
		return
	}

	listing, err := marketplace.GetListing(id)
	if err != nil {
		// Listing was created; read-back failed — still report success.
		respond(w, 200, map[string]any{"listing_id": id}, "")
		return
	}
	respond(w, 200, map[string]any{"listing_id": id, "listing": listing}, "")
}

// GET /v1/marketplace/listings?status=&limit=&offset= — not tier-gated
// (spec 5.2 Katman 1 note: base-tier users already see marketplace listings).
func HandleMarketplaceListListings(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	status := r.URL.Query().Get("status")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	listings, err := marketplace.ListListings(status, limit, offset)
	if err != nil {
		respond(w, 500, nil, "İlanlar listelenemedi: "+err.Error())
		return
	}
	respond(w, 200, map[string]any{
		"listings": listings,
		"count":    len(listings),
	}, "")
}

// GET /v1/marketplace/listings/{id}
func HandleMarketplaceGetListing(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	id := mux.Vars(r)["id"]
	if id == "" {
		respond(w, 400, nil, "listing id zorunlu")
		return
	}

	listing, err := marketplace.GetListing(id)
	if err != nil {
		respond(w, marketplaceErrCode(err), nil, err.Error())
		return
	}
	respond(w, 200, listing, "")
}

// PATCH /v1/marketplace/listings/{id}
// Body: { title?, description?, price?, category?, status? } — any subset,
// only non-null fields are applied. Seller only (marketplace.ErrNotSeller →
// 403 otherwise).
func HandleMarketplaceUpdateListing(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	id := mux.Vars(r)["id"]
	if id == "" {
		respond(w, 400, nil, "listing id zorunlu")
		return
	}

	var req struct {
		Title       *string `json:"title"`
		Description *string `json:"description"`
		Price       *string `json:"price"`
		Category    *string `json:"category"`
		Status      *string `json:"status"`
	}
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz JSON")
		return
	}

	patch := marketplace.ListingPatch{
		Title:       req.Title,
		Description: req.Description,
		Price:       req.Price,
		Category:    req.Category,
		Status:      req.Status,
	}
	if err := marketplace.UpdateListing(r.Context(), id, user.DID, patch); err != nil {
		respond(w, marketplaceErrCode(err), nil, err.Error())
		return
	}

	listing, err := marketplace.GetListing(id)
	if err != nil {
		respond(w, 200, map[string]any{"listing_id": id}, "")
		return
	}
	respond(w, 200, listing, "")
}

// DELETE /v1/marketplace/listings/{id} — soft-remove (status='removed' via
// marketplace.UpdateListing, same ownership gate as PATCH). Seller only.
func HandleMarketplaceDeleteListing(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	id := mux.Vars(r)["id"]
	if id == "" {
		respond(w, 400, nil, "listing id zorunlu")
		return
	}

	removed := marketplace.StatusRemoved
	if err := marketplace.UpdateListing(r.Context(), id, user.DID, marketplace.ListingPatch{Status: &removed}); err != nil {
		respond(w, marketplaceErrCode(err), nil, err.Error())
		return
	}
	respond(w, 200, map[string]any{"listing_id": id, "status": marketplace.StatusRemoved}, "")
}

// POST /v1/marketplace/listings/{id}/purchase — not tier-gated. Rejects
// self-purchase and already-closed listings inside marketplace.Purchase.
func HandleMarketplacePurchase(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	id := mux.Vars(r)["id"]
	if id == "" {
		respond(w, 400, nil, "listing id zorunlu")
		return
	}

	result, err := marketplace.Purchase(r.Context(), id, user.DID)
	if err != nil {
		respond(w, marketplaceErrCode(err), nil, err.Error())
		return
	}
	respond(w, 200, result, "")
}

// POST /v1/marketplace/listings/{id}/report — #36: extends the existing
// moderation pipeline (spam_reports/review_queue, handlers.go:1031
// HandleSpamReport ist the message-report sibling) to marketplace listings.
// Body: { reason, category }.
//
// Unlike HandleSpamReport, no cryptographic evidence step: a listing is
// public marketplace data (not an E2E-encrypted message), so there is no
// ciphertext hash to verify against — the listing_id itself IS the
// admin-visible evidence (admin opens it directly via GetListing).
// EvidenceVerified is set true to reflect that ("nothing to forge", not
// "cryptographically confirmed"). No auto-processing path either: there is
// no content-scoring heuristic for a listing (moderation.Score expects
// message ciphertext) — every listing report goes to human review (İlke 5),
// same as HandleSpamReport falls back to when its own auto-process branch
// doesn't apply.
//
// admin_handlers.go's confirm_remove path (adminRemoveTargetContent →
// removeContentByType "listing") already reads spam_reports.listing_id —
// that code has existed since migration 154 with no writer (see its comment
// at admin_handlers.go:262-264). This handler is that writer.
func HandleReportListing(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	listingID := mux.Vars(r)["id"]
	if listingID == "" {
		respond(w, 400, nil, "listing id zorunlu")
		return
	}

	var req struct {
		Reason   string `json:"reason"`
		Category string `json:"category"`
	}
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz istek")
		return
	}
	if !moderation.IsKnownCategory(req.Category) {
		respond(w, 400, nil, "Geçersiz kategori")
		return
	}

	listing, err := marketplace.GetListing(listingID)
	if err != nil {
		respond(w, marketplaceErrCode(err), nil, err.Error())
		return
	}
	if listing.SellerDID == user.DID {
		respond(w, 400, nil, "Kendi ilanınızı raporlayamazsınız")
		return
	}

	reportID := uuid.New().String()
	if err := moderation.Report(r.Context(), db.DB, moderation.ReportInput{
		ID:               reportID,
		ListingID:        listingID,
		ReporterDID:      user.DID,
		ReportedDID:      listing.SellerDID,
		Reason:           req.Reason,
		Category:         req.Category,
		EvidenceVerified: true,
	}); err != nil {
		respond(w, 500, nil, "Rapor kaydedilemedi")
		return
	}

	// Brigading: aynı satıcıya kısa sürede toplu şikayet → otomatik ceza yok,
	// insan incelemesine düşer (Bölüm 4) — HandleSpamReport ile aynı desen.
	brigading, _ := moderation.IsBrigading(r.Context(), db.DB, listing.SellerDID)
	if brigading {
		_ = moderation.EnqueueReview(r.Context(), db.DB, reportID, "brigading")
		respond(w, 200, map[string]string{"status": "queued_for_review", "report_id": reportID}, "")
		return
	}

	_ = moderation.EnqueueReview(r.Context(), db.DB, reportID, "insan incelemesi bekliyor")
	respond(w, 200, map[string]string{"status": "reported", "report_id": reportID}, "")
}
