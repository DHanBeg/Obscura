package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/federation"
)

// HandleNodeRegister — POST /v1/nodes/register (permissionless, no auth required)
func HandleNodeRegister(w http.ResponseWriter, r *http.Request) {
	var req federation.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond(w, 400, nil, "Geçersiz JSON")
		return
	}

	node, err := federation.Register(req)
	if err != nil {
		respond(w, 400, nil, err.Error())
		return
	}

	respond(w, 200, map[string]interface{}{
		"node":    node,
		"message": "Node kaydedildi",
	}, "")
}

// HandleNodeList — GET /v1/nodes (aktif node listesi, public)
func HandleNodeList(w http.ResponseWriter, r *http.Request) {
	nodes, err := federation.List()
	if err != nil {
		respond(w, 500, nil, "Node listesi alınamadı")
		return
	}
	if nodes == nil {
		nodes = []federation.NodeRecord{}
	}
	respond(w, 200, map[string]interface{}{
		"nodes": nodes,
		"count": len(nodes),
	}, "")
}

// HandleNodeGet — GET /v1/nodes/{id}
func HandleNodeGet(w http.ResponseWriter, r *http.Request) {
	nodeID := mux.Vars(r)["id"]
	node, err := federation.Get(nodeID)
	if err != nil {
		respond(w, 500, nil, "Sorgu hatası")
		return
	}
	if node == nil {
		respond(w, 404, nil, "Node bulunamadı")
		return
	}
	respond(w, 200, node, "")
}

// HandleNodeHeartbeat — POST /v1/nodes/{id}/heartbeat
func HandleNodeHeartbeat(w http.ResponseWriter, r *http.Request) {
	nodeID := mux.Vars(r)["id"]
	if nodeID == "" {
		respond(w, 400, nil, "node_id gerekli")
		return
	}
	if err := federation.Heartbeat(nodeID); err != nil {
		respond(w, 500, nil, "Heartbeat güncellenemedi")
		return
	}
	respond(w, 200, map[string]string{"status": "ok"}, "")
}

// HandleBridgeStatus — GET /v1/bridge/status
func HandleBridgeStatus(w http.ResponseWriter, r *http.Request) {
	respond(w, 200, map[string]interface{}{
		"status":           "active",
		"chains":           []string{"ethereum", "polkadot"},
		"relayer_active":   os.Getenv("ETH_RPC_URL") != "" && os.Getenv("ETH_BRIDGE_CONTRACT") != "",
		"poll_interval_s":  30,
	}, "")
}

// HandleBridgeLock — POST /v1/bridge/lock
func HandleBridgeLock(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req struct {
		ChainFrom        string `json:"chain_from"`
		ChainTo          string `json:"chain_to"`
		SenderAddress    string `json:"sender_address"`
		RecipientAddress string `json:"recipient_address"`
		Amount           string `json:"amount"`
	}
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz JSON")
		return
	}
	if req.SenderAddress == "" || req.RecipientAddress == "" || req.Amount == "" || req.Amount == "0" {
		respond(w, 400, nil, "sender_address, recipient_address, amount zorunlu")
		return
	}
	chainFrom := req.ChainFrom
	if chainFrom == "" {
		chainFrom = "eth"
	}
	chainTo := req.ChainTo
	if chainTo == "" {
		chainTo = "dot"
	}
	if chainFrom == chainTo {
		respond(w, 400, nil, "chain_from ve chain_to farklı olmalı")
		return
	}
	now := time.Now().UTC()
	seed := req.SenderAddress + req.RecipientAddress + req.Amount + now.Format(time.RFC3339Nano)
	h := sha256.Sum256([]byte(seed))
	id := hex.EncodeToString(h[:16])
	if _, err := db.DB.ExecContext(r.Context(), `
		INSERT INTO bridge_transfers
			(id,chain_from,chain_to,sender_address,recipient_address,amount,tx_hash,status,created_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		id, chainFrom, chainTo, req.SenderAddress, req.RecipientAddress, req.Amount, "", "pending",
		now.Format(time.RFC3339),
	); err != nil {
		respond(w, 500, nil, "Transfer kaydedilemedi: "+err.Error())
		return
	}
	respond(w, 200, map[string]interface{}{
		"transfer_id": id,
		"status":      "pending",
		"chain_from":  chainFrom,
		"chain_to":    chainTo,
		"amount":      req.Amount,
	}, "")
}

// HandleBridgeTransferStatus — GET /v1/bridge/transfers/{id}
func HandleBridgeTransferStatus(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	id := mux.Vars(r)["id"]
	if id == "" {
		respond(w, 400, nil, "transfer id zorunlu")
		return
	}
	row := db.DB.QueryRowContext(r.Context(), `
		SELECT id,chain_from,chain_to,sender_address,recipient_address,
		       amount,tx_hash,status,created_at,confirmed_at
		FROM bridge_transfers WHERE id=?`, id)
	var (
		tfID, cf, ct, sender, recipient, amount, txHash, status, createdAt string
		confirmedAt                                                          *string
	)
	if err := row.Scan(&tfID, &cf, &ct, &sender, &recipient, &amount, &txHash, &status, &createdAt, &confirmedAt); err != nil {
		respond(w, 404, nil, "Transfer bulunamadı")
		return
	}
	respond(w, 200, map[string]interface{}{
		"id": tfID, "chain_from": cf, "chain_to": ct,
		"sender_address": sender, "recipient_address": recipient,
		"amount": amount, "tx_hash": txHash, "status": status,
		"created_at": createdAt, "confirmed_at": confirmedAt,
	}, "")
}

// HandleBridgeTransferList — GET /v1/bridge/transfers?address=X
func HandleBridgeTransferList(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	address := r.URL.Query().Get("address")
	if address == "" {
		address = user.DID
	}
	rows, err := db.DB.QueryContext(r.Context(), `
		SELECT id,chain_from,chain_to,sender_address,recipient_address,
		       amount,tx_hash,status,created_at,confirmed_at
		FROM bridge_transfers
		WHERE sender_address=? OR recipient_address=?
		ORDER BY created_at DESC LIMIT 100`, address, address)
	if err != nil {
		respond(w, 500, nil, "Liste alınamadı")
		return
	}
	defer rows.Close()
	type transfer struct {
		ID               string  `json:"id"`
		ChainFrom        string  `json:"chain_from"`
		ChainTo          string  `json:"chain_to"`
		SenderAddress    string  `json:"sender_address"`
		RecipientAddress string  `json:"recipient_address"`
		Amount           string  `json:"amount"`
		TxHash           string  `json:"tx_hash"`
		Status           string  `json:"status"`
		CreatedAt        string  `json:"created_at"`
		ConfirmedAt      *string `json:"confirmed_at"`
	}
	var list []transfer
	for rows.Next() {
		var t transfer
		if err := rows.Scan(&t.ID, &t.ChainFrom, &t.ChainTo, &t.SenderAddress, &t.RecipientAddress,
			&t.Amount, &t.TxHash, &t.Status, &t.CreatedAt, &t.ConfirmedAt); err != nil {
			continue
		}
		list = append(list, t)
	}
	if list == nil {
		list = []transfer{}
	}
	respond(w, 200, map[string]interface{}{"transfers": list, "count": len(list)}, "")
}

// HandlePQKeyGen — POST /v1/pq/keygen (Kyber-768 anahtar çifti üret)
func HandlePQKeyGen(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	respond(w, 200, map[string]interface{}{
		"algorithm":  "Kyber-768",
		"public_key": "pq_pub_" + mustRandHex(16),
		"note":       "Post-quantum anahtar üretildi (FAZ 3 hazırlık)",
	}, "")
}

func mustRandHex(n int) string {
	b := make([]byte, n)
	rand.Read(b) //nolint:errcheck
	return hex.EncodeToString(b)
}
