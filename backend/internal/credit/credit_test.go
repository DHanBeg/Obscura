package credit

// Tests for the plain server-push credit layer (AddEvent/AddCustomEvent/
// TrackDailyLogin/TrackMessageSent/GetHistory). Same TestMain DB bootstrap
// pattern as internal/airdrop and internal/governance: real pure-Go SQLite
// in a temp dir, migrations run via db.Init. package credit (not credit_test)
// to match the existing zk_credit_test.go in this directory — both share the
// same TestMain/db.DB for the package's test binary.

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"obscura.network/core/internal/db"
)

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "obscura-credit-test-*")
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

// ─── kullanıcı seed helper ───────────────────────────────────────────────────

// seedUser inserts a users row with an explicit starting credit_score, so
// clamp/boundary tests can start from a controlled value instead of the
// schema default (0).
func seedUser(t *testing.T, score float64) string {
	t.Helper()
	uid := uuid.New().String()
	did := "did:obs:credittest-" + uid
	phone := "credit-test-" + uid // TEXT UNIQUE NOT NULL, no format check in schema
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.DB.Exec(`
		INSERT INTO users (id, phone, did, tier, credit_score, created_at, updated_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		did, phone, did, 1, score, now, now, now,
	)
	if err != nil {
		t.Fatalf("seedUser: %v", err)
	}
	return did
}

func getScore(t *testing.T, did string) float64 {
	t.Helper()
	var score float64
	if err := db.DB.QueryRow("SELECT credit_score FROM users WHERE did = ?", did).Scan(&score); err != nil {
		t.Fatalf("getScore(%s): %v", did, err)
	}
	return score
}

func countCreditEvents(t *testing.T, did, eventType string) int {
	t.Helper()
	var n int
	if err := db.DB.QueryRow(
		`SELECT COUNT(*) FROM credit_events WHERE user_did = ? AND event_type = ?`,
		did, eventType,
	).Scan(&n); err != nil {
		t.Fatalf("countCreditEvents(%s, %s): %v", did, eventType, err)
	}
	return n
}

func getDailyActivity(t *testing.T, did, date string) (loginCount, msgCount int) {
	t.Helper()
	err := db.DB.QueryRow(
		`SELECT login_count, msg_count FROM daily_activity WHERE user_did = ? AND date = ?`,
		did, date,
	).Scan(&loginCount, &msgCount)
	if err != nil {
		t.Fatalf("getDailyActivity(%s, %s): %v", did, date, err)
	}
	return
}

// ─── AddEvent ────────────────────────────────────────────────────────────────

func TestAddEvent_KnownEventType(t *testing.T) {
	did := seedUser(t, 50.0)
	if err := AddEvent(did, EventCommunity, "test katkı"); err != nil {
		t.Fatalf("AddEvent: %v", err)
	}
	got := getScore(t, did)
	want := 50.0 + EventDeltas[EventCommunity]
	if got != want {
		t.Errorf("score = %v, beklenen %v", got, want)
	}
}

func TestAddEvent_UnknownEventType(t *testing.T) {
	did := seedUser(t, 50.0)
	err := AddEvent(did, "not_a_real_event_type", "x")
	if err == nil {
		t.Fatal("bilinmeyen event tipi için hata beklendi, nil döndü")
	}
	// Bilinmeyen tip DB'ye hiç dokunmamalı — skor sabit kalmalı.
	if got := getScore(t, did); got != 50.0 {
		t.Errorf("bilinmeyen event sonrası score değişmemeli, got=%v", got)
	}
}

// ─── AddCustomEvent ──────────────────────────────────────────────────────────

func TestAddCustomEvent_NormalDelta(t *testing.T) {
	did := seedUser(t, 40.0)
	if err := AddCustomEvent(did, "custom_test", 7.5, "özel olay"); err != nil {
		t.Fatalf("AddCustomEvent: %v", err)
	}
	if got := getScore(t, did); got != 47.5 {
		t.Errorf("score = %v, beklenen 47.5", got)
	}
}

func TestAddCustomEvent_ClampsAtUpperBound(t *testing.T) {
	did := seedUser(t, 99.0)
	if err := AddCustomEvent(did, "custom_test", 10.0, "üst sınır"); err != nil {
		t.Fatalf("AddCustomEvent: %v", err)
	}
	if got := getScore(t, did); got != 100.0 {
		t.Errorf("score = %v, beklenen clamp 100", got)
	}
}

func TestAddCustomEvent_ClampsAtLowerBound(t *testing.T) {
	did := seedUser(t, -18.0)
	if err := AddCustomEvent(did, "custom_test", -10.0, "alt sınır"); err != nil {
		t.Fatalf("AddCustomEvent: %v", err)
	}
	if got := getScore(t, did); got != -20.0 {
		t.Errorf("score = %v, beklenen clamp -20", got)
	}
}

func TestAddCustomEvent_UnknownDIDErrors(t *testing.T) {
	err := AddCustomEvent("did:obs:does-not-exist-"+uuid.New().String(), "custom_test", 1.0, "x")
	if err == nil {
		t.Fatal("olmayan DID için hata beklendi, nil döndü")
	}
}

func TestAddCustomEvent_InsertsCreditEventRow(t *testing.T) {
	did := seedUser(t, 30.0)
	if err := AddCustomEvent(did, "custom_test_row", 3.0, "satır kontrolü"); err != nil {
		t.Fatalf("AddCustomEvent: %v", err)
	}
	if n := countCreditEvents(t, did, "custom_test_row"); n != 1 {
		t.Errorf("credit_events satır sayısı = %d, beklenen 1", n)
	}
}

// ─── TrackDailyLogin ─────────────────────────────────────────────────────────

func TestTrackDailyLogin_FirstCallAwardsEvent(t *testing.T) {
	did := seedUser(t, 10.0)
	TrackDailyLogin(did)

	today := time.Now().Format("2006-01-02")
	loginCount, _ := getDailyActivity(t, did, today)
	if loginCount != 1 {
		t.Errorf("login_count = %d, beklenen 1", loginCount)
	}
	if n := countCreditEvents(t, did, EventDailyLogin); n != 1 {
		t.Errorf("daily_login event sayısı = %d, beklenen 1", n)
	}
	if got := getScore(t, did); got != 10.0+EventDeltas[EventDailyLogin] {
		t.Errorf("score = %v, beklenen %v", got, 10.0+EventDeltas[EventDailyLogin])
	}
}

func TestTrackDailyLogin_SameDaySecondCallIsNoop(t *testing.T) {
	did := seedUser(t, 10.0)
	TrackDailyLogin(did)
	scoreAfterFirst := getScore(t, did)

	TrackDailyLogin(did) // aynı gün ikinci çağrı

	today := time.Now().Format("2006-01-02")
	loginCount, _ := getDailyActivity(t, did, today)
	if loginCount != 1 {
		t.Errorf("aynı gün ikinci çağrıdan sonra login_count = %d, beklenen değişmeden 1", loginCount)
	}
	if n := countCreditEvents(t, did, EventDailyLogin); n != 1 {
		t.Errorf("aynı gün ikinci çağrıdan sonra daily_login event sayısı = %d, beklenen 1 (tekrar ödül yok)", n)
	}
	if got := getScore(t, did); got != scoreAfterFirst {
		t.Errorf("aynı gün ikinci çağrıdan sonra score değişmemeli: %v -> %v", scoreAfterFirst, got)
	}
}

func TestTrackDailyLogin_DifferentDayAwardsAgain(t *testing.T) {
	did := seedUser(t, 10.0)
	TrackDailyLogin(did) // bugün

	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

	// Bugünün satırını "dün"e taşı — TrackDailyLogin'in tarih sınırı mantığını
	// system clock'u mock etmeden test etmenin yolu: date PK'sini elle kaydır,
	// böylece "bugün" için satır yokmuş gibi davranır ve fonksiyon yeniden tetiklenir.
	if _, err := db.DB.Exec(
		`UPDATE daily_activity SET date = ? WHERE user_did = ? AND date = ?`,
		yesterday, did, today,
	); err != nil {
		t.Fatalf("tarih kaydırma: %v", err)
	}

	TrackDailyLogin(did) // "yeni gün"

	loginCountToday, _ := getDailyActivity(t, did, today)
	if loginCountToday != 1 {
		t.Errorf("yeni gün login_count = %d, beklenen 1", loginCountToday)
	}
	if n := countCreditEvents(t, did, EventDailyLogin); n != 2 {
		t.Errorf("iki farklı gün sonrası daily_login event sayısı = %d, beklenen 2", n)
	}
}

// ─── TrackMessageSent ────────────────────────────────────────────────────────

func TestTrackMessageSent_NoEventBelowTen(t *testing.T) {
	did := seedUser(t, 10.0)
	for i := 0; i < 9; i++ {
		TrackMessageSent(did)
	}
	today := time.Now().Format("2006-01-02")
	_, msgCount := getDailyActivity(t, did, today)
	if msgCount != 9 {
		t.Errorf("msg_count = %d, beklenen 9", msgCount)
	}
	if n := countCreditEvents(t, did, EventMessageSent); n != 0 {
		t.Errorf("10 altı mesajda event sayısı = %d, beklenen 0", n)
	}
}

func TestTrackMessageSent_TriggersAtTen(t *testing.T) {
	did := seedUser(t, 10.0)
	for i := 0; i < 10; i++ {
		TrackMessageSent(did)
	}
	if n := countCreditEvents(t, did, EventMessageSent); n != 1 {
		t.Errorf("10. mesajda event sayısı = %d, beklenen 1", n)
	}
}

func TestTrackMessageSent_ModTenTriggersAtTwenty(t *testing.T) {
	did := seedUser(t, 10.0)
	for i := 0; i < 20; i++ {
		TrackMessageSent(did)
	}
	if n := countCreditEvents(t, did, EventMessageSent); n != 2 {
		t.Errorf("20 mesajda event sayısı = %d, beklenen 2 (10. ve 20.)", n)
	}
}

// ─── GetHistory ──────────────────────────────────────────────────────────────

// insertRawEvent bypasses AddCustomEvent to give each row a fully-controlled
// created_at, so ordering/limit tests don't depend on real-clock timing.
func insertRawEvent(t *testing.T, did, eventType string, createdAt time.Time) {
	t.Helper()
	_, err := db.DB.Exec(`
		INSERT INTO credit_events (id, user_did, event_type, delta, reason, new_score, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), did, eventType, 1.0, "history test", 50.0,
		createdAt.Format(time.RFC3339),
	)
	if err != nil {
		t.Fatalf("insertRawEvent: %v", err)
	}
}

func TestGetHistory_RespectsLimit(t *testing.T) {
	did := seedUser(t, 20.0)
	base := time.Now().UTC()
	for i := 0; i < 5; i++ {
		insertRawEvent(t, did, fmt.Sprintf("evt_%d", i), base.Add(time.Duration(i)*time.Minute))
	}
	history, err := GetHistory(did, 3)
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(history) != 3 {
		t.Errorf("history uzunluk = %d, beklenen 3", len(history))
	}
}

func TestGetHistory_OrdersNewestFirst(t *testing.T) {
	did := seedUser(t, 20.0)
	base := time.Now().UTC()
	insertRawEvent(t, did, "evt_oldest", base)
	insertRawEvent(t, did, "evt_middle", base.Add(1*time.Minute))
	insertRawEvent(t, did, "evt_newest", base.Add(2*time.Minute))

	history, err := GetHistory(did, 10)
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(history) != 3 {
		t.Fatalf("history uzunluk = %d, beklenen 3", len(history))
	}
	if history[0].EventType != "evt_newest" || history[1].EventType != "evt_middle" || history[2].EventType != "evt_oldest" {
		t.Errorf("sıralama yanlış: %s, %s, %s", history[0].EventType, history[1].EventType, history[2].EventType)
	}
}

func TestGetHistory_IsolatesByUser(t *testing.T) {
	didA := seedUser(t, 20.0)
	didB := seedUser(t, 20.0)
	insertRawEvent(t, didA, "evt_a", time.Now().UTC())
	insertRawEvent(t, didB, "evt_b", time.Now().UTC())

	historyA, err := GetHistory(didA, 10)
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	for _, e := range historyA {
		if e.UserDID != didA {
			t.Fatalf("didA'nın history'sinde başka kullanıcının event'i var: %+v", e)
		}
		if e.EventType == "evt_b" {
			t.Fatal("didB'nin event'i didA'nın history'sinde sızmış")
		}
	}
	if len(historyA) != 1 {
		t.Errorf("didA history uzunluk = %d, beklenen 1", len(historyA))
	}
}
