package api

// credit_zk_handler.go — POST /v1/credit/claim-zk
//
// ZK proof tabanlı kredi claim endpoint'i.
// Spec Bölüm 5.5 — 6 circuit: age, activity, node, endorsement, streak, msg_count.
//
// RequireAuth: evet (JWT middleware)
// Body limit: 1 MB (MaxBytesReader)
// Cooldown: proof tipine göre 1-30 gün; aynı periyotta tekrar claim edilemez.

import (
	"encoding/json"
	"log"
	"net/http"

	"obscura.network/core/internal/credit"
)

// claimZKRequest — POST /v1/credit/claim-zk istek gövdesi.
type claimZKRequest struct {
	// ProofType: "age" | "activity" | "node" | "endorsement" | "streak" | "msg_count"
	ProofType string `json:"proof_type"`

	// Proof: snarkjs Groth16 proof nesnesi, base64 veya ham JSON string olabilir.
	// Örnek (ham JSON): {"pi_a":["..."],"pi_b":[["..."]],"pi_c":["..."],"protocol":"groth16","curve":"bn128"}
	Proof json.RawMessage `json:"proof"`

	// PublicSignals: ZK kanıtının genel sinyalleri (string-encoded field element dizisi).
	// Örnek: ["12345", "67890"]
	PublicSignals json.RawMessage `json:"public_signals"`
}

// HandleClaimZKCredit — POST /v1/credit/claim-zk
//
// Akış:
//  1. JWT auth (AuthMiddleware tarafından sağlanır)
//  2. Body boyut sınırı (1 MB)
//  3. Input validasyonu
//  4. credit.ClaimZKCredit çağrısı (ZK doğrulama + cooldown + puan kaydet)
//  5. Başarı yanıtı: {points_awarded, new_score, next_claim_at}
func HandleClaimZKCredit(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	// Body boyut sınırı
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB

	var req claimZKRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond(w, 400, nil, "Geçersiz istek gövdesi")
		return
	}

	// --- Validasyon ---
	if req.ProofType == "" {
		respond(w, 400, nil, "proof_type zorunlu")
		return
	}
	if !credit.IsValidProofType(req.ProofType) {
		respond(w, 400, nil, "geçersiz proof_type: age|activity|node|endorsement|streak|msg_count olmalı")
		return
	}
	if len(req.Proof) == 0 || string(req.Proof) == "null" {
		respond(w, 400, nil, "proof zorunlu")
		return
	}
	if len(req.PublicSignals) == 0 || string(req.PublicSignals) == "null" {
		respond(w, 400, nil, "public_signals zorunlu")
		return
	}

	// proof alanı nesne veya base64 string olabilir.
	// Eğer JSON object ise string'e çevir (credit.ClaimZKCredit base64 veya ham JSON kabul eder).
	proofStr := string(req.Proof)
	// JSON string literal mi? (tırnaklar içinde)
	if len(proofStr) >= 2 && proofStr[0] == '"' && proofStr[len(proofStr)-1] == '"' {
		// JSON string → tırnakları çıkar
		var s string
		if err := json.Unmarshal(req.Proof, &s); err != nil {
			respond(w, 400, nil, "proof alanı geçersiz JSON string")
			return
		}
		proofStr = s
	}

	publicSignalsStr := string(req.PublicSignals)

	// --- ZK Claim ---
	result, err := credit.ClaimZKCredit(
		user.DID,
		credit.ProofType(req.ProofType),
		proofStr,
		publicSignalsStr,
	)
	if err != nil {
		log.Printf("ZK credit claim başarısız (did=%s, type=%s): %v", user.DID, req.ProofType, err)
		respond(w, 400, nil, err.Error())
		return
	}

	respond(w, 200, map[string]interface{}{
		"points_awarded": result.PointsAwarded,
		"new_score":      result.NewScore,
		"next_claim_at":  result.NextClaimAt.Format("2006-01-02T15:04:05Z07:00"),
	}, "")
}
