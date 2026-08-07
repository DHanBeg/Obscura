package credit

// zk_credit.go — ZK proof tabanlı kredi claim sistemi (Spec Bölüm 5.5)
//
// 6 circuit ile puan güncelleme:
//   age_proof        → +1/ay
//   activity_proof   → +0.5/gün (günde bir)
//   node_proof       → +10/ay
//   endorsement_proof → +1/onay (7 günde bir)
//   streak_proof     → +0.5/streak (günde bir)
//   msg_count_proof  → +0.1/100msg (7 günde bir)

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/zk"
)

// ProofType — desteklenen ZK proof tipleri.
type ProofType string

const (
	ProofTypeAge         ProofType = "age"
	ProofTypeActivity    ProofType = "activity"
	ProofTypeNode        ProofType = "node"
	ProofTypeEndorsement ProofType = "endorsement"
	ProofTypeStreak      ProofType = "streak"
	ProofTypeMsgCount    ProofType = "msg_count"
)

// proofCircuit — proof tipini ilgili ZK circuit ID'sine eşler.
var proofCircuit = map[ProofType]zk.CircuitID{
	ProofTypeAge:         zk.CircuitAgeProof,
	ProofTypeActivity:    zk.CircuitActivityProof,
	ProofTypeNode:        zk.CircuitNodeProof,
	ProofTypeEndorsement: zk.CircuitEndorsementProof,
	ProofTypeStreak:      zk.CircuitStreakProof,
	ProofTypeMsgCount:    zk.CircuitMsgCountProof,
}

// PointsForProofType — proof tipine göre kazanılan puan miktarını döndürür.
func PointsForProofType(proofType ProofType) float64 {
	switch proofType {
	case ProofTypeAge:
		return 1.0 // +1/ay
	case ProofTypeActivity:
		return 0.5 // +0.5/gün
	case ProofTypeNode:
		return 10.0 // +10/ay
	case ProofTypeEndorsement:
		return 1.0 // +1/onay
	case ProofTypeStreak:
		return 0.5 // +0.5/streak
	case ProofTypeMsgCount:
		return 0.1 // +0.1/100msg
	default:
		return 0
	}
}

// cooldownDuration — proof tipine göre bir sonraki claim için bekleme süresi.
func cooldownDuration(proofType ProofType) time.Duration {
	switch proofType {
	case ProofTypeAge:
		return 30 * 24 * time.Hour
	case ProofTypeActivity:
		return 24 * time.Hour
	case ProofTypeNode:
		return 30 * 24 * time.Hour
	case ProofTypeEndorsement:
		return 7 * 24 * time.Hour
	case ProofTypeStreak:
		return 24 * time.Hour
	case ProofTypeMsgCount:
		return 7 * 24 * time.Hour
	default:
		return 24 * time.Hour
	}
}

// IsValidProofType — proof tipinin desteklenip desteklenmediğini döndürür.
func IsValidProofType(pt string) bool {
	_, ok := proofCircuit[ProofType(pt)]
	return ok
}

// ZKClaimResult — başarılı claim sonucu.
type ZKClaimResult struct {
	PointsAwarded float64   `json:"points_awarded"`
	NewScore      float64   `json:"new_score"`
	NextClaimAt   time.Time `json:"next_claim_at"`
}

// ClaimZKCredit — ZK proof doğrular, cooldown kontrolü yapar ve puanı kaydeder.
//
// Akış:
//  1. proofType geçerlilik kontrolü
//  2. Circuit'e göre vkey seçimi
//  3. Base64 proof decode + JSON unmarshal
//  4. Public signals JSON unmarshal
//  5. ZK Groth16 doğrulama
//  6. Cooldown kontrolü (did + proof_type + valid_until UNIQUE)
//  7. credit_score güncelleme (max 100)
//  8. credit_proof_claims kaydı
func ClaimZKCredit(did string, proofType ProofType, proofB64 string, publicSignalsJSON string) (*ZKClaimResult, error) {
	// 1. Proof tipi geçerlilik
	circuitID, ok := proofCircuit[proofType]
	if !ok {
		return nil, fmt.Errorf("geçersiz proof tipi: %s", proofType)
	}

	// 2. Circuit yüklü mü?
	if !zk.IsCircuitKnown(circuitID) {
		return nil, fmt.Errorf("circuit yüklü değil: %s (trusted setup gerekli)", circuitID)
	}

	// 3. Base64 proof decode
	proofBytes, err := base64.StdEncoding.DecodeString(proofB64)
	if err != nil {
		// Ham JSON olabilir (development kolaylığı)
		proofBytes = []byte(proofB64)
	}

	// Proof JSON geçerli mi?
	var probeProof map[string]interface{}
	if err := json.Unmarshal(proofBytes, &probeProof); err != nil {
		return nil, fmt.Errorf("proof JSON geçersiz: %w", err)
	}

	// 4. Public signals decode
	var publicSignals []string
	if err := json.Unmarshal([]byte(publicSignalsJSON), &publicSignals); err != nil {
		// Sayısal dizi olabilir
		var nums []interface{}
		if err2 := json.Unmarshal([]byte(publicSignalsJSON), &nums); err2 != nil {
			return nil, fmt.Errorf("public_signals JSON geçersiz: %w", err)
		}
		publicSignals = make([]string, len(nums))
		for i, v := range nums {
			switch n := v.(type) {
			case string:
				publicSignals[i] = n
			case float64:
				publicSignals[i] = fmt.Sprintf("%.0f", n)
			default:
				return nil, fmt.Errorf("public_signals[%d] beklenmedik tip: %T", i, v)
			}
		}
	}

	// 5. ZK Groth16 doğrulama
	if err := zk.VerifyGroth16(circuitID, proofBytes, publicSignals); err != nil {
		log.Printf("ZK proof doğrulama başarısız (did=%s, type=%s): %v", did, proofType, err)
		return nil, fmt.Errorf("ZK proof geçersiz: %w", err)
	}

	// 6. Cooldown kontrolü
	cooldown := cooldownDuration(proofType)
	now := time.Now()
	nextClaimAt := now.Add(cooldown)

	// valid_until = cooldown periyodunun sonu; bu periyotta tekrar claim edilemez
	// UNIQUE(did, proof_type, valid_until) — aynı periyotta birden fazla claim engellenir
	// Periyodu "round" olarak hesapla: cooldown başlangıcı = şimdiki zamanın cooldown'a yuvarlanması
	periodStart := now.Truncate(cooldown)
	periodEnd := periodStart.Add(cooldown)

	var existingID string
	err = db.DB.QueryRow(`
		SELECT id FROM credit_proof_claims
		WHERE did = ? AND proof_type = ? AND valid_until = ?`,
		did, string(proofType), periodEnd.Format(time.RFC3339),
	).Scan(&existingID)

	if err == nil {
		// Kayıt bulundu — cooldown süresi dolmamış
		return nil, fmt.Errorf("cooldown aktif: %s için bir sonraki claim %s'den sonra",
			proofType, periodEnd.Format(time.RFC3339))
	}
	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("cooldown kontrolü DB hatası: %w", err)
	}

	// 7. Puan güncelle
	points := PointsForProofType(proofType)
	eventType := "zk_" + string(proofType)
	var currentScore float64
	if err := db.DB.QueryRow("SELECT credit_score FROM users WHERE did = ?", did).Scan(&currentScore); err != nil {
		return nil, fmt.Errorf("kullanıcı bulunamadı: %w", err)
	}

	// Kategori tavanı (Spec §7.1 Max kolonu) — plain path'teki aynı kategoriyle
	// birlikte sayılır, bkz. category_cap.go.
	effectivePoints := points
	if category := canonicalCategory(eventType); category != "" {
		ep, capErr := applyCategoryCap(did, category, points)
		if capErr != nil {
			return nil, capErr
		}
		effectivePoints = ep
	}

	newScore := currentScore + effectivePoints
	if newScore > 100 {
		newScore = 100
	}

	_, err = db.DB.Exec(`
		UPDATE users SET credit_score = ?, updated_at = ?
		WHERE did = ?`,
		newScore, now.Format(time.RFC3339), did,
	)
	if err != nil {
		return nil, fmt.Errorf("kredi puanı güncellenemedi: %w", err)
	}

	// credit_events tablosuna olay kaydı (GetHistory ile görüntülenebilsin)
	_, _ = db.DB.Exec(`
		INSERT INTO credit_events (id, user_did, event_type, delta, reason, new_score, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), did,
		eventType,
		points,
		fmt.Sprintf("ZK proof claim: %s", proofType),
		newScore,
		now.Format(time.RFC3339),
	)

	// 8. Claim kaydı
	_, err = db.DB.Exec(`
		INSERT INTO credit_proof_claims
			(id, did, proof_type, proof_b64, public_signals, points_awarded, claimed_at, valid_until)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.New().String(),
		did,
		string(proofType),
		proofB64,
		publicSignalsJSON,
		points,
		now.Format(time.RFC3339),
		periodEnd.Format(time.RFC3339),
	)
	if err != nil {
		// INSERT başarısız olsa bile puan zaten güncellendi; logla ama başarısız sayma
		log.Printf("claim kaydı hata (did=%s, type=%s): %v", did, proofType, err)
	}

	log.Printf("ZK kredit claim: did=%s tip=%s puan=+%.1f yeni=%.1f sonraki=%s",
		did, proofType, points, newScore, nextClaimAt.Format(time.RFC3339))

	return &ZKClaimResult{
		PointsAwarded: points,
		NewScore:      newScore,
		NextClaimAt:   nextClaimAt,
	}, nil
}

// GetCooldownStatus — kullanıcının her proof tipi için cooldown durumunu döndürür.
func GetCooldownStatus(did string) (map[string]time.Time, error) {
	rows, err := db.DB.Query(`
		SELECT proof_type, MAX(valid_until) AS next_claim_at
		FROM credit_proof_claims
		WHERE did = ?
		GROUP BY proof_type`,
		did,
	)
	if err != nil {
		return nil, fmt.Errorf("cooldown sorgusu başarısız: %w", err)
	}
	defer rows.Close()

	result := make(map[string]time.Time)
	for rows.Next() {
		var pt string
		var validUntilStr string
		if err := rows.Scan(&pt, &validUntilStr); err != nil {
			continue
		}
		t, _ := time.Parse(time.RFC3339, validUntilStr)
		result[pt] = t
	}
	return result, nil
}
