// Package migrations — standalone, idempotent DDL for the ZK proof audit log.
//
// proof_log_schema.go — SHA-256 hash zincirli immutable ZK proof audit log
// (spec Bölüm 7.3 Adım 5).
//
// NOT: Bu dosya database.go migration'ından BAĞIMSIZ olarak test harness'ları
// tarafından çağrılabilir. Üretim node'u şemayı database.go üzerinden alır
// (migration 086_zk_proof_log_chain → zk.ProofLogChainMigrationSQL).
package migrations

import "obscura.network/core/internal/zk"

// ProofLogMigrations — zk_proof_log tablosunu ve indekslerini oluşturan DDL
// ifadelerini sırayla döndürür. Tüm ifadeler IF NOT EXISTS — birden fazla kez
// çalıştırmak güvenlidir.
//
// Bağımsız bir kurulum veya entegrasyon testi:
//
//	for _, ddl := range migrations.ProofLogMigrations() {
//	    db.Exec(ddl)
//	}
func ProofLogMigrations() []string {
	// Tek kaynak: zk paketi. SQL burada tekrar edilmez.
	return []string{zk.ProofLogChainMigrationSQL}
}
