package credit

import (
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/models"
)

// ─── KREDİ OLAY TİPLERİ ──────────────────────────────────────────────────────

const (
	EventAccountAge      = "account_age"       // +1/ay
	EventDailyLogin      = "daily_login"        // +0.5/gün
	EventMessageSent     = "message_sent"       // +0.1/mesaj
	EventVoiceCall       = "voice_call"         // +0.2/arama
	EventGroupCreated    = "group_created"      // +2/grup
	EventSpamReceived    = "spam_received"      // -5/rapor
	EventSpamFalse       = "spam_false"         // -3/yanlış rapor
	EventFraud           = "fraud"              // -20/olay
	EventCommunity       = "community"          // +5/katkı
	EventNodeRunning     = "node_running"       // +10/ay
	EventEndorsement     = "endorsement"        // +1/onay
	EventGoodStreak      = "good_streak"        // +2/7gün streak
)

var EventDeltas = map[string]float64{
	EventAccountAge:   1.0,
	EventDailyLogin:   0.5,
	EventMessageSent:  0.1,
	EventVoiceCall:    0.2,
	EventGroupCreated: 2.0,
	EventSpamReceived: -5.0,
	EventSpamFalse:    -3.0,
	EventFraud:        -20.0,
	EventCommunity:    5.0,
	EventNodeRunning:  10.0,
	EventEndorsement:  1.0,
	EventGoodStreak:   2.0,
}

// ─── BAŞLANGIÇ PUANI ──────────────────────────────────────────────────────────

// InitialScore: Yeni kullanıcıya 20-100 arası rastgele puan (Sybil koruması)
func InitialScore() float64 {
	return float64(rand.Intn(81) + 20) // 20-100 arası
}

// ─── KREDİ GÜNCELLE ──────────────────────────────────────────────────────────

func AddEvent(userDID, eventType, reason string) error {
	delta, ok := EventDeltas[eventType]
	if !ok {
		return fmt.Errorf("bilinmeyen olay tipi: %s", eventType)
	}
	return AddCustomEvent(userDID, eventType, delta, reason)
}

func AddCustomEvent(userDID, eventType string, delta float64, reason string) error {
	// Mevcut puanı al
	var currentScore float64
	err := db.DB.QueryRow("SELECT credit_score FROM users WHERE did = ?", userDID).Scan(&currentScore)
	if err == sql.ErrNoRows {
		return fmt.Errorf("kullanıcı bulunamadı: %s", userDID)
	}
	if err != nil {
		return err
	}

	// Kategori tavanı (Spec §7.1 Max kolonu) — plain+zk aynı kategoriye
	// birlikte sayılır, bkz. category_cap.go.
	effectiveDelta := delta
	if category := canonicalCategory(eventType); category != "" {
		var capErr error
		effectiveDelta, capErr = applyCategoryCap(userDID, category, delta)
		if capErr != nil {
			return capErr
		}
	}

	// Yeni puanı hesapla (sınırları koru: -20 ile 100)
	newScore := currentScore + effectiveDelta
	if newScore > 100 {
		newScore = 100
	}
	if newScore < -20 {
		newScore = -20
	}

	// Tier hesapla
	newTier := models.ScoreToTier(newScore)

	// Kullanıcıyı güncelle
	now := time.Now()
	_, err = db.DB.Exec(`
		UPDATE users SET
			credit_score = ?,
			tier = ?,
			updated_at = ?
		WHERE did = ?`,
		newScore, newTier, now.Format(time.RFC3339), userDID,
	)
	if err != nil {
		return fmt.Errorf("kullanıcı güncellenemedi: %w", err)
	}

	// Olay kaydet
	_, err = db.DB.Exec(`
		INSERT INTO credit_events (id, user_did, event_type, delta, reason, new_score, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), userDID, eventType, delta, reason, newScore, now.Format(time.RFC3339),
	)
	if err != nil {
		log.Printf("⚠️ Kredi olay kaydedilemedi: %v", err)
	}

	didPrefix := userDID
	if len(didPrefix) > 12 {
		didPrefix = didPrefix[:12]
	}
	log.Printf("💳 Kredi: %s | %s | Δ%.1f | Yeni: %.1f | Tier: %d",
		didPrefix, eventType, delta, newScore, newTier)

	return nil
}

// ─── GÜNLÜK AKTİVİTE ─────────────────────────────────────────────────────────

func TrackDailyLogin(userDID string) {
	today := time.Now().Format("2006-01-02")

	var loginCount int
	db.DB.QueryRow(`
		SELECT login_count FROM daily_activity
		WHERE user_did = ? AND date = ?`, userDID, today,
	).Scan(&loginCount)

	if loginCount == 0 {
		// İlk günlük giriş — kredi ver
		db.DB.Exec(`
			INSERT INTO daily_activity (user_did, date, login_count)
			VALUES (?, ?, 1)
			ON CONFLICT(user_did, date) DO UPDATE SET login_count = login_count + 1`,
			userDID, today,
		)
		AddEvent(userDID, EventDailyLogin, "Günlük giriş bonusu")
	}
}

func TrackMessageSent(userDID string) {
	today := time.Now().Format("2006-01-02")

	// Günlük mesaj sayısını artır
	db.DB.Exec(`
		INSERT INTO daily_activity (user_did, date, msg_count)
		VALUES (?, ?, 1)
		ON CONFLICT(user_did, date) DO UPDATE SET msg_count = msg_count + 1`,
		userDID, today,
	)

	// Her 10 mesajda bir kredi (spam koruması için sınır)
	var msgCount int
	db.DB.QueryRow(`
		SELECT msg_count FROM daily_activity
		WHERE user_did = ? AND date = ?`, userDID, today,
	).Scan(&msgCount)

	if msgCount%10 == 0 {
		AddEvent(userDID, EventMessageSent, fmt.Sprintf("Günlük %d mesaj", msgCount))
	}
}

// ─── KREDİ GEÇMİŞİ ───────────────────────────────────────────────────────────

func GetHistory(userDID string, limit int) ([]models.CreditEvent, error) {
	rows, err := db.DB.Query(`
		SELECT id, user_did, event_type, delta, reason, new_score, created_at
		FROM credit_events
		WHERE user_did = ?
		ORDER BY created_at DESC
		LIMIT ?`,
		userDID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []models.CreditEvent
	for rows.Next() {
		var e models.CreditEvent
		var createdAt string
		rows.Scan(&e.ID, &e.UserDID, &e.EventType, &e.Delta, &e.Reason, &e.NewScore, &createdAt)
		e.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		events = append(events, e)
	}
	return events, nil
}

// NOT: Otomatik -20-puan → 48s inceleme → kalıcı ban akışı (scheduleAccountReview)
// kaldırıldı (Denetim ve Topluluk Katmanı, Oturum 1 retrofit). credit_score
// artık hiçbir kısıtlama/ban tetiklemiyor — o katman internal/moderation'daki
// user_violations'a taşındı (kademeli: uyarı→7gün→30gün, Bölüm 3). credit_score
// yalnızca Bölüm 5'in kilit-açma katmanı (users.tier) için kullanılmaya devam
// ediyor, ayrı bir eksen. Eski mekanizmanın sorunları: (1) kanıtsız otomatik
// ceza (İlke 5 ihlali), (2) in-memory time.Sleep — süreç restart'ında kaybolan
// bekleyen inceleme, (3) tek kalıcı "ban" durumu, kademeli değil.
