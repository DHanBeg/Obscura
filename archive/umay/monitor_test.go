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
		target_type TEXT DEFAULT '', target_id TEXT DEFAULT '',
		resolved_at TEXT, resolved_by TEXT, resolution TEXT,
		created_at TEXT NOT NULL
	);
	CREATE TABLE marketplace_listings (
		id TEXT PRIMARY KEY, seller_did TEXT NOT NULL, title TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '', price TEXT NOT NULL DEFAULT '0',
		currency TEXT NOT NULL DEFAULT 'OBS', category TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'active', created_at TEXT NOT NULL, updated_at TEXT NOT NULL
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

func TestMonitorScanMessages_OnlyPublicNonSealedRecent(t *testing.T) {
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
	if err := m.scanMessages(context.Background()); err != nil {
		t.Fatalf("scan: %v", err)
	}

	if mock.calls != 1 {
		t.Fatalf("classifier calls = %d, want 1 (only msg-public should be scanned)", mock.calls)
	}
}

func TestMonitorScanMessages_RoutesVerdictToReviewQueue(t *testing.T) {
	db := newMonitorFixture(t)
	insertConv(t, db, "conv-public", true)
	insertMsg(t, db, "msg-1", "conv-public", "signal", "buy crypto now", time.Now())

	mock := &mockClassifier{verdict: Verdict{Category: moderation.CategoryScam, Confidence: 0.8}}
	restore := SetClassifierForTest(mock)
	defer restore()

	m := NewMonitor(db)
	if err := m.scanMessages(context.Background()); err != nil {
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

func TestMonitorScanMessages_NoPublicMessages_NoClassifierCalls(t *testing.T) {
	db := newMonitorFixture(t)
	insertConv(t, db, "conv-private", false)
	insertMsg(t, db, "msg-1", "conv-private", "signal", "hi", time.Now())

	mock := &mockClassifier{verdict: Verdict{Category: CategoryNone}}
	restore := SetClassifierForTest(mock)
	defer restore()

	m := NewMonitor(db)
	if err := m.scanMessages(context.Background()); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if mock.calls != 0 {
		t.Fatalf("classifier calls = %d, want 0 (no public messages)", mock.calls)
	}
}

func insertListing(t *testing.T, db *sql.DB, id, sellerDID, status, title, description string, updatedAt time.Time) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO marketplace_listings
		(id, seller_did, title, description, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, sellerDID, title, description, status,
		updatedAt.UTC().Format(time.RFC3339), updatedAt.UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert listing: %v", err)
	}
}

func TestMonitorScanListings_OnlyActiveRecent(t *testing.T) {
	db := newMonitorFixture(t)
	now := time.Now()
	insertListing(t, db, "listing-active", "did:obs:seller1", "active", "Laptop", "Used", now)
	insertListing(t, db, "listing-sold", "did:obs:seller1", "sold", "Phone", "Used", now)                // not active — excluded
	insertListing(t, db, "listing-removed", "did:obs:seller1", "removed", "Bike", "Old", now)            // not active — excluded
	insertListing(t, db, "listing-old", "did:obs:seller1", "active", "Desk", "Old", now.Add(-time.Hour)) // outside window

	mock := &mockClassifier{verdict: Verdict{Category: CategoryNone, Confidence: 1.0}}
	restore := SetClassifierForTest(mock)
	defer restore()

	m := NewMonitor(db)
	if err := m.scanListings(context.Background()); err != nil {
		t.Fatalf("scanListings: %v", err)
	}
	if mock.calls != 1 {
		t.Fatalf("classifier calls = %d, want 1 (only listing-active should be scanned)", mock.calls)
	}
	if mock.lastContent != "Laptop\nUsed" {
		t.Fatalf("classified content = %q, want %q (title+\\n+description)", mock.lastContent, "Laptop\nUsed")
	}
}

func TestMonitorScanListings_RoutesVerdictToReviewQueue(t *testing.T) {
	db := newMonitorFixture(t)
	insertListing(t, db, "listing-1", "did:obs:seller1", "active", "Cheap watches", "DM me", time.Now())

	mock := &mockClassifier{verdict: Verdict{Category: moderation.CategoryScam, Confidence: 0.8}}
	restore := SetClassifierForTest(mock)
	defer restore()

	m := NewMonitor(db)
	if err := m.scanListings(context.Background()); err != nil {
		t.Fatalf("scanListings: %v", err)
	}

	var count int
	var source string
	if err := db.QueryRow(`SELECT COUNT(*), source FROM review_queue`).Scan(&count, &source); err != nil {
		t.Fatalf("query review_queue: %v", err)
	}
	if count != 1 || source != "auto_scan" {
		t.Fatalf("review_queue = (%d, %q), want (1, auto_scan)", count, source)
	}

	var status string
	db.QueryRow(`SELECT status FROM marketplace_listings WHERE id = 'listing-1'`).Scan(&status)
	if status != "active" {
		t.Fatalf("listing status = %q, want unchanged 'active' (scam routes to review, not auto-remove)", status)
	}
}

func TestMonitorScanListings_HighConfidenceSpam_AutoRemoves(t *testing.T) {
	db := newMonitorFixture(t)
	insertListing(t, db, "listing-1", "did:obs:seller1", "active", "free money click here", "", time.Now())

	mock := &mockClassifier{verdict: Verdict{Category: moderation.CategorySpam, Confidence: 0.95}}
	restore := SetClassifierForTest(mock)
	defer restore()

	m := NewMonitor(db)
	if err := m.scanListings(context.Background()); err != nil {
		t.Fatalf("scanListings: %v", err)
	}

	var status string
	if err := db.QueryRow(`SELECT status FROM marketplace_listings WHERE id = 'listing-1'`).Scan(&status); err != nil {
		t.Fatalf("query listing: %v", err)
	}
	if status != "removed" {
		t.Fatalf("listing status = %q, want removed (high-confidence spam auto-removes)", status)
	}

	var count int
	db.QueryRow(`SELECT COUNT(*) FROM review_queue`).Scan(&count)
	if count != 0 {
		t.Fatalf("review_queue rows = %d, want 0 (auto-removed, not queued)", count)
	}
}

func TestMonitorScanListings_NoActiveListings_NoClassifierCalls(t *testing.T) {
	db := newMonitorFixture(t)
	insertListing(t, db, "listing-1", "did:obs:seller1", "removed", "Old item", "", time.Now())

	mock := &mockClassifier{verdict: Verdict{Category: CategoryNone}}
	restore := SetClassifierForTest(mock)
	defer restore()

	m := NewMonitor(db)
	if err := m.scanListings(context.Background()); err != nil {
		t.Fatalf("scanListings: %v", err)
	}
	if mock.calls != 0 {
		t.Fatalf("classifier calls = %d, want 0 (no active listings)", mock.calls)
	}
}
