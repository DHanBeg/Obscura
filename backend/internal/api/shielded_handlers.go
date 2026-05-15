package api

// Shielded transfer HTTP surface — spec Bölüm 8.3 ("Gizli Transfer Akışı (ZK)").
// See internal/token/shielded.go for the design and FAZ 2 limitations.
//
// All routes are registered under the `priv` subrouter (auth required):
//
//   POST /v1/wallet/shield             → burn transparent → insert commitment
//   POST /v1/wallet/shielded-transfer  → ZK proof spends note, creates new leaf
//   POST /v1/wallet/unshield           → ZK proof spends note → mint transparent
//   GET  /v1/wallet/shielded/root      → current Merkle root + leaf count
//   GET  /v1/wallet/shielded/notes     → list opaque commitment leaves
//
// Commitments and proofs are computed entirely client-side; the server only
// verifies and indexes them.

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"obscura.network/core/internal/token"
)

// shieldedErrCode maps token shielded sentinels to HTTP status codes.
func shieldedErrCode(err error) int {
	switch {
	case errors.Is(err, token.ErrShieldedNullifierUsed):
		return 409
	case errors.Is(err, token.ErrShieldedProofInvalid),
		errors.Is(err, token.ErrShieldedRootMismatch),
		errors.Is(err, token.ErrShieldedInvalidInput),
		errors.Is(err, token.ErrInsufficientBalance),
		errors.Is(err, token.ErrInvalidAmount):
		return 400
	default:
		return 500
	}
}

// extractProofObject normalizes either a bare snarkjs proof or a wrapped
// { "proof": {...} } payload to the bare proof JSON string that
// zk.VerifyGroth16 expects. Mirrors HandleAirdropClaim.
func extractProofObject(proofJSON string) string {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(proofJSON), &raw); err == nil {
		if pj, ok := raw["proof"]; ok {
			return string(pj)
		}
	}
	return proofJSON
}

// POST /v1/wallet/shield
// Body: { amount, commitment }
// Burns `amount` from the caller's transparent balance and inserts the
// (caller-supplied) commitment as a new shielded leaf.
func HandleWalletShield(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	var req struct {
		Amount     string `json:"amount"`
		Commitment string `json:"commitment"`
	}
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz JSON")
		return
	}

	amount, err := token.ParseAmount(req.Amount)
	if err != nil {
		respond(w, 400, nil, "Geçersiz tutar: "+err.Error())
		return
	}
	if req.Commitment == "" {
		respond(w, 400, nil, "commitment zorunlu")
		return
	}

	rootAfter, idx, err := token.Shield(r.Context(), user.DID, amount, req.Commitment)
	if err != nil {
		respond(w, shieldedErrCode(err), nil, err.Error())
		return
	}

	respond(w, 200, map[string]any{
		"root_after": rootAfter,
		"leaf_index": idx,
	}, "")
}

// POST /v1/wallet/shielded-transfer
// Body: { proof_json, public_inputs[] }
// Verifies the ZK proof, marks the input note spent, and inserts the new
// recipient commitment leaf.
func HandleWalletShieldedTransfer(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
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

	proofJSON := extractProofObject(req.ProofJSON)
	if err := token.ShieldedTransfer(r.Context(), proofJSON, req.PublicInputs); err != nil {
		respond(w, shieldedErrCode(err), nil, err.Error())
		return
	}

	info, _ := token.GetShieldedRoot()
	respond(w, 200, map[string]any{
		"accepted": true,
		"root":     info,
	}, "")
}

// POST /v1/wallet/unshield
// Body: { proof_json, public_inputs[], recipient_did, amount }
// Verifies the ZK proof, marks the note spent, and mints `amount` transparent
// OBS to recipient_did. recipient_did is supplied by the caller (typically
// themselves) because the circuit hides ownership.
func HandleWalletUnshield(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	var req struct {
		ProofJSON    string   `json:"proof_json"`
		PublicInputs []string `json:"public_inputs"`
		RecipientDID string   `json:"recipient_did"`
		Amount       string   `json:"amount"`
	}
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz JSON")
		return
	}
	if req.ProofJSON == "" {
		respond(w, 400, nil, "proof_json zorunlu")
		return
	}
	if req.RecipientDID == "" {
		respond(w, 400, nil, "recipient_did zorunlu")
		return
	}
	amount, err := token.ParseAmount(req.Amount)
	if err != nil {
		respond(w, 400, nil, "Geçersiz tutar: "+err.Error())
		return
	}

	proofJSON := extractProofObject(req.ProofJSON)
	if err := token.Unshield(r.Context(), req.RecipientDID, proofJSON, req.PublicInputs, amount); err != nil {
		respond(w, shieldedErrCode(err), nil, err.Error())
		return
	}

	respond(w, 200, map[string]any{
		"recipient_did": req.RecipientDID,
		"amount":        amount.String(),
		"currency":      "OBS",
		"minted":        true,
	}, "")
	_ = user // recipient_did need not equal caller's DID
}

// GET /v1/wallet/shielded/root → current root + leaf_count snapshot.
func HandleWalletShieldedRoot(w http.ResponseWriter, r *http.Request) {
	if user := getUser(r); user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	info, err := token.GetShieldedRoot()
	if err != nil {
		respond(w, 500, nil, "Shielded root okunamadı: "+err.Error())
		return
	}
	respond(w, 200, info, "")
}

// GET /v1/wallet/shielded/notes?limit=100 → list opaque commitment leaves.
// Public on purpose: privacy comes from the commitments being opaque
// (Poseidon hashes), not from hiding the leaf list.
func HandleWalletShieldedNotes(w http.ResponseWriter, r *http.Request) {
	if user := getUser(r); user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	limit := 100
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 {
			limit = n
		}
	}
	notes, err := token.ListShieldedNotes(limit)
	if err != nil {
		respond(w, 500, nil, "Notlar listelenemedi: "+err.Error())
		return
	}
	respond(w, 200, map[string]any{
		"notes": notes,
		"count": len(notes),
	}, "")
}
