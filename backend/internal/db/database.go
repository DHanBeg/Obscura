package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"

	"obscura.network/core/internal/zk"
)

var DB *sql.DB

func Init(dataDir string) error {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("data dir oluşturulamadı: %w", err)
	}

	dbPath := filepath.Join(dataDir, "obscura.db")
	var err error
	DB, err = sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=10000&_synchronous=NORMAL")
	if err != nil {
		return fmt.Errorf("veritabanı açılamadı: %w", err)
	}

	// SQLite: tek yazar, WAL okuma concurrent.
	// MaxOpenConns(1) write serialization'ı garanti eder — SQLite'ın tek-yazar modeliyle uyumlu.
	DB.SetMaxOpenConns(1)
	DB.SetMaxIdleConns(1)
	DB.SetConnMaxLifetime(30 * 60 * 1e9) // 30 dakika

	// Yazma çakışmalarında 15 saniyeye kadar bekle, hemen hata verme
	if _, err := DB.Exec("PRAGMA busy_timeout = 15000"); err != nil {
		log.Printf("⚠️ busy_timeout ayarlanamadı: %v", err)
	}

	// WAL checkpoint — büyük WAL dosyasının birikmesini önle
	if _, err := DB.Exec("PRAGMA wal_autocheckpoint = 1000"); err != nil {
		log.Printf("⚠️ wal_autocheckpoint ayarlanamadı: %v", err)
	}

	if err := createTables(); err != nil {
		return fmt.Errorf("tablolar oluşturulamadı: %w", err)
	}
	if err := runMigrations(); err != nil {
		return fmt.Errorf("migration hatası: %w", err)
	}
	if err := BackfillUserODI(); err != nil {
		return fmt.Errorf("ODI backfill hatası: %w", err)
	}
	if err := BackfillUserDisplayName(); err != nil {
		return fmt.Errorf("nickname backfill hatası: %w", err)
	}

	SeedGovernanceConversation()

	log.Printf("✅ Veritabanı hazır: %s", dbPath)
	return nil
}

// SeedGovernanceConversation — ilk başlangıçta OBS holder community conv'u oluşturur.
// İdempotent: kayıt zaten varsa hiçbir şey yapmaz.
func SeedGovernanceConversation() {
	var count int
	DB.QueryRow("SELECT COUNT(*) FROM conversations WHERE id = 'obs-community-v1'").Scan(&count)
	if count > 0 {
		return
	}
	_, err := DB.Exec(`INSERT INTO conversations
		(id, name, is_group, conv_type, is_public, description, created_at, updated_at)
		VALUES ('obs-community-v1', 'OBS Topluluk', 1, 'community', 1,
		        'OBS coin sahipleri için genel sohbet', datetime('now'), datetime('now'))`)
	if err != nil {
		log.Printf("⚠️ SeedGovernanceConversation hatası: %v", err)
	}
}

// runMigrations — mevcut tablolara yeni kolonları idempotent ekler
func runMigrations() error {
	migrations := []struct {
		id  string
		sql string
	}{
		{"001_add_fcm_token", "ALTER TABLE users ADD COLUMN fcm_token TEXT DEFAULT ''"},
		{"002_add_apns_token", "ALTER TABLE users ADD COLUMN apns_token TEXT DEFAULT ''"},
		// MLS (RFC 9420) — see docs/adr/0007-openmls-for-groups.md
		{"003_mls_key_packages", `CREATE TABLE IF NOT EXISTS mls_key_packages (
			id              TEXT PRIMARY KEY,
			user_did        TEXT NOT NULL,
			key_package_b64 TEXT NOT NULL,
			ciphersuite     TEXT NOT NULL DEFAULT 'MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519',
			used            INTEGER NOT NULL DEFAULT 0,
			created_at      TEXT NOT NULL,
			expires_at      TEXT NOT NULL,
			used_at         TEXT
		)`},
		{"004_mls_key_packages_idx", "CREATE INDEX IF NOT EXISTS idx_mls_kp_did ON mls_key_packages(user_did, used, expires_at)"},
		{"005_mls_groups", `CREATE TABLE IF NOT EXISTS mls_groups (
			id              TEXT PRIMARY KEY,
			creator_did     TEXT NOT NULL,
			name            TEXT DEFAULT '',
			ciphersuite     TEXT NOT NULL,
			epoch           INTEGER NOT NULL DEFAULT 0,
			ratchet_tree_b64 TEXT,
			created_at      TEXT NOT NULL,
			updated_at      TEXT NOT NULL
		)`},
		{"006_mls_group_members", `CREATE TABLE IF NOT EXISTS mls_group_members (
			group_id   TEXT NOT NULL,
			user_did   TEXT NOT NULL,
			role       TEXT DEFAULT 'member',
			joined_at  TEXT NOT NULL,
			joined_at_epoch INTEGER NOT NULL,
			PRIMARY KEY (group_id, user_did),
			FOREIGN KEY (group_id) REFERENCES mls_groups(id) ON DELETE CASCADE
		)`},
		{"007_mls_pending_proposals", `CREATE TABLE IF NOT EXISTS mls_pending_proposals (
			id           TEXT PRIMARY KEY,
			group_id     TEXT NOT NULL,
			proposer_did TEXT NOT NULL,
			proposal_b64 TEXT NOT NULL,
			proposal_type TEXT NOT NULL,
			epoch        INTEGER NOT NULL,
			created_at   TEXT NOT NULL,
			FOREIGN KEY (group_id) REFERENCES mls_groups(id) ON DELETE CASCADE
		)`},
		{"008_mls_messages", `CREATE TABLE IF NOT EXISTS mls_messages (
			id            TEXT PRIMARY KEY,
			group_id      TEXT NOT NULL,
			sender_did    TEXT NOT NULL,
			ciphertext_b64 TEXT NOT NULL,
			content_type  TEXT NOT NULL DEFAULT 'application',
			epoch         INTEGER NOT NULL,
			created_at    TEXT NOT NULL,
			FOREIGN KEY (group_id) REFERENCES mls_groups(id) ON DELETE CASCADE
		)`},
		{"009_mls_messages_idx", "CREATE INDEX IF NOT EXISTS idx_mls_msg_group ON mls_messages(group_id, created_at DESC)"},
		// Welcome'lar offline alıcılar için kuyruğa alınır
		{"010_mls_welcome_queue", `CREATE TABLE IF NOT EXISTS mls_welcome_queue (
			id           TEXT PRIMARY KEY,
			group_id     TEXT NOT NULL,
			recipient_did TEXT NOT NULL,
			welcome_b64  TEXT NOT NULL,
			created_at   TEXT NOT NULL,
			delivered_at TEXT
		)`},
		{"011_mls_welcome_idx", "CREATE INDEX IF NOT EXISTS idx_mls_welcome_recipient ON mls_welcome_queue(recipient_did, delivered_at)"},
		// Cross-signing — multi-device (spec Bölüm 5.4)
		{"012_devices", `CREATE TABLE IF NOT EXISTS devices (
			id              TEXT PRIMARY KEY,
			user_did        TEXT NOT NULL,
			device_pubkey   TEXT NOT NULL,
			device_name     TEXT DEFAULT '',
			device_type     TEXT DEFAULT 'secondary',
			signed_by       TEXT,
			signature_b64   TEXT,
			created_at      TEXT NOT NULL,
			revoked_at      TEXT,
			last_seen_at    TEXT
		)`},
		{"013_devices_idx", "CREATE INDEX IF NOT EXISTS idx_devices_user ON devices(user_did, revoked_at)"},
		{"014_pairing_requests", `CREATE TABLE IF NOT EXISTS pairing_requests (
			id              TEXT PRIMARY KEY,
			user_did        TEXT NOT NULL,
			challenge       TEXT NOT NULL,
			new_device_pubkey TEXT NOT NULL,
			new_device_name TEXT DEFAULT '',
			status          TEXT DEFAULT 'pending',
			created_at      TEXT NOT NULL,
			expires_at      TEXT NOT NULL,
			approved_at     TEXT,
			signature_b64   TEXT
		)`},
		{"015_pairing_idx", "CREATE INDEX IF NOT EXISTS idx_pairing_user ON pairing_requests(user_did, status)"},
		// ZK nullifier table — replay protection across all circuits (security audit C1)
		{"016_zk_nullifiers", `CREATE TABLE IF NOT EXISTS zk_nullifiers (
			circuit_id   TEXT NOT NULL,
			nullifier    TEXT NOT NULL,
			user_did     TEXT NOT NULL,
			used_at      TEXT NOT NULL,
			PRIMARY KEY (circuit_id, nullifier)
		)`},
		{"017_zk_nullifiers_idx", "CREATE INDEX IF NOT EXISTS idx_zk_nullifiers_user ON zk_nullifiers(user_did, circuit_id)"},
		// Per-user credit_threshold binding commitment (security audit C3)
		// Client computes Poseidon(user_did_secret, BINDING_TAG) at registration,
		// uploads it once. credit_upgrade compares proof's user_hash to this stored value.
		{"018_users_credit_binding", "ALTER TABLE users ADD COLUMN credit_user_hash TEXT DEFAULT ''"},
		// ─── OBS TOKEN STATE LAYER (ADR-0010) ──────────────────────────────
		// Off-chain transparent ledger. A zk-Rollup will later settle to this.
		// Amounts stored as TEXT decimal strings (18 decimals → exceeds int64),
		// arithmetic via math/big.Int. See internal/token/token.go.
		{"019_obs_accounts", `CREATE TABLE IF NOT EXISTS obs_accounts (
			user_did            TEXT PRIMARY KEY,
			transparent_balance TEXT NOT NULL DEFAULT '0',
			updated_at          TEXT NOT NULL
		)`},
		{"020_obs_transactions", `CREATE TABLE IF NOT EXISTS obs_transactions (
			id         TEXT PRIMARY KEY,
			from_did   TEXT NOT NULL,
			to_did     TEXT NOT NULL,
			amount     TEXT NOT NULL,
			fee        TEXT NOT NULL DEFAULT '0',
			tx_type    TEXT NOT NULL,
			memo       TEXT DEFAULT '',
			status     TEXT NOT NULL DEFAULT 'confirmed',
			created_at TEXT NOT NULL
		)`},
		{"021_obs_tx_from_idx", "CREATE INDEX IF NOT EXISTS idx_obs_tx_from ON obs_transactions(from_did, created_at DESC)"},
		{"022_obs_tx_to_idx", "CREATE INDEX IF NOT EXISTS idx_obs_tx_to ON obs_transactions(to_did, created_at DESC)"},
		// Singleton supply row. Seeded with 1B total (ADR-0010), 0 circulating
		// until genesis mint runs. id is fixed to 1 to enforce singleton.
		{"023_obs_supply", `CREATE TABLE IF NOT EXISTS obs_supply (
			id           INTEGER PRIMARY KEY CHECK (id = 1),
			total_supply TEXT NOT NULL,
			circulating  TEXT NOT NULL DEFAULT '0',
			burned       TEXT NOT NULL DEFAULT '0',
			last_updated TEXT NOT NULL
		);
		INSERT OR IGNORE INTO obs_supply (id, total_supply, circulating, burned, last_updated)
			VALUES (1, '1000000000000000000000000000', '0', '0', datetime('now'));`},
		// Mini App Motoru (spec Bölüm 10) — FAZ 2 skeleton
		{"024_mini_apps", `CREATE TABLE IF NOT EXISTS mini_apps (
			id            TEXT PRIMARY KEY,
			name          TEXT NOT NULL,
			version       TEXT NOT NULL,
			developer_did TEXT NOT NULL,
			manifest_json TEXT NOT NULL,
			code_hash     TEXT NOT NULL,
			signed_by     TEXT DEFAULT '',
			status        TEXT NOT NULL DEFAULT 'pending',
			created_at    TEXT NOT NULL
		)`},
		{"025_mini_apps_idx", "CREATE INDEX IF NOT EXISTS idx_mini_apps_status ON mini_apps(status, created_at DESC)"},
		{"026_mini_app_installs", `CREATE TABLE IF NOT EXISTS mini_app_installs (
			user_did                  TEXT NOT NULL,
			app_id                    TEXT NOT NULL,
			granted_permissions_json  TEXT NOT NULL DEFAULT '[]',
			installed_at              TEXT NOT NULL,
			PRIMARY KEY (user_did, app_id)
		)`},
		{"027_mini_app_permissions_log", `CREATE TABLE IF NOT EXISTS mini_app_permissions_log (
			id         TEXT PRIMARY KEY,
			user_did   TEXT NOT NULL,
			app_id     TEXT NOT NULL,
			permission TEXT NOT NULL,
			action     TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`},
		{"028_mini_app_perm_log_idx", "CREATE INDEX IF NOT EXISTS idx_mini_app_perm_log ON mini_app_permissions_log(user_did, app_id, created_at DESC)"},
		// ─── STAKING + SLASHING (ADR-0011) ─────────────────────────────────
		// Off-chain staking ledger. Amounts are TEXT decimal strings (18
		// decimals, math/big.Int) — same convention as obs_accounts. Staked
		// principal is moved out of obs_accounts (locked) and back on withdraw.
		{"029_stakes", `CREATE TABLE IF NOT EXISTS stakes (
			id                   TEXT PRIMARY KEY,
			user_did             TEXT NOT NULL,
			amount               TEXT NOT NULL,
			stake_type           TEXT NOT NULL DEFAULT 'user',
			locked_until         TEXT NOT NULL,
			apy_bps              INTEGER NOT NULL DEFAULT 1000,
			status               TEXT NOT NULL DEFAULT 'active',
			created_at           TEXT NOT NULL,
			unstake_requested_at TEXT,
			withdrawn_at         TEXT
		)`},
		{"030_stakes_idx", "CREATE INDEX IF NOT EXISTS idx_stakes_user ON stakes(user_did, status)"},
		{"031_slash_events", `CREATE TABLE IF NOT EXISTS slash_events (
			id              TEXT PRIMARY KEY,
			user_did        TEXT NOT NULL,
			stake_id        TEXT NOT NULL,
			reason          TEXT NOT NULL,
			severity_pct    INTEGER NOT NULL,
			amount_slashed  TEXT NOT NULL DEFAULT '0',
			status          TEXT NOT NULL DEFAULT 'pending',
			reviewed_by     TEXT DEFAULT '',
			created_at      TEXT NOT NULL,
			applied_at      TEXT
		)`},
		{"032_slash_events_idx", "CREATE INDEX IF NOT EXISTS idx_slash_events_user ON slash_events(user_did, status)"},
		{"033_slash_events_stake_idx", "CREATE INDEX IF NOT EXISTS idx_slash_events_stake ON slash_events(stake_id)"},
		// Multisig review approvals — one row per reviewer per slash event.
		{"034_slash_reviews", `CREATE TABLE IF NOT EXISTS slash_reviews (
			slash_event_id TEXT NOT NULL,
			reviewer_did   TEXT NOT NULL,
			approve        INTEGER NOT NULL,
			created_at     TEXT NOT NULL,
			PRIMARY KEY (slash_event_id, reviewer_did)
		)`},
		{"035_node_uptime", `CREATE TABLE IF NOT EXISTS node_uptime (
			id           TEXT PRIMARY KEY,
			node_id      TEXT NOT NULL,
			user_did     TEXT NOT NULL,
			window_start TEXT NOT NULL,
			window_end   TEXT NOT NULL,
			uptime_pct   REAL NOT NULL,
			recorded_at  TEXT NOT NULL
		)`},
		{"036_node_uptime_idx", "CREATE INDEX IF NOT EXISTS idx_node_uptime_user ON node_uptime(user_did, recorded_at DESC)"},
		// ─── AIRDROP DISTRIBUTION (spec Bölüm 12.2 — ZK-gated, Sybil-resistant) ──
		// Admin creates a campaign with a fixed pool, per-claim amount and frozen
		// eligibility criteria. Each user claims once: Sybil resistance comes from
		// an identity_proof ZK proof (1 phone-verified DID = 1 identity) plus a
		// per-(campaign,identity) nullifier that blocks double-claims even across
		// devices. Claim mints OBS to the claimer's transparent balance.
		// See internal/airdrop/airdrop.go. Amounts are TEXT decimal strings
		// (18 decimals, math/big.Int) — same convention as obs_accounts.
		{"042_airdrop_campaigns", `CREATE TABLE IF NOT EXISTS airdrop_campaigns (
			id                   TEXT PRIMARY KEY,
			name                 TEXT NOT NULL,
			total_pool           TEXT NOT NULL,
			per_claim            TEXT NOT NULL,
			min_tier             INTEGER NOT NULL DEFAULT 1,
			min_account_age_days INTEGER NOT NULL DEFAULT 0,
			claims_count         INTEGER NOT NULL DEFAULT 0,
			max_claims           INTEGER NOT NULL,
			status               TEXT NOT NULL DEFAULT 'active',
			created_by           TEXT NOT NULL,
			created_at           TEXT NOT NULL,
			ends_at              TEXT NOT NULL
		)`},
		{"043_airdrop_claims", `CREATE TABLE IF NOT EXISTS airdrop_claims (
			id          TEXT PRIMARY KEY,
			campaign_id TEXT NOT NULL,
			user_did    TEXT NOT NULL,
			nullifier   TEXT NOT NULL UNIQUE,
			amount      TEXT NOT NULL,
			claimed_at  TEXT NOT NULL
		)`},
		{"044_airdrop_claims_campaign_idx", "CREATE INDEX IF NOT EXISTS idx_airdrop_claims_campaign ON airdrop_claims(campaign_id)"},
		// ─── GOVERNANCE — ZK VOTING (ADR-0012) ─────────────────────────────
		// Proposal lifecycle, anonymous ZK votes, tallies, voter snapshots.
		// See internal/governance/governance.go.
		{"045_proposals", `CREATE TABLE IF NOT EXISTS proposals (
			id                TEXT PRIMARY KEY,
			poll_id           TEXT NOT NULL UNIQUE,
			proposer_did      TEXT NOT NULL,
			title             TEXT NOT NULL,
			description       TEXT NOT NULL DEFAULT '',
			proposal_type     TEXT NOT NULL CHECK (proposal_type IN ('param','protocol')),
			execution_payload TEXT NOT NULL DEFAULT '',
			status            TEXT NOT NULL DEFAULT 'active'
				CHECK (status IN ('active','passed','rejected','vetoed','executed')),
			voting_ends_at    TEXT NOT NULL,
			timelock_ends_at  TEXT,
			quorum_required   TEXT NOT NULL DEFAULT '0',
			created_at        TEXT NOT NULL
		)`},
		{"046_proposals_status_idx", "CREATE INDEX IF NOT EXISTS idx_proposals_status ON proposals(status, voting_ends_at DESC)"},
		// proposal_votes — anonymous. NO voter_did column on purpose: a vote
		// cannot be linked back to an identity. The nullifier (UNIQUE) is the
		// only double-vote guard; it is derived inside the ZK circuit from the
		// voter secret + poll_id, so the voter cannot forge a second one.
		{"047_proposal_votes", `CREATE TABLE IF NOT EXISTS proposal_votes (
			id               TEXT PRIMARY KEY,
			proposal_id      TEXT NOT NULL,
			nullifier        TEXT NOT NULL UNIQUE,
			vote_commitment  TEXT NOT NULL,
			choice_encrypted TEXT NOT NULL,
			voter_root       TEXT NOT NULL,
			created_at       TEXT NOT NULL,
			FOREIGN KEY (proposal_id) REFERENCES proposals(id) ON DELETE CASCADE
		)`},
		{"048_proposal_votes_idx", "CREATE INDEX IF NOT EXISTS idx_proposal_votes_proposal ON proposal_votes(proposal_id)"},
		{"049_proposal_tallies", `CREATE TABLE IF NOT EXISTS proposal_tallies (
			proposal_id    TEXT PRIMARY KEY,
			yes_weight     TEXT NOT NULL DEFAULT '0',
			no_weight      TEXT NOT NULL DEFAULT '0',
			abstain_weight TEXT NOT NULL DEFAULT '0',
			veto_count     INTEGER NOT NULL DEFAULT 0,
			total_voters   INTEGER NOT NULL DEFAULT 0,
			finalized_at   TEXT NOT NULL,
			FOREIGN KEY (proposal_id) REFERENCES proposals(id) ON DELETE CASCADE
		)`},
		// governance_eligibility_snapshots — voter set frozen at proposal
		// creation. The ZK vote_proof must carry this exact merkle_root.
		{"050_governance_eligibility_snapshots", `CREATE TABLE IF NOT EXISTS governance_eligibility_snapshots (
			proposal_id    TEXT PRIMARY KEY,
			merkle_root    TEXT NOT NULL,
			voter_count    INTEGER NOT NULL DEFAULT 0,
			snapshot_at    TEXT NOT NULL,
			FOREIGN KEY (proposal_id) REFERENCES proposals(id) ON DELETE CASCADE
		)`},
		// ─── SIGNAL PROTOCOL SESSION STATE (Double Ratchet — spec Bölüm 4.3) ──
		// Backend is E2EE-blind: stores only opaque, client-encrypted blobs.
		// The server never derives or inspects ratchet keys.
		{"057_signal_sessions", `CREATE TABLE IF NOT EXISTS signal_sessions (
			id                TEXT PRIMARY KEY,
			owner_did         TEXT NOT NULL,
			peer_did          TEXT NOT NULL,
			session_state_b64 TEXT NOT NULL,
			created_at        TEXT NOT NULL,
			updated_at        TEXT NOT NULL,
			UNIQUE(owner_did, peer_did)
		)`},
		{"058_signal_sessions_idx", "CREATE INDEX IF NOT EXISTS idx_signal_sessions_owner ON signal_sessions(owner_did, updated_at DESC)"},
		// ─── MESSAGES: encryption metadata columns ───────────────────────────
		// is_encrypted = 1 by default (all messages must be ciphertext).
		// encryption_type distinguishes Signal (1-1) from MLS (group) and legacy.
		{"059_messages_is_encrypted", "ALTER TABLE messages ADD COLUMN is_encrypted INTEGER NOT NULL DEFAULT 1"},
		{"060_messages_encryption_type", "ALTER TABLE messages ADD COLUMN encryption_type TEXT NOT NULL DEFAULT 'signal'"},
		// ─── POST-QUANTUM imza anahtarı (Dilithium3 — FAZ 4) ─────────────────
		// Mesaj gönderiminde isteğe bağlı Dilithium3 imzası için public key.
		// Private key asla sunucuda saklanmaz; client imzalar, sunucu doğrular.
		{"055_users_dilithium_pub_key", "ALTER TABLE users ADD COLUMN dilithium_pub_key TEXT DEFAULT ''"},
		// Dilithium imzalı mesajlar için messages tablosuna ek kolon
		{"056_messages_dilithium_sig", "ALTER TABLE messages ADD COLUMN dilithium_sig TEXT DEFAULT ''"},
		// Dilithium3 private key — server-side auto-signing için (FAZ 4, prototype)
		{"068_users_dilithium_priv_key", "ALTER TABLE users ADD COLUMN dilithium_priv_key TEXT DEFAULT ''"},
		// ─── ZK-ID Kimlik Sistemi (Spec Bölüm 5.2-5.3) ───────────────────────
		// Client kayıt sırasında identity_proof.circom circuit'i ile ZK kanıt üretir.
		// Backend kanıtı doğrular, secret'i hiçbir zaman görmez.
		// zk_id_proof_b64: snarkjs Groth16 proof JSON (base64)
		// zk_id_public_params: publicSignals JSON dizisi (base64)
		// zk_id_verified: 1 = kanıt doğrulandı, 0 = henüz doğrulanmadı (backward compat)
		{"061_users_zk_id_proof", "ALTER TABLE users ADD COLUMN zk_id_proof_b64 TEXT"},
		{"062_users_zk_id_public_params", "ALTER TABLE users ADD COLUMN zk_id_public_params TEXT"},
		{"063_users_zk_id_verified", "ALTER TABLE users ADD COLUMN zk_id_verified INTEGER NOT NULL DEFAULT 0"},
		// ─── MESAJ DURUM SİSTEMİ (Spec Bölüm 6.4) ──────────────────────────────
		// messages tablosu createTables()'da zaten status/delivered_at/read_at
		// kolonlarını içeriyor; aşağıdaki migration'lar mevcut DB'lere idempotent
		// olarak ALTER TABLE ekler (duplicate column hatası tolere edilir).
		{"064_messages_status_col", "ALTER TABLE messages ADD COLUMN status TEXT NOT NULL DEFAULT 'sent'"},
		{"065_messages_delivered_at_col", "ALTER TABLE messages ADD COLUMN delivered_at TEXT"},
		{"066_messages_read_at_col", "ALTER TABLE messages ADD COLUMN read_at TEXT"},
		// İndeks: status bazlı sorgular (delivery ack güncelleme, expired temizleme)
		{"067_messages_status_idx", "CREATE INDEX IF NOT EXISTS idx_msg_status ON messages(status, expires_at)"},
		// ─── SHIELDED TRANSFER (spec Bölüm 8.3 — Gizli Transfer Akışı / ZK) ──
		// Aztec-inspired UTXO commitment scheme — simplified for FAZ 2:
		//   - shielded_notes:      append-only leaves; each leaf is a Poseidon
		//                          commitment (value, owner_pubkey, salt) opaque
		//                          to the server. The server stores leaves but
		//                          cannot read amounts or owners.
		//   - shielded_nullifiers: spent-note markers. UNIQUE blocks double-spend
		//                          even across concurrent transactions.
		//   - shielded_root:       single-row snapshot of the current Merkle root
		//                          + leaf count. The ZK proof's public root must
		//                          match this snapshot.
		//
		// FAZ 3 additions (NOT in scope here):
		//   - real Merkle inclusion proof inside the circuit (depth=32 path)
		//   - shielded→shielded with change note (1-in/2-out)
		//   - multi-asset notes
		// See internal/token/shielded.go.
		{"051_shielded_notes", `CREATE TABLE IF NOT EXISTS shielded_notes (
			id          TEXT PRIMARY KEY,
			leaf_index  INTEGER NOT NULL UNIQUE,
			commitment  TEXT NOT NULL,
			created_at  TEXT NOT NULL
		)`},
		{"052_shielded_notes_idx", "CREATE INDEX IF NOT EXISTS idx_shielded_notes_leaf ON shielded_notes(leaf_index)"},
		{"053_shielded_nullifiers", `CREATE TABLE IF NOT EXISTS shielded_nullifiers (
			nullifier  TEXT PRIMARY KEY,
			used_at    TEXT NOT NULL
		)`},
		// Singleton: id always = 1. Seeded with an "empty tree" root of "0" so
		// the first Shield call has something to update rather than insert.
		{"054_shielded_root", `CREATE TABLE IF NOT EXISTS shielded_root (
			id          INTEGER PRIMARY KEY CHECK (id = 1),
			root        TEXT NOT NULL DEFAULT '0',
			leaf_count  INTEGER NOT NULL DEFAULT 0,
			updated_at  TEXT NOT NULL
		);
		INSERT OR IGNORE INTO shielded_root (id, root, leaf_count, updated_at)
			VALUES (1, '0', 0, datetime('now'));`},
		// ─── ZK KREDİ CLAIM GEÇMİŞİ (Spec Bölüm 5.5) ─────────────────────────
		// Her proof tipi için cooldown periyoduna göre tekrar claim engeli.
		// UNIQUE(did, proof_type, valid_until): aynı kullanıcı aynı periyotta
		// aynı proof tipini iki kez claim edemez.
		{"064_credit_proof_claims", `CREATE TABLE IF NOT EXISTS credit_proof_claims (
			id             TEXT PRIMARY KEY,
			did            TEXT NOT NULL,
			proof_type     TEXT NOT NULL,
			proof_b64      TEXT NOT NULL,
			public_signals TEXT NOT NULL,
			points_awarded REAL NOT NULL,
			claimed_at     TEXT NOT NULL,
			valid_until    TEXT NOT NULL,
			UNIQUE(did, proof_type, valid_until)
		)`},
		{"065_credit_proof_claims_idx", "CREATE INDEX IF NOT EXISTS idx_credit_proof_claims_did ON credit_proof_claims(did, proof_type, valid_until)"},
		// ─── FİZİKSEL ENTEGRASYon (Spec Bölüm 11) ─────────────────────────────────
		// Etkinlik yönetimi + QR check-in + 1km grid konum keşfi
		// Koordinatlar int olarak saklanır (float hassasiyet sorunlarını önler):
		//   lat_int = lat * 1000  (3 ondalık hassasiyet, ~111m precision)
		//   lon_int = lon * 1000
		// grid_id = "lat_grid:lon_grid" where lat_grid = int(lat*100), lon_grid = int(lon*100)
		// ~1.11km x ~1.11km grid hücresi (ekvatorda ~1km)
		{"070_events", `CREATE TABLE IF NOT EXISTS events (
			id               TEXT PRIMARY KEY,
			creator_did      TEXT NOT NULL,
			title            TEXT NOT NULL,
			description      TEXT NOT NULL,
			location_name    TEXT NOT NULL,
			grid_id          TEXT NOT NULL,
			lat_int          INTEGER NOT NULL,
			lon_int          INTEGER NOT NULL,
			starts_at        TEXT NOT NULL,
			ends_at          TEXT NOT NULL,
			capacity         INTEGER,
			fee_obs          REAL NOT NULL DEFAULT 0,
			min_credit_tier  INTEGER NOT NULL DEFAULT 1,
			created_at       TEXT NOT NULL
		)`},
		{"071_events_grid_idx", "CREATE INDEX IF NOT EXISTS idx_events_grid ON events(grid_id)"},
		{"072_events_starts_idx", "CREATE INDEX IF NOT EXISTS idx_events_starts ON events(starts_at)"},
		{"073_event_attendees", `CREATE TABLE IF NOT EXISTS event_attendees (
			event_id          TEXT NOT NULL,
			attendee_did      TEXT NOT NULL,
			checked_in        INTEGER NOT NULL DEFAULT 0,
			checked_in_at     TEXT,
			zk_checkin_proof  TEXT,
			joined_at         TEXT NOT NULL,
			PRIMARY KEY (event_id, attendee_did)
		)`},
		{"074_event_attendees_event_idx", "CREATE INDEX IF NOT EXISTS idx_event_attendees_event ON event_attendees(event_id)"},
		{"075_event_attendees_did_idx", "CREATE INDEX IF NOT EXISTS idx_event_attendees_did ON event_attendees(attendee_did)"},
		// ZK anonim check-in (Spec Bölüm 11.1) — organizatör kimliği göremez
		{"080_event_attendees_anonymous", "ALTER TABLE event_attendees ADD COLUMN anonymous INTEGER NOT NULL DEFAULT 0"},
		// NFC nonce tablosu — replay attack koruması (Spec Bölüm 11.4)
		{"081_nfc_nonces", `CREATE TABLE IF NOT EXISTS nfc_nonces (
			nonce      TEXT PRIMARY KEY,
			created_at INTEGER NOT NULL
		)`},
		// Kullanıcı konum grid tablosu (Spec Bölüm 11.2) — tam koordinat ASLA saklanmaz
		{"082_user_locations", `CREATE TABLE IF NOT EXISTS user_locations (
			user_did   TEXT PRIMARY KEY,
			grid_id    TEXT NOT NULL,
			updated_at INTEGER NOT NULL
		)`},
		{"083_user_locations_grid_idx", "CREATE INDEX IF NOT EXISTS idx_user_locations_grid ON user_locations(grid_id)"},
		// ─── MİNİ APP OTURUM KAYITLARI ─────────────────────────────────────────
		{"076_mini_app_sessions", `CREATE TABLE IF NOT EXISTS mini_app_sessions (
			id          TEXT PRIMARY KEY,
			app_id      TEXT NOT NULL,
			user_did    TEXT NOT NULL,
			started_at  TEXT NOT NULL,
			ended_at    TEXT,
			process_id  INTEGER,
			exit_code   INTEGER,
			stdout      TEXT DEFAULT '',
			FOREIGN KEY (app_id) REFERENCES mini_apps(id) ON DELETE CASCADE
		)`},
		{"077_mini_app_sessions_idx", "CREATE INDEX IF NOT EXISTS idx_mini_app_sessions_user ON mini_app_sessions(user_did, app_id, started_at DESC)"},
		// ─── MİNİ APP KOD DEPOLAMA (MinIO object key) ──────────────────────────
		// code_object_key: MinIO bucket içindeki nesne anahtarı (miniapp/{id}/code.ts)
		// Yayınlama sırasında doldurulur; çalıştırma sırasında okunur.
		{"078_mini_apps_code_key", "ALTER TABLE mini_apps ADD COLUMN code_object_key TEXT DEFAULT ''"},
		// ─── SPK İMZA DOĞRULAMA (Ed25519 signing key) ──────────────────────────
		// signing_key: Ed25519 imzalama public key (Base64).
		// bundleToUpload() artık identity_key (X25519) ile birlikte signing_key de gönderiyor.
		// Backend signed_prekey_sig'i bu key ile doğrular.
		{"079_prekey_bundles_signing_key", "ALTER TABLE prekey_bundles ADD COLUMN signing_key TEXT DEFAULT ''"},
		// ─── MESAJ GERİ ALMA + SONA ERME (Spec Bölüm 6.4 / 6.6) ─────────────────
		// expired_at: scheduler tarafından doldurulan sona erme timestamp'i.
		//   expires_at (mevcut): mesajın silme eşiği (otomatik hesaplanmış)
		//   expired_at (yeni):   scheduler'ın işaretlediği an (status='expired' ile eş zamanlı)
		// recall_proof: geri alma sırasında sunulan ZK kanıtının SHA256 hex hash'i.
		//   Kanıtın kendisi sunucuda saklanmaz; sadece hash audit trail için tutulur.
		{"080_messages_expired_at", "ALTER TABLE messages ADD COLUMN expired_at TEXT"},
		{"081_messages_recall_proof", "ALTER TABLE messages ADD COLUMN recall_proof TEXT DEFAULT ''"},
		{"082_messages_recall_idx", "CREATE INDEX IF NOT EXISTS idx_msg_recall ON messages(recall_proof) WHERE recall_proof != ''"},
		// Expired sorgulama için bileşik index: scheduler'ın periyodik taraması
		// (status='sent'|'delivered', expires_at < NOW()) için optimize edilmiş.
		{"083_messages_expires_status_idx", "CREATE INDEX IF NOT EXISTS idx_msg_expires_status ON messages(expires_at, status) WHERE deleted_at IS NULL"},
		// ─── ZK PROOF ANOMALY LOG (replay/rate-limit tespiti) ────────────────
		// zk.ProofAnomalyDetector.RecordProof bu tabloya yazar. Önceden hiçbir
		// migration bu tabloyu oluşturmuyordu; detector başlatılınca ilk yazımda
		// "no such table: proof_log" hatası veriyordu. SQL'i zk paketinden
		// referans alıyoruz (tek doğruluk kaynağı).
		{"084_zk_proof_log", zk.ProofLogMigrationSQL},
		// ─── MİNİ APP ZK İZİN GRANT'LERİ (runtime consent) ────────────────────
		// miniapp bridge'i (handleZK / requestZkPermission) bu tabloyu okur/yazar.
		// Önceden tablo hiç oluşturulmuyordu; "no such table: app_zk_permissions"
		// hatasına yol açıyordu. Şema miniapp.MigrateZKPermissionsTable ile aynıdır.
		{"085_app_zk_permissions", `CREATE TABLE IF NOT EXISTS app_zk_permissions (
			app_id     TEXT NOT NULL,
			permission TEXT NOT NULL,
			granted_at INTEGER NOT NULL,
			PRIMARY KEY (app_id, permission)
		)`},
		// ─── IMMUTABLE ZK PROOF AUDIT LOG (spec Bölüm 7.3 Adım 5) ────────────
		// Doğrulanmış her ZK proof'u SHA-256 hash zinciriyle immutable olarak
		// kaydeder. Tam blockchain yerine local append-only log + chain hash.
		// proof_log (anomaly.go) tablosundan AYRI: bu tablo audit trail içindir,
		// anomaly log replay guard ve rate limit için kullanılır.
		// SQL canonical tanımı zk.ProofLogChainMigrationSQL sabitindedir.
		{"086_zk_proof_log_chain", zk.ProofLogChainMigrationSQL},
		// ─── GRUP MODERASYON SİSTEMİ ─────────────────────────────────────────
		// Kullanıcı başlatılan grup raporları + otomatik inceleme/askıya alma.
		// group_reports: her rapor kaydı (reporter, reason, zaman).
		// group_moderation: grup başına tek satır moderasyon durumu (reviewing/suspended).
		// 24 saatte 3+ rapor → is_reviewing=1 (otomatik). Askıya alma (is_suspended=1)
		// moderatör aksiyonu gerektirir (admin API — gelecek iterasyon).
		{"087_group_reports", `CREATE TABLE IF NOT EXISTS group_reports (
			id           TEXT PRIMARY KEY,
			group_id     TEXT NOT NULL,
			reporter_did TEXT NOT NULL,
			reason       TEXT NOT NULL,
			created_at   INTEGER NOT NULL
		)`},
		{"088_group_reports_idx", "CREATE INDEX IF NOT EXISTS idx_group_reports_group ON group_reports(group_id, created_at)"},
		{"089_group_moderation", `CREATE TABLE IF NOT EXISTS group_moderation (
			group_id     TEXT PRIMARY KEY,
			is_reviewing INTEGER NOT NULL DEFAULT 0,
			is_suspended INTEGER NOT NULL DEFAULT 0,
			report_count INTEGER NOT NULL DEFAULT 0,
			last_review  INTEGER NOT NULL
		)`},
		// ─── SCANNER: 7/24 TARAMA SİSTEMİ ──────────────────────────────────────
		// scanner_flags: kullanıcı başına tek bayrak satırı (ON CONFLICT upsert).
		// is_flagged / flag_reason: mesaj düzeyinde spam işaretleme.
		// ALTER TABLE hataları (kolon zaten varsa) migration runner tarafından tolere edilir.
		{"090_scanner_flags", `CREATE TABLE IF NOT EXISTS scanner_flags (
			user_did   TEXT PRIMARY KEY,
			reason     TEXT NOT NULL,
			flagged_at INTEGER NOT NULL
		)`},
		{"091_messages_is_flagged", "ALTER TABLE messages ADD COLUMN is_flagged INTEGER NOT NULL DEFAULT 0"},
		{"092_messages_flag_reason", "ALTER TABLE messages ADD COLUMN flag_reason TEXT DEFAULT ''"},
		{"093_bridge_transfers", `CREATE TABLE IF NOT EXISTS bridge_transfers (
			id                TEXT PRIMARY KEY,
			chain_from        TEXT NOT NULL,
			chain_to          TEXT NOT NULL,
			sender_address    TEXT NOT NULL,
			recipient_address TEXT NOT NULL,
			amount            TEXT NOT NULL,
			tx_hash           TEXT NOT NULL DEFAULT '',
			status            TEXT NOT NULL DEFAULT 'pending',
			created_at        DATETIME NOT NULL,
			confirmed_at      DATETIME
		)`},
		{"094_bridge_transfers_sender_idx", "CREATE INDEX IF NOT EXISTS idx_bridge_transfers_sender ON bridge_transfers(sender_address, created_at DESC)"},
		{"095_bridge_transfers_status_idx", "CREATE INDEX IF NOT EXISTS idx_bridge_transfers_status ON bridge_transfers(status, created_at DESC)"},
		// Bridge madde 4, PARÇA 3: ETH Locked event -> DOT transfer bağlama.
		// log_index, aynı tx_hash içindeki birden fazla Locked event'ini
		// (tx_hash tek başına yeterli DEĞİL) ayırt etmek için zorunlu —
		// idempotency kimliği tx_hash+log_index'in ikisine birden dayanır.
		{"133_bridge_transfers_log_index", "ALTER TABLE bridge_transfers ADD COLUMN log_index TEXT NOT NULL DEFAULT ''"},
		{"134_bridge_transfers_dot_tx_hash", "ALTER TABLE bridge_transfers ADD COLUMN dot_tx_hash TEXT NOT NULL DEFAULT ''"},
		{"135_bridge_transfers_retry_count", "ALTER TABLE bridge_transfers ADD COLUMN retry_count INTEGER NOT NULL DEFAULT 0"},
		{"136_bridge_transfers_error_message", "ALTER TABLE bridge_transfers ADD COLUMN error_message TEXT NOT NULL DEFAULT ''"},
		{"137_bridge_transfers_dot_extrinsic_hex", "ALTER TABLE bridge_transfers ADD COLUMN dot_extrinsic_hex TEXT NOT NULL DEFAULT ''"},
		// tx_hash+log_index DB düzeyinde de benzersiz — uygulama katmanındaki
		// idempotency kontrolüne ek savunma (çift INSERT bu index'e çarpıp hata verir).
		{"138_bridge_transfers_txhash_logindex_uidx", "CREATE UNIQUE INDEX IF NOT EXISTS idx_bridge_transfers_txhash_logindex ON bridge_transfers(tx_hash, log_index)"},
		{"139_bridge_transfers_dot_amount_planck", "ALTER TABLE bridge_transfers ADD COLUMN dot_amount_planck TEXT NOT NULL DEFAULT ''"},
		// BFT konsensüs — commit edilen bloklar (ADIM 6, bkz. ADR-0017). height
		// PRIMARY KEY: aynı height iki kez commit edilemez (idempotency, ADIM 7
		// için de savunma). parentHash artık buradan okunacak — engine.go'daki
		// eski "parent_%d" sahte placeholder yerine.
		{"140_consensus_blocks", `CREATE TABLE IF NOT EXISTS consensus_blocks (
			height       INTEGER PRIMARY KEY,
			round        INTEGER NOT NULL,
			parent_hash  TEXT NOT NULL,
			tx_root      TEXT NOT NULL,
			proposer     TEXT NOT NULL,
			block_hash   TEXT NOT NULL,
			block_ts     INTEGER NOT NULL,
			committed_at TEXT NOT NULL
		)`},
		// ADIM 7 (ADR-0017, "sonradan-tasdik" deseni): zaten senkron uygulanmış
		// ledger operasyonlarının (token.Transfer/Mint tx-id'leri) hangi BFT
		// bloğunda mutabık kalındığını kaydeden audit-log. op_id TEK BAŞINA
		// PRIMARY KEY — bu, bir operasyonun birden fazla yükseklikte/bloğunda
		// tekrar tasdik edilmesini (replay) veritabanı seviyesinde engeller.
		{"141_consensus_block_ops", `CREATE TABLE IF NOT EXISTS consensus_block_ops (
			op_id       TEXT PRIMARY KEY,
			height      INTEGER NOT NULL,
			recorded_at TEXT NOT NULL
		)`},
		{"142_consensus_block_ops_height_idx", "CREATE INDEX IF NOT EXISTS idx_consensus_block_ops_height ON consensus_block_ops(height)"},
		// N3: Timelocked DID rotation — prevents attacker from immediately locking out
		// original user after temporary account access. 7-day window allows cancellation.
		{"096_binding_rotations", `CREATE TABLE IF NOT EXISTS binding_rotations (
			id           TEXT PRIMARY KEY,
			old_did      TEXT NOT NULL,
			new_did      TEXT NOT NULL,
			new_did_proof TEXT NOT NULL DEFAULT '',
			requested_at TEXT NOT NULL,
			unlock_at    TEXT NOT NULL,
			status       TEXT NOT NULL DEFAULT 'pending',
			applied_at   TEXT,
			cancelled_at TEXT
		)`},
		{"097_binding_rotations_idx", "CREATE INDEX IF NOT EXISTS idx_binding_rotations_did ON binding_rotations(old_did, status)"},
		// ─── AKTİF ARAMALAR (WebRTC sinyalizasyon durumu) ─────────────────────────
		// call_handlers.go'nun kullandığı tablo. Yoksa callerDID boş kalır →
		// call_answer WS mesajı iletilemez → WebRTC hiç bağlanamaz.
		{"098_active_calls", `CREATE TABLE IF NOT EXISTS active_calls (
			id         TEXT PRIMARY KEY,
			caller_did TEXT NOT NULL,
			callee_did TEXT NOT NULL,
			status     TEXT NOT NULL DEFAULT 'ringing',
			sdp_offer  TEXT NOT NULL DEFAULT '',
			sdp_answer TEXT DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`},
		{"099_active_calls_caller_idx", "CREATE INDEX IF NOT EXISTS idx_active_calls_caller ON active_calls(caller_did, status)"},
		{"100_active_calls_callee_idx", "CREATE INDEX IF NOT EXISTS idx_active_calls_callee ON active_calls(callee_did, status)"},
		// ─── KONUŞMA TİPİ + AÇIKLAMA (kanal/topluluk desteği) ────────────────────
		{"101_conversations_conv_type", "ALTER TABLE conversations ADD COLUMN conv_type TEXT NOT NULL DEFAULT 'direct'"},
		{"102_conversations_description", "ALTER TABLE conversations ADD COLUMN description TEXT DEFAULT ''"},
		{"103_conversations_is_public", "ALTER TABLE conversations ADD COLUMN is_public INTEGER NOT NULL DEFAULT 0"},
		{"104_contacts", `CREATE TABLE IF NOT EXISTS contacts (
			id          TEXT PRIMARY KEY,
			owner_did   TEXT NOT NULL,
			contact_did TEXT NOT NULL,
			nickname    TEXT NOT NULL DEFAULT '',
			created_at  TEXT NOT NULL,
			UNIQUE(owner_did, contact_did)
		)`},
		{"105_contacts_idx", "CREATE INDEX IF NOT EXISTS idx_contacts_owner ON contacts(owner_did)"},
		{"106_users_bio", "ALTER TABLE users ADD COLUMN bio TEXT NOT NULL DEFAULT ''"},
		{"107_users_hide_online", "ALTER TABLE users ADD COLUMN hide_online INTEGER NOT NULL DEFAULT 0"},
		// ─── GELİŞMİŞ DAVET LİNKİ SİSTEMİ ─────────────────────────────────────────
		// invite_links: slug (görünen ad), max_uses, max_members, expires_at destekli
		// token: güvenli UUID (tahmin edilemez). slug: kullanıcı tanımlı, opsiyonel.
		{"108_invite_links", `CREATE TABLE IF NOT EXISTS invite_links (
			id          TEXT PRIMARY KEY,
			conv_id     TEXT NOT NULL,
			token       TEXT NOT NULL UNIQUE,
			slug        TEXT UNIQUE,
			max_uses    INTEGER NOT NULL DEFAULT 0,
			used_count  INTEGER NOT NULL DEFAULT 0,
			max_members INTEGER NOT NULL DEFAULT 0,
			expires_at  TEXT,
			created_by  TEXT NOT NULL,
			created_at  TEXT NOT NULL,
			FOREIGN KEY (conv_id) REFERENCES conversations(id)
		)`},
		{"109_invite_links_token_idx", "CREATE INDEX IF NOT EXISTS idx_invite_links_token ON invite_links(token)"},
		{"110_invite_links_slug_idx", "CREATE INDEX IF NOT EXISTS idx_invite_links_slug ON invite_links(slug) WHERE slug IS NOT NULL"},
		// ─── ABONE KİMLİK KATMANI (identity at the door) ──────────────────────────
		// phone_migrated: users satırının plaintext telefonunun ayrı, şifreli
		// subscriber.db kimlik deposuna taşınıp taşınmadığını izler. Backfill
		// internal/subscriber.MigratePhoneToSubscriberStore tarafından yapılır
		// (her boot'ta idempotent). Bu kolonun eklenmesi database.go'ya yapılan
		// tek subscriber değişikliğidir.
		{"111_users_phone_migrated", "ALTER TABLE users ADD COLUMN phone_migrated INTEGER DEFAULT 0"},

		// ─── DENETİM VE TOPLULUK KATMANI — Oturum 1 (şikayet/kanıt/kademeli ceza) ──
		// Tasarım: docs/spec/obscura_denetim_topluluk_katmani.md Bölüm 2,3,4.
		// spam_reports genişletiliyor (yeni paralel tablo yok — DRY): kanıt alanları
		// eklendi, artık kanıtsız şikayet kabul edilmiyor (Bölüm 2.2).
		{"112_spam_reports_message_id", "ALTER TABLE spam_reports ADD COLUMN message_id TEXT DEFAULT ''"},
		{"113_spam_reports_evidence_screenshot", "ALTER TABLE spam_reports ADD COLUMN evidence_screenshot_url TEXT DEFAULT ''"},
		{"114_spam_reports_evidence_hash", "ALTER TABLE spam_reports ADD COLUMN evidence_ciphertext_hash TEXT DEFAULT ''"},
		{"115_spam_reports_evidence_verified", "ALTER TABLE spam_reports ADD COLUMN evidence_verified INTEGER DEFAULT 0"},
		{"116_spam_reports_verdict", "ALTER TABLE spam_reports ADD COLUMN verdict TEXT DEFAULT ''"},
		{"117_spam_reports_category", "ALTER TABLE spam_reports ADD COLUMN category TEXT DEFAULT ''"},

		// user_violations: kademeli cezanın TEK kaynağı (Bölüm 3). credit_score/
		// users.tier (Bölüm 5 kilit-açma) ayrı eksen, bu tabloya dokunmaz.
		{"118_user_violations", `CREATE TABLE IF NOT EXISTS user_violations (
			id              TEXT PRIMARY KEY,
			user_did        TEXT NOT NULL,
			category        TEXT NOT NULL,
			tier            INTEGER NOT NULL,
			action          TEXT NOT NULL,
			source_report_id TEXT DEFAULT '',
			created_at      TEXT NOT NULL,
			expires_at      TEXT
		)`},
		{"119_user_violations_idx", "CREATE INDEX IF NOT EXISTS idx_user_violations_did ON user_violations(user_did, created_at DESC)"},

		// complainant_credibility: Bölüm 4 — şikayetçi geçmişi, ağırlık zamanla düşer.
		{"120_complainant_credibility", `CREATE TABLE IF NOT EXISTS complainant_credibility (
			user_did     TEXT PRIMARY KEY,
			upheld_count INTEGER NOT NULL DEFAULT 0,
			false_count  INTEGER NOT NULL DEFAULT 0,
			weight       REAL NOT NULL DEFAULT 1.0,
			updated_at   TEXT NOT NULL
		)`},

		// review_queue: İlke 5 — bariz olmayan her şey insan incelemesine düşer.
		{"121_review_queue", `CREATE TABLE IF NOT EXISTS review_queue (
			id         TEXT PRIMARY KEY,
			report_id  TEXT NOT NULL,
			reason     TEXT NOT NULL,
			status     TEXT NOT NULL DEFAULT 'pending',
			created_at TEXT NOT NULL
		)`},
		{"122_review_queue_idx", "CREATE INDEX IF NOT EXISTS idx_review_queue_status ON review_queue(status, created_at)"},
		// ─── SELF-DESTRUCT MESAJLAR (spec dışı, kullanıcı seçimli) ────────────
		// expires_at (mevcut, satır ~683 HandleSendMessage) TÜM mesajlara otomatik
		// uygulanan sabit 30 gün saklama TTL'idir — bununla KARIŞMAMASI için ayrı.
		// self_destruct_seconds: NULL = devre dışı, 0 = "okununca" (read_at anında
		//   silinir), >0 = gönderimden N saniye sonra (10/60/300/3600).
		// self_destruct_at: hesaplanmış mutlak silme zamanı. Sabit süre modunda
		//   gönderimde dolar; "okununca" modunda read_at set edilene kadar NULL
		//   kalır, okunduğunda dolar (bkz. okuma alındısı handler'ı).
		{"123_messages_self_destruct_seconds", "ALTER TABLE messages ADD COLUMN self_destruct_seconds INTEGER"},
		{"124_messages_self_destruct_at", "ALTER TABLE messages ADD COLUMN self_destruct_at TEXT"},
		{"125_messages_self_destruct_idx", "CREATE INDEX IF NOT EXISTS idx_msg_self_destruct ON messages(self_destruct_at) WHERE self_destruct_at IS NOT NULL AND deleted_at IS NULL"},
		// ─── MADDE 13: PANİK BUTONU / BULUŞMA GÜVENLİĞİ ────────────────────────
		// user_locations (082/083) hiçbir handler tarafından hiç kullanılmamış
		// ölü şemaydı — İlke 6 (operatör konum tutmaz) ihlal etmiyordu (grid_id
		// dışında kolon yoktu) ama kullanılmadığı için sessizce ilkeyi delme
		// riski taşıyordu. Kaldırıldı; panik/buluşma konumu hiçbir zaman
		// sunucuda saklanmaz, yalnızca uçtan uca şifreli mesaj içinde taşınır.
		{"126_drop_user_locations", "DROP TABLE IF EXISTS user_locations"},
		// contacts.is_trusted — güven kişisi işareti. Varsayılan 0: güven
		// varsayılan DEĞİLDİR, kullanıcı elle seçmeden kimse güven kişisi olmaz.
		{"127_contacts_is_trusted", "ALTER TABLE contacts ADD COLUMN is_trusted INTEGER NOT NULL DEFAULT 0"},
		// Madde 15, Adım 7: sealed mesajlarda from_did boş olduğundan
		// (kalıcı saklanmıyor) fromDID != user.DID yetki kontrolü kırılıyordu.
		// owner_hash = HMAC-SHA256(pepper, DID+":"+msgID) — karşılaştırılabilir
		// ama tersine çevrilemez (bkz. ADR-0016). Eski/zarfsız mesajlarda boş
		// kalır, mevcut fromDID karşılaştırması AYNEN kullanılmaya devam eder.
		{"128_messages_owner_hash", "ALTER TABLE messages ADD COLUMN owner_hash TEXT DEFAULT ''"},
		// ODI (Obscura Display Identifier) — DID'in SHA-256 hash'inden
		// deterministik/tek yönlü türetilen, kullanıcıya gösterilen kimlik
		// kodu (bkz. internal/identity/odi.go). DID hiçbir ekranda ham
		// gösterilmez; backend/API/state DID'i kullanmaya devam eder, odi
		// sadece görüntüleme + "ODI ile bul" akışı içindir.
		{"129_users_odi", "ALTER TABLE users ADD COLUMN odi TEXT"},
		{"130_users_odi_idx", "CREATE UNIQUE INDEX IF NOT EXISTS idx_users_odi ON users(odi) WHERE odi IS NOT NULL"},
		// phone_visible — kullanıcının telefon numarasını profilinde
		// göstermeyi seçip seçmediği. Varsayılan 0 (gizli): telefon zaten
		// hiçbir API response'unda dönmüyordu, bu kolon ileride şartlı
		// expose etmek için (Bölüm B, Telegram deseni) eklendi.
		{"131_users_phone_visible", "ALTER TABLE users ADD COLUMN phone_visible INTEGER NOT NULL DEFAULT 0"},
		// conv_members PK'si (conv_id, user_did) — user_did PK'nin İKİNCİ
		// kolonu olduğu için GET /v1/conversations'daki `cm.user_did = ?`
		// filtresi bu index'i kullanamıyordu, tüm conv_members'ı tarıyordu.
		// En sık çağrılan endpoint bu — sistem büyüdükçe HERKES için
		// yavaşlıyordu (kullanıcının kendi satır sayısından bağımsız).
		{"132_conv_members_user_did_idx", "CREATE INDEX IF NOT EXISTS idx_conv_members_user_did ON conv_members(user_did)"},
		// review_queue rebuild — Umay (spec Bölüm 1.4) otomatik-tarama bulgularının
		// da bu kuyruğa düşebilmesi için report_id artık zorunlu değil (auto_scan
		// kaynaklı satırlarda NULL kalır) ve source hangi akıştan geldiğini ayırt
		// eder. SQLite ALTER COLUMN ile NOT NULL kaldırılamadığı için tablo
		// rebuild gerekiyor (ADD COLUMN yeterli olmazdı). 143-147 tek bir mantıksal
		// değişikliğin ayrı, tek-statement adımlarıdır — mevcut migration
		// deseniyle (bkz. 123/124/125) tutarlı. Numaralandırma 143'ten başlıyor:
		// dosyada (satır ~641) commit'lenmemiş bridge/consensus işi 133-142'yi
		// zaten kullanmış durumda (id string'leri farklı olduğu için çakışma
		// yok, ama okunurluk için sıradaki boş numaradan devam edildi).
		{"143_review_queue_rebuild_new", `CREATE TABLE IF NOT EXISTS review_queue_new (
			id         TEXT PRIMARY KEY,
			report_id  TEXT,
			reason     TEXT NOT NULL,
			status     TEXT NOT NULL DEFAULT 'pending',
			source     TEXT NOT NULL DEFAULT 'user_report',
			created_at TEXT NOT NULL
		)`},
		{"144_review_queue_copy", `INSERT INTO review_queue_new (id, report_id, reason, status, source, created_at)
			SELECT id, report_id, reason, status, 'user_report', created_at FROM review_queue`},
		{"145_review_queue_drop_old", "DROP TABLE review_queue"},
		{"146_review_queue_rename", "ALTER TABLE review_queue_new RENAME TO review_queue"},
		{"147_review_queue_status_idx", "CREATE INDEX IF NOT EXISTS idx_review_queue_status ON review_queue(status, created_at)"},
		// Marketplace (spec Bölüm 5.2 Katman 3) — native listing + purchase
		// tabloları. price/amount TEXT decimal string, obs_transactions.amount
		// ile aynı konvansiyon (big.Int, int64'ü aşıyor).
		{"148_marketplace_listings", `CREATE TABLE IF NOT EXISTS marketplace_listings (
			id           TEXT PRIMARY KEY,
			seller_did   TEXT NOT NULL,
			title        TEXT NOT NULL,
			description  TEXT NOT NULL DEFAULT '',
			price        TEXT NOT NULL,
			currency     TEXT NOT NULL DEFAULT 'OBS',
			category     TEXT NOT NULL,
			status       TEXT NOT NULL DEFAULT 'active',
			created_at   TEXT NOT NULL,
			updated_at   TEXT NOT NULL
		)`},
		{"149_marketplace_listings_seller_idx", "CREATE INDEX IF NOT EXISTS idx_marketplace_listings_seller ON marketplace_listings(seller_did, created_at DESC)"},
		{"150_marketplace_listings_status_idx", "CREATE INDEX IF NOT EXISTS idx_marketplace_listings_status ON marketplace_listings(status, created_at DESC)"},
		// token_tx_id -> obs_transactions.id (token.Transfer'ın döndürdüğü id).
		// Transfer kendi DB tx'ini yönetiyor (MaxOpenConns(1) altında iç içe tx
		// deadlock olur, bkz. internal/airdrop/airdrop.go:404-412 aynı gerekçe)
		// — bu yüzden marketplace_transactions satırı Transfer başarılı
		// DÖNDÜKTEN SONRA ayrı bir INSERT ile yazılır.
		{"151_marketplace_transactions", `CREATE TABLE IF NOT EXISTS marketplace_transactions (
			id          TEXT PRIMARY KEY,
			listing_id  TEXT NOT NULL,
			buyer_did   TEXT NOT NULL,
			seller_did  TEXT NOT NULL,
			amount      TEXT NOT NULL,
			token_tx_id TEXT NOT NULL,
			status      TEXT NOT NULL DEFAULT 'completed',
			created_at  TEXT NOT NULL
		)`},
		{"152_marketplace_transactions_listing_idx", "CREATE INDEX IF NOT EXISTS idx_marketplace_tx_listing ON marketplace_transactions(listing_id, created_at DESC)"},
		{"153_marketplace_transactions_buyer_idx", "CREATE INDEX IF NOT EXISTS idx_marketplace_tx_buyer ON marketplace_transactions(buyer_did, created_at DESC)"},
		// listing_id — spam_reports'un bir ilanı hedef alabilmesi için.
		// message_id (migration 112) ile aynı idiom: nullable FK yerine
		// DEFAULT '' (mevcut desen, review_queue'daki nullable report_id'den
		// FARKLI — burada rebuild gerekmiyor, düz ADD COLUMN yeterli).
		{"154_spam_reports_listing_id", "ALTER TABLE spam_reports ADD COLUMN listing_id TEXT DEFAULT ''"},
		// Admin inceleme arayüzü (İlke 5: sistem ön eleyici, ciddi kararı insan
		// verir — bkz. docs/spec/obscura_denetim_topluluk_katmani.md Bölüm 0).
		// target_type/target_id: auto_scan kayıtlarında reason alanına gömülü
		// kesilmiş 8-karakter ID (bkz. umay/notify.go truncate) gerçek bir
		// aksiyon almak için yetersizdi — artık tam ID ayrı kolonlarda.
		// user_report kayıtlarında (source='user_report') bu kolonlar boş
		// kalabilir; gerçek hedef zaten report_id üzerinden spam_reports'a
		// (message_id/listing_id) JOIN ile çözülüyor.
		{"155_review_queue_target", "ALTER TABLE review_queue ADD COLUMN target_type TEXT DEFAULT ''"},
		{"156_review_queue_target_id", "ALTER TABLE review_queue ADD COLUMN target_id TEXT DEFAULT ''"},
		{"157_review_queue_resolved_at", "ALTER TABLE review_queue ADD COLUMN resolved_at TEXT"},
		{"158_review_queue_resolved_by", "ALTER TABLE review_queue ADD COLUMN resolved_by TEXT"},
		{"159_review_queue_resolution", "ALTER TABLE review_queue ADD COLUMN resolution TEXT"},
	}

	for _, m := range migrations {
		var applied string
		err := DB.QueryRow("SELECT id FROM _migrations WHERE id = ?", m.id).Scan(&applied)
		if err == nil {
			continue // Zaten uygulandı
		}
		// Run migration. Tolerate "duplicate column" / "table exists" errors
		// (idempotent ALTER/CREATE). Fail loudly for other errors so we don't
		// silently mask broken schema.
		_, execErr := DB.Exec(m.sql)
		if execErr != nil {
			msg := execErr.Error()
			if !(contains(msg, "duplicate column") || contains(msg, "already exists")) {
				return fmt.Errorf("migration %s failed: %w", m.id, execErr)
			}
		}
		// Only record as applied when SQL ran (or was idempotently a no-op)
		if _, err := DB.Exec("INSERT OR IGNORE INTO _migrations (id, applied_at) VALUES (?, datetime('now'))", m.id); err != nil {
			return fmt.Errorf("migration %s record failed: %w", m.id, err)
		}
		log.Printf("🔄 Migration: %s", m.id)
	}
	return nil
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func createTables() error {
	schema := `
	-- KULLANICILAR
	CREATE TABLE IF NOT EXISTS users (
		id            TEXT PRIMARY KEY,
		phone         TEXT UNIQUE NOT NULL,
		username      TEXT UNIQUE,
		display_name  TEXT,
		did           TEXT UNIQUE NOT NULL,
		identity_key  TEXT,
		avatar_url    TEXT DEFAULT '',
		tier          INTEGER DEFAULT 1,
		credit_score  REAL DEFAULT 0,
		is_active     INTEGER DEFAULT 1,
		is_banned     INTEGER DEFAULT 0,
		ban_expires_at TEXT,
		node_id       TEXT DEFAULT '',
		fcm_token     TEXT DEFAULT '',
		apns_token    TEXT DEFAULT '',
		created_at    TEXT NOT NULL,
		updated_at    TEXT NOT NULL,
		last_seen_at  TEXT NOT NULL
	);
	-- Mevcut tabloları migration ile güncelle (idempotent)
	-- SQLite ALTER TABLE sadece ADD COLUMN destekler
	CREATE TABLE IF NOT EXISTS _migrations (id TEXT PRIMARY KEY, applied_at TEXT);


	-- OTP
	CREATE TABLE IF NOT EXISTS otp_records (
		id         TEXT PRIMARY KEY,
		phone      TEXT NOT NULL,
		code       TEXT NOT NULL,
		attempts   INTEGER DEFAULT 0,
		expires_at TEXT NOT NULL,
		used       INTEGER DEFAULT 0,
		created_at TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_otp_phone ON otp_records(phone);

	-- OTP kilit — 3 hatalı denemeden sonra telefonu 15dk kilitler. Ayrı tablo
	-- çünkü otp_records her yeni OTP isteğinde silinir; kilidin yeni OTP
	-- talebiyle sıfırlanmaması (bypass) için lockout state OTP kaydından bağımsız.
	CREATE TABLE IF NOT EXISTS otp_lockouts (
		phone        TEXT PRIMARY KEY,
		locked_until TEXT NOT NULL
	);

	-- KONUŞMALAR
	CREATE TABLE IF NOT EXISTS conversations (
		id            TEXT PRIMARY KEY,
		is_group      INTEGER DEFAULT 0,
		name          TEXT DEFAULT '',
		avatar_url    TEXT DEFAULT '',
		last_msg_id   TEXT DEFAULT '',
		last_msg_text TEXT DEFAULT '',
		last_msg_at   TEXT,
		created_at    TEXT NOT NULL,
		updated_at    TEXT NOT NULL
	);

	-- KONUŞMA ÜYELERİ
	CREATE TABLE IF NOT EXISTS conv_members (
		conv_id     TEXT NOT NULL,
		user_did    TEXT NOT NULL,
		role        TEXT DEFAULT 'member',
		unread_count INTEGER DEFAULT 0,
		joined_at   TEXT NOT NULL,
		muted_until TEXT,
		PRIMARY KEY (conv_id, user_did),
		FOREIGN KEY (conv_id) REFERENCES conversations(id) ON DELETE CASCADE
	);

	-- MESAJLAR
	CREATE TABLE IF NOT EXISTS messages (
		id           TEXT PRIMARY KEY,
		conv_id      TEXT NOT NULL,
		from_did     TEXT NOT NULL,
		to_did       TEXT NOT NULL,
		type         TEXT DEFAULT 'text',
		ciphertext   TEXT NOT NULL,
		media_url    TEXT DEFAULT '',
		status       TEXT DEFAULT 'sent',
		is_group     INTEGER DEFAULT 0,
		reply_to_id  TEXT DEFAULT '',
		sent_at      TEXT NOT NULL,
		delivered_at TEXT,
		read_at      TEXT,
		expires_at   TEXT NOT NULL,
		deleted_at   TEXT,
		FOREIGN KEY (conv_id) REFERENCES conversations(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_msg_conv ON messages(conv_id, sent_at DESC);
	CREATE INDEX IF NOT EXISTS idx_msg_to ON messages(to_did, status);

	-- KREDİ OLAYLAR
	CREATE TABLE IF NOT EXISTS credit_events (
		id         TEXT PRIMARY KEY,
		user_did   TEXT NOT NULL,
		event_type TEXT NOT NULL,
		delta      REAL NOT NULL,
		reason     TEXT NOT NULL,
		new_score  REAL NOT NULL,
		created_at TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_credit_user ON credit_events(user_did, created_at DESC);

	-- ZK PROOF'LAR
	CREATE TABLE IF NOT EXISTS zk_proofs (
		id            TEXT PRIMARY KEY,
		user_did      TEXT NOT NULL,
		circuit_id    TEXT NOT NULL,
		proof_data    TEXT NOT NULL,
		public_inputs TEXT NOT NULL,
		verified      INTEGER DEFAULT 0,
		created_at    TEXT NOT NULL,
		expires_at    TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_zk_user ON zk_proofs(user_did, circuit_id);

	-- PREKEY BUNDLE'LAR (X3DH için — her kullanıcı için en son bundle)
	CREATE TABLE IF NOT EXISTS prekey_bundles (
		did               TEXT PRIMARY KEY,
		identity_key      TEXT NOT NULL,
		signed_prekey     TEXT NOT NULL,
		signed_prekey_sig TEXT NOT NULL,
		signed_prekey_id  INTEGER DEFAULT 0,
		updated_at        TEXT NOT NULL
	);

	-- TEK KULLANIMLIK PREYKEY'LER (OPK pool — sunucu bir kez dağıtır, sonra siler)
	CREATE TABLE IF NOT EXISTS one_time_prekeys (
		id         TEXT PRIMARY KEY,
		did        TEXT NOT NULL,
		opk_id     INTEGER NOT NULL,
		public_key TEXT NOT NULL,
		used       INTEGER DEFAULT 0,
		created_at TEXT NOT NULL,
		used_at    TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_opk_did ON one_time_prekeys(did, used);

	-- SPAM RAPORLAR
	CREATE TABLE IF NOT EXISTS spam_reports (
		id          TEXT PRIMARY KEY,
		reporter_did TEXT NOT NULL,
		reported_did TEXT NOT NULL,
		reason      TEXT,
		status      TEXT DEFAULT 'pending',
		created_at  TEXT NOT NULL,
		reviewed_at TEXT
	);

	-- GÜNLÜK AKTİVİTE (kredi hesaplama)
	CREATE TABLE IF NOT EXISTS daily_activity (
		user_did    TEXT NOT NULL,
		date        TEXT NOT NULL,
		msg_count   INTEGER DEFAULT 0,
		call_count  INTEGER DEFAULT 0,
		login_count INTEGER DEFAULT 0,
		PRIMARY KEY (user_did, date)
	);
	`

	_, err := DB.Exec(schema)
	return err
}

func Close() {
	if DB != nil {
		DB.Close()
	}
}
