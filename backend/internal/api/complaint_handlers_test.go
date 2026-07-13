package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/models"
)

// These tests call the handlers directly (bypassing the OTP/JWT flow) by
// injecting the authenticated user into the request context the same way
// AuthMiddleware does — the pre-existing integration tests in this package
// have an unrelated, already-broken OTP fixture (see moderation_test.go /
// oturum notları), so complaint-flow coverage avoids that dependency
// entirely rather than inheriting its breakage.

func hashHex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func withUser(req *http.Request, user *models.User) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), "user", user))
}

func seedTestMessage(t *testing.T, id, fromDID, toDID, ciphertext string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.DB.Exec(`
		INSERT INTO messages (id, conv_id, from_did, to_did, type, ciphertext, sent_at, expires_at)
		VALUES (?, ?, ?, ?, 'text', ?, ?, ?)`,
		id, "conv-"+id, fromDID, toDID, ciphertext, now, now)
	if err != nil {
		t.Fatalf("seed message: %v", err)
	}
}

func decodeResp(t *testing.T, rec *httptest.ResponseRecorder) models.APIResponse {
	t.Helper()
	var resp models.APIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v (body: %s)", err, rec.Body.String())
	}
	return resp
}

func TestHandleSpamReport_RejectsMissingEvidence(t *testing.T) {
	body, _ := json.Marshal(map[string]string{
		"reported_did": "did:obs:accused", "category": "spam",
	})
	req := withUser(httptest.NewRequest("POST", "/v1/spam/report", bytes.NewReader(body)),
		&models.User{DID: "did:obs:reporter-a"})
	rec := httptest.NewRecorder()

	HandleSpamReport(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 (missing message_id/hash)", rec.Code)
	}
}

func TestHandleSpamReport_RejectsUnknownCategory(t *testing.T) {
	body, _ := json.Marshal(map[string]string{
		"reported_did": "did:obs:accused", "category": "uygunsuzluk",
		"message_id": "m1", "evidence_ciphertext_hash": "x",
	})
	req := withUser(httptest.NewRequest("POST", "/v1/spam/report", bytes.NewReader(body)),
		&models.User{DID: "did:obs:reporter-b"})
	rec := httptest.NewRecorder()

	HandleSpamReport(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 (unknown category)", rec.Code)
	}
}

func TestHandleSpamReport_RejectsEvidenceMismatch(t *testing.T) {
	seedTestMessage(t, "msg-mismatch", "did:obs:accused-1", "did:obs:reporter-c", "real-ciphertext")

	body, _ := json.Marshal(map[string]string{
		"reported_did": "did:obs:accused-1", "category": "spam",
		"message_id": "msg-mismatch", "evidence_ciphertext_hash": hashHex("wrong-ciphertext"),
	})
	req := withUser(httptest.NewRequest("POST", "/v1/spam/report", bytes.NewReader(body)),
		&models.User{DID: "did:obs:reporter-c"})
	rec := httptest.NewRecorder()

	HandleSpamReport(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 (evidence mismatch)", rec.Code)
	}
}

func TestHandleSpamReport_ObviousSpamAutoProcessed(t *testing.T) {
	// Obvious spam needs BOTH a low-entropy/repetitive ciphertext AND a real
	// recent fanout (Score's mass-blast signal) — a lone 1-1 message can
	// never mathematically clear the threshold on entropy/length/repetition
	// alone (they cap out exactly AT the 0.7 boundary). Simulate the sender
	// having just blasted 20 distinct recipients.
	now := time.Now().UTC().Format(time.RFC3339)
	lowEntropyCiphertext := "zzzzzzzz" // 8 bytes, single repeated byte
	for i := 0; i < 20; i++ {
		to := "did:obs:fanout-victim-" + string(rune('a'+i))
		db.DB.Exec(`INSERT INTO messages (id, conv_id, from_did, to_did, type, ciphertext, sent_at, expires_at)
			VALUES (?, ?, 'did:obs:spammer-1', ?, 'text', ?, ?, ?)`,
			uuid.New().String(), "conv-fanout-"+string(rune('a'+i)), to, lowEntropyCiphertext, now, now)
	}
	seedTestMessage(t, "msg-spam-1", "did:obs:spammer-1", "did:obs:reporter-d", lowEntropyCiphertext)

	body, _ := json.Marshal(map[string]string{
		"reported_did": "did:obs:spammer-1", "category": "spam",
		"message_id": "msg-spam-1", "evidence_ciphertext_hash": hashHex(lowEntropyCiphertext),
	})
	req := withUser(httptest.NewRequest("POST", "/v1/spam/report", bytes.NewReader(body)),
		&models.User{DID: "did:obs:reporter-d"})
	rec := httptest.NewRecorder()

	HandleSpamReport(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	var violationCount int
	db.DB.QueryRow(`SELECT COUNT(*) FROM user_violations WHERE user_did = ?`, "did:obs:spammer-1").Scan(&violationCount)
	if violationCount != 1 {
		t.Fatalf("expected 1 auto-recorded violation for obvious spam, got %d", violationCount)
	}
}

func TestHandleSpamReport_NonObviousQueuedForReview(t *testing.T) {
	seedTestMessage(t, "msg-harass-1", "did:obs:accused-2", "did:obs:reporter-e", "normal-looking-ciphertext-not-spammy")

	body, _ := json.Marshal(map[string]string{
		"reported_did": "did:obs:accused-2", "category": "harassment",
		"message_id": "msg-harass-1", "evidence_ciphertext_hash": hashHex("normal-looking-ciphertext-not-spammy"),
	})
	req := withUser(httptest.NewRequest("POST", "/v1/spam/report", bytes.NewReader(body)),
		&models.User{DID: "did:obs:reporter-e"})
	rec := httptest.NewRecorder()

	HandleSpamReport(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	// Non-obvious (harassment is never auto-processed) → no immediate
	// violation, must sit in review_queue for a human decision (İlke 5).
	var violationCount int
	db.DB.QueryRow(`SELECT COUNT(*) FROM user_violations WHERE user_did = ?`, "did:obs:accused-2").Scan(&violationCount)
	if violationCount != 0 {
		t.Fatalf("harassment must not be auto-punished, got %d violations", violationCount)
	}
	var queued int
	db.DB.QueryRow(`SELECT COUNT(*) FROM review_queue WHERE reason = 'insan incelemesi bekliyor'`).Scan(&queued)
	if queued < 1 {
		t.Fatal("expected report to land in review_queue")
	}
}

func TestHandleComplaintVerdict_RequiresAdminTier(t *testing.T) {
	reportID := uuid.New().String()
	db.DB.Exec(`INSERT INTO spam_reports (id, reporter_did, reported_did, category, status, created_at)
		VALUES (?, 'did:obs:reporter-f', 'did:obs:accused-3', 'spam', 'pending', ?)`,
		reportID, time.Now().UTC().Format(time.RFC3339))

	body, _ := json.Marshal(map[string]bool{"upheld": true})
	req := withUser(httptest.NewRequest("POST", "/v1/complaints/"+reportID+"/verdict", bytes.NewReader(body)),
		&models.User{DID: "did:obs:lowtier", Tier: 1})
	req = mux.SetURLVars(req, map[string]string{"id": reportID})
	rec := httptest.NewRecorder()

	HandleComplaintVerdict(rec, req)

	if rec.Code != 403 {
		t.Fatalf("status = %d, want 403 (non-admin tier)", rec.Code)
	}
}

func TestHandleComplaintVerdict_ResolvesReviewQueue(t *testing.T) {
	reportID := uuid.New().String()
	db.DB.Exec(`INSERT INTO spam_reports (id, reporter_did, reported_did, category, status, created_at)
		VALUES (?, 'did:obs:reporter-h', 'did:obs:accused-5', 'spam', 'pending', ?)`,
		reportID, time.Now().UTC().Format(time.RFC3339))
	db.DB.Exec(`INSERT INTO review_queue (id, report_id, reason, status, created_at)
		VALUES (?, ?, 'insan incelemesi bekliyor', 'pending', ?)`,
		uuid.New().String(), reportID, time.Now().UTC().Format(time.RFC3339))

	body, _ := json.Marshal(map[string]bool{"upheld": true})
	req := withUser(httptest.NewRequest("POST", "/v1/complaints/"+reportID+"/verdict", bytes.NewReader(body)),
		&models.User{DID: "did:obs:admin2", Tier: 5})
	req = mux.SetURLVars(req, map[string]string{"id": reportID})
	rec := httptest.NewRecorder()

	HandleComplaintVerdict(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var status string
	if err := db.DB.QueryRow(`SELECT status FROM review_queue WHERE report_id = ?`, reportID).Scan(&status); err != nil {
		t.Fatalf("read review_queue: %v", err)
	}
	if status != "resolved" {
		t.Fatalf("review_queue status = %q, want resolved after verdict (no dangling pending rows)", status)
	}
}

func TestHandleComplaintVerdict_RepeatFalseAgainstSameTarget_Escalates(t *testing.T) {
	reporter, target := "did:obs:serial-h", "did:obs:target-h"
	for i, id := range []string{"rep-h1", "rep-h2"} {
		db.DB.Exec(`INSERT INTO spam_reports (id, reporter_did, reported_did, category, status, created_at)
			VALUES (?, ?, ?, 'spam', 'pending', ?)`, id, reporter, target, time.Now().UTC().Format(time.RFC3339))
		body, _ := json.Marshal(map[string]bool{"upheld": false})
		req := withUser(httptest.NewRequest("POST", "/v1/complaints/"+id+"/verdict", bytes.NewReader(body)),
			&models.User{DID: "did:obs:admin3", Tier: 5})
		req = mux.SetURLVars(req, map[string]string{"id": id})
		rec := httptest.NewRecorder()
		HandleComplaintVerdict(rec, req)
		if rec.Code != 200 {
			t.Fatalf("verdict %d status = %d, want 200", i, rec.Code)
		}
	}

	// 3rd false report against the SAME target — must escalate straight to
	// harassment/top-tier through the live HTTP path, not just the
	// package-internal function.
	thirdID := "rep-h3"
	db.DB.Exec(`INSERT INTO spam_reports (id, reporter_did, reported_did, category, status, created_at)
		VALUES (?, ?, ?, 'spam', 'pending', ?)`, thirdID, reporter, target, time.Now().UTC().Format(time.RFC3339))
	body, _ := json.Marshal(map[string]bool{"upheld": false})
	req := withUser(httptest.NewRequest("POST", "/v1/complaints/"+thirdID+"/verdict", bytes.NewReader(body)),
		&models.User{DID: "did:obs:admin3", Tier: 5})
	req = mux.SetURLVars(req, map[string]string{"id": thirdID})
	rec := httptest.NewRecorder()
	HandleComplaintVerdict(rec, req)
	if rec.Code != 200 {
		t.Fatalf("3rd verdict status = %d, want 200", rec.Code)
	}

	var category, action string
	if err := db.DB.QueryRow(
		`SELECT category, action FROM user_violations WHERE user_did = ? AND source_report_id = ?`, reporter, thirdID,
	).Scan(&category, &action); err != nil {
		t.Fatalf("read escalated violation: %v", err)
	}
	if category != "harassment" || action != "restrict_30d" {
		t.Fatalf("category=%q action=%q, want harassment/restrict_30d (repeat-target escalation)", category, action)
	}
}

func TestHandleComplaintVerdict_UpheldAppliesViolation(t *testing.T) {
	reportID := uuid.New().String()
	db.DB.Exec(`INSERT INTO spam_reports (id, reporter_did, reported_did, category, status, created_at)
		VALUES (?, 'did:obs:reporter-g', 'did:obs:accused-4', 'spam', 'pending', ?)`,
		reportID, time.Now().UTC().Format(time.RFC3339))

	body, _ := json.Marshal(map[string]bool{"upheld": true})
	req := withUser(httptest.NewRequest("POST", "/v1/complaints/"+reportID+"/verdict", bytes.NewReader(body)),
		&models.User{DID: "did:obs:admin", Tier: 5})
	req = mux.SetURLVars(req, map[string]string{"id": reportID})
	rec := httptest.NewRecorder()

	HandleComplaintVerdict(rec, req)

	resp := decodeResp(t, rec)
	if !resp.Success {
		t.Fatalf("expected success, got error: %s", resp.Error)
	}

	var violationCount int
	db.DB.QueryRow(`SELECT COUNT(*) FROM user_violations WHERE user_did = ?`, "did:obs:accused-4").Scan(&violationCount)
	if violationCount != 1 {
		t.Fatalf("expected 1 violation on accused after upheld verdict, got %d", violationCount)
	}
}
