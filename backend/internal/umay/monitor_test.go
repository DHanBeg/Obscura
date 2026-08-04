package umay

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"obscura.network/core/internal/moderation"
)

func newMonitorFixture(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	schema := `
	CREATE TABLE conversations (id TEXT PRIMARY KEY, is_public INTEGER NOT NULL DEFAULT 0);
	CREATE TABLE messages (
		id TEXT PRIMARY KEY, conv_id TEXT NOT NULL, from_did TEXT, ciphertext TEXT,
		encryption_type TEXT NOT NULL DEFAULT 'signal', sent_at TEXT NOT NULL, deleted_at TEXT
	);
	CREATE TABLE review_queue (
		id TEXT PRIMARY KEY, report_id TEXT, reason TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending', source TEXT NOT NULL DEFAULT 'user_report',
		created_at TEXT NOT NULL
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return db
}

func insertConv(t *testing.T, db *sql.DB, id string, isPublic bool) {
	t.Helper()
	pub := 0
	if isPublic {
		pub = 1
	}
	if _, err := db.Exec(`INSERT INTO conversations (id, is_public) VALUES (?, ?)`, id, pub); err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
}

func insertMsg(t *testing.T, db *sql.DB, id, convID, encType, content string, sentAt time.Time) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO messages (id, conv_id, from_did, ciphertext, encryption_type, sent_at)
		VALUES (?, ?, 'did:obs:alice', ?, ?, ?)`,
		id, convID, content, encType, sentAt.UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert message: %v", err)
	}
}

func TestMonitorScan_OnlyPublicNonSealedRecent(t *testing.T) {
	db := newMonitorFixture(t)
	insertConv(t, db, "conv-public", true)
	insertConv(t, db, "conv-private", false)

	now := time.Now()
	insertMsg(t, db, "msg-public", "conv-public", "signal", "hello", now)
	insertMsg(t, db, "msg-private", "conv-private", "signal", "hello", now)            // DM — must be excluded
	insertMsg(t, db, "msg-sealed", "conv-public", "sealed", "hello", now)              // sealed — must be excluded
	insertMsg(t, db, "msg-old", "conv-public", "signal", "hello", now.Add(-time.Hour)) // outside window

	mock := &mockClassifier{verdict: Verdict{Category: CategoryNone, Confidence: 1.0}}
	restore := SetClassifierForTest(mock)
	defer restore()

	m := NewMonitor(db)
	if err := m.scan(context.Background()); err != nil {
		t.Fatalf("scan: %v", err)
	}

	if mock.calls != 1 {
		t.Fatalf("classifier calls = %d, want 1 (only msg-public should be scanned)", mock.calls)
	}
}

func TestMonitorScan_RoutesVerdictToReviewQueue(t *testing.T) {
	db := newMonitorFixture(t)
	insertConv(t, db, "conv-public", true)
	insertMsg(t, db, "msg-1", "conv-public", "signal", "buy crypto now", time.Now())

	mock := &mockClassifier{verdict: Verdict{Category: moderation.CategoryScam, Confidence: 0.8}}
	restore := SetClassifierForTest(mock)
	defer restore()

	m := NewMonitor(db)
	if err := m.scan(context.Background()); err != nil {
		t.Fatalf("scan: %v", err)
	}

	var count int
	var source string
	if err := db.QueryRow(`SELECT COUNT(*), source FROM review_queue`).Scan(&count, &source); err != nil {
		t.Fatalf("query review_queue: %v", err)
	}
	if count != 1 || source != "auto_scan" {
		t.Fatalf("review_queue = (%d, %q), want (1, auto_scan)", count, source)
	}
}

func TestMonitorScan_NoPublicMessages_NoClassifierCalls(t *testing.T) {
	db := newMonitorFixture(t)
	insertConv(t, db, "conv-private", false)
	insertMsg(t, db, "msg-1", "conv-private", "signal", "hi", time.Now())

	mock := &mockClassifier{verdict: Verdict{Category: CategoryNone}}
	restore := SetClassifierForTest(mock)
	defer restore()

	m := NewMonitor(db)
	if err := m.scan(context.Background()); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if mock.calls != 0 {
		t.Fatalf("classifier calls = %d, want 0 (no public messages)", mock.calls)
	}
}
