package api

// OBS wallet API — the HTTP surface over the off-chain token ledger.
//
// See internal/token/token.go and docs/adr/0010-obs-token-economics.md.
// All routes require authentication (registered under the `priv` subrouter).
// The supply endpoint is "public-ish" but kept behind auth deliberately.
//
//   GET  /v1/wallet/balance        → caller's transparent balance
//   POST /v1/wallet/transfer       → execute a transfer
//   GET  /v1/wallet/transactions   → caller's transaction history
//   GET  /v1/wallet/supply         → total / circulating / burned

import (
	"errors"
	"net/http"
	"strconv"

	"obscura.network/core/internal/token"
)

// GET /v1/wallet/balance
func HandleWalletBalance(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	bal, err := token.Balance(user.DID)
	if err != nil {
		respond(w, 500, nil, "Bakiye okunamadı: "+err.Error())
		return
	}

	respond(w, 200, map[string]any{
		"transparent_balance": bal.String(),
		"currency":            "OBS",
		"decimals":            token.Decimals,
	}, "")
}

// POST /v1/wallet/transfer
// Body: { to_did, amount, memo }
func HandleWalletTransfer(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	var req struct {
		ToDID  string `json:"to_did"`
		Amount string `json:"amount"`
		Memo   string `json:"memo"`
	}
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz JSON")
		return
	}
	if req.ToDID == "" {
		respond(w, 400, nil, "to_did zorunlu")
		return
	}
	if req.ToDID == user.DID {
		respond(w, 400, nil, "Kendinize transfer yapamazsınız")
		return
	}
	if len(req.Memo) > 256 {
		respond(w, 400, nil, "memo en fazla 256 karakter olabilir")
		return
	}

	// Validate amount: positive base-10 decimal string that fits in 256 bits.
	amount, err := token.ParseAmount(req.Amount)
	if err != nil {
		respond(w, 400, nil, "Geçersiz tutar: "+err.Error())
		return
	}

	// Transfer is atomic; sufficient-balance is enforced inside the DB tx.
	txID, err := token.Transfer(r.Context(), user.DID, req.ToDID, amount, req.Memo)
	if err != nil {
		if errors.Is(err, token.ErrInsufficientBalance) {
			respond(w, 400, nil, "Yetersiz bakiye")
			return
		}
		if errors.Is(err, token.ErrInvalidAmount) {
			respond(w, 400, nil, "Geçersiz tutar: "+err.Error())
			return
		}
		respond(w, 500, nil, "Transfer başarısız: "+err.Error())
		return
	}

	respond(w, 200, map[string]any{
		"tx_id":    txID,
		"from_did": user.DID,
		"to_did":   req.ToDID,
		"amount":   amount.String(),
		"fee":      token.TransferFee().String(),
		"currency": "OBS",
	}, "")
}

// GET /v1/wallet/transactions?limit=50
func HandleWalletTransactions(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	limit := 50
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 {
			limit = n
		}
	}

	txs, err := token.History(user.DID, limit)
	if err != nil {
		respond(w, 500, nil, "Geçmiş okunamadı: "+err.Error())
		return
	}

	respond(w, 200, map[string]any{
		"transactions": txs,
		"count":        len(txs),
	}, "")
}

// GET /v1/wallet/supply
func HandleWalletSupply(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	info, err := token.Supply()
	if err != nil {
		respond(w, 500, nil, "Arz bilgisi okunamadı: "+err.Error())
		return
	}

	respond(w, 200, map[string]any{
		"total":       info.Total,
		"circulating": info.Circulating,
		"burned":      info.Burned,
		"currency":    "OBS",
		"decimals":    token.Decimals,
	}, "")
}
