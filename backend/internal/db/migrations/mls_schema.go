// Package migrations holds standalone, idempotent DDL for subsystems.
//
// mls_schema.go — RFC 9420 MLS grup şifreleme tabloları (spec Bölüm 6.3).
//
// NOT: Bu DDL, database.go içindeki yerleşik migration'larla (003-011) BİREBİR
// uyumludur. Kolon adları handler'ların (internal/api/mls_handlers.go) ve
// rotation tarayıcısının (internal/mls/rotation.go) beklediği şemaya göredir:
//
//	key paketi BLOB değil, base64 TEXT olarak saklanır (key_package_b64) —
//	subprocess JSON-RPC sınırında zaten base64 taşındığı için TEXT seçildi,
//	böylece ekstra encode/decode adımı yok.
//
// MLSMigrations() bağımsız bir kurulum/test harness'ı (örn. salt MLS şeması
// gereken bir entegrasyon testi) tarafından çağrılabilir. Üretim node'u şemayı
// database.go üzerinden alır; bu dosya o şemanın tek-kaynak (canonical) SQL
// kopyasıdır ve database.go'ya DOKUNMADAN paylaşılabilir hale getirir.
//
// Tüm ifadeler IF NOT EXISTS — birden fazla kez çalıştırmak güvenlidir.
package migrations

// MLSMigrations, MLS alt sistemi için sırayla uygulanması gereken DDL
// ifadelerini döndürür. Sıra önemlidir: indeksler ve foreign key referansları
// tablolardan sonra gelir.
func MLSMigrations() []string {
	return []string{
		// ── KeyPackage havuzu ───────────────────────────────────────────────
		// Üyeler önceden KeyPackage yükler; add_member sırasında tek kullanımlık
		// olarak tüketilir (used=1). 90 gün TTL — spec Bölüm 4.2.
		`CREATE TABLE IF NOT EXISTS mls_key_packages (
			id              TEXT PRIMARY KEY,
			user_did        TEXT NOT NULL,
			key_package_b64 TEXT NOT NULL,
			ciphersuite     TEXT NOT NULL DEFAULT 'MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519',
			used            INTEGER NOT NULL DEFAULT 0,
			created_at      TEXT NOT NULL,
			expires_at      TEXT NOT NULL,
			used_at         TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_mls_kp_did
			ON mls_key_packages(user_did, used, expires_at)`,

		// ── Grup metadata ───────────────────────────────────────────────────
		// Sunucu yalnızca wire-state (epoch, opak ratchet_tree_b64) tutar;
		// plaintext ağaca veya mesaj içeriğine erişemez (spec Bölüm 4.5).
		`CREATE TABLE IF NOT EXISTS mls_groups (
			id               TEXT PRIMARY KEY,
			creator_did      TEXT NOT NULL,
			name             TEXT DEFAULT '',
			ciphersuite      TEXT NOT NULL DEFAULT 'MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519',
			epoch            INTEGER NOT NULL DEFAULT 0,
			ratchet_tree_b64 TEXT,
			created_at       TEXT NOT NULL,
			updated_at       TEXT NOT NULL
		)`,

		// ── Üye listesi (roster) ────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS mls_group_members (
			group_id        TEXT NOT NULL,
			user_did        TEXT NOT NULL,
			role            TEXT DEFAULT 'member',
			leaf_index      INTEGER,
			joined_at       TEXT NOT NULL,
			joined_at_epoch INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (group_id, user_did),
			FOREIGN KEY (group_id) REFERENCES mls_groups(id) ON DELETE CASCADE
		)`,

		// ── Bekleyen proposal/commit kayıtları (audit + replay) ─────────────
		`CREATE TABLE IF NOT EXISTS mls_pending_proposals (
			id            TEXT PRIMARY KEY,
			group_id      TEXT NOT NULL,
			proposer_did  TEXT NOT NULL,
			proposal_b64  TEXT NOT NULL,
			proposal_type TEXT NOT NULL,
			epoch         INTEGER NOT NULL,
			created_at    TEXT NOT NULL,
			FOREIGN KEY (group_id) REFERENCES mls_groups(id) ON DELETE CASCADE
		)`,
		// B10.2 Tuğla 1 — CAS'in (advanceGroupEpoch) DB-seviyesi son savunması.
		// Her başarılı commit/remove/update-key çağrısı bu tabloya tam 1 satır
		// yazar; aynı (group_id, epoch) ikinci satırı = çatallanma kanıtı.
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_mls_pending_proposals_epoch_unique
			ON mls_pending_proposals(group_id, epoch)`,

		// ── Şifreli uygulama mesajları (sunucu sadece ciphertext görür) ──────
		`CREATE TABLE IF NOT EXISTS mls_messages (
			id             TEXT PRIMARY KEY,
			group_id       TEXT NOT NULL,
			sender_did     TEXT NOT NULL,
			ciphertext_b64 TEXT NOT NULL,
			content_type   TEXT NOT NULL DEFAULT 'application',
			epoch          INTEGER NOT NULL,
			created_at     TEXT NOT NULL,
			FOREIGN KEY (group_id) REFERENCES mls_groups(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_mls_msg_group
			ON mls_messages(group_id, created_at DESC)`,
		// B10.2 Tuğla 1 — partial (content_type='commit' filtreli): aynı
		// epoch'ta çoklu application mesajı NORMAL, ama commit tekil olmalı.
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_mls_messages_commit_epoch_unique
			ON mls_messages(group_id, epoch) WHERE content_type = 'commit'`,

		// ── Welcome kuyruğu (offline alıcılar için) ─────────────────────────
		`CREATE TABLE IF NOT EXISTS mls_welcome_queue (
			id            TEXT PRIMARY KEY,
			group_id      TEXT NOT NULL,
			recipient_did TEXT NOT NULL,
			welcome_b64   TEXT NOT NULL,
			created_at    TEXT NOT NULL,
			delivered_at  TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_mls_welcome_recipient
			ON mls_welcome_queue(recipient_did, delivered_at)`,
	}
}
