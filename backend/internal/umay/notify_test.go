package umay

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"obscura.network/core/internal/moderation"
)

func newNotifyFixture(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	schema := `
	CREATE TABLE messages (
		id TEXT PRIMARY KEY, from_did TEXT, sent_at TEXT, deleted_at TEXT
	);
	CREATE TABLE review_queue (
		id TEXT PRIMARY KEY, report_id TEXT, reason TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending', source TEXT NOT NULL DEFAULT 'user_report',
		created_at TEXT NOT NULL
	);
	CREATE TABLE marketplace_listings (
		id TEXT PRIMARY KEY, seller_did TEXT NOT NULL, title TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'active',
		created_at TEXT NOT NULL, updated_at TEXT NOT NULL
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return db
}

func seedMessage(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO messages (id, from_did, sent_at) VALUES (?, 'did:obs:alice', datetime('now'))`, id); err != nil {
		t.Fatalf("seed message: %v", err)
	}
}

func TestHandle_ClassifyError_RoutesToReviewQueue(t *testing.T) {
	db := newNotifyFixture(t)
	seedMessage(t, db, "msg-1")

	err := Handle(context.Background(), db, "msg-1", "did:obs:alice", Verdict{}, fmt.Errorf("ollama down"))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}

	var count int
	var source string
	var reportID sql.NullString
	if err := db.QueryRow(`SELECT COUNT(*), source, report_id FROM review_queue`).Scan(&count, &source, &reportID); err != nil {
		t.Fatalf("query review_queue: %v", err)
	}
	if count != 1 {
		t.Fatalf("review_queue rows = %d, want 1", count)
	}
	if source != "auto_scan" {
		t.Fatalf("source = %q, want auto_scan", source)
	}
	if reportID.Valid {
		t.Fatalf("report_id = %q, want NULL", reportID.String)
	}

	var deletedAt sql.NullString
	db.QueryRow(`SELECT deleted_at FROM messages WHERE id = 'msg-1'`).Scan(&deletedAt)
	if deletedAt.Valid {
		t.Fatal("message was deleted on classify error, want left alone (not obvious spam)")
	}
}

func TestHandle_CategoryNone_NoAction(t *testing.T) {
	db := newNotifyFixture(t)
	seedMessage(t, db, "msg-1")

	if err := Handle(context.Background(), db, "msg-1", "did:obs:alice", Verdict{Category: CategoryNone, Confidence: 0.99}, nil); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	var count int
	db.QueryRow(`SELECT COUNT(*) FROM review_queue`).Scan(&count)
	if count != 0 {
		t.Fatalf("review_queue rows = %d, want 0 for clean content", count)
	}
}

func TestHandle_HighConfidenceSpam_AutoDeletes(t *testing.T) {
	db := newNotifyFixture(t)
	seedMessage(t, db, "msg-1")

	v := Verdict{Category: moderation.CategorySpam, Confidence: 0.95} // >= default 0.9
	if err := Handle(context.Background(), db, "msg-1", "did:obs:alice", v, nil); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	var deletedAt sql.NullString
	db.QueryRow(`SELECT deleted_at FROM messages WHERE id = 'msg-1'`).Scan(&deletedAt)
	if !deletedAt.Valid {
		t.Fatal("message not deleted, want auto-delete for high-confidence spam")
	}

	var count int
	db.QueryRow(`SELECT COUNT(*) FROM review_queue`).Scan(&count)
	if count != 0 {
		t.Fatalf("review_queue rows = %d, want 0 (auto-deleted, not queued)", count)
	}
}

func TestHandle_LowConfidenceSpam_RoutesToReviewQueue(t *testing.T) {
	db := newNotifyFixture(t)
	seedMessage(t, db, "msg-1")

	v := Verdict{Category: moderation.CategorySpam, Confidence: 0.6} // below auto-delete threshold
	if err := Handle(context.Background(), db, "msg-1", "did:obs:alice", v, nil); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	var deletedAt sql.NullString
	db.QueryRow(`SELECT deleted_at FROM messages WHERE id = 'msg-1'`).Scan(&deletedAt)
	if deletedAt.Valid {
		t.Fatal("message deleted despite confidence below auto-delete threshold")
	}

	var count int
	var source string
	db.QueryRow(`SELECT COUNT(*), source FROM review_queue`).Scan(&count, &source)
	if count != 1 || source != "auto_scan" {
		t.Fatalf("review_queue = (%d, %q), want (1, auto_scan)", count, source)
	}
}

func TestHandle_NonSpamViolation_RoutesToReviewQueue_NeverAutoDeletes(t *testing.T) {
	db := newNotifyFixture(t)
	seedMessage(t, db, "msg-1")

	// Even at very high confidence, only spam auto-deletes (spec 1.4).
	v := Verdict{Category: moderation.CategoryHarassment, Confidence: 0.99}
	if err := Handle(context.Background(), db, "msg-1", "did:obs:alice", v, nil); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	var deletedAt sql.NullString
	db.QueryRow(`SELECT deleted_at FROM messages WHERE id = 'msg-1'`).Scan(&deletedAt)
	if deletedAt.Valid {
		t.Fatal("non-spam category auto-deleted, want human review regardless of confidence")
	}

	var count int
	db.QueryRow(`SELECT COUNT(*) FROM review_queue`).Scan(&count)
	if count != 1 {
		t.Fatalf("review_queue rows = %d, want 1", count)
	}
}

func seedListing(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO marketplace_listings (id, seller_did, title, status, created_at, updated_at)
		VALUES (?, 'did:obs:seller1', 'title', 'active', datetime('now'), datetime('now'))`, id,
	); err != nil {
		t.Fatalf("seed listing: %v", err)
	}
}

func TestHandleListing_ClassifyError_RoutesToReviewQueue(t *testing.T) {
	db := newNotifyFixture(t)
	seedListing(t, db, "listing-1")

	err := HandleListing(context.Background(), db, "listing-1", "did:obs:seller1", Verdict{}, fmt.Errorf("ollama down"))
	if err != nil {
		t.Fatalf("HandleListing: %v", err)
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
		t.Fatalf("listing status = %q, want unchanged 'active' on classify error", status)
	}
}

func TestHandleListing_CategoryNone_NoAction(t *testing.T) {
	db := newNotifyFixture(t)
	seedListing(t, db, "listing-1")

	if err := HandleListing(context.Background(), db, "listing-1", "did:obs:seller1", Verdict{Category: CategoryNone, Confidence: 0.99}, nil); err != nil {
		t.Fatalf("HandleListing: %v", err)
	}

	var count int
	db.QueryRow(`SELECT COUNT(*) FROM review_queue`).Scan(&count)
	if count != 0 {
		t.Fatalf("review_queue rows = %d, want 0 for clean content", count)
	}
}

func TestHandleListing_HighConfidenceSpam_AutoRemoves(t *testing.T) {
	db := newNotifyFixture(t)
	seedListing(t, db, "listing-1")

	v := Verdict{Category: moderation.CategorySpam, Confidence: 0.95}
	if err := HandleListing(context.Background(), db, "listing-1", "did:obs:seller1", v, nil); err != nil {
		t.Fatalf("HandleListing: %v", err)
	}

	var status string
	db.QueryRow(`SELECT status FROM marketplace_listings WHERE id = 'listing-1'`).Scan(&status)
	if status != "removed" {
		t.Fatalf("listing status = %q, want removed", status)
	}

	var count int
	db.QueryRow(`SELECT COUNT(*) FROM review_queue`).Scan(&count)
	if count != 0 {
		t.Fatalf("review_queue rows = %d, want 0 (auto-removed, not queued)", count)
	}
}

func TestHandleListing_NonSpamViolation_RoutesToReviewQueue_WithListingIDAndSeller(t *testing.T) {
	db := newNotifyFixture(t)
	seedListing(t, db, "listing-1")

	v := Verdict{Category: moderation.CategoryIllegalSale, Confidence: 0.9}
	if err := HandleListing(context.Background(), db, "listing-1", "did:obs:seller1", v, nil); err != nil {
		t.Fatalf("HandleListing: %v", err)
	}

	var status string
	db.QueryRow(`SELECT status FROM marketplace_listings WHERE id = 'listing-1'`).Scan(&status)
	if status != "active" {
		t.Fatalf("listing status = %q, want unchanged 'active' (only spam auto-removes)", status)
	}

	var count int
	var source, reason string
	var reportID sql.NullString
	if err := db.QueryRow(`SELECT COUNT(*), source, reason, report_id FROM review_queue`).Scan(&count, &source, &reason, &reportID); err != nil {
		t.Fatalf("query review_queue: %v", err)
	}
	if count != 1 || source != "auto_scan" {
		t.Fatalf("review_queue = (%d, %q), want (1, auto_scan)", count, source)
	}
	if reportID.Valid {
		t.Fatalf("report_id = %q, want NULL (auto_scan finding, no spam_reports row)", reportID.String)
	}
	// review_queue has no listing_id column (only spam_reports does) — the
	// listing is identified inside reason, same as msg_id is for messages.
	// reason embeds truncate()'d ids (same as Handle does for msg/from), so
	// assert against the truncated form, not the full id.
	if !strings.Contains(reason, truncate("listing-1", 8)) {
		t.Fatalf("reason %q missing listing id", reason)
	}
	if !strings.Contains(reason, truncate("did:obs:seller1", 12)) {
		t.Fatalf("reason %q missing seller did", reason)
	}
}
