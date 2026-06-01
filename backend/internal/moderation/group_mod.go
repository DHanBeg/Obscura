// Package moderation — grup moderasyon sistemi
//
// Grup raporlama, otomatik inceleme bayrağı ve mesaj içerik taraması.
// Kullanım:
//   - ReportGroup: kullanıcı bir grubu raporlar; 3+ rapor → otomatik inceleme
//   - IsGroupSuspended: mesaj göndermeden önce grubun askıda olup olmadığını kontrol et
//   - AutoModerateMessage: basit keyword tabanlı içerik taraması
//   - MaxGroupSize: kredi skoruna göre maksimum grup üye sayısı
package moderation

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"time"
)

// GroupReport — grup raporu kaydı.
type GroupReport struct {
	ID          string
	GroupID     string
	ReporterDID string
	Reason      string // spam | inappropriate | scam | other
	CreatedAt   int64
}

// GroupModerationStatus — bir grubun moderasyon durumu.
type GroupModerationStatus struct {
	GroupID     string
	IsReviewing bool
	IsSuspended bool
	ReportCount int
	LastReview  int64
}

// MaxGroupSize — kullanıcının kredi skoruna göre maksimum grup üye sayısını döndürür.
// Spec Bölüm 7.2 — Tier ayrıcalıkları.
func MaxGroupSize(creditScore float64) int {
	switch {
	case creditScore >= 90:
		return 10000
	case creditScore >= 70:
		return 500
	case creditScore >= 40:
		return 50
	default:
		return 10
	}
}

// ReportGroup — kullanıcı bir grubu raporlar.
// 24 saat içinde 3 veya daha fazla rapor alınırsa grup otomatik olarak incelemeye alınır.
func ReportGroup(db *sql.DB, groupID, reporterDID, reason string) error {
	if db == nil {
		return fmt.Errorf("moderation.ReportGroup: nil db")
	}
	if groupID == "" || reporterDID == "" {
		return fmt.Errorf("moderation.ReportGroup: group_id ve reporter_did zorunlu")
	}

	id := generateGroupModID()
	_, err := db.Exec(`
		INSERT INTO group_reports (id, group_id, reporter_did, reason, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		id, groupID, reporterDID, reason, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("grup raporu kaydedilemedi: %w", err)
	}

	return checkAutoFlag(db, groupID)
}

// checkAutoFlag — son 24 saatteki rapor sayısını kontrol eder; 3+ ise grubu incelemeye alır.
func checkAutoFlag(db *sql.DB, groupID string) error {
	cutoff := time.Now().Add(-24 * time.Hour).Unix()

	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM group_reports WHERE group_id = ? AND created_at > ?`,
		groupID, cutoff,
	).Scan(&count); err != nil {
		return fmt.Errorf("rapor sayısı sorgulanamadı: %w", err)
	}

	if count >= 3 {
		log.Printf("[MODERASYON] Grup incelemeye alındı: %s (%d rapor/24s)", groupID, count)
		_, err := db.Exec(`
			INSERT OR REPLACE INTO group_moderation (group_id, is_reviewing, is_suspended, report_count, last_review)
			VALUES (?, 1, 0, ?, ?)`,
			groupID, count, time.Now().Unix())
		if err != nil {
			return fmt.Errorf("grup moderasyon durumu güncellenemedi: %w", err)
		}
	}
	return nil
}

// IsGroupSuspended — grubun askıda olup olmadığını kontrol eder.
// Mesaj gönderiminden önce çağrılmalıdır.
func IsGroupSuspended(db *sql.DB, groupID string) (bool, error) {
	if db == nil {
		return false, fmt.Errorf("moderation.IsGroupSuspended: nil db")
	}

	var suspended int
	err := db.QueryRow(
		`SELECT is_suspended FROM group_moderation WHERE group_id = ?`,
		groupID,
	).Scan(&suspended)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("grup durum sorgulanamadı: %w", err)
	}
	return suspended == 1, nil
}

// GetGroupModerationStatus — bir grubun tam moderasyon durumunu döndürür.
func GetGroupModerationStatus(db *sql.DB, groupID string) (*GroupModerationStatus, error) {
	if db == nil {
		return nil, fmt.Errorf("moderation.GetGroupModerationStatus: nil db")
	}

	var s GroupModerationStatus
	var isReviewing, isSuspended int
	err := db.QueryRow(`
		SELECT group_id, is_reviewing, is_suspended, report_count, last_review
		FROM group_moderation WHERE group_id = ?`,
		groupID,
	).Scan(&s.GroupID, &isReviewing, &isSuspended, &s.ReportCount, &s.LastReview)
	if err == sql.ErrNoRows {
		return &GroupModerationStatus{GroupID: groupID}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("moderasyon durumu sorgulanamadı: %w", err)
	}
	s.IsReviewing = isReviewing == 1
	s.IsSuspended = isSuspended == 1
	return &s, nil
}

// AutoModerateMessage — mesaj içeriğini basit keyword tabanlı tara.
// Gelecekte zkml entegrasyonu bu fonksiyon üzerinden yapılacak (ADR-0013).
// isSuspicious: true ise içerik şüpheli, score: 0.0-1.0 şüphe skoru.
func AutoModerateMessage(content string) (isSuspicious bool, score float64) {
	suspicious := []string{"spam", "scam", "hack", "phishing", "malware", "fraud"}
	contentLower := toLower(content)
	for _, kw := range suspicious {
		if containsSubstr(contentLower, kw) {
			return true, 0.9
		}
	}
	return false, 0.0
}

// ─── dahili yardımcılar ───────────────────────────────────────────────────────

func generateGroupModID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// fallback: nanosecond timestamp (kolizyon ihtimali düşük, test dışında kabul edilir)
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// toLower — ASCII küçük harf dönüşümü (strings paketinden kaçınmak için).
func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

func containsSubstr(s, sub string) bool {
	if len(sub) == 0 || len(s) < len(sub) {
		return false
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
