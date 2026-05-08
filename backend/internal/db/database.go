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
	}

	for _, m := range migrations {
		var applied string
		err := DB.QueryRow("SELECT id FROM _migrations WHERE id = ?", m.id).Scan(&applied)
		if err == nil {
			continue // Zaten uygulandı
		}
		// Hatayı yoksay — kolon zaten varsa ALTER TABLE hata verir, bu normal
		DB.Exec(m.sql)
		DB.Exec("INSERT OR IGNORE INTO _migrations (id, applied_at) VALUES (?, datetime('now'))", m.id)
		log.Printf("🔄 Migration: %s", m.id)
	}
	return nil
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
