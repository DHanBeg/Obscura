// Package zk — proof anomaly detection ve rate limiting.
//
// Spec Bölüm 19.3 güvenlik katmanı:
//   1. Rate limit:    Aynı DID'den 1 dakikada >10 proof → RateLimitExceeded
//   2. Replay:        Aynı proof hash'i daha önce kullanıldı → ReplayAttack
//   3. Stale:         Proof timestamp >5 dakika geride → StaleProof
//   4. Unknown:       Circuit ID yüklü değil → UnknownCircuit
//
// DB tablosu: proof_log — persistent audit log + replay guard.
// Rate limit: in-memory sliding window (node restart'ta sıfırlanır; persist
// gerekirse proof_log tablosundaki zaman damgaları üzerinden sorgulanabilir).
package zk

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"obscura.network/core/internal/dbi"
	"obscura.network/core/internal/logredact"
)

// AnomalyFlag — bitfield. Birden fazla anomali aynı anda set edilebilir.
type AnomalyFlag uint8

const (
	FlagNone            AnomalyFlag = 0
	FlagRateLimitExceeded AnomalyFlag = 1 << 0 // 1 dk'da >10 proof (aynı DID)
	FlagReplayAttack      AnomalyFlag = 1 << 1 // aynı proof hash tekrar gönderildi
	FlagStaleProof        AnomalyFlag = 1 << 2 // timestamp >5 dakika eski
	FlagUnknownCircuit    AnomalyFlag = 1 << 3 // circuit vkey yüklü değil
)

// AnomalyFlags — tespit edilen anomalilerin birleşimi.
type AnomalyFlags struct {
	Flags AnomalyFlag
}

// HasAnomaly true döner eğer en az bir anomaly flag set edilmişse.
func (a AnomalyFlags) HasAnomaly() bool { return a.Flags != FlagNone }

// String — okunabilir açıklama (loglama için).
func (a AnomalyFlags) String() string {
	if a.Flags == FlagNone {
		return "none"
	}
	names := []string{}
	if a.Flags&FlagRateLimitExceeded != 0 {
		names = append(names, "rate_limit_exceeded")
	}
	if a.Flags&FlagReplayAttack != 0 {
		names = append(names, "replay_attack")
	}
	if a.Flags&FlagStaleProof != 0 {
		names = append(names, "stale_proof")
	}
	if a.Flags&FlagUnknownCircuit != 0 {
		names = append(names, "unknown_circuit")
	}
	out := names[0]
	for _, n := range names[1:] {
		out += "|" + n
	}
	return out
}

// ─── Sabitler ─────────────────────────────────────────────────────────────

const (
	rateLimitWindow   = time.Minute       // sliding window genişliği
	rateLimitMaxProof = 10                // pencere başına izin verilen max proof sayısı
	staleProofWindow  = 5 * time.Minute   // proof timestamp ne kadar eski olabilir
)

// ─── In-memory rate limiter ────────────────────────────────────────────────

// proofTimestamp — tek bir proof gönderim zamanı.
type proofTimestamp struct {
	ts time.Time
}

// ProofAnomalyDetector — DID başına sliding window rate limiter +
// proof_log DB tablosuyla replay guard.
// Zero-value kullanılabilir değil; NewProofAnomalyDetector ile oluştur.
type ProofAnomalyDetector struct {
	mu      sync.Mutex
	windows map[string][]proofTimestamp // did → son N proof zamanı
}

// NewProofAnomalyDetector — yeni dedektör oluşturur.
func NewProofAnomalyDetector() *ProofAnomalyDetector {
	return &ProofAnomalyDetector{
		windows: make(map[string][]proofTimestamp),
	}
}

// globalDetector — package-level singleton. handlers/main tarafından InitDetector
// ile kurulur; test kodları kendi instance'larını kullanabilir.
var (
	globalDetector   *ProofAnomalyDetector
	globalDetectorMu sync.RWMutex
)

// InitDetector — globalDetector'ı set eder. main() başlatma sırasında çağrılır.
func InitDetector(d *ProofAnomalyDetector) {
	globalDetectorMu.Lock()
	defer globalDetectorMu.Unlock()
	globalDetector = d
}

// GlobalDetector — package-dışı kullanım için.
func GlobalDetector() *ProofAnomalyDetector {
	globalDetectorMu.RLock()
	defer globalDetectorMu.RUnlock()
	return globalDetector
}

// ─── DB migration (proof_log tablosu) ─────────────────────────────────────

// ProofLogMigrationSQL — proof_log tablosunu oluşturan SQL.
// database.go migrations listesine eklenmeli (migration 080).
const ProofLogMigrationSQL = `CREATE TABLE IF NOT EXISTS proof_log (
	proof_hash    TEXT PRIMARY KEY,
	circuit_id    TEXT NOT NULL,
	user_did      TEXT NOT NULL,
	timestamp     INTEGER NOT NULL,
	anomaly_flags TEXT NOT NULL DEFAULT 'none'
);
CREATE INDEX IF NOT EXISTS idx_proof_log_did_ts ON proof_log(user_did, timestamp DESC);`

// ─── Ana fonksiyon ────────────────────────────────────────────────────────

// RecordProof — proof'u anomaly açısından kontrol eder ve proof_log tablosuna kaydeder.
//
// proofTimestamp: proof'un oluşturulma zamanı (client'tan gelen; proof.ProofJSON
// içindeki public input'lardan çıkarılması gerekmez — RecordProof'a ayrı parametre).
// Eğer sıfır değer verilirse zaman damgası kontrolü atlanır.
//
// Dönüş değerleri:
//   - AnomalyFlags: tespit edilen anomaliler (HasAnomaly() ile kontrol et)
//   - error: DB yazma hatası — anomaly tespiti etkilenmez
func (d *ProofAnomalyDetector) RecordProof(
	dbConn dbi.Querier,
	proof ProofData,
	userDID string,
	circuitID CircuitID,
	proofTime time.Time,
) (AnomalyFlags, error) {
	flags := FlagNone
	now := time.Now().UTC()

	// ── 4. UnknownCircuit ──────────────────────────────────────────────────
	if !IsCircuitKnown(circuitID) {
		flags |= FlagUnknownCircuit
		log.Printf("[AnomalyDetector] unknown circuit did=%s circuit=%s", logredact.DID(userDID), circuitID)
	}

	// ── 3. StaleProof ─────────────────────────────────────────────────────
	if !proofTime.IsZero() {
		age := now.Sub(proofTime)
		if age > staleProofWindow || age < -staleProofWindow {
			flags |= FlagStaleProof
			log.Printf("[AnomalyDetector] stale proof did=%s age=%v", logredact.DID(userDID), age)
		}
	}

	// ── Proof hash hesapla ────────────────────────────────────────────────
	h := sha256.New()
	h.Write([]byte(proof.ProofJSON))
	h.Write([]byte(circuitID))
	for _, s := range proof.PublicInputs {
		h.Write([]byte(s))
	}
	proofHash := hex.EncodeToString(h.Sum(nil))

	// ── 2. ReplayAttack ───────────────────────────────────────────────────
	if dbConn != nil {
		var existing string
		err := dbConn.QueryRow(
			`SELECT proof_hash FROM proof_log WHERE proof_hash = ? LIMIT 1`,
			proofHash,
		).Scan(&existing)
		if err == nil {
			// Satır bulundu → replay
			flags |= FlagReplayAttack
			log.Printf("[AnomalyDetector] replay attack did=%s hash=%s", logredact.DID(userDID), proofHash[:8])
		}
		// sql.ErrNoRows → yeni proof, normal akış
	}

	// ── 1. RateLimitExceeded ──────────────────────────────────────────────
	if d.checkRateLimit(userDID, now) {
		flags |= FlagRateLimitExceeded
		log.Printf("[AnomalyDetector] rate limit exceeded did=%s", logredact.DID(userDID))
	}

	// ── DB'ye kaydet ──────────────────────────────────────────────────────
	result := AnomalyFlags{Flags: flags}
	flagStr := result.String()

	var dbErr error
	if dbConn != nil {
		_, dbErr = dbConn.Exec(
			`INSERT INTO proof_log (proof_hash, circuit_id, user_did, timestamp, anomaly_flags)
			 VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT DO NOTHING`,
			proofHash,
			string(circuitID),
			userDID,
			now.Unix(),
			flagStr,
		)
		if dbErr != nil {
			dbErr = fmt.Errorf("proof_log yazılamadı: %w", dbErr)
		}
	}

	return result, dbErr
}

// checkRateLimit — sliding window kontrolü. true döner eğer limit aşıldıysa.
// Bu çağrı aynı zamanda pencereyi günceller (yeni timestamp ekler).
func (d *ProofAnomalyDetector) checkRateLimit(userDID string, now time.Time) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	cutoff := now.Add(-rateLimitWindow)
	existing := d.windows[userDID]

	// Pencere dışı kayıtları temizle
	filtered := existing[:0]
	for _, ts := range existing {
		if ts.ts.After(cutoff) {
			filtered = append(filtered, ts)
		}
	}

	// Yeni timestamp ekle (rate limit aşılmış olsa da kaydedilir)
	filtered = append(filtered, proofTimestamp{ts: now})
	d.windows[userDID] = filtered

	// Penceredeki sayı limiti aşıyor mu?
	// len(filtered) zaten yeni timestamp'i içeriyor; >rateLimitMaxProof kontrolü
	// "bu proof dahil pencerede kaç tane var" sorusuna cevap verir.
	return len(filtered) > rateLimitMaxProof
}

// ─── Yardımcı: anomaly_flags JSON serileştirme ────────────────────────────

// AnomalyFlagsFromString — DB'den okunan string → AnomalyFlags.
func AnomalyFlagsFromString(s string) AnomalyFlags {
	// Sadece human-readable ayrıştırma; bitfield string değil pipe-separated.
	f := FlagNone
	if s == "" || s == "none" {
		return AnomalyFlags{Flags: f}
	}
	parts := splitPipe(s)
	for _, p := range parts {
		switch p {
		case "rate_limit_exceeded":
			f |= FlagRateLimitExceeded
		case "replay_attack":
			f |= FlagReplayAttack
		case "stale_proof":
			f |= FlagStaleProof
		case "unknown_circuit":
			f |= FlagUnknownCircuit
		}
	}
	return AnomalyFlags{Flags: f}
}

func splitPipe(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '|' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

// MarshalJSON — AnomalyFlags JSON kodlama (API yanıtı için).
func (a AnomalyFlags) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.String())
}
