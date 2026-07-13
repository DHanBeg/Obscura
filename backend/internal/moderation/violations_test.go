package moderation

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newViolationsFixture(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	schema := `
	CREATE TABLE users (did TEXT PRIMARY KEY, is_banned INTEGER DEFAULT 0, ban_expires_at TEXT);
	CREATE TABLE user_violations (
		id TEXT PRIMARY KEY, user_did TEXT, category TEXT, tier INTEGER, action TEXT,
		source_report_id TEXT, created_at TEXT, expires_at TEXT
	);
	CREATE TABLE spam_reports (
		id TEXT PRIMARY KEY, reporter_did TEXT, reported_did TEXT, reason TEXT,
		status TEXT, created_at TEXT, reviewed_at TEXT,
		message_id TEXT, evidence_screenshot_url TEXT, evidence_ciphertext_hash TEXT,
		evidence_verified INTEGER, category TEXT, verdict TEXT
	);
	CREATE TABLE complainant_credibility (
		user_did TEXT PRIMARY KEY, upheld_count INTEGER, false_count INTEGER,
		weight REAL, updated_at TEXT
	);
	CREATE TABLE review_queue (
		id TEXT PRIMARY KEY, report_id TEXT, reason TEXT, status TEXT, created_at TEXT
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return db
}

func TestRecordViolation_TieredEscalation(t *testing.T) {
	db := newViolationsFixture(t)
	ctx := context.Background()
	if _, err := db.Exec(`INSERT INTO users (did) VALUES ('did:obs:alice')`); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	action, err := RecordViolation(ctx, db, "did:obs:alice", CategorySpam, "rep-1", false)
	if err != nil {
		t.Fatalf("1st violation: %v", err)
	}
	if action != ActionWarning {
		t.Fatalf("1st violation action = %q, want %q", action, ActionWarning)
	}
	var isBanned int
	db.QueryRow(`SELECT is_banned FROM users WHERE did = ?`, "did:obs:alice").Scan(&isBanned)
	if isBanned != 0 {
		t.Fatal("warning must not ban the user")
	}

	action, err = RecordViolation(ctx, db, "did:obs:alice", CategorySpam, "rep-2", false)
	if err != nil {
		t.Fatalf("2nd violation: %v", err)
	}
	if action != ActionRestrict7d {
		t.Fatalf("2nd violation action = %q, want %q", action, ActionRestrict7d)
	}
	assertBannedWithExpiryNear(t, db, "did:obs:alice", 7*24*time.Hour)

	action, err = RecordViolation(ctx, db, "did:obs:alice", CategorySpam, "rep-3", false)
	if err != nil {
		t.Fatalf("3rd violation: %v", err)
	}
	if action != ActionRestrict30d {
		t.Fatalf("3rd violation action = %q, want %q", action, ActionRestrict30d)
	}
	assertBannedWithExpiryNear(t, db, "did:obs:alice", 30*24*time.Hour)

	// 4th+ violation: doc defines no step beyond 30 days — caps at restrict_30d.
	action, err = RecordViolation(ctx, db, "did:obs:alice", CategorySpam, "rep-4", false)
	if err != nil {
		t.Fatalf("4th violation: %v", err)
	}
	if action != ActionRestrict30d {
		t.Fatalf("4th violation action = %q, want %q (capped)", action, ActionRestrict30d)
	}
}

func TestRecordViolation_SevereSkipsToTopTier(t *testing.T) {
	db := newViolationsFixture(t)
	ctx := context.Background()
	db.Exec(`INSERT INTO users (did) VALUES ('did:obs:bob')`)

	// First-ever violation, but severe (e.g. scam/harassment) — must skip
	// straight to the top tier per Bölüm 3.1, not start at warning.
	action, err := RecordViolation(ctx, db, "did:obs:bob", CategoryScam, "rep-1", true)
	if err != nil {
		t.Fatalf("severe violation: %v", err)
	}
	if action != ActionRestrict30d {
		t.Fatalf("severe 1st violation action = %q, want %q", action, ActionRestrict30d)
	}
	assertBannedWithExpiryNear(t, db, "did:obs:bob", 30*24*time.Hour)
}

func assertBannedWithExpiryNear(t *testing.T, db *sql.DB, did string, wantDuration time.Duration) {
	t.Helper()
	var isBanned int
	var expiresAt string
	if err := db.QueryRow(`SELECT is_banned, ban_expires_at FROM users WHERE did = ?`, did).Scan(&isBanned, &expiresAt); err != nil {
		t.Fatalf("read user: %v", err)
	}
	if isBanned != 1 {
		t.Fatal("expected user to be banned")
	}
	parsed, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		t.Fatalf("ban_expires_at not RFC3339: %v", err)
	}
	wantAt := time.Now().UTC().Add(wantDuration)
	diff := parsed.Sub(wantAt)
	if diff < -1*time.Minute || diff > 1*time.Minute {
		t.Fatalf("ban_expires_at = %v, want ~%v (diff %v)", parsed, wantAt, diff)
	}
}

func TestIsBrigading(t *testing.T) {
	db := newViolationsFixture(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Under threshold — not brigading.
	for i := 0; i < BrigadingThreshold-1; i++ {
		db.Exec(`INSERT INTO spam_reports (id, reporter_did, reported_did, status, created_at) VALUES (?, ?, 'did:obs:target', 'pending', ?)`,
			"r"+string(rune('a'+i)), "did:obs:reporter"+string(rune('a'+i)), now.Format(time.RFC3339))
	}
	brigading, err := IsBrigading(ctx, db, "did:obs:target")
	if err != nil {
		t.Fatalf("IsBrigading: %v", err)
	}
	if brigading {
		t.Fatal("expected not brigading under threshold")
	}

	// One more report crosses the threshold.
	db.Exec(`INSERT INTO spam_reports (id, reporter_did, reported_did, status, created_at) VALUES (?, ?, 'did:obs:target', 'pending', ?)`,
		"r-final", "did:obs:reporter-final", now.Format(time.RFC3339))
	brigading, err = IsBrigading(ctx, db, "did:obs:target")
	if err != nil {
		t.Fatalf("IsBrigading: %v", err)
	}
	if !brigading {
		t.Fatal("expected brigading at threshold")
	}
}

func TestIsBrigading_OutsideWindowDoesNotCount(t *testing.T) {
	db := newViolationsFixture(t)
	ctx := context.Background()
	old := time.Now().UTC().Add(-2 * BrigadingWindow).Format(time.RFC3339)

	for i := 0; i < BrigadingThreshold+2; i++ {
		db.Exec(`INSERT INTO spam_reports (id, reporter_did, reported_did, status, created_at) VALUES (?, ?, 'did:obs:target', 'pending', ?)`,
			"old"+string(rune('a'+i)), "did:obs:reporter"+string(rune('a'+i)), old)
	}
	brigading, err := IsBrigading(ctx, db, "did:obs:target")
	if err != nil {
		t.Fatalf("IsBrigading: %v", err)
	}
	if brigading {
		t.Fatal("old reports outside the window must not count toward brigading")
	}
}

func TestRecordComplaintVerdict_Upheld(t *testing.T) {
	db := newViolationsFixture(t)
	ctx := context.Background()
	db.Exec(`INSERT INTO users (did) VALUES ('did:obs:accused')`)
	db.Exec(`INSERT INTO spam_reports (id, reporter_did, reported_did, category, status, created_at) VALUES (?, ?, ?, ?, 'pending', ?)`,
		"rep-1", "did:obs:victim", "did:obs:accused", CategoryHarassment, time.Now().UTC().Format(time.RFC3339))

	if err := RecordComplaintVerdict(ctx, db, "rep-1", true); err != nil {
		t.Fatalf("RecordComplaintVerdict: %v", err)
	}

	// Harassment is severe → straight to 30-day restriction on the accused.
	assertBannedWithExpiryNear(t, db, "did:obs:accused", 30*24*time.Hour)

	var verdict, status string
	db.QueryRow(`SELECT verdict, status FROM spam_reports WHERE id = 'rep-1'`).Scan(&verdict, &status)
	if verdict != "upheld" || status != "reviewed" {
		t.Fatalf("verdict=%q status=%q, want upheld/reviewed", verdict, status)
	}

	var upheldCount int
	db.QueryRow(`SELECT upheld_count FROM complainant_credibility WHERE user_did = 'did:obs:victim'`).Scan(&upheldCount)
	if upheldCount != 1 {
		t.Fatalf("victim upheld_count = %d, want 1", upheldCount)
	}
}

func TestRecordComplaintVerdict_False_PunishesReporter(t *testing.T) {
	db := newViolationsFixture(t)
	ctx := context.Background()
	db.Exec(`INSERT INTO users (did) VALUES ('did:obs:falsereporter')`)
	db.Exec(`INSERT INTO spam_reports (id, reporter_did, reported_did, category, status, created_at) VALUES (?, ?, ?, ?, 'pending', ?)`,
		"rep-2", "did:obs:falsereporter", "did:obs:innocent", CategorySpam, time.Now().UTC().Format(time.RFC3339))

	if err := RecordComplaintVerdict(ctx, db, "rep-2", false); err != nil {
		t.Fatalf("RecordComplaintVerdict: %v", err)
	}

	// False complaint: the REPORTER (not the accused) takes the violation.
	var violCategory string
	if err := db.QueryRow(`SELECT category FROM user_violations WHERE user_did = 'did:obs:falsereporter'`).Scan(&violCategory); err != nil {
		t.Fatalf("expected a violation recorded against the reporter: %v", err)
	}
	if violCategory != CategoryFalseReport {
		t.Fatalf("violation category = %q, want %q", violCategory, CategoryFalseReport)
	}

	var innocentHasViolation int
	db.QueryRow(`SELECT COUNT(*) FROM user_violations WHERE user_did = 'did:obs:innocent'`).Scan(&innocentHasViolation)
	if innocentHasViolation != 0 {
		t.Fatal("the falsely-accused user must not receive any violation")
	}

	var falseCount int
	db.QueryRow(`SELECT false_count FROM complainant_credibility WHERE user_did = 'did:obs:falsereporter'`).Scan(&falseCount)
	if falseCount != 1 {
		t.Fatalf("reporter false_count = %d, want 1", falseCount)
	}
}

func TestEnqueueReview(t *testing.T) {
	db := newViolationsFixture(t)
	ctx := context.Background()
	if err := EnqueueReview(ctx, db, "rep-1", "brigading"); err != nil {
		t.Fatalf("EnqueueReview: %v", err)
	}
	var status, reason string
	if err := db.QueryRow(`SELECT status, reason FROM review_queue WHERE report_id = 'rep-1'`).Scan(&status, &reason); err != nil {
		t.Fatalf("read review_queue: %v", err)
	}
	if status != "pending" || reason != "brigading" {
		t.Fatalf("status=%q reason=%q, want pending/brigading", status, reason)
	}
}

func TestRecordComplaintVerdict_False_ResolvesReviewQueue(t *testing.T) {
	db := newViolationsFixture(t)
	ctx := context.Background()
	db.Exec(`INSERT INTO users (did) VALUES ('did:obs:falsereporter2')`)
	db.Exec(`INSERT INTO spam_reports (id, reporter_did, reported_did, category, status, created_at) VALUES (?, ?, ?, ?, 'pending', ?)`,
		"rep-q1", "did:obs:falsereporter2", "did:obs:innocent2", CategorySpam, time.Now().UTC().Format(time.RFC3339))
	if err := EnqueueReview(ctx, db, "rep-q1", "insan incelemesi bekliyor"); err != nil {
		t.Fatalf("EnqueueReview: %v", err)
	}

	if err := RecordComplaintVerdict(ctx, db, "rep-q1", false); err != nil {
		t.Fatalf("RecordComplaintVerdict: %v", err)
	}

	var status string
	if err := db.QueryRow(`SELECT status FROM review_queue WHERE report_id = 'rep-q1'`).Scan(&status); err != nil {
		t.Fatalf("read review_queue: %v", err)
	}
	if status != "resolved" {
		t.Fatalf("review_queue status = %q, want %q (dangling pending row after verdict)", status, "resolved")
	}
}

func TestRecordComplaintVerdict_Upheld_ResolvesReviewQueue(t *testing.T) {
	db := newViolationsFixture(t)
	ctx := context.Background()
	db.Exec(`INSERT INTO users (did) VALUES ('did:obs:accused2')`)
	db.Exec(`INSERT INTO spam_reports (id, reporter_did, reported_did, category, status, created_at) VALUES (?, ?, ?, ?, 'pending', ?)`,
		"rep-q2", "did:obs:victim2", "did:obs:accused2", CategorySpam, time.Now().UTC().Format(time.RFC3339))
	if err := EnqueueReview(ctx, db, "rep-q2", "insan incelemesi bekliyor"); err != nil {
		t.Fatalf("EnqueueReview: %v", err)
	}

	if err := RecordComplaintVerdict(ctx, db, "rep-q2", true); err != nil {
		t.Fatalf("RecordComplaintVerdict: %v", err)
	}

	var status string
	db.QueryRow(`SELECT status FROM review_queue WHERE report_id = 'rep-q2'`).Scan(&status)
	if status != "resolved" {
		t.Fatalf("review_queue status = %q, want resolved after upheld verdict too", status)
	}
}

func TestRecordComplaintVerdict_RepeatFalseAgainstSameTarget_EscalatesToHarassment(t *testing.T) {
	db := newViolationsFixture(t)
	ctx := context.Background()
	db.Exec(`INSERT INTO users (did) VALUES ('did:obs:serialaccuser')`)

	// 1st and 2nd false reports against the SAME target — ordinary
	// false_report tiering (warning, then 7d), no escalation yet.
	db.Exec(`INSERT INTO spam_reports (id, reporter_did, reported_did, category, status, created_at) VALUES (?, ?, ?, ?, 'pending', ?)`,
		"rep-r1", "did:obs:serialaccuser", "did:obs:target-x", CategorySpam, time.Now().UTC().Format(time.RFC3339))
	if err := RecordComplaintVerdict(ctx, db, "rep-r1", false); err != nil {
		t.Fatalf("1st false verdict: %v", err)
	}
	db.Exec(`INSERT INTO spam_reports (id, reporter_did, reported_did, category, status, created_at) VALUES (?, ?, ?, ?, 'pending', ?)`,
		"rep-r2", "did:obs:serialaccuser", "did:obs:target-x", CategorySpam, time.Now().UTC().Format(time.RFC3339))
	if err := RecordComplaintVerdict(ctx, db, "rep-r2", false); err != nil {
		t.Fatalf("2nd false verdict: %v", err)
	}

	// 3rd false report against the SAME target (did:obs:target-x again) —
	// Bölüm 4 "tekrar tekrar asılsız şikayet" pattern must fire: category
	// escalates to harassment and skips straight to the top tier, regardless
	// of the reporter's own global violation count.
	db.Exec(`INSERT INTO spam_reports (id, reporter_did, reported_did, category, status, created_at) VALUES (?, ?, ?, ?, 'pending', ?)`,
		"rep-r3", "did:obs:serialaccuser", "did:obs:target-x", CategorySpam, time.Now().UTC().Format(time.RFC3339))
	if err := RecordComplaintVerdict(ctx, db, "rep-r3", false); err != nil {
		t.Fatalf("3rd false verdict: %v", err)
	}

	var lastCategory, lastAction string
	if err := db.QueryRow(
		`SELECT category, action FROM user_violations WHERE user_did = 'did:obs:serialaccuser' AND source_report_id = 'rep-r3'`,
	).Scan(&lastCategory, &lastAction); err != nil {
		t.Fatalf("read escalated violation: %v", err)
	}
	if lastCategory != CategoryHarassment {
		t.Fatalf("3rd repeat-false-against-same-target category = %q, want %q", lastCategory, CategoryHarassment)
	}
	if lastAction != ActionRestrict30d {
		t.Fatalf("3rd repeat-false-against-same-target action = %q, want %q (doğrudan üst basamak)", lastAction, ActionRestrict30d)
	}
}

func TestRecordComplaintVerdict_RepeatFalseAgainstDifferentTargets_DoesNotEscalate(t *testing.T) {
	db := newViolationsFixture(t)
	ctx := context.Background()
	db.Exec(`INSERT INTO users (did) VALUES ('did:obs:scattershot')`)

	targets := []string{"did:obs:t1", "did:obs:t2", "did:obs:t3"}
	for i, target := range targets {
		reportID := "rep-scatter-" + string(rune('a'+i))
		db.Exec(`INSERT INTO spam_reports (id, reporter_did, reported_did, category, status, created_at) VALUES (?, ?, ?, ?, 'pending', ?)`,
			reportID, "did:obs:scattershot", target, CategorySpam, time.Now().UTC().Format(time.RFC3339))
		if err := RecordComplaintVerdict(ctx, db, reportID, false); err != nil {
			t.Fatalf("false verdict against %s: %v", target, err)
		}
	}

	// Three false reports total, but spread across three DIFFERENT targets —
	// the "aynı kişiyi hedef alma" pattern never fires. Each must stay a plain
	// false_report (global tiering escalates the ACTION via prior count, but
	// never the CATEGORY to harassment).
	rows, err := db.Query(`SELECT category FROM user_violations WHERE user_did = 'did:obs:scattershot'`)
	if err != nil {
		t.Fatalf("query violations: %v", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		count++
		var cat string
		rows.Scan(&cat)
		if cat != CategoryFalseReport {
			t.Fatalf("category = %q, want %q (no same-target pattern, must not escalate)", cat, CategoryFalseReport)
		}
	}
	if count != 3 {
		t.Fatalf("expected 3 violations recorded, got %d", count)
	}
}

func TestIsKnownCategory(t *testing.T) {
	for _, c := range []string{CategorySpam, CategoryScam, CategoryHarassment, CategoryCopyright, CategoryIllegalSale, CategoryChildSafety} {
		if !IsKnownCategory(c) {
			t.Errorf("IsKnownCategory(%q) = false, want true", c)
		}
	}
	if IsKnownCategory(CategoryFalseReport) {
		t.Error("false_report must NOT be a filable category — it's an internal sanction only")
	}
	if IsKnownCategory("uygunsuzluk") {
		t.Error("subjective categories outside the closed list must be rejected (İlke 2)")
	}
}
