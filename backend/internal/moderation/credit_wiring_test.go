package moderation

// Tests for the Madde 4 credit wiring added to RecordComplaintVerdict
// (spam_received / spam_false / fraud). RecordComplaintVerdict itself is
// exercised against newViolationsFixture's local, minimal *sql.DB (same as
// every other test in this package — user_violations/spam_reports/etc.
// bookkeeping doesn't need the full schema). credit.AddEvent, however,
// always reads/writes through the GLOBAL db.DB singleton (internal/db),
// which no test in this package previously initialized — this file's
// TestMain fixes that (same db.Init(tmpDir) pattern as internal/credit,
// internal/governance, internal/airdrop). A DID used in a credit-wiring
// assertion must be seeded into BOTH: the local fixture (so RecordViolation/
// updateCredibility have a row to update) and the global db.DB (so
// credit.AddEvent has a users.credit_score row to read).

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"obscura.network/core/internal/credit"
	"obscura.network/core/internal/db"
)

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "obscura-moderation-test-*")
	if err != nil {
		panic("temp dir: " + err.Error())
	}
	if err := db.Init(tmpDir); err != nil {
		panic("test DB init: " + err.Error())
	}
	code := m.Run()
	db.Close()
	os.RemoveAll(tmpDir)
	os.Exit(code)
}

func seedCreditUser(t *testing.T, did string) {
	t.Helper()
	uid := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.DB.Exec(`
		INSERT INTO users (id, phone, did, tier, credit_score, created_at, updated_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		uid, "mod-credit-"+uid, did, 1, 0.0, now, now, now,
	)
	if err != nil {
		t.Fatalf("seedCreditUser(%s): %v", did, err)
	}
}

func creditScore(t *testing.T, did string) float64 {
	t.Helper()
	var score float64
	if err := db.DB.QueryRow("SELECT credit_score FROM users WHERE did = ?", did).Scan(&score); err != nil {
		t.Fatalf("creditScore(%s): %v", did, err)
	}
	return score
}

func creditEventDelta(t *testing.T, did, eventType string) (delta float64, found bool) {
	t.Helper()
	err := db.DB.QueryRow(
		`SELECT delta FROM credit_events WHERE user_did = ? AND event_type = ?`,
		did, eventType,
	).Scan(&delta)
	if err == sql.ErrNoRows {
		return 0, false
	}
	if err != nil {
		t.Fatalf("creditEventDelta(%s, %s): %v", did, eventType, err)
	}
	return delta, true
}

func TestRecordComplaintVerdict_UpheldSpam_AwardsSpamReceivedCredit(t *testing.T) {
	fixture := newViolationsFixture(t)
	ctx := context.Background()
	reportedDID := "did:obs:credit-spam-accused-" + uuid.New().String()
	reporterDID := "did:obs:credit-spam-reporter-" + uuid.New().String()
	seedCreditUser(t, reportedDID)
	fixture.Exec(`INSERT INTO users (did) VALUES (?)`, reportedDID)
	fixture.Exec(`INSERT INTO spam_reports (id, reporter_did, reported_did, category, status, created_at) VALUES (?, ?, ?, ?, 'pending', ?)`,
		"rep-credit-spam", reporterDID, reportedDID, CategorySpam, time.Now().UTC().Format(time.RFC3339))

	if err := RecordComplaintVerdict(ctx, fixture, "rep-credit-spam", true); err != nil {
		t.Fatalf("RecordComplaintVerdict: %v", err)
	}

	delta, found := creditEventDelta(t, reportedDID, credit.EventSpamReceived)
	if !found {
		t.Fatal("spam_received credit_events satırı bulunamadı")
	}
	if delta != credit.EventDeltas[credit.EventSpamReceived] {
		t.Errorf("delta = %v, beklenen %v", delta, credit.EventDeltas[credit.EventSpamReceived])
	}
	if got := creditScore(t, reportedDID); got != delta {
		t.Errorf("credit_score = %v, beklenen %v (0 + delta)", got, delta)
	}
}

func TestRecordComplaintVerdict_UpheldScam_AwardsFraudCredit(t *testing.T) {
	fixture := newViolationsFixture(t)
	ctx := context.Background()
	reportedDID := "did:obs:credit-scam-accused-" + uuid.New().String()
	reporterDID := "did:obs:credit-scam-reporter-" + uuid.New().String()
	seedCreditUser(t, reportedDID)
	fixture.Exec(`INSERT INTO users (did) VALUES (?)`, reportedDID)
	fixture.Exec(`INSERT INTO spam_reports (id, reporter_did, reported_did, category, status, created_at) VALUES (?, ?, ?, ?, 'pending', ?)`,
		"rep-credit-scam", reporterDID, reportedDID, CategoryScam, time.Now().UTC().Format(time.RFC3339))

	if err := RecordComplaintVerdict(ctx, fixture, "rep-credit-scam", true); err != nil {
		t.Fatalf("RecordComplaintVerdict: %v", err)
	}

	delta, found := creditEventDelta(t, reportedDID, credit.EventFraud)
	if !found {
		t.Fatal("fraud credit_events satırı bulunamadı")
	}
	if delta != credit.EventDeltas[credit.EventFraud] {
		t.Errorf("delta = %v, beklenen %v", delta, credit.EventDeltas[credit.EventFraud])
	}
	// EventFraud = -20, global taban -20 ile tam çakışıyor — kategori tavanı
	// (-100) burada devreye girmiyor.
	if got := creditScore(t, reportedDID); got != -20.0 {
		t.Errorf("credit_score = %v, beklenen -20.0", got)
	}
}

func TestRecordComplaintVerdict_False_AwardsSpamFalseCredit(t *testing.T) {
	fixture := newViolationsFixture(t)
	ctx := context.Background()
	reportedDID := "did:obs:credit-false-accused-" + uuid.New().String()
	reporterDID := "did:obs:credit-false-reporter-" + uuid.New().String()
	seedCreditUser(t, reporterDID)
	fixture.Exec(`INSERT INTO users (did) VALUES (?)`, reporterDID)
	fixture.Exec(`INSERT INTO spam_reports (id, reporter_did, reported_did, category, status, created_at) VALUES (?, ?, ?, ?, 'pending', ?)`,
		"rep-credit-false", reporterDID, reportedDID, CategorySpam, time.Now().UTC().Format(time.RFC3339))

	if err := RecordComplaintVerdict(ctx, fixture, "rep-credit-false", false); err != nil {
		t.Fatalf("RecordComplaintVerdict: %v", err)
	}

	delta, found := creditEventDelta(t, reporterDID, credit.EventSpamFalse)
	if !found {
		t.Fatal("spam_false credit_events satırı bulunamadı")
	}
	if delta != credit.EventDeltas[credit.EventSpamFalse] {
		t.Errorf("delta = %v, beklenen %v", delta, credit.EventDeltas[credit.EventSpamFalse])
	}
	if got := creditScore(t, reporterDID); got != delta {
		t.Errorf("credit_score = %v, beklenen %v", got, delta)
	}
}

// TestRecordComplaintVerdict_FalseNonSpamCategory_NoCreditEvent — spam_false
// SADECE orijinal complaint spam-kategorisiyken tetiklenmeli (spec §7.1
// "Spam raporu (verme, yanlış)" spam-spesifik). Taciz gibi başka bir
// kategoride açılan şikayet asılsız çıkarsa credit_events'e hiçbir satır
// düşmemeli — ama RecordViolation'ın kendisi (user_violations, Bölüm 4
// "yalan şikayet ihlaldir") hâlâ eskisi gibi çalışmalı, sadece kredi
// tarafı etkilenmemeli.
func TestRecordComplaintVerdict_FalseNonSpamCategory_NoCreditEvent(t *testing.T) {
	fixture := newViolationsFixture(t)
	ctx := context.Background()
	reportedDID := "did:obs:credit-falsenonspam-accused-" + uuid.New().String()
	reporterDID := "did:obs:credit-falsenonspam-reporter-" + uuid.New().String()
	seedCreditUser(t, reporterDID)
	fixture.Exec(`INSERT INTO users (did) VALUES (?)`, reporterDID)
	fixture.Exec(`INSERT INTO spam_reports (id, reporter_did, reported_did, category, status, created_at) VALUES (?, ?, ?, ?, 'pending', ?)`,
		"rep-credit-falsenonspam", reporterDID, reportedDID, CategoryHarassment, time.Now().UTC().Format(time.RFC3339))

	if err := RecordComplaintVerdict(ctx, fixture, "rep-credit-falsenonspam", false); err != nil {
		t.Fatalf("RecordComplaintVerdict: %v", err)
	}

	if _, found := creditEventDelta(t, reporterDID, credit.EventSpamFalse); found {
		t.Error("taciz kategorisi asılsız çıkınca spam_false tetiklenmemeli")
	}
	if got := creditScore(t, reporterDID); got != 0.0 {
		t.Errorf("credit_score = %v, beklenen 0.0 (değişmemeli)", got)
	}

	// RecordViolation'ın kendisi (moderasyon/ban ekseni) hâlâ çalışmalı —
	// kredi tarafı etkilenmemesi, ihlal kaydının atlanması demek değil.
	var violationCount int
	if err := fixture.QueryRow(
		`SELECT COUNT(*) FROM user_violations WHERE user_did = ? AND source_report_id = ?`,
		reporterDID, "rep-credit-falsenonspam",
	).Scan(&violationCount); err != nil {
		t.Fatalf("user_violations sayımı: %v", err)
	}
	if violationCount != 1 {
		t.Errorf("user_violations satır sayısı = %d, beklenen 1 (RecordViolation etkilenmemeli)", violationCount)
	}
}

// TestRecordComplaintVerdict_UpheldHarassment_NoCreditEvent — kredi olayı
// SADECE spam/scam kategorisinde tetiklenmeli (Adım A talimatı). Taciz gibi
// diğer kapalı-liste kategorileri user_violations'ı etkiler ama spec §7.1
// kredi matrisinde ayrı bir davranış değil — credit_events'e hiçbir satır
// düşmemeli.
func TestRecordComplaintVerdict_UpheldHarassment_NoCreditEvent(t *testing.T) {
	fixture := newViolationsFixture(t)
	ctx := context.Background()
	reportedDID := "did:obs:credit-harassment-accused-" + uuid.New().String()
	reporterDID := "did:obs:credit-harassment-reporter-" + uuid.New().String()
	seedCreditUser(t, reportedDID)
	fixture.Exec(`INSERT INTO users (did) VALUES (?)`, reportedDID)
	fixture.Exec(`INSERT INTO spam_reports (id, reporter_did, reported_did, category, status, created_at) VALUES (?, ?, ?, ?, 'pending', ?)`,
		"rep-credit-harassment", reporterDID, reportedDID, CategoryHarassment, time.Now().UTC().Format(time.RFC3339))

	if err := RecordComplaintVerdict(ctx, fixture, "rep-credit-harassment", true); err != nil {
		t.Fatalf("RecordComplaintVerdict: %v", err)
	}

	if _, found := creditEventDelta(t, reportedDID, credit.EventSpamReceived); found {
		t.Error("taciz kategorisi spam_received tetiklememeli")
	}
	if _, found := creditEventDelta(t, reportedDID, credit.EventFraud); found {
		t.Error("taciz kategorisi fraud tetiklememeli")
	}
	if got := creditScore(t, reportedDID); got != 0.0 {
		t.Errorf("credit_score = %v, beklenen 0.0 (değişmemeli)", got)
	}
}
