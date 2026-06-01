package scanner

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// setupTestDB — in-memory SQLite veritabanı kurar ve scanner için gereken tabloları oluşturur.
// Her CREATE TABLE ayrı Exec çağrısıyla çalıştırılır; modernc SQLite tek seferde
// birden fazla DDL ifadesini desteklemeyebilir.
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("test DB açılamadı: %v", err)
	}
	// In-memory SQLite: her bağlantı ayrı veritabanı açar.
	// MaxOpenConns(1) ile tüm sorgular tek bağlantı üzerinden gider.
	db.SetMaxOpenConns(1)

	statements := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id           TEXT PRIMARY KEY,
			phone        TEXT,
			did          TEXT UNIQUE NOT NULL,
			credit_score REAL DEFAULT 100,
			created_at   TEXT NOT NULL,
			updated_at   TEXT NOT NULL,
			last_seen_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id          TEXT PRIMARY KEY,
			conv_id     TEXT NOT NULL DEFAULT '',
			from_did    TEXT NOT NULL,
			to_did      TEXT NOT NULL DEFAULT '',
			type        TEXT DEFAULT 'text',
			ciphertext  TEXT NOT NULL DEFAULT '',
			media_url   TEXT DEFAULT '',
			status      TEXT DEFAULT 'sent',
			is_group    INTEGER DEFAULT 0,
			reply_to_id TEXT DEFAULT '',
			sent_at     TEXT NOT NULL,
			expires_at  TEXT NOT NULL DEFAULT '',
			deleted_at  TEXT,
			is_flagged  INTEGER NOT NULL DEFAULT 0,
			flag_reason TEXT DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS spam_reports (
			id           TEXT PRIMARY KEY,
			reporter_did TEXT NOT NULL,
			reported_did TEXT NOT NULL,
			reason       TEXT,
			status       TEXT DEFAULT 'pending',
			created_at   TEXT NOT NULL,
			reviewed_at  TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS group_reports (
			id           TEXT PRIMARY KEY,
			group_id     TEXT NOT NULL,
			reporter_did TEXT NOT NULL,
			reason       TEXT NOT NULL,
			created_at   INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS group_moderation (
			group_id     TEXT PRIMARY KEY,
			is_reviewing INTEGER NOT NULL DEFAULT 0,
			is_suspended INTEGER NOT NULL DEFAULT 0,
			report_count INTEGER NOT NULL DEFAULT 0,
			last_review  INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS federation_nodes (
			node_id       TEXT PRIMARY KEY,
			peer_addr     TEXT NOT NULL DEFAULT '',
			http_url      TEXT NOT NULL DEFAULT '',
			pubkey        TEXT NOT NULL DEFAULT '',
			version       TEXT NOT NULL DEFAULT '',
			region        TEXT NOT NULL DEFAULT '',
			registered_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_seen     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			status        TEXT NOT NULL DEFAULT 'active'
		)`,
		`CREATE TABLE IF NOT EXISTS zk_proof_log (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			proof_hash  TEXT NOT NULL,
			circuit_id  TEXT NOT NULL,
			user_did    TEXT NOT NULL,
			verified_at INTEGER NOT NULL,
			prev_hash   TEXT NOT NULL DEFAULT '',
			chain_hash  TEXT NOT NULL UNIQUE
		)`,
		`CREATE TABLE IF NOT EXISTS scanner_flags (
			user_did   TEXT PRIMARY KEY,
			reason     TEXT NOT NULL,
			flagged_at INTEGER NOT NULL
		)`,
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("tablo oluşturulamadı: %v\nSQL: %s", err, stmt[:60])
		}
	}
	return db
}

// TestScanMessages_SpamDetected — spam içerik tespit edilince mesaj flaglenir
// ve gönderenin kredisi düşer.
func TestScanMessages_SpamDetected(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Kullanıcı ekle
	_, err := db.Exec(`INSERT INTO users (id, did, credit_score, created_at, updated_at, last_seen_at)
		VALUES ('u1', 'did:obs:abc123', 100, datetime('now'), datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("kullanıcı eklenemedi: %v", err)
	}

	// Spam mesaj ekle (şu anki zamanda)
	sentAt := time.Now().UTC().Format(time.RFC3339)
	_, err = db.Exec(`INSERT INTO messages (id, from_did, to_did, ciphertext, sent_at, expires_at)
		VALUES ('msg1', 'did:obs:abc123', 'did:obs:def456', 'free money giveaway', ?, '2099-01-01T00:00:00Z')`,
		sentAt)
	if err != nil {
		t.Fatalf("mesaj eklenemedi: %v", err)
	}

	s := New(db)
	if err := s.scanMessages(context.Background()); err != nil {
		t.Fatalf("scanMessages hatası: %v", err)
	}

	// Mesaj flaglenmiş olmalı
	var isFlagged int
	var flagReason string
	db.QueryRow("SELECT is_flagged, flag_reason FROM messages WHERE id = 'msg1'").Scan(&isFlagged, &flagReason)
	if isFlagged != 1 {
		t.Errorf("mesaj flaglenmedi, is_flagged = %d", isFlagged)
	}
	if flagReason != "spam_content" {
		t.Errorf("flag_reason beklenen 'spam_content', alınan '%s'", flagReason)
	}

	// Kredi düşmeli
	var score float64
	db.QueryRow("SELECT credit_score FROM users WHERE did = 'did:obs:abc123'").Scan(&score)
	if score >= 100 {
		t.Errorf("kredi düşmedi, score = %f", score)
	}
}

// TestScanMessages_NoSpam — temiz mesaj flaglenmemeli.
func TestScanMessages_NoSpam(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, _ = db.Exec(`INSERT INTO users (id, did, credit_score, created_at, updated_at, last_seen_at)
		VALUES ('u2', 'did:obs:clean001', 100, datetime('now'), datetime('now'), datetime('now'))`)

	sentAt := time.Now().UTC().Format(time.RFC3339)
	_, _ = db.Exec(`INSERT INTO messages (id, from_did, to_did, ciphertext, sent_at, expires_at)
		VALUES ('msg2', 'did:obs:clean001', 'did:obs:other', 'merhaba nasılsın', ?, '2099-01-01T00:00:00Z')`,
		sentAt)

	s := New(db)
	if err := s.scanMessages(context.Background()); err != nil {
		t.Fatalf("scanMessages hatası: %v", err)
	}

	var isFlagged int
	db.QueryRow("SELECT is_flagged FROM messages WHERE id = 'msg2'").Scan(&isFlagged)
	if isFlagged != 0 {
		t.Errorf("temiz mesaj yanlışlıkla flaglendi")
	}
}

// TestScanGroups_HighReports — 3+ rapor alan grup incelemeye alınmalı.
func TestScanGroups_HighReports(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	now := time.Now().Unix()
	groupID := "group-abc-001"

	for i := 0; i < 4; i++ {
		_, _ = db.Exec(`INSERT INTO group_reports (id, group_id, reporter_did, reason, created_at)
			VALUES (?, ?, 'did:obs:reporter', 'uygunsuz', ?)`,
			fmt.Sprintf("rep%d", i), groupID, now)
	}

	s := New(db)
	if err := s.scanGroups(context.Background()); err != nil {
		t.Fatalf("scanGroups hatası: %v", err)
	}

	var isReviewing, reportCount int
	db.QueryRow("SELECT is_reviewing, report_count FROM group_moderation WHERE group_id = ?", groupID).
		Scan(&isReviewing, &reportCount)
	if isReviewing != 1 {
		t.Errorf("grup incelemeye alınmadı, is_reviewing = %d", isReviewing)
	}
	if reportCount != 4 {
		t.Errorf("report_count beklenen 4, alınan %d", reportCount)
	}
}

// TestScanGroups_LowReports — 2 rapor eşiği geçmemeli.
func TestScanGroups_LowReports(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	now := time.Now().Unix()
	groupID := "group-few-001"

	for i := 0; i < 2; i++ {
		_, _ = db.Exec(`INSERT INTO group_reports (id, group_id, reporter_did, reason, created_at)
			VALUES (?, ?, 'did:obs:reporter', 'uygunsuz', ?)`,
			fmt.Sprintf("frep%d", i), groupID, now)
	}

	s := New(db)
	if err := s.scanGroups(context.Background()); err != nil {
		t.Fatalf("scanGroups hatası: %v", err)
	}

	var count int
	db.QueryRow("SELECT COUNT(*) FROM group_moderation WHERE group_id = ?", groupID).Scan(&count)
	if count != 0 {
		t.Errorf("az raporlu grup yanlışlıkla moderasyona alındı")
	}
}

// TestScanNodes_StaleNodeMarkedInactive — 5 dakikadan fazla sessiz node pasife çekilmeli.
func TestScanNodes_StaleNodeMarkedInactive(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	staleTime := time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339)
	_, _ = db.Exec(`INSERT INTO federation_nodes (node_id, peer_addr, pubkey, last_seen, status)
		VALUES ('node-stale', '/ip4/1.2.3.4/tcp/9000', 'pubkey123', ?, 'active')`, staleTime)

	s := New(db)
	if err := s.scanNodes(context.Background()); err != nil {
		t.Fatalf("scanNodes hatası: %v", err)
	}

	var status string
	db.QueryRow("SELECT status FROM federation_nodes WHERE node_id = 'node-stale'").Scan(&status)
	if status != "inactive" {
		t.Errorf("pasif node 'inactive' işaretlenmedi, status = '%s'", status)
	}
}

// TestScanNodes_ActiveNodeUnchanged — yakın zamanda görülen node aktif kalmalı.
func TestScanNodes_ActiveNodeUnchanged(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	recentTime := time.Now().Add(-1 * time.Minute).UTC().Format(time.RFC3339)
	_, _ = db.Exec(`INSERT INTO federation_nodes (node_id, peer_addr, pubkey, last_seen, status)
		VALUES ('node-active', '/ip4/5.6.7.8/tcp/9000', 'pubkey456', ?, 'active')`, recentTime)

	s := New(db)
	if err := s.scanNodes(context.Background()); err != nil {
		t.Fatalf("scanNodes hatası: %v", err)
	}

	var status string
	db.QueryRow("SELECT status FROM federation_nodes WHERE node_id = 'node-active'").Scan(&status)
	if status != "active" {
		t.Errorf("aktif node yanlışlıkla pasife çekildi, status = '%s'", status)
	}
}

// TestScanAnomalies_ProofFlood — 45s'de 21+ proof ZK anomali bayrağı tetiklemeli.
func TestScanAnomalies_ProofFlood(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	did := "did:obs:zkflood001"
	now := time.Now().Unix()

	for i := 0; i < 25; i++ {
		_, _ = db.Exec(`INSERT INTO zk_proof_log (proof_hash, circuit_id, user_did, verified_at, chain_hash)
			VALUES (?, 'credit', ?, ?, ?)`,
			fmt.Sprintf("hash%d", i), did, now, fmt.Sprintf("chain%d", i))
	}

	s := New(db)
	if err := s.scanAnomalies(context.Background()); err != nil {
		t.Fatalf("scanAnomalies hatası: %v", err)
	}

	var reason string
	db.QueryRow("SELECT reason FROM scanner_flags WHERE user_did = ?", did).Scan(&reason)
	if reason != "zk_proof_flood" {
		t.Errorf("ZK flood bayrağı beklendi, alınan '%s'", reason)
	}
}

// TestScanSpamReports_CreditReduction — 5+ spam raporu olan kullanıcının kredisi düşmeli.
func TestScanSpamReports_CreditReduction(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	spammerDID := "did:obs:spammer001"
	_, _ = db.Exec(`INSERT INTO users (id, did, credit_score, created_at, updated_at, last_seen_at)
		VALUES ('u3', ?, 50, datetime('now'), datetime('now'), datetime('now'))`, spammerDID)

	createdAt := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	for i := 0; i < 6; i++ {
		_, _ = db.Exec(`INSERT INTO spam_reports (id, reporter_did, reported_did, created_at)
			VALUES (?, 'did:obs:reporter', ?, ?)`,
			fmt.Sprintf("sr%d", i), spammerDID, createdAt)
	}

	s := New(db)
	if err := s.scanSpamReports(context.Background()); err != nil {
		t.Fatalf("scanSpamReports hatası: %v", err)
	}

	var score float64
	db.QueryRow("SELECT credit_score FROM users WHERE did = ?", spammerDID).Scan(&score)
	if score >= 50 {
		t.Errorf("kredi düşmedi, score = %f (beklenen < 50)", score)
	}
}

// TestIsSpamContent — anahtar kelime tespiti doğru çalışmalı.
func TestIsSpamContent(t *testing.T) {
	cases := []struct {
		content string
		want    bool
	}{
		{"free money for everyone", true},
		{"crypto giveaway click here", true},
		{"scam alert", true},
		{"merhaba nasılsın", false},
		{"", false},
		{"normal bir mesaj içeriği", false},
		{"phishing link gönderiyorum", true},
	}
	for _, tc := range cases {
		got := isSpamContent(tc.content)
		if got != tc.want {
			t.Errorf("isSpamContent(%q) = %v, beklenen %v", tc.content, got, tc.want)
		}
	}
}

// TestTruncate — kısaltma fonksiyonu doğru çalışmalı.
func TestTruncate(t *testing.T) {
	if truncate("abcdefgh", 4) != "abcd" {
		t.Error("uzun string kısaltma hatalı")
	}
	if truncate("ab", 4) != "ab" {
		t.Error("kısa string değişmemeli")
	}
	if truncate("", 4) != "" {
		t.Error("boş string değişmemeli")
	}
}
