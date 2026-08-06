// Package zk — immutable ZK proof audit log with hash chaining.
//
// Spec Bölüm 7.3 Adım 5: "Proof hash'i blockchain'e kaydet (immutable)."
// Tam blockchain entegrasyonu yerine local append-only log + SHA-256 hash
// zinciri kullanılır. Zincir bütünlüğü VerifyLogIntegrity ile doğrulanır.
//
// DB tablosu: zk_proof_log (database.go migration 086_zk_proof_log_chain).
// Dikkat: Bu tablo proof_log tablosundan (anomaly detector) AYRIDIR.
// proof_log: replay guard + rate limit (anomaly.go)
// zk_proof_log: immutable audit trail + hash chain (bu dosya)
package zk

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
	"obscura.network/core/internal/dbi"
)

// ProofLogEntry — zk_proof_log tablosundaki tek bir satırı temsil eder.
type ProofLogEntry struct {
	ID         int64
	ProofHash  string // SHA-256(proof_json + public_inputs...)
	CircuitID  string
	UserDID    string
	VerifiedAt int64  // Unix timestamp
	PrevHash   string // önceki satırın chain_hash'i (ilk satırda boş string)
	ChainHash  string // SHA-256(proof_hash + prev_hash + verified_at)
}

// ProofLogChainMigrationSQL — zk_proof_log tablosunu ve indekslerini oluşturan DDL.
// database.go migrations listesindeki 086_zk_proof_log_chain migration'ı bu
// sabite referans verir; tek kaynak (single source of truth) olarak burada tutulur.
const ProofLogChainMigrationSQL = `CREATE TABLE IF NOT EXISTS zk_proof_log (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	proof_hash  TEXT NOT NULL,
	circuit_id  TEXT NOT NULL,
	user_did    TEXT NOT NULL,
	verified_at INTEGER NOT NULL,
	prev_hash   TEXT NOT NULL DEFAULT '',
	chain_hash  TEXT NOT NULL UNIQUE
);
CREATE INDEX IF NOT EXISTS idx_zk_proof_log_did     ON zk_proof_log(user_did);
CREATE INDEX IF NOT EXISTS idx_zk_proof_log_circuit ON zk_proof_log(circuit_id, verified_at)`

// ProofLogChainMigrationSQLPostgres — ProofLogChainMigrationSQL'in Postgres
// karşılığı. SQLite'ın AUTOINCREMENT'ı Postgres'te yok (syntax error);
// BIGSERIAL aynı garantiyi verir (sequence tabanlı, silinen satırın id'si
// asla yeniden kullanılmaz — audit hash-chain için gereken özellik SQLite'ın
// AUTOINCREMENT'ından bu yana korunuyor).
const ProofLogChainMigrationSQLPostgres = `CREATE TABLE IF NOT EXISTS zk_proof_log (
	id          BIGSERIAL PRIMARY KEY,
	proof_hash  TEXT NOT NULL,
	circuit_id  TEXT NOT NULL,
	user_did    TEXT NOT NULL,
	verified_at INTEGER NOT NULL,
	prev_hash   TEXT NOT NULL DEFAULT '',
	chain_hash  TEXT NOT NULL UNIQUE
);
CREATE INDEX IF NOT EXISTS idx_zk_proof_log_did     ON zk_proof_log(user_did);
CREATE INDEX IF NOT EXISTS idx_zk_proof_log_circuit ON zk_proof_log(circuit_id, verified_at)`

// AppendProofLog — doğrulanmış bir proof'u immutable zincir log'una ekler.
// Fonksiyon append-only çalışır: INSERT dışında hiçbir DML komutu kullanmaz.
//
// proofJSON:    snarkjs'ten gelen ham proof string
// publicInputs: circuit public signals dizisi
//
// Dönüş değeri: yeni satırın chain_hash'i veya DB hatası.
// Çağıran: log hataları non-fatal olarak ele almalıdır — doğrulama sonucu
// bu fonksiyonun başarısına bağlı değildir.
func AppendProofLog(
	dbConn dbi.Querier,
	circuitID, userDID string,
	proofJSON string,
	publicInputs []string,
) (chainHash string, err error) {
	if dbConn == nil {
		return "", fmt.Errorf("AppendProofLog: db nil")
	}

	// 1. Proof hash hesapla: SHA-256(proof_json + public_inputs...)
	h := sha256.New()
	h.Write([]byte(proofJSON))
	for _, pi := range publicInputs {
		h.Write([]byte(pi))
	}
	proofHash := hex.EncodeToString(h.Sum(nil))

	// 2. Son chain_hash'i al (zincir bağlantısı için)
	var prevHash string
	row := dbConn.QueryRow(
		`SELECT chain_hash FROM zk_proof_log ORDER BY id DESC LIMIT 1`,
	)
	// sql.ErrNoRows → tablo boş, prevHash boş string kalır.
	_ = row.Scan(&prevHash)

	// 3. Yeni chain hash: SHA-256(proof_hash + prev_hash + verified_at)
	now := time.Now().Unix()
	ch := sha256.New()
	ch.Write([]byte(proofHash))
	ch.Write([]byte(prevHash))
	ch.Write([]byte(fmt.Sprintf("%d", now)))
	chainHash = hex.EncodeToString(ch.Sum(nil))

	// 4. Append-only INSERT — asla UPDATE veya DELETE kullanılmaz.
	_, err = dbConn.Exec(
		`INSERT INTO zk_proof_log (proof_hash, circuit_id, user_did, verified_at, prev_hash, chain_hash)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		proofHash, circuitID, userDID, now, prevHash, chainHash,
	)
	if err != nil {
		return "", fmt.Errorf("zk_proof_log INSERT: %w", err)
	}
	return chainHash, nil
}

// VerifyLogIntegrity — tüm zk_proof_log tablosunu baştan sona tarar ve her
// satırın chain_hash'ini yeniden hesaplayarak saklanan değerle karşılaştırır.
// prev_hash zincirini de doğrular: her satırın prev_hash'i önceki satırın
// chain_hash'ine eşit olmalıdır.
//
// Dönüş değerleri:
//   - isValid bool: zincir bütünlüklü mü
//   - brokenID int64: bütünlük bozulan ilk satırın id'si (isValid=true ise 0)
//   - err error: DB okuma hatası
func VerifyLogIntegrity(dbConn dbi.Querier) (isValid bool, brokenID int64, err error) {
	if dbConn == nil {
		return false, 0, fmt.Errorf("VerifyLogIntegrity: db nil")
	}

	rows, err := dbConn.Query(
		`SELECT id, proof_hash, prev_hash, chain_hash, verified_at
		 FROM zk_proof_log ORDER BY id ASC`,
	)
	if err != nil {
		return false, 0, fmt.Errorf("zk_proof_log sorgu hatası: %w", err)
	}
	defer rows.Close()

	var expectedPrevHash string // ilk satır için boş string
	for rows.Next() {
		var (
			id         int64
			proofHash  string
			prevHash   string
			chainHash  string
			verifiedAt int64
		)
		if err := rows.Scan(&id, &proofHash, &prevHash, &chainHash, &verifiedAt); err != nil {
			return false, id, fmt.Errorf("zk_proof_log satır okuma: %w", err)
		}

		// prev_hash zinciri kontrolü
		if prevHash != expectedPrevHash {
			return false, id, nil
		}

		// chain_hash yeniden hesapla
		ch := sha256.New()
		ch.Write([]byte(proofHash))
		ch.Write([]byte(prevHash))
		ch.Write([]byte(fmt.Sprintf("%d", verifiedAt)))
		expected := hex.EncodeToString(ch.Sum(nil))

		if expected != chainHash {
			return false, id, nil
		}

		// Bir sonraki satırın beklenen prev_hash'i bu satırın chain_hash'i
		expectedPrevHash = chainHash
	}

	if err := rows.Err(); err != nil {
		return false, 0, fmt.Errorf("zk_proof_log tarama hatası: %w", err)
	}

	return true, 0, nil
}
