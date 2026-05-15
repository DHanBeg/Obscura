package api

// Airdrop API — HTTP surface over the ZK-gated, Sybil-resistant airdrop
// distribution layer (spec Bölüm 12.2). See internal/airdrop/airdrop.go.
//
// All routes require authentication (registered under the `priv` subrouter).
// Campaign creation and early-end are additionally gated on admin tier inside
// the airdrop package (returns airdrop.ErrNotAdmin → 403 here).
//
//   POST /v1/airdrop/campaigns            → create a campaign (admin only)
//   GET  /v1/airdrop/campaigns            → list active campaigns
//   GET  /v1/airdrop/campaigns/{id}       → campaign detail + status
//   POST /v1/airdrop/campaigns/{id}/claim → claim with an identity_proof ZK proof
//   POST /v1/airdrop/campaigns/{id}/end   → end a campaign early (admin only)

import (
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"obscura.network/core/internal/airdrop"
)

// airdropErrCode maps an airdrop sentinel error to an HTTP status code.
func airdropErrCode(err error) int {
	switch {
	case errors.Is(err, airdrop.ErrNotFound):
		return 404
	case errors.Is(err, airdrop.ErrNotAdmin):
		return 403
	case errors.Is(err, airdrop.ErrIneligible):
		return 403
	case errors.Is(err, airdrop.ErrAlreadyClaimed):
		return 409
	case errors.Is(err, airdrop.ErrCampaignClosed),
		errors.Is(err, airdrop.ErrDeadlinePassed),
		errors.Is(err, airdrop.ErrPoolExhausted),
		errors.Is(err, airdrop.ErrInvalidProof),
		errors.Is(err, airdrop.ErrInvalidInput):
		return 400
	default:
		return 500
	}
}

// POST /v1/airdrop/campaigns
// Body: { name, total_pool, per_claim, min_tier, min_account_age_days, max_claims, ends_at }
// Admin only (enforced inside airdrop.CreateCampaign via tier check).
func HandleAirdropCreateCampaign(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	var req struct {
		Name              string `json:"name"`
		TotalPool         string `json:"total_pool"`
		PerClaim          string `json:"per_claim"`
		MinTier           int    `json:"min_tier"`
		MinAccountAgeDays int    `json:"min_account_age_days"`
		MaxClaims         int    `json:"max_claims"`
		EndsAt            string `json:"ends_at"` // RFC3339
	}
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz JSON")
		return
	}
	if req.Name == "" {
		respond(w, 400, nil, "name zorunlu")
		return
	}
	if len(req.Name) > 128 {
		respond(w, 400, nil, "name en fazla 128 karakter olabilir")
		return
	}

	totalPool, ok := new(big.Int).SetString(req.TotalPool, 10)
	if !ok || totalPool.Sign() <= 0 {
		respond(w, 400, nil, "total_pool geçersiz (pozitif decimal string)")
		return
	}
	perClaim, ok := new(big.Int).SetString(req.PerClaim, 10)
	if !ok || perClaim.Sign() <= 0 {
		respond(w, 400, nil, "per_claim geçersiz (pozitif decimal string)")
		return
	}
	endsAt, err := time.Parse(time.RFC3339, req.EndsAt)
	if err != nil {
		respond(w, 400, nil, "ends_at geçersiz (RFC3339 olmalı)")
		return
	}

	id, err := airdrop.CreateCampaign(r.Context(), user.DID, req.Name,
		totalPool, perClaim, req.MinTier, req.MinAccountAgeDays, req.MaxClaims, endsAt)
	if err != nil {
		respond(w, airdropErrCode(err), nil, err.Error())
		return
	}

	info, err := airdrop.CampaignStatus(id)
	if err != nil {
		// Campaign was created; status read-back failed — still report success.
		respond(w, 200, map[string]any{"campaign_id": id}, "")
		return
	}
	respond(w, 200, map[string]any{"campaign_id": id, "campaign": info}, "")
}

// GET /v1/airdrop/campaigns — list active campaigns.
func HandleAirdropListCampaigns(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	campaigns, err := airdrop.ListActiveCampaigns()
	if err != nil {
		respond(w, 500, nil, "Kampanyalar listelenemedi: "+err.Error())
		return
	}
	respond(w, 200, map[string]any{
		"campaigns": campaigns,
		"count":     len(campaigns),
	}, "")
}

// GET /v1/airdrop/campaigns/{id} — campaign detail + derived status.
func HandleAirdropGetCampaign(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	id := mux.Vars(r)["id"]
	if id == "" {
		respond(w, 400, nil, "campaign id zorunlu")
		return
	}

	info, err := airdrop.CampaignStatus(id)
	if err != nil {
		respond(w, airdropErrCode(err), nil, err.Error())
		return
	}
	respond(w, 200, info, "")
}

// POST /v1/airdrop/campaigns/{id}/claim
// Body: { proof_json, public_inputs[] }
// Claims the per-campaign reward. Sybil resistance: the identity_proof ZK proof
// attests DID ownership, and a per-(campaign, identity) nullifier blocks a
// second claim. The reward is minted to the caller's transparent balance.
func HandleAirdropClaim(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	id := mux.Vars(r)["id"]
	if id == "" {
		respond(w, 400, nil, "campaign id zorunlu")
		return
	}

	var req struct {
		ProofJSON    string   `json:"proof_json"`
		PublicInputs []string `json:"public_inputs"`
	}
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz JSON")
		return
	}
	if req.ProofJSON == "" {
		respond(w, 400, nil, "proof_json zorunlu")
		return
	}

	// The proof_json may be either a bare snarkjs proof object or a wrapper
	// { "proof": {...} } — normalize to the bare proof object that
	// zk.VerifyGroth16 expects (mirrors HandleCreditUpgrade).
	proofJSON := req.ProofJSON
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(req.ProofJSON), &raw); err == nil {
		if pj, ok := raw["proof"]; ok {
			proofJSON = string(pj)
		}
	}

	amount, err := airdrop.Claim(r.Context(), id, user.DID, proofJSON, req.PublicInputs)
	if err != nil {
		respond(w, airdropErrCode(err), nil, err.Error())
		return
	}

	respond(w, 200, map[string]any{
		"campaign_id": id,
		"claimer":     user.DID,
		"amount":      amount.String(),
		"currency":    "OBS",
		"minted":      true,
	}, "")
}

// POST /v1/airdrop/campaigns/{id}/end — admin ends a campaign early.
func HandleAirdropEndCampaign(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	id := mux.Vars(r)["id"]
	if id == "" {
		respond(w, 400, nil, "campaign id zorunlu")
		return
	}

	if err := airdrop.EndCampaign(r.Context(), id, user.DID); err != nil {
		respond(w, airdropErrCode(err), nil, err.Error())
		return
	}
	respond(w, 200, map[string]any{"campaign_id": id, "status": airdrop.StatusEnded}, "")
}
