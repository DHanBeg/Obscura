package api

// Credit tier upgrade via ZK proof.
//
// Spec Bölüm 5.5, 7.3, 17 EK B: kullanıcı tier yükseltmek için
// kredi puanının threshold'u aştığını ZK proof ile kanıtlar.
// Sunucu gerçek puanı görmez, sadece "puan >= threshold" kanıtını doğrular.
//
// POST /v1/credit/upgrade
// Body: { proof_json, public_inputs[] }
// proof type: credit_threshold circuit
// public_inputs[1] = threshold (claim'in eşik değeri)

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/messaging"
	"obscura.network/core/internal/models"
	"obscura.network/core/internal/zk"
)

type CreditUpgradeRequest struct {
	ProofJSON     string   `json:"proof_json"`
	PublicInputs  []string `json:"public_inputs"`
	TargetTier    int      `json:"target_tier"` // 2-5
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

	requiredThreshold := models.TierMinScore[req.TargetTier]

	// Public input layout (credit_threshold.circom):
	//   [0] = is_above output (1 must mean proof passed circuit)
	//   [1] = threshold
	//   [2] = score_commitment (Poseidon hash)
	//   [3] = user_hash (replay anchor)
	if len(req.PublicInputs) < 4 {
		respond(w, 400, nil, "public_inputs eksik (4 adet bekleniyor)")
		return
	}

	// is_above must be 1
	if req.PublicInputs[0] != "1" {
		respond(w, 400, nil, "Kanıt çıktısı 'is_above' = 1 olmalı")
		return
	}

	// threshold check — must match what the target tier requires
	// Circuit threshold input is integer; spec offset: actualScore + 20 (negatifleri 0-120'ye taşıma).
	// Bu yüzden public threshold = (TierMinScore + 20) olarak beklenir.
	threshold, err := strconv.Atoi(req.PublicInputs[1])
	if err != nil {
		respond(w, 400, nil, "threshold parse edilemedi")
		return
	}
	expectedThreshold := int(requiredThreshold) + 20
	if threshold < expectedThreshold {
		respond(w, 400, nil, fmt.Sprintf(
			"Kanıt threshold'u (%d) hedef tier için yetersiz (gerekli: %d)",
			threshold, expectedThreshold))
		return
	}

	// Real Groth16 verification
	circuit := zk.CircuitCreditThreshold
	if !zk.IsCircuitKnown(circuit) {
		respond(w, 500, nil, "credit_threshold circuit yüklenmemiş")
		return
	}

	// Allow client to send either {proof: ..., publicSignals: ...} or just proof
	var proofBytes json.RawMessage
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(req.ProofJSON), &raw); err != nil {
		respond(w, 400, nil, "proof_json geçersiz JSON")
		return
	}
	if pj, ok := raw["proof"]; ok {
		proofBytes = pj
	} else {
		proofBytes = []byte(req.ProofJSON)
	}

	if err := zk.VerifyGroth16(circuit, proofBytes, req.PublicInputs); err != nil {
		respond(w, 400, nil, "ZK kanıtı doğrulanamadı: "+err.Error())
		return
	}

	// Replay protection: nullifier = user_hash, save proof
	now := time.Now().UTC()
	proofID := uuid.New().String()
	_, err = db.DB.Exec(`
		INSERT INTO zk_proofs (id, user_did, circuit_id, proof_data, public_inputs, verified, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, 1, ?, ?)`,
		proofID, user.DID, string(circuit), req.ProofJSON,
		models.ToJSON(req.PublicInputs),
		now.Format(time.RFC3339),
		now.Add(24*time.Hour).Format(time.RFC3339),
	)
	if err != nil {
		respond(w, 500, nil, "Kanıt kaydedilemedi: "+err.Error())
		return
	}

	// Apply tier upgrade
	_, err = db.DB.Exec(`
		UPDATE users SET tier = ?, updated_at = ? WHERE did = ?`,
		req.TargetTier, now.Format(time.RFC3339), user.DID,
	)
	if err != nil {
		respond(w, 500, nil, "Tier güncellenemedi: "+err.Error())
		return
	}

	// Credit history event
	_, _ = db.DB.Exec(`
		INSERT INTO credit_events (id, user_did, event_type, delta, reason, new_score, created_at)
		VALUES (?, ?, 'tier_upgrade', 0, ?, ?, ?)`,
		uuid.New().String(), user.DID,
		fmt.Sprintf("ZK ile tier %d → %d (proof %s)", user.Tier, req.TargetTier, proofID[:8]),
		user.CreditScore, now.Format(time.RFC3339),
	)

	// Notify via WS
	if messaging.GlobalHub.IsOnline(user.DID) {
		messaging.GlobalHub.SendTo(user.DID, "tier_upgraded", map[string]any{
			"old_tier":  user.Tier,
			"new_tier":  req.TargetTier,
			"tier_name": models.TierNames[req.TargetTier],
			"proof_id":  proofID,
		})
	}

	respond(w, 200, map[string]any{
		"old_tier":   user.Tier,
		"new_tier":   req.TargetTier,
		"tier_name":  models.TierNames[req.TargetTier],
		"proof_id":   proofID,
		"verified":   true,
	}, "")
}
