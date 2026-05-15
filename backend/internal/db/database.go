package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
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

	// SQLite: tek bağlantı — yazma serileştirme (WAL okuma eşzamanlılığı için yeterli)
	// Nested query deadlock'u önlemek için tüm handler'lar tek sorgu kullanmalı
	DB.SetMaxOpenConns(1)
	DB.SetMaxIdleConns(1)

	// Busy timeout ayarla
	if _, err := DB.Exec("PRAGMA busy_timeout = 10000"); err != nil {
		log.Printf("⚠️ busy_timeout ayarlanamadı: %v", err)
	}

	if err := createTables(); err != nil {
		return fmt.Errorf("tablolar oluşturulamadı: %w", err)
	}
	if err := runMigrations(); err != nil {
		return fmt.Errorf("migration hatası: %w", err)
	}

	log.Printf("✅ Veritabanı hazır: %s", dbPath)
	return nil
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
