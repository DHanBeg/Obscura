package moderation

import (
	"context"
	"crypto/rand"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestScore_ReturnsFloat(t *testing.T) {
	// Well-encrypted random ciphertext, single recipient, normal cadence.
	buf := make([]byte, 256)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("rand: %v", err)
	}
	score, err := Score(context.Background(), buf, Metadata{
		SenderDID: "did:obs:alice", RecipientCount: 1, TimeSinceLastMs: 5000,
	})
	if err != nil {
		t.Fatalf("Score err: %v", err)
	}
	if score < 0 || score > 1 {
		t.Fatalf("score %f outside [0,1]", score)
	}
	if score >= SpamThreshold {
		t.Errorf("random 256B single-recipient message scored as spam (%f)", score)
	}
}

func TestScore_HighFanoutAndLowEntropyFlagsSpam(t *testing.T) {
	// All-zero ciphertext (entropy 0) blasted to 1000 recipients in a burst.
	buf := make([]byte, 256) // zeros
	score, err := Score(context.Background(), buf, Metadata{
		SenderDID: "did:obs:spammer", RecipientCount: 1000, TimeSinceLastMs: 100,
	})
	if err != nil {
		t.Fatalf("Score err: %v", err)
	}
	if !IsSpam(score) {
		t.Errorf("mass-blast zero-entropy payload not flagged: score=%f", score)
	}
}

func TestIsSpam_ThresholdBoundary(t *testing.T) {
	cases := []struct {
		score float64
		want  bool
	}{
		{0.0, false},
		{SpamThreshold - 0.0001, false},
		{SpamThreshold, true},
		{SpamThreshold + 0.0001, true},
		{1.0, true},
	}
	for _, c := range cases {
		if got := IsSpam(c.score); got != c.want {
			t.Errorf("IsSpam(%f) = %v, want %v", c.score, got, c.want)
		}
	}
}

func TestReport_PersistsToSpamReports(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE spam_reports (
		id TEXT PRIMARY KEY, reporter_did TEXT, reported_did TEXT,
		reason TEXT, status TEXT, created_at TEXT, reviewed_at TEXT,
		message_id TEXT, evidence_screenshot_url TEXT, evidence_ciphertext_hash TEXT,
		evidence_verified INTEGER, category TEXT
	)`); err != nil {
		t.Fatalf("schema: %v", err)
	}

	err = Report(context.Background(), db, ReportInput{
		ID: "rep-1", MessageID: "msg-1", ReporterDID: "did:obs:alice", ReportedDID: "did:obs:bob",
		Reason: "spammy", Category: CategorySpam, EvidenceCiphertextHash: "deadbeef", EvidenceVerified: true,
	})
	if err != nil {
		t.Fatalf("Report: %v", err)
	}

	var cnt int
	if err := db.QueryRow(`SELECT COUNT(*) FROM spam_reports WHERE reported_did=?`, "did:obs:bob").Scan(&cnt); err != nil {
		t.Fatalf("query: %v", err)
	}
	if cnt != 1 {
		t.Errorf("expected 1 row, got %d", cnt)
	}
}

func TestReport_RejectsEmptyDIDs(t *testing.T) {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	err := Report(context.Background(), db, ReportInput{ReportedDID: "did:obs:bob", Reason: "x"})
	if err == nil {
		t.Error("expected error for empty reporter DID")
	}
}
