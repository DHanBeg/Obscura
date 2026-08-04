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

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"obscura.network/core/internal/marketplace"
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
