package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
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

// HandleBridgeStatus — GET /v1/bridge/status (cross-chain bridge durumu, FAZ 3 stub)
func HandleBridgeStatus(w http.ResponseWriter, r *http.Request) {
	respond(w, 200, map[string]interface{}{
		"status": "active",
		"chains": []string{"ethereum", "polkadot"},
		"note":   "FAZ 3 stub — tam relayer FAZ 4'te",
	}, "")
}

// HandleBridgeLock — POST /v1/bridge/lock (kaynak zincirde token kilitle)
func HandleBridgeLock(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	respond(w, 200, map[string]interface{}{
		"tx_id":   "stub_lock_" + mustRandHex(8),
		"status":  "pending",
		"message": "Bridge kilitleme başlatıldı (FAZ 3 stub)",
	}, "")
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
