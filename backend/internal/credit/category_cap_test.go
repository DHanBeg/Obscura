package credit

// Tests for the Spec §7.1 per-category cap (category_cap.go). Reuses
// seedUser/getScore/countCreditEvents from credit_test.go — same package,
// same TestMain-bootstrapped db.DB.

import (
	"testing"

	"obscura.network/core/internal/db"
)

func TestApplyCategoryCap_AccumulatesNormallyUnderCap(t *testing.T) {
	did := seedUser(t, 0.0)
	// CategoryDailyLogin cap = 15 — iki 0.5'lik olay çok altında kalır.
	if err := AddCustomEvent(did, CategoryDailyLogin, 0.5, "gün1"); err != nil {
		t.Fatalf("AddCustomEvent 1: %v", err)
	}
	if err := AddCustomEvent(did, CategoryDailyLogin, 0.5, "gün2"); err != nil {
		t.Fatalf("AddCustomEvent 2: %v", err)
	}
	if got := getScore(t, did); got != 1.0 {
		t.Errorf("score = %v, beklenen 1.0 (cap altında, kırpma yok)", got)
	}
}

func TestApplyCategoryCap_ClipsWhenExceedingCap(t *testing.T) {
	did := seedUser(t, 0.0)
	// CategoryVoiceCall cap = 5 — tek istekte 7 talep, sadece 5'e kadar uygulanmalı.
	if err := AddCustomEvent(did, CategoryVoiceCall, 7.0, "tek büyük arama"); err != nil {
		t.Fatalf("AddCustomEvent: %v", err)
	}
	if got := getScore(t, did); got != 5.0 {
		t.Errorf("score = %v, beklenen 5.0 (cap'te kırpılmış)", got)
	}
	// credit_events'e HAM delta (7.0) loglanmalı — audit tam kalmalı.
	var rawDelta float64
	if err := db.DB.QueryRow(
		`SELECT delta FROM credit_events WHERE user_did = ? AND event_type = ?`,
		did, CategoryVoiceCall,
	).Scan(&rawDelta); err != nil {
		t.Fatalf("ham delta okunamadı: %v", err)
	}
	if rawDelta != 7.0 {
		t.Errorf("credit_events.delta = %v, beklenen ham 7.0", rawDelta)
	}
}

func TestApplyCategoryCap_AtCapEffectiveDeltaZero_ButEventLogged(t *testing.T) {
	did := seedUser(t, 0.0)
	// CategoryCommunity cap = 25 — ilk olay tam cap'e doldurur.
	if err := AddCustomEvent(did, CategoryCommunity, 25.0, "cap'e dolduran"); err != nil {
		t.Fatalf("AddCustomEvent 1: %v", err)
	}
	scoreAtCap := getScore(t, did)
	if scoreAtCap != 25.0 {
		t.Fatalf("cap dolduktan sonra score = %v, beklenen 25.0", scoreAtCap)
	}

	// İkinci olay: cap zaten dolu, effectiveDelta 0 olmalı — score değişmemeli.
	if err := AddCustomEvent(did, CategoryCommunity, 5.0, "cap doluyken ek katkı"); err != nil {
		t.Fatalf("AddCustomEvent 2: %v", err)
	}
	if got := getScore(t, did); got != scoreAtCap {
		t.Errorf("cap doluyken score değişmemeli: %v -> %v", scoreAtCap, got)
	}

	// Ama event yine de loglanmalı (şeffaflık) — 2 satır olmalı.
	if n := countCreditEvents(t, did, CategoryCommunity); n != 2 {
		t.Errorf("credit_events satır sayısı = %d, beklenen 2 (ikisi de loglanmalı)", n)
	}
}

// TestApplyCategoryCap_PlainAndZKShareSameCategoryCap — bu bulgunun asıl
// kanıtı: plain event_type ("account_age") ve zk event_type ("zk_age")
// AYNI kanonik kategoriye (account_age, cap=24) sayılmalı. Aksi halde her
// biri kendi 24'üne kadar dolar, toplamda 48'e kadar çıkabilirdi.
func TestApplyCategoryCap_PlainAndZKShareSameCategoryCap(t *testing.T) {
	did := seedUser(t, 0.0)

	if err := AddCustomEvent(did, "account_age", 20.0, "plain birikim"); err != nil {
		t.Fatalf("AddCustomEvent (plain): %v", err)
	}
	if got := getScore(t, did); got != 20.0 {
		t.Fatalf("plain sonrası score = %v, beklenen 20.0", got)
	}

	// zk_age aynı kategoriye (account_age) sayılmalı: kalan alan sadece
	// 24-20=4, istenen 20 olsa da sadece 4 uygulanmalı.
	if err := AddCustomEvent(did, "zk_age", 20.0, "zk birikim"); err != nil {
		t.Fatalf("AddCustomEvent (zk): %v", err)
	}
	if got := getScore(t, did); got != 24.0 {
		t.Errorf("plain+zk karışık sonrası score = %v, beklenen 24.0 (ortak cap uygulanmış)", got)
	}

	// Her iki event de HAM haliyle ayrı ayrı loglanmış olmalı (audit tam).
	if n := countCreditEvents(t, did, "account_age"); n != 1 {
		t.Errorf("account_age event sayısı = %d, beklenen 1", n)
	}
	if n := countCreditEvents(t, did, "zk_age"); n != 1 {
		t.Errorf("zk_age event sayısı = %d, beklenen 1", n)
	}
}

// TestApplyCategoryCap_NegativeCapClipsAtFloor — negatif cap dalını
// (applyCategoryCap'teki else/max kolu) ayrıca doğrular.
func TestApplyCategoryCap_NegativeCapClipsAtFloor(t *testing.T) {
	did := seedUser(t, 50.0)
	// CategoryFraud cap = -100 — tek istekte -150 talep, taban -100'de
	// kırpılmalı (kategori tavanı). Sonra global [-20,100] sınırı da
	// devreye girip skoru -20'ye çeker — iki katman birlikte doğru çalışıyor.
	if err := AddCustomEvent(did, CategoryFraud, -150.0, "aşırı ceza"); err != nil {
		t.Fatalf("AddCustomEvent: %v", err)
	}
	if got := getScore(t, did); got != -20.0 {
		t.Errorf("score = %v, beklenen -20.0 (kategori tabanı -100'de kırpıldı, sonra global taban -20'de kırpıldı)", got)
	}
	var rawDelta float64
	if err := db.DB.QueryRow(
		`SELECT delta FROM credit_events WHERE user_did = ? AND event_type = ?`,
		did, CategoryFraud,
	).Scan(&rawDelta); err != nil {
		t.Fatalf("ham delta okunamadı: %v", err)
	}
	if rawDelta != -150.0 {
		t.Errorf("credit_events.delta = %v, beklenen ham -150.0", rawDelta)
	}
}
