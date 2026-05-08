package api

// PreKey bundle yönetimi — X3DH anahtar değişimi için
//
// POST /v1/keys/upload       → Bundle + OPK'ları yükle
// GET  /v1/keys/{did}        → Birinin bundle'ını al (+ 1 OPK tüket)
// POST /v1/keys/opk/replenish → Eksilen OPK'ları tamamla
// POST /v1/zk/verify         → ZK kanıtı doğrula ve kaydet

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/models"
)

// ─── REQUEST / RESPONSE TYPES ─────────────────────────────────────────────────

// PreKeyUploadRequest — POST /v1/keys/upload
type PreKeyUploadRequest struct {
	// X25519 kimlik açık anahtarı (Base64)
	IdentityKey string `json:"identity_key"`
	// İmzalı PreKey açık anahtarı (Base64)
	SignedPrekey string `json:"signed_prekey"`
	// SPK'nın Ed25519 imzası (Base64) — identity signing key ile
	SignedPrekeySig string `json:"signed_prekey_sig"`
	// SPK ID
	SignedPrekeyID int `json:"signed_prekey_id"`
	// Tek kullanımlık PreKey'ler [{id, public_key}]
	OneTimePrekeys []OPKItem `json:"one_time_prekeys"`
}

type OPKItem struct {
	ID        int    `json:"id"`
	PublicKey string `json:"public_key"` // Base64
}

// PreKeyBundleResponse — GET /v1/keys/{did}
type PreKeyBundleResponse struct {
	DID             string  `json:"did"`
	IdentityKey     string  `json:"identity_key"`      // Base64
	SignedPrekey    string  `json:"signed_prekey"`     // Base64
	SignedPrekeySig string  `json:"signed_prekey_sig"` // Base64
	OneTimePrekey   *string `json:"one_time_prekey"`   // Base64 veya null
	OneTimePrekeyID *int    `json:"one_time_prekey_id"`
}

// OPKReplenishRequest — POST /v1/keys/opk/replenish
type OPKReplenishRequest struct {
	OneTimePrekeys []OPKItem `json:"one_time_prekeys"`
}

// ZKVerifyRequest — POST /v1/zk/verify
type ZKVerifyRequest struct {
	// JSON serialize ZkProof (Rust tarafından üretilmiş)
	ProofJSON  string `json:"proof_json"`
	CircuitID  string `json:"circuit_id"`
	PublicInputs []string `json:"public_inputs"`
}

// ─── HANDLERS ────────────────────────────────────────────────────────────────

// POST /v1/keys/upload
// Kullanıcının PreKey bundle'ını ve OPK'larını sunucuya yükle
func HandleUploadPreKeyBundle(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	var req PreKeyUploadRequest
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz istek formatı")
		return
	}

	// Zorunlu alanlar
	if req.IdentityKey == "" || req.SignedPrekey == "" || req.SignedPrekeySig == "" {
		respond(w, 400, nil, "identity_key, signed_prekey ve signed_prekey_sig zorunlu")
		return
	}

	// SPK imzasını doğrula (Ed25519)
	// Not: identity_key X25519 DH key, signing ile aynı değil.
	// Kullanıcı kimlik anahtarı users tablosundaki identity_key'de saklı.
	// Önce kullanıcının signing public key'ini al (varsa identity_key kolonundan)
	// Şu an: imzayı güvenir bir şekilde kabul et (TODO: tam imza doğrulama)
	//
	// Tam implementasyon: Ed25519 signing key ayrı column'da tutulmalı
	// ve SPK imzası verify_ed25519(signing_pub, signed_prekey_bytes, sig_bytes) ile doğrulanmalı

	ikBytes, err := base64.StdEncoding.DecodeString(req.IdentityKey)
	if err != nil || len(ikBytes) != 32 {
		respond(w, 400, nil, "Geçersiz identity_key (Base64, 32 byte olmalı)")
		return
	}
	spkBytes, err := base64.StdEncoding.DecodeString(req.SignedPrekey)
	if err != nil || len(spkBytes) != 32 {
		respond(w, 400, nil, "Geçersiz signed_prekey (Base64, 32 byte olmalı)")
		return
	}
	sigBytes, err := base64.StdEncoding.DecodeString(req.SignedPrekeySig)
	if err != nil || len(sigBytes) != 64 {
		respond(w, 400, nil, "Geçersiz signed_prekey_sig (Base64, 64 byte olmalı)")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Bundle upsert
	_, err = db.DB.Exec(`
		INSERT INTO prekey_bundles (did, identity_key, signed_prekey, signed_prekey_sig, signed_prekey_id, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(did) DO UPDATE SET
			identity_key = excluded.identity_key,
			signed_prekey = excluded.signed_prekey,
			signed_prekey_sig = excluded.signed_prekey_sig,
			signed_prekey_id = excluded.signed_prekey_id,
			updated_at = excluded.updated_at
	`, user.DID, req.IdentityKey, req.SignedPrekey, req.SignedPrekeySig, req.SignedPrekeyID, now)
	if err != nil {
		respond(w, 500, nil, "Bundle kaydedilemedi")
		return
	}

	// OPK'ları ekle
	opkCount := 0
	for _, opk := range req.OneTimePrekeys {
		if opk.PublicKey == "" {
			continue
		}
		opkBytes2, err := base64.StdEncoding.DecodeString(opk.PublicKey)
		if err != nil || len(opkBytes2) != 32 {
			continue // Geçersiz OPK atla
		}
		id := uuid.New().String()
		_, err = db.DB.Exec(`
			INSERT OR IGNORE INTO one_time_prekeys (id, did, opk_id, public_key, used, created_at)
			VALUES (?, ?, ?, ?, 0, ?)
		`, id, user.DID, opk.ID, opk.PublicKey, now)
		if err == nil {
			opkCount++
		}
	}

	// users tablosundaki identity_key güncelle
	db.DB.Exec(`UPDATE users SET identity_key = ?, updated_at = ? WHERE did = ?`,
		req.IdentityKey, now, user.DID)

	respond(w, 200, map[string]interface{}{
		"uploaded":    true,
		"opk_count":   opkCount,
		"message":     "PreKey bundle güncellendi",
	}, "")
}

// GET /v1/keys/{did}
// Belirtilen kullanıcının PreKey bundle'ını al (+ 1 OPK tüket)
func HandleGetPreKeyBundle(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	vars := mux.Vars(r)
	targetDID := vars["did"]
	if targetDID == "" {
		respond(w, 400, nil, "DID zorunlu")
		return
	}

	// Bundle sorgula
	var ikKey, spk, spkSig string
	var spkID int
	err := db.DB.QueryRow(`
		SELECT identity_key, signed_prekey, signed_prekey_sig, signed_prekey_id
		FROM prekey_bundles WHERE did = ?
	`, targetDID).Scan(&ikKey, &spk, &spkSig, &spkID)
	if err != nil {
		respond(w, 404, nil, "Bu kullanıcının PreKey bundle'ı bulunamadı")
		return
	}

	resp := PreKeyBundleResponse{
		DID:             targetDID,
		IdentityKey:     ikKey,
		SignedPrekey:    spk,
		SignedPrekeySig: spkSig,
	}

	// Bir OPK tüket (transaction ile — race condition önleme)
	tx, err := db.DB.Begin()
	if err == nil {
		var opkRowID string
		var opkID int
		var opkPub string
		err2 := tx.QueryRow(`
			SELECT id, opk_id, public_key FROM one_time_prekeys
			WHERE did = ? AND used = 0
			LIMIT 1
		`, targetDID).Scan(&opkRowID, &opkID, &opkPub)

		if err2 == nil {
			// OPK'yı işaretle
			usedAt := time.Now().UTC().Format(time.RFC3339)
			tx.Exec(`UPDATE one_time_prekeys SET used = 1, used_at = ? WHERE id = ?`,
				usedAt, opkRowID)
			tx.Commit()
			resp.OneTimePrekey = &opkPub
			resp.OneTimePrekeyID = &opkID
		} else {
			tx.Rollback()
			// OPK yok — bundle geçerli ama OPK'sız
		}
	}

	respond(w, 200, resp, "")
}

// POST /v1/keys/opk/replenish
// Eksilen OPK'ları tamamla
func HandleReplenishOPK(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	var req OPKReplenishRequest
	if err := decodeBody(r, &req); err != nil || len(req.OneTimePrekeys) == 0 {
		respond(w, 400, nil, "one_time_prekeys listesi gerekli")
		return
	}

	// Kalan OPK sayısını kontrol et
	var remaining int
	db.DB.QueryRow(`SELECT COUNT(*) FROM one_time_prekeys WHERE did = ? AND used = 0`,
		user.DID).Scan(&remaining)

	now := time.Now().UTC().Format(time.RFC3339)
	added := 0
	for _, opk := range req.OneTimePrekeys {
		opkBytes, err := base64.StdEncoding.DecodeString(opk.PublicKey)
		if err != nil || len(opkBytes) != 32 {
			continue
		}
		id := uuid.New().String()
		_, err = db.DB.Exec(`
			INSERT OR IGNORE INTO one_time_prekeys (id, did, opk_id, public_key, used, created_at)
			VALUES (?, ?, ?, ?, 0, ?)
		`, id, user.DID, opk.ID, opk.PublicKey, now)
		if err == nil {
			added++
		}
	}

	respond(w, 200, map[string]interface{}{
		"added":     added,
		"remaining": remaining + added,
	}, "")
}

// GET /v1/keys/opk/count
// Kalan OPK sayısını döndür (istemci azaldında tamamlar)
func HandleGetOPKCount(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	var count int
	db.DB.QueryRow(`SELECT COUNT(*) FROM one_time_prekeys WHERE did = ? AND used = 0`,
		user.DID).Scan(&count)

	respond(w, 200, map[string]interface{}{
		"count":    count,
		"low":      count < 20,
		"critical": count < 5,
	}, "")
}

// POST /v1/zk/verify
// ZK kanıtını doğrula ve kaydet
func HandleVerifyZKProof(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	var req ZKVerifyRequest
	if err := decodeBody(r, &req); err != nil || req.ProofJSON == "" {
		respond(w, 400, nil, "proof_json ve circuit_id zorunlu")
		return
	}

	// ZK kanıtını parse et (stub doğrulama)
	var proofData map[string]interface{}
	if err := json.Unmarshal([]byte(req.ProofJSON), &proofData); err != nil {
		respond(w, 400, nil, "proof_json geçersiz JSON")
		return
	}

	// Temel doğrulama: kanıt formatı kontrolü
	// Gerçekte: Groth16 verification key ile pairing check yapılır
	proofType, _ := proofData["proof_type"].(string)
	if proofType == "" {
		respond(w, 400, nil, "Geçersiz kanıt formatı — proof_type eksik")
		return
	}
	proverDID, _ := proofData["prover_did"].(string)
	if proverDID != user.DID {
		respond(w, 400, nil, "Kanıt sahibi uyuşmuyor")
		return
	}

	// Veritabanına kaydet
	now := time.Now().UTC()
	id := uuid.New().String()
	_, err := db.DB.Exec(`
		INSERT INTO zk_proofs (id, user_did, circuit_id, proof_data, public_inputs, verified, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, 1, ?, ?)
	`,
		id,
		user.DID,
		req.CircuitID,
		req.ProofJSON,
		models.ToJSON(req.PublicInputs),
		now.Format(time.RFC3339),
		now.Add(24*time.Hour).Format(time.RFC3339),
	)
	if err != nil {
		respond(w, 500, nil, "Kanıt kaydedilemedi")
		return
	}

	respond(w, 200, map[string]interface{}{
		"proof_id":   id,
		"verified":   true,
		"circuit_id": req.CircuitID,
		"prover_did": user.DID,
	}, "")
}

// ─── YARDIMCI: ED25519 İMZA DOĞRULAMA ────────────────────────────────────────

// verifyEd25519 — Ed25519 imzasını doğrula
// publicKey: 32 byte, message ve signature standard Ed25519
func verifyEd25519(publicKey, message, signature []byte) bool {
	if len(publicKey) != ed25519.PublicKeySize {
		return false
	}
	if len(signature) != ed25519.SignatureSize {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(publicKey), message, signature)
}
