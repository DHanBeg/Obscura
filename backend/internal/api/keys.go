package api

// PreKey bundle yönetimi — X3DH anahtar değişimi için
//
// POST /v1/keys/upload       → Bundle + OPK'ları yükle
// GET  /v1/keys/{did}        → Birinin bundle'ını al (+ 1 OPK tüket)
// POST /v1/keys/opk/replenish → Eksilen OPK'ları tamamla
// POST /v1/zk/verify         → ZK kanıtı doğrula ve kaydet

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log"
	"math/big"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/models"
	"obscura.network/core/internal/zk"
)

// ─── REQUEST / RESPONSE TYPES ─────────────────────────────────────────────────

// PreKeyUploadRequest — POST /v1/keys/upload
type PreKeyUploadRequest struct {
	// X25519 kimlik açık anahtarı (Base64)
	IdentityKey string `json:"identity_key"`
	// İmzalama açık anahtarı (Base64) — SPK imzasını doğrulamak için.
	// Ed25519 (32 byte, mobil) veya ECDSA P-256 uncompressed (65 byte, web) kabul edilir.
	SigningKey string `json:"signing_key"`
	// İmzalı PreKey açık anahtarı (Base64)
	SignedPrekey string `json:"signed_prekey"`
	// SPK'nın Ed25519 imzası (Base64) — signing_key'in özel anahtarı ile
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
	SigningKey      string  `json:"signing_key"`       // Base64 — SPK imzasını doğrulamak için
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

	// SPK imza doğrulaması — signing_key varsa zorunlu.
	// İki algoritma desteklenir (anahtar uzunluğuna göre ayrılır):
	//   - Ed25519 (mobil): raw public key = 32 byte, signature = 64 byte.
	//   - ECDSA P-256 (web / WebCrypto): uncompressed public key = 65 byte
	//     (04 || x || y), signature = 64 byte IEEE P1363 (r || s, 32'şer byte).
	if req.SigningKey != "" {
		skBytes, err := base64.StdEncoding.DecodeString(req.SigningKey)
		if err != nil {
			respond(w, 400, nil, "Geçersiz signing_key (Base64; 32 byte Ed25519 veya 65 byte P-256 uncompressed public key olmalı)")
			return
		}
		switch len(skBytes) {
		case 32:
			// Ed25519 doğrulama
			if !ed25519.Verify(ed25519.PublicKey(skBytes), spkBytes, sigBytes) {
				respond(w, 400, nil, "signed_prekey_sig doğrulaması başarısız — imza geçersiz")
				return
			}
		case 65:
			// ECDSA P-256 doğrulama
			x, y := elliptic.Unmarshal(elliptic.P256(), skBytes)
			if x == nil {
				respond(w, 400, nil, "signing_key geçerli bir P-256 noktası değil")
				return
			}
			pub := &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}
			if len(sigBytes) != 64 {
				respond(w, 400, nil, "signed_prekey_sig 64 byte IEEE P1363 formatında olmalı")
				return
			}
			hash := sha256.Sum256(spkBytes)
			r := new(big.Int).SetBytes(sigBytes[:32])
			s := new(big.Int).SetBytes(sigBytes[32:])
			if !ecdsa.Verify(pub, hash[:], r, s) {
				respond(w, 400, nil, "signed_prekey_sig doğrulaması başarısız — imza geçersiz")
				return
			}
		default:
			respond(w, 400, nil, "Geçersiz signing_key (Base64; 32 byte Ed25519 veya 65 byte P-256 uncompressed public key olmalı)")
			return
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Bundle upsert (signing_key sütunu migration 079 ile eklendi)
	_, err = db.DB.Exec(`
		INSERT INTO prekey_bundles (did, identity_key, signing_key, signed_prekey, signed_prekey_sig, signed_prekey_id, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(did) DO UPDATE SET
			identity_key = excluded.identity_key,
			signing_key = excluded.signing_key,
			signed_prekey = excluded.signed_prekey,
			signed_prekey_sig = excluded.signed_prekey_sig,
			signed_prekey_id = excluded.signed_prekey_id,
			updated_at = excluded.updated_at
	`, user.DID, req.IdentityKey, req.SigningKey, req.SignedPrekey, req.SignedPrekeySig, req.SignedPrekeyID, now)
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
			INSERT INTO one_time_prekeys (id, did, opk_id, public_key, used, created_at)
			VALUES (?, ?, ?, ?, 0, ?)
			ON CONFLICT DO NOTHING
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
	var ikKey, signingKey, spk, spkSig string
	var spkID int
	err := db.DB.QueryRow(`
		SELECT identity_key, signing_key, signed_prekey, signed_prekey_sig, signed_prekey_id
		FROM prekey_bundles WHERE did = ?
	`, targetDID).Scan(&ikKey, &signingKey, &spk, &spkSig, &spkID)
	if err != nil {
		respond(w, 404, nil, "Bu kullanıcının PreKey bundle'ı bulunamadı")
		return
	}

	resp := PreKeyBundleResponse{
		DID:             targetDID,
		IdentityKey:     ikKey,
		SigningKey:      signingKey,
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
			INSERT INTO one_time_prekeys (id, did, opk_id, public_key, used, created_at)
			VALUES (?, ?, ?, ?, 0, ?)
			ON CONFLICT DO NOTHING
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

// parseZKVerifyProof — ZKVerifyRequest'ten circuit ID + proof bytes + public
// signals çıkarır (snarkjs'in {proof:...,publicSignals:...} kapsayıcı formatı
// ya da düz proof objesi — ikisi de desteklenir). HandleVerifyZKProof
// (kullanıcı, JWT) ve HandleVerifyZKProofInternal (node-to-node, nodeMAC —
// bkz. zk_internal_verify.go) arasında paylaşılır.
func parseZKVerifyProof(req ZKVerifyRequest) (circuit zk.CircuitID, proofBytes json.RawMessage, publicSignals []string, errMsg string) {
	circuit = zk.CircuitID(req.CircuitID)
	if !zk.IsCircuitKnown(circuit) {
		return circuit, nil, nil, "Bilinmeyen circuit: " + req.CircuitID
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(req.ProofJSON), &raw); err != nil {
		return circuit, nil, nil, "proof_json geçersiz JSON"
	}

	publicSignals = req.PublicInputs
	if pj, ok := raw["proof"]; ok {
		// Kapsayıcı format
		proofBytes = pj
		if len(publicSignals) == 0 {
			if ps, ok := raw["publicSignals"]; ok {
				_ = json.Unmarshal(ps, &publicSignals)
			} else if ps, ok := raw["public_signals"]; ok {
				_ = json.Unmarshal(ps, &publicSignals)
			}
		}
	} else {
		// Düz proof
		proofBytes = []byte(req.ProofJSON)
	}
	return circuit, proofBytes, publicSignals, ""
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
	if err := decodeBody(r, &req); err != nil || req.ProofJSON == "" || req.CircuitID == "" {
		respond(w, 400, nil, "proof_json ve circuit_id zorunlu")
		return
	}

	circuit, proofBytes, publicSignals, errMsg := parseZKVerifyProof(req)
	if errMsg != "" {
		respond(w, 400, nil, errMsg)
		return
	}

	// Anomaly detection: rate limit, replay, stale proof (spec Bölüm 19.3)
	pd := zk.ProofData{ProofJSON: req.ProofJSON, PublicInputs: publicSignals}
	if d := zk.GlobalDetector(); d != nil {
		flags, _ := d.RecordProof(db.DB, pd, user.DID, circuit, time.Now().UTC())
		if flags.Flags&zk.FlagReplayAttack != 0 {
			respond(w, 400, nil, "Proof replay saldırısı tespit edildi")
			return
		}
		if flags.Flags&zk.FlagRateLimitExceeded != 0 {
			respond(w, 429, nil, "Proof rate limit aşıldı")
			return
		}
	}

	// GERÇEK Groth16 doğrulama (BN254 pairing check)
	if err := zk.VerifyGroth16(circuit, proofBytes, publicSignals); err != nil {
		respond(w, 400, nil, "Kanıt geçersiz: "+err.Error())
		return
	}

	// Multi-node verification (spec Bölüm 19.3 — en az 2 node)
	// Best-effort: NODE_PEERS tanımlıysa peer'lardan onay iste.
	// Tek node ortamında (dev) atla.
	if os.Getenv("NODE_PEERS") != "" {
		ctx := r.Context()
		ok, _, err := zk.MultiVerify(ctx, pd, circuit, 1)
		if err != nil {
			log.Printf("⚠️ MultiVerify hatası (non-fatal): %v", err)
		} else if !ok {
			respond(w, 400, nil, "Peer node'lar kanıtı reddetti")
			return
		}
	}

	// Doğrulanan proof'u kaydet
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
		models.ToJSON(publicSignals),
		now.Format(time.RFC3339),
		now.Add(24*time.Hour).Format(time.RFC3339),
	)
	if err != nil {
		respond(w, 500, nil, "Kanıt kaydedilemedi: "+err.Error())
		return
	}

	// Immutable proof audit log'una ekle (spec Bölüm 7.3 Adım 5).
	// Non-fatal: log yazma hatası proof doğrulama yanıtını engellemez.
	if _, logErr := zk.AppendProofLog(db.DB, req.CircuitID, user.DID, req.ProofJSON, publicSignals); logErr != nil {
		log.Printf("proof_log append hatası (non-fatal, did=%s circuit=%s): %v",
			shortDIDStr(user.DID), req.CircuitID, logErr)
	}

	respond(w, 200, map[string]interface{}{
		"proof_id":       id,
		"verified":       true,
		"circuit_id":     req.CircuitID,
		"prover_did":     user.DID,
		"public_signals": publicSignals,
	}, "")
}

