// Package api — ZK circuit keşif ve proof yönlendirme handler'ları.
//
// GET  /v1/zk/circuits — Mevcut ZK circuit listesi ve artifact path'leri
// POST /v1/zk/prove    — Circuit artifact URL'lerini döndür (proof client'ta üretilir)
//
// Spec Bölüm 4.5 Kural 2: ZK proof HER ZAMAN client tarafında üretilir.
// /v1/zk/prove endpoint'i sunucu tarafında proof üretmez; yalnızca client'ın
// hangi WASM/zkey dosyasını indireceğini söyler.
package api

import (
	"net/http"
	"os"

	"obscura.network/core/internal/db"
	"obscura.network/core/internal/zk"
)

// circuitMeta — bir circuit'in dışa açık metadata'sı.
type circuitMeta struct {
	ID          string `json:"id"`
	Version     string `json:"version"`
	Description string `json:"description"`
	WasmURL     string `json:"wasm_url"`
	ZkeyURL     string `json:"zkey_url"`
	VkeyHash    string `json:"vkey_hash,omitempty"` // SHA-256 hex (gelecek: artifact integrity)
	Loaded      bool   `json:"loaded"`              // Verification key yüklü mü?
	NumPublic   int    `json:"num_public_signals"`
}

// knownCircuits — tüm Obscura circuit'lerinin sabit metadata listesi.
// wasm_url ve zkey_url, frontend/public/zk/ dizini altında dağıtılan artifact'leri
// gösterir; BASE_URL env değişkeni ile public URL'ye çevrilir.
var knownCircuits = []circuitMeta{
	// ─── FAZ 1 ──────────────────────────────────────────────────────────────────
	{
		ID:          "credit_threshold",
		Version:     "1.0",
		Description: "Kredi puanı eşik kanıtı — Poseidon hash, 270 non-linear constraints",
		WasmURL:     "/zk/credit_threshold.wasm",
		ZkeyURL:     "/zk/credit_threshold_final.zkey",
		Loaded:      false,
		NumPublic:   4,
	},
	{
		ID:          "identity_proof",
		Version:     "1.0",
		Description: "DID sahiplik kanıtı — gizli anahtar sıfırdan türetme",
		WasmURL:     "/zk/identity_proof.wasm",
		ZkeyURL:     "/zk/identity_proof_final.zkey",
		Loaded:      false,
		NumPublic:   4,
	},
	{
		ID:          "message_integrity",
		Version:     "1.0",
		Description: "Anonim grup mesajı bütünlük kanıtı",
		WasmURL:     "/zk/message_integrity.wasm",
		ZkeyURL:     "/zk/message_integrity_final.zkey",
		Loaded:      false,
		NumPublic:   5,
	},
	{
		ID:          "storage_proof",
		Version:     "1.0",
		Description: "Dağıtık depolama kanıtı — shard sahibi doğrulama",
		WasmURL:     "/zk/storage_proof.wasm",
		ZkeyURL:     "/zk/storage_proof_final.zkey",
		Loaded:      false,
		NumPublic:   6,
	},
	// ─── FAZ 2 ──────────────────────────────────────────────────────────────────
	{
		ID:          "token_balance",
		Version:     "1.0",
		Description: "OBS token bakiye taahhüt kanıtı — Merkle, 944 constraints",
		WasmURL:     "/zk/token_balance.wasm",
		ZkeyURL:     "/zk/token_balance_final.zkey",
		Loaded:      false,
		NumPublic:   5,
	},
	{
		ID:          "vote_proof",
		Version:     "1.0",
		Description: "Gizli oy kanıtı — governance, 733 constraints",
		WasmURL:     "/zk/vote_proof.wasm",
		ZkeyURL:     "/zk/vote_proof_final.zkey",
		Loaded:      false,
		NumPublic:   5,
	},
	// ─── FAZ 3 ──────────────────────────────────────────────────────────────────
	{
		ID:          "recursive_proof",
		Version:     "1.0",
		Description: "Recursive ZK kanıtı — Poseidon commitment, circuit_id, epoch",
		WasmURL:     "/zk/recursive_proof.wasm",
		ZkeyURL:     "/zk/recursive_proof_final.zkey",
		Loaded:      false,
		NumPublic:   4,
	},
	{
		ID:          "zkml_moderation",
		Version:     "1.0",
		Description: "ZK-ML moderasyon — spam skor bucket, model taahhüt kanıtı",
		WasmURL:     "/zk/zkml_moderation.wasm",
		ZkeyURL:     "/zk/zkml_moderation_final.zkey",
		Loaded:      false,
		NumPublic:   4,
	},
	// ─── FAZ 4 ──────────────────────────────────────────────────────────────────
	{
		ID:          "location_proof",
		Version:     "1.0",
		Description: "GPS + ZK konum kanıtı — grid hashing, 1km doğruluk",
		WasmURL:     "/zk/location_proof.wasm",
		ZkeyURL:     "/zk/location_proof_final.zkey",
		Loaded:      false,
		NumPublic:   7,
	},
	{
		ID:          "age_proof",
		Version:     "1.0",
		Description: "Hesap yaşı kanıtı — kredi puanı bileşeni",
		WasmURL:     "/zk/age_proof.wasm",
		ZkeyURL:     "/zk/age_proof_final.zkey",
		Loaded:      false,
		NumPublic:   4,
	},
	{
		ID:          "activity_proof",
		Version:     "1.0",
		Description: "Aktivite kanıtı — günlük etkinlik eşik doğrulama",
		WasmURL:     "/zk/activity_proof.wasm",
		ZkeyURL:     "/zk/activity_proof_final.zkey",
		Loaded:      false,
		NumPublic:   6,
	},
	{
		ID:          "msg_count_proof",
		Version:     "1.0",
		Description: "Mesaj sayısı kanıtı — kredi puanı bileşeni",
		WasmURL:     "/zk/msg_count_proof.wasm",
		ZkeyURL:     "/zk/msg_count_proof_final.zkey",
		Loaded:      false,
		NumPublic:   4,
	},
	{
		ID:          "node_proof",
		Version:     "1.0",
		Description: "Node çalıştırma kanıtı — uptime + stake doğrulama",
		WasmURL:     "/zk/node_proof.wasm",
		ZkeyURL:     "/zk/node_proof_final.zkey",
		Loaded:      false,
		NumPublic:   6,
	},
	{
		ID:          "endorsement_proof",
		Version:     "1.0",
		Description: "Onay kanıtı — kullanıcı güven endeksi bileşeni",
		WasmURL:     "/zk/endorsement_proof.wasm",
		ZkeyURL:     "/zk/endorsement_proof_final.zkey",
		Loaded:      false,
		NumPublic:   5,
	},
	{
		ID:          "streak_proof",
		Version:     "1.0",
		Description: "Süreklilik kanıtı — ardışık gün aktivite",
		WasmURL:     "/zk/streak_proof.wasm",
		ZkeyURL:     "/zk/streak_proof_final.zkey",
		Loaded:      false,
		NumPublic:   4,
	},
}

// resolveBaseURL — frontend'in statik asset URL'sini döndürür.
// ZK_BASE_URL env ile override edilebilir; yoksa varsayılan olarak boş string
// kullanılır (yani yol relative olarak döner — nginx /zk/ alias'ı üstlenir).
func resolveBaseURL() string {
	if u := os.Getenv("ZK_BASE_URL"); u != "" {
		return u
	}
	return ""
}

// ─── GET /v1/zk/circuits ─────────────────────────────────────────────────────

// HandleZKCircuitList — mevcut ZK circuit'lerin metadata listesini döndürür.
// Public endpoint — auth gerekmez; client hangi circuit'lerin aktif olduğunu
// ve artifact URL'lerini bu endpoint üzerinden keşfeder.
func HandleZKCircuitList(w http.ResponseWriter, r *http.Request) {
	base := resolveBaseURL()

	// Yüklenen circuit'leri verifier paketinden al
	loaded := make(map[string]bool)
	for _, cid := range zk.LoadedCircuits() {
		loaded[string(cid)] = true
	}

	result := make([]circuitMeta, len(knownCircuits))
	for i, c := range knownCircuits {
		c.Loaded = loaded[c.ID]
		c.WasmURL = base + c.WasmURL
		c.ZkeyURL = base + c.ZkeyURL
		result[i] = c
	}

	respond(w, 200, map[string]interface{}{
		"circuits": result,
		"count":    len(result),
		"loaded":   len(loaded),
	}, "")
}

// ─── POST /v1/zk/prove ───────────────────────────────────────────────────────

// ─── GET /v1/zk/audit ────────────────────────────────────────────────────────

// HandleZKAuditLog — zk_proof_log tablosunun SHA-256 hash zincirini baştan
// sona doğrular ve sonucu döndürür.
//
// Yetkilendirme: X-Internal-Secret header (INTERNAL_SECRET env) ile korunur.
// JWT auth yerine internal secret kullanılmasının nedeni: bu endpoint yalnızca
// node operatörü / monitoring altyapısı tarafından çağrılır; dışarıya açık
// değildir.
//
// Public değil — neden: audit sonuçları (hangi satırda bütünlük bozulmuş)
// internal bilgi; dışarıya sızdırılmamalı.
func HandleZKAuditLog(w http.ResponseWriter, r *http.Request) {
	// INTERNAL_SECRET header kontrolü — yalnızca yetkili iç servisler erişebilir.
	secret := r.Header.Get("X-Internal-Secret")
	expected := os.Getenv("INTERNAL_SECRET")
	if expected == "" || secret != expected {
		respond(w, http.StatusForbidden, nil, "forbidden")
		return
	}

	valid, brokenID, err := zk.VerifyLogIntegrity(db.DB)
	if err != nil {
		respond(w, http.StatusInternalServerError, nil, err.Error())
		return
	}

	respond(w, http.StatusOK, map[string]interface{}{
		"valid":     valid,
		"broken_at": brokenID,
	}, "")
}

// ─── POST /v1/zk/prove ───────────────────────────────────────────────────────

// HandleZKProve — client'a, belirtilen circuit için WASM ve zkey artifact
// URL'lerini döndürür. Proof üretimi her zaman client tarafında yapılır
// (spec Bölüm 4.5 Kural 2). Bu endpoint sunucu tarafında proof üretmez.
func HandleZKProve(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	var req struct {
		CircuitID    string                 `json:"circuit_id"`
		PublicInputs map[string]interface{} `json:"public_inputs"`
	}
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz istek gövdesi")
		return
	}

	if req.CircuitID == "" {
		respond(w, 400, nil, "circuit_id zorunlu")
		return
	}

	// Circuit'in tanımlı olup olmadığını kontrol et
	var found *circuitMeta
	for i := range knownCircuits {
		if knownCircuits[i].ID == req.CircuitID {
			found = &knownCircuits[i]
			break
		}
	}
	if found == nil {
		respond(w, 404, nil, "Bilinmeyen circuit: "+req.CircuitID)
		return
	}

	// Verification key yüklü mü? (trusted setup tamamlanmış mı?)
	loaded := zk.IsCircuitKnown(zk.CircuitID(req.CircuitID))

	base := resolveBaseURL()

	respond(w, 200, map[string]interface{}{
		"circuit_id":         req.CircuitID,
		"wasm_url":           base + found.WasmURL,
		"zkey_url":           base + found.ZkeyURL,
		"num_public_signals": found.NumPublic,
		"verify_ready":       loaded,
		// Yönlendirme: proof üretildikten sonra /v1/zk/verify'a gönderin
		"verify_endpoint": "/v1/zk/verify",
		"note":            "Proof üretimi client'ta yapılır. wasm_url ve zkey_url'i indirip snarkjs kullanın.",
	}, "")
}
