package moderation

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// openTestDB — her test için izole in-memory SQLite DB açar.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("test DB açılamadı: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	schema := `
	CREATE TABLE IF NOT EXISTS group_reports (
		id           TEXT PRIMARY KEY,
		group_id     TEXT NOT NULL,
		reporter_did TEXT NOT NULL,
		reason       TEXT NOT NULL,
		created_at   INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_group_reports_group ON group_reports(group_id, created_at);
	CREATE TABLE IF NOT EXISTS group_moderation (
		group_id     TEXT PRIMARY KEY,
		is_reviewing INTEGER NOT NULL DEFAULT 0,
		is_suspended INTEGER NOT NULL DEFAULT 0,
		report_count INTEGER NOT NULL DEFAULT 0,
		last_review  INTEGER NOT NULL
	);`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("şema oluşturulamadı: %v", err)
	}
	return db
}

// ─── MaxGroupSize testleri ────────────────────────────────────────────────────

func TestMaxGroupSize(t *testing.T) {
	cases := []struct {
		score    float64
		expected int
	}{
		{90.0, 10000},
		{95.0, 10000},
		{70.0, 500},
		{85.0, 500},
		{40.0, 50},
		{60.0, 50},
		{0.0, 10},
		{39.9, 10},
	}
	for _, c := range cases {
		got := MaxGroupSize(c.score)
		if got != c.expected {
			t.Errorf("MaxGroupSize(%.1f) = %d; istenen %d", c.score, got, c.expected)
		}
	}
}

// ─── ReportGroup testleri ─────────────────────────────────────────────────────

func TestReportGroup_SingleReport(t *testing.T) {
	db := openTestDB(t)

	if err := ReportGroup(db, "group-1", "did:obs:alice", "spam"); err != nil {
		t.Fatalf("ReportGroup hata: %v", err)
	}

	var count int
	db.QueryRow("SELECT COUNT(*) FROM group_reports WHERE group_id = 'group-1'").Scan(&count)
	if count != 1 {
		t.Errorf("beklenen 1 rapor, %d bulundu", count)
	}

	// 1 rapor → incelemeye alınmamış olmalı
	status, err := GetGroupModerationStatus(db, "group-1")
	if err != nil {
		t.Fatalf("GetGroupModerationStatus hata: %v", err)
	}
	if status.IsReviewing {
		t.Error("1 rapordan sonra incelemeye alınmamalıydı")
	}
}

func TestReportGroup_AutoFlag_ThreeReports(t *testing.T) {
	db := openTestDB(t)

	for i, reporter := range []string{"did:obs:user1", "did:obs:user2", "did:obs:user3"} {
		reason := "spam"
		if i == 2 {
			reason = "scam"
		}
		if err := ReportGroup(db, "group-flagged", reporter, reason); err != nil {
			t.Fatalf("ReportGroup hata (i=%d): %v", i, err)
		}
	}

	status, err := GetGroupModerationStatus(db, "group-flagged")
	if err != nil {
		t.Fatalf("GetGroupModerationStatus hata: %v", err)
	}
	if !status.IsReviewing {
		t.Error("3 rapordan sonra grup incelemeye alınmalıydı")
	}
	if status.IsSuspended {
		t.Error("otomatik inceleme askıya alma değildir; is_suspended 0 olmalı")
	}
	if status.ReportCount != 3 {
		t.Errorf("beklenen report_count=3, %d bulundu", status.ReportCount)
	}
}

func TestReportGroup_NilDB(t *testing.T) {
	err := ReportGroup(nil, "group-x", "did:obs:alice", "spam")
	if err == nil {
		t.Error("nil DB ile hata bekleniyor")
	}
}

func TestReportGroup_EmptyGroupID(t *testing.T) {
	db := openTestDB(t)
	err := ReportGroup(db, "", "did:obs:alice", "spam")
	if err == nil {
		t.Error("boş group_id ile hata bekleniyor")
	}
}

// ─── IsGroupSuspended testleri ────────────────────────────────────────────────

func TestIsGroupSuspended_NotInTable(t *testing.T) {
	db := openTestDB(t)
	suspended, err := IsGroupSuspended(db, "group-unknown")
	if err != nil {
		t.Fatalf("IsGroupSuspended hata: %v", err)
	}
	if suspended {
		t.Error("tabloda olmayan grup askıda sayılmamalı")
	}
}

func TestIsGroupSuspended_Suspended(t *testing.T) {
	db := openTestDB(t)
	db.Exec(`INSERT INTO group_moderation (group_id, is_reviewing, is_suspended, report_count, last_review)
		VALUES ('group-bad', 1, 1, 5, 9999999999)`)

	suspended, err := IsGroupSuspended(db, "group-bad")
	if err != nil {
		t.Fatalf("IsGroupSuspended hata: %v", err)
	}
	if !suspended {
		t.Error("is_suspended=1 olan grup askıda görünmeli")
	}
}

func TestIsGroupSuspended_Reviewing_NotSuspended(t *testing.T) {
	db := openTestDB(t)
	db.Exec(`INSERT INTO group_moderation (group_id, is_reviewing, is_suspended, report_count, last_review)
		VALUES ('group-review', 1, 0, 3, 9999999999)`)

	suspended, err := IsGroupSuspended(db, "group-review")
	if err != nil {
		t.Fatalf("IsGroupSuspended hata: %v", err)
	}
	if suspended {
		t.Error("is_suspended=0 olan grup askıda sayılmamalı")
	}
}

// ─── AutoModerateMessage testleri ─────────────────────────────────────────────

func TestAutoModerateMessage_Clean(t *testing.T) {
	suspicious, score := AutoModerateMessage("Merhaba, bugün nasılsın?")
	if suspicious {
		t.Error("temiz mesaj şüpheli işaretlenmemeli")
	}
	if score != 0.0 {
		t.Errorf("temiz mesaj skoru 0.0 olmalı, %f geldi", score)
	}
}

func TestAutoModerateMessage_Spam(t *testing.T) {
	suspicious, score := AutoModerateMessage("Bu bir spam mesajı")
	if !suspicious {
		t.Error("spam içerikli mesaj şüpheli işaretlenmeli")
	}
	if score < 0.8 {
		t.Errorf("spam skoru >= 0.8 olmalı, %f geldi", score)
	}
}

func TestAutoModerateMessage_Scam(t *testing.T) {
	suspicious, _ := AutoModerateMessage("SCAM ALERT buy now")
	if !suspicious {
		t.Error("scam içerikli mesaj (büyük harf) şüpheli işaretlenmeli")
	}
}

func TestAutoModerateMessage_Phishing(t *testing.T) {
	suspicious, _ := AutoModerateMessage("phishing link click here")
	if !suspicious {
		t.Error("phishing içerikli mesaj şüpheli işaretlenmeli")
	}
}

// ─── containsSubstr / toLower testleri ───────────────────────────────────────

func TestContainsSubstr(t *testing.T) {
	if !containsSubstr("hello world", "world") {
		t.Error("'world' 'hello world' içinde bulunmalı")
	}
	if containsSubstr("hello", "world") {
		t.Error("'world' 'hello' içinde bulunmamalı")
	}
	if containsSubstr("", "x") {
		t.Error("boş string'de alt string bulunmamalı")
	}
	if containsSubstr("x", "") {
		t.Error("boş alt string — false döndürmeli")
	}
}

func TestToLower(t *testing.T) {
	if toLower("SPAM") != "spam" {
		t.Error("SPAM → spam dönüşümü başarısız")
	}
	if toLower("MixEd") != "mixed" {
		t.Error("MixEd → mixed dönüşümü başarısız")
	}
}
