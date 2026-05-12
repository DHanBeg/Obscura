package api

// Credit tier upgrade via ZK proof.
//
// Spec Bölüm 5.5, 7.3, 17 EK B: kullanıcı tier yükseltmek için
// kredi puanının threshold'u aştığını ZK proof ile kanıtlar.
// Sunucu gerçek puanı görmez, sadece "puan >= threshold" kanıtını doğrular.
//
// SECURITY (post-audit hardening — see security-auditor report 2026-05-10):
//   - C1 fix: nullifier table prevents replay across requests
//   - C2 fix: server computes expected user_hash from JWT.DID and compares
//     to public input — proof cannot be lifted from one user to another
//   - C4 fix: explicit named indices for public inputs, not magic positions
//   - C6 fix: per-circuit expected public input count enforced
//
// POST /v1/credit/upgrade
// Body: { proof_json, public_inputs[], target_tier }

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/messaging"
	"obscura.network/core/internal/models"
	"obscura.network/core/internal/zk"
)

var _ = sha256.Size // keep sha256 import for future nullifier domain extension

type CreditUpgradeRequest struct {
	ProofJSON    string   `json:"proof_json"`
	PublicInputs []string `json:"public_inputs"`
	TargetTier   int      `json:"target_tier"` // 2-5
}

// credit_threshold.circom public signal layout (locked):
//   [0] = is_above output     (1 = score >= threshold)
//   [1] = threshold           (TierMinScore[target] + 20 offset)
//   [2] = score_commitment    (Poseidon(score, salt))
//   [3] = user_hash           (sha256(did) — server-computed nullifier anchor)
const (
	creditPubIdxIsAbove        = 0
	creditPubIdxThreshold      = 1
	creditPubIdxScoreCommit    = 2
	creditPubIdxUserHash       = 3
	creditPubInputCount        = 4
	creditScoreOffset          = 20 // spec: actualScore + 20 → 0..120 range
)

// loadCreditUserHash returns the per-user credit binding commitment
// (Poseidon(user_did_secret, BINDING_TAG)) stored at registration time.
// Returns ErrNoBinding if user hasn't uploaded one yet.
var ErrNoBinding = fmt.Errorf("kullanıcı henüz credit binding hash'ini yüklemedi")

func loadCreditUserHash(did string) (string, error) {
	var h string
	err := db.DB.QueryRow(`SELECT COALESCE(credit_user_hash, '') FROM users WHERE did = ?`, did).Scan(&h)
	if err != nil {
		return "", err
	}
	if h == "" {
		return "", ErrNoBinding
	}
	return h, nil
}

// POST /v1/credit/binding
// Body: { user_hash: "decimal-string-of-poseidon" }
// One-time upload of Poseidon(user_did_secret, BINDING_TAG) — the value the
// circuit will produce as user_hash for credit_threshold proofs.
// Idempotent: refuses to overwrite an existing binding (security: prevent
// account-takeover by changing binding to attacker's secret).
func HandleCreditBinding(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	var req struct {
		UserHash string `json:"user_hash"`
	}
	if err := decodeBody(r, &req); err != nil || req.UserHash == "" {
		respond(w, 400, nil, "user_hash zorunlu")
		return
	}
	// Validate it's a non-empty decimal string < 2^254 (BN254 modulus check)
	bi, ok := new(big.Int).SetString(req.UserHash, 10)
	if !ok || bi.Sign() < 0 || bi.BitLen() > 254 {
		respond(w, 400, nil, "user_hash geçersiz (decimal, 0 < x < 2^254)")
		return
	}

	var existing string
	err := db.DB.QueryRow(`SELECT COALESCE(credit_user_hash, '') FROM users WHERE did = ?`, user.DID).Scan(&existing)
	if err != nil {
		respond(w, 500, nil, err.Error())
		return
	}
	if existing != "" {
		if existing == req.UserHash {
			respond(w, 200, map[string]any{"already_set": true}, "")
			return
		}
		respond(w, 409, nil, "Binding zaten ayarlanmış ve farklı (değiştirilemez)")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.DB.Exec(
		`UPDATE users SET credit_user_hash = ?, updated_at = ? WHERE did = ?`,
		req.UserHash, now, user.DID,
	); err != nil {
		respond(w, 500, nil, err.Error())
		return
	}
	respond(w, 200, map[string]any{"set": true}, "")
}

func HandleCreditUpgrade(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	var req CreditUpgradeRequest
	if err := decodeBody(r, &req); err != nil || req.ProofJSON == "" {
		respond(w, 400, nil, "proof_json zorunlu")
		return
	}

	if req.TargetTier < 2 || req.TargetTier > 5 {
		respond(w, 400, nil, "target_tier 2..5 aralığında olmalı")
		return
	}
	if req.TargetTier <= user.Tier {
		respond(w, 400, nil, "Hedef tier mevcut tier'dan büyük olmalı")
		return
	}

	// C6: lock public input count
	if len(req.PublicInputs) != creditPubInputCount {
		respond(w, 400, nil, fmt.Sprintf(
			"public_inputs %d adet olmalı, %d alındı",
			creditPubInputCount, len(req.PublicInputs)))
		return
	}

	// is_above must be exactly "1"
	if req.PublicInputs[creditPubIdxIsAbove] != "1" {
		respond(w, 400, nil, "Kanıt çıktısı is_above != 1")
		return
	}

	// threshold: client must claim >= TierMinScore[target] + offset
	requiredThreshold := models.TierMinScore[req.TargetTier]
	threshold, err := strconv.Atoi(req.PublicInputs[creditPubIdxThreshold])
	if err != nil {
		respond(w, 400, nil, "threshold parse edilemedi")
		return
	}
	expectedThreshold := int(requiredThreshold) + creditScoreOffset
	if threshold < expectedThreshold {
		respond(w, 400, nil, fmt.Sprintf(
			"Kanıt threshold'u (%d) hedef tier için yetersiz (gerekli: %d)",
			threshold, expectedThreshold))
		return
	}

	// C2+C3: user_hash must match the binding commitment user uploaded at registration.
	// Circuit constrains user_hash = Poseidon(user_did_secret, BINDING_TAG).
	// Server stored this value in users.credit_user_hash.
	// Therefore proof can only be generated by someone who knows user_did_secret.
	expectedUserHash, err := loadCreditUserHash(user.DID)
	if err == ErrNoBinding {
		respond(w, 412, nil, "Önce POST /v1/credit/binding ile credit binding commitment yükleyin")
		return
	}
	if err != nil {
		respond(w, 500, nil, "Binding lookup hatası: "+err.Error())
		return
	}
	if req.PublicInputs[creditPubIdxUserHash] != expectedUserHash {
		respond(w, 403, nil, "user_hash sizin binding commitment'inizle eşleşmiyor")
		return
	}

	// C1: nullifier replay check.
	// Nullifier = hash(circuit_id || score_commitment || user_hash || target_tier)
	// — same proof can't be replayed for the same target tier.
	nullifierIn := fmt.Sprintf("%s|%s|%s|%d",
		zk.CircuitCreditThreshold,
		req.PublicInputs[creditPubIdxScoreCommit],
		req.PublicInputs[creditPubIdxUserHash],
		req.TargetTier)
	nullifierBytes := sha256.Sum256([]byte(nullifierIn))
	nullifier := hex.EncodeToString(nullifierBytes[:])

	var existingUsedAt string
	err = db.DB.QueryRow(
		`SELECT used_at FROM zk_nullifiers WHERE circuit_id = ? AND nullifier = ?`,
		string(zk.CircuitCreditThreshold), nullifier,
	).Scan(&existingUsedAt)
	if err == nil {
		respond(w, 409, nil, "Bu kanıt zaten kullanıldı (nullifier match: "+existingUsedAt+")")
		return
	}

	// Real Groth16 verification — only after all input checks pass
	if !zk.IsCircuitKnown(zk.CircuitCreditThreshold) {
		respond(w, 500, nil, "credit_threshold circuit yüklenmemiş")
		return
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(req.ProofJSON), &raw); err != nil {
		respond(w, 400, nil, "proof_json geçersiz JSON")
		return
	}
	var proofBytes json.RawMessage
	if pj, ok := raw["proof"]; ok {
		proofBytes = pj
	} else {
		proofBytes = []byte(req.ProofJSON)
	}

	if err := zk.VerifyGroth16(zk.CircuitCreditThreshold, proofBytes, req.PublicInputs); err != nil {
		respond(w, 400, nil, "ZK kanıtı doğrulanamadı: "+err.Error())
		return
	}

	// All checks passed — apply atomically in a transaction
	now := time.Now().UTC()
	tx, err := db.DB.Begin()
	if err != nil {
		respond(w, 500, nil, "tx begin: "+err.Error())
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Insert nullifier first — UNIQUE on PK enforces "no replay even under concurrent requests"
	if _, err := tx.Exec(
		`INSERT INTO zk_nullifiers (circuit_id, nullifier, user_did, used_at) VALUES (?, ?, ?, ?)`,
		string(zk.CircuitCreditThreshold), nullifier, user.DID, now.Format(time.RFC3339),
	); err != nil {
		respond(w, 409, nil, "Nullifier zaten var (yarış): "+err.Error())
		return
	}

	proofID := uuid.New().String()
	if _, err := tx.Exec(`
		INSERT INTO zk_proofs (id, user_did, circuit_id, proof_data, public_inputs, verified, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, 1, ?, ?)`,
		proofID, user.DID, string(zk.CircuitCreditThreshold), req.ProofJSON,
		models.ToJSON(req.PublicInputs),
		now.Format(time.RFC3339),
		now.Add(24*time.Hour).Format(time.RFC3339),
	); err != nil {
		respond(w, 500, nil, "Kanıt kaydedilemedi: "+err.Error())
		return
	}

	oldTier := user.Tier
	if _, err := tx.Exec(
		`UPDATE users SET tier = ?, updated_at = ? WHERE did = ?`,
		req.TargetTier, now.Format(time.RFC3339), user.DID,
	); err != nil {
		respond(w, 500, nil, "Tier güncellenemedi: "+err.Error())
		return
	}

	if _, err := tx.Exec(`
		INSERT INTO credit_events (id, user_did, event_type, delta, reason, new_score, created_at)
		VALUES (?, ?, 'tier_upgrade', 0, ?, ?, ?)`,
		uuid.New().String(), user.DID,
		fmt.Sprintf("ZK ile tier %d → %d (proof %s)", oldTier, req.TargetTier, proofID[:8]),
		user.CreditScore, now.Format(time.RFC3339),
	); err != nil {
		respond(w, 500, nil, "Credit event kaydedilemedi: "+err.Error())
		return
	}

	if err := tx.Commit(); err != nil {
		respond(w, 500, nil, "tx commit: "+err.Error())
		return
	}

	if messaging.GlobalHub.IsOnline(user.DID) {
		messaging.GlobalHub.SendTo(user.DID, "tier_upgraded", map[string]any{
			"old_tier":  oldTier,
			"new_tier":  req.TargetTier,
			"tier_name": models.TierNames[req.TargetTier],
			"proof_id":  proofID,
		})
	}

	respond(w, 200, map[string]any{
		"old_tier":  oldTier,
		"new_tier":  req.TargetTier,
		"tier_name": models.TierNames[req.TargetTier],
		"proof_id":  proofID,
		"verified":  true,
	}, "")
}
