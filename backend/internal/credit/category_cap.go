package credit

// category_cap.go — Spec §7.1 kredi puanı matrisindeki per-kategori tavanı
// (Max kolonu) uygular. Daha önce credit.go/zk_credit.go hiçbir yerde bu
// tavanı uygulamıyordu, sadece users.credit_score için global [-20,100]
// sınırı vardı.
//
// Aynı spec kategorisi iki farklı event_type string'iyle loglanabiliyor:
// plain yol ham kategori adını yazar ("account_age"), ZK claim yolu
// "zk_"+ProofType yazar ("zk_age") — bkz. zk_credit.go ClaimZKCredit. Cap,
// bu ikisinin TOPLAMINA uygulanmalı, aksi halde aynı kategori plain+zk
// üzerinden ayrı ayrı cap'e kadar doldurulup tavan iki katına çıkarılabilir.

import (
	"database/sql"
	"fmt"
	"strings"

	"obscura.network/core/internal/db"
)

// Kanonik kategori adları — plain event_type sabitleriyle (credit.go)
// birebir aynı string değerler, ayrı bir isimlendirme icat edilmedi.
const (
	CategoryAccountAge   = "account_age"
	CategoryDailyLogin   = "daily_login"
	CategoryMessageSent  = "message_sent"
	CategoryVoiceCall    = "voice_call"
	CategoryGroupCreated = "group_created"
	CategorySpamReceived = "spam_received"
	CategorySpamFalse    = "spam_false"
	CategoryFraud        = "fraud"
	CategoryCommunity    = "community"
	CategoryNodeRunning  = "node_running"
	CategoryEndorsement  = "endorsement"
	CategoryGoodStreak   = "good_streak"
)

// CategoryCaps — spec §7.1 Max kolonu. Pozitif değerler tavan (min ile
// kırpılır), negatif değerler taban (max ile kırpılır).
var CategoryCaps = map[string]float64{
	CategoryAccountAge:   24,
	CategoryDailyLogin:   15,
	CategoryMessageSent:  10,
	CategoryVoiceCall:    5,
	CategoryGroupCreated: 10,
	CategorySpamReceived: -50,
	CategorySpamFalse:    -30,
	CategoryFraud:        -100,
	CategoryCommunity:    25,
	CategoryNodeRunning:  60,
	CategoryEndorsement:  20,
	CategoryGoodStreak:   20,
}

// zkEventTypeToCategory — ClaimZKCredit'in ürettiği "zk_"+ProofType
// event_type string'lerini kanonik kategoriye eşler. ProofTypeActivity
// spec'teki "Gunluk giris" (activity_proof.circom) davranışına karşılık
// gelir — bu yüzden CategoryDailyLogin'e, ProofTypeMsgCount ise
// CategoryMessageSent'e eşleniyor (proof tipi adı farklı ama aynı davranış).
var zkEventTypeToCategory = map[string]string{
	"zk_age":         CategoryAccountAge,
	"zk_activity":    CategoryDailyLogin,
	"zk_node":        CategoryNodeRunning,
	"zk_endorsement": CategoryEndorsement,
	"zk_streak":      CategoryGoodStreak,
	"zk_msg_count":   CategoryMessageSent,
}

// categoryToEventTypes — kanonik kategori -> o kategoriye sayılan tüm ham
// event_type string'leri (plain + varsa zk_ varyantı). applyCategoryCap'in
// SUM sorgusu bunu kullanır, tek bir tarafı unutup cap'i delmesin diye.
var categoryToEventTypes = buildCategoryToEventTypes()

func buildCategoryToEventTypes() map[string][]string {
	m := make(map[string][]string, len(CategoryCaps))
	for cat := range CategoryCaps {
		m[cat] = []string{cat}
	}
	for zkType, cat := range zkEventTypeToCategory {
		m[cat] = append(m[cat], zkType)
	}
	return m
}

// canonicalCategory bir event_type'ı (plain veya "zk_" önekli) spec §7.1
// kategorisine normalize eder. Kategori dışı (ad-hoc/custom) event_type'lar
// için "" döner — çağıran bu durumda cap uygulamamalı.
func canonicalCategory(eventType string) string {
	if _, ok := CategoryCaps[eventType]; ok {
		return eventType
	}
	if cat, ok := zkEventTypeToCategory[eventType]; ok {
		return cat
	}
	return ""
}

// applyCategoryCap, requestedDelta'yı kategori tavanını aşmayacak şekilde
// kırpar. Kararı credit_events'teki HAM (hiç kırpılmamış) delta'ların
// toplamına göre verir — credit_events her zaman ham veriyi tutar (audit
// tam kalsın diye), yalnızca bu fonksiyonun döndürdüğü effectiveDelta
// users.credit_score'a uygulanır.
//
// category CategoryCaps'te yoksa (ad-hoc/custom event) cap uygulanmaz,
// requestedDelta olduğu gibi döner.
func applyCategoryCap(userDID, category string, requestedDelta float64) (effectiveDelta float64, err error) {
	cap, ok := CategoryCaps[category]
	if !ok {
		return requestedDelta, nil
	}

	eventTypes := categoryToEventTypes[category]
	placeholders := make([]string, len(eventTypes))
	args := make([]interface{}, 0, len(eventTypes)+1)
	args = append(args, userDID)
	for i, et := range eventTypes {
		placeholders[i] = "?"
		args = append(args, et)
	}

	query := fmt.Sprintf(
		`SELECT COALESCE(SUM(delta), 0) FROM credit_events WHERE user_did = ? AND event_type IN (%s)`,
		strings.Join(placeholders, ", "),
	)

	var current float64
	if err := db.DB.QueryRow(query, args...).Scan(&current); err != nil && err != sql.ErrNoRows {
		return 0, fmt.Errorf("kategori tavanı hesaplanamadı: %w", err)
	}

	newCumulative := current + requestedDelta
	if cap >= 0 {
		if newCumulative > cap {
			newCumulative = cap
		}
	} else {
		if newCumulative < cap {
			newCumulative = cap
		}
	}

	return newCumulative - current, nil
}
