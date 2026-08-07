package moderation

// violations.go — Bölüm 3 (kademeli yaptırım) + Bölüm 4 (yalan şikayet cezası,
// şikayetçi güvenilirliği, brigading koruması).
//
// user_violations tek kaynaktır: credit_score/users.tier (Bölüm 5 kilit-açma
// katmanı, ayrı bir eksen, Oturum 3'e ait) bu paket tarafından değiştirilmez.

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"obscura.network/core/internal/credit"
	"obscura.network/core/internal/dbi"
)

// Kapalı ihlal listesi (Bölüm 1.2, İlke 2 — bu listenin dışına çıkılmaz).
const (
	CategorySpam        = "spam"
	CategoryScam        = "scam"
	CategoryHarassment  = "harassment"
	CategoryCopyright   = "copyright"
	CategoryIllegalSale = "illegal_sale"
	CategoryChildSafety = "child_safety"
	// CategoryFalseReport is not a Bölüm 1.2 content category — it's the
	// sanction category applied to a complainant whose evidence was rejected
	// (Bölüm 4: "Yalan şikayet, ihlaldir").
	CategoryFalseReport = "false_report"
)

// Kademeli ceza aksiyonları (Bölüm 3.1).
const (
	ActionWarning     = "warning"
	ActionRestrict7d  = "restrict_7d"
	ActionRestrict30d = "restrict_30d"
)

// IsKnownCategory reports whether category is in the closed Bölüm 1.2 list.
// CategoryFalseReport is deliberately excluded — it's an internal sanction
// category applied by RecordComplaintVerdict, not something a complainant may
// file a report under.
func IsKnownCategory(category string) bool {
	switch category {
	case CategorySpam, CategoryScam, CategoryHarassment, CategoryCopyright, CategoryIllegalSale, CategoryChildSafety:
		return true
	default:
		return false
	}
}

// isSevereCategory reports whether category skips straight to the top tier
// (Bölüm 3.1: "Ağır ihlal (dolandırıcılık, taciz) → doğrudan üst basamak").
// child_safety is legally the most severe (Bölüm 1.2) and included here too.
func isSevereCategory(category string) bool {
	switch category {
	case CategoryScam, CategoryHarassment, CategoryChildSafety:
		return true
	default:
		return false
	}
}

// RecordViolation applies Bölüm 3's discrete tiered punishment to userDID.
//
// Tier is derived from the user's TOTAL prior violation count (any category —
// doc does not specify per-category counting; global counting chosen as the
// conservative default, tunable later per the doc's own "ince ayar" note):
// 1st → warning, 2nd → 7 day restriction, 3rd+ → 30 day restriction. severe
// forces a minimum of the 30-day tier regardless of prior count (severe skips
// intermediate steps, it never downgrades an already-higher tier).
//
// NOTE: the doc defines no 4th+ step beyond 30 days — this implementation caps
// at restrict_30d for all further violations. This is an explicit placeholder,
// not a doc decision; revisit once real moderation volume exists.
func RecordViolation(ctx context.Context, db dbi.Querier, userDID, category, sourceReportID string, severe bool) (action string, err error) {
	if db == nil {
		return "", fmt.Errorf("moderation: RecordViolation nil db")
	}
	if userDID == "" || category == "" {
		return "", fmt.Errorf("moderation: RecordViolation user_did ve category zorunlu")
	}

	var priorCount int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM user_violations WHERE user_did = ?`, userDID,
	).Scan(&priorCount); err != nil {
		return "", fmt.Errorf("moderation: prior violation count: %w", err)
	}

	tier := priorCount + 1
	if tier > 3 {
		tier = 3
	}
	if severe && tier < 3 {
		tier = 3
	}

	switch tier {
	case 1:
		action = ActionWarning
	case 2:
		action = ActionRestrict7d
	default:
		action = ActionRestrict30d
	}

	now := time.Now().UTC()
	var expiresAt sql.NullString
	switch action {
	case ActionRestrict7d:
		expiresAt = sql.NullString{String: now.Add(7 * 24 * time.Hour).Format(time.RFC3339), Valid: true}
	case ActionRestrict30d:
		expiresAt = sql.NullString{String: now.Add(30 * 24 * time.Hour).Format(time.RFC3339), Valid: true}
	}

	if _, err := db.ExecContext(ctx, `
		INSERT INTO user_violations (id, user_did, category, tier, action, source_report_id, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), userDID, category, tier, action, sourceReportID,
		now.Format(time.RFC3339), expiresAt,
	); err != nil {
		return "", fmt.Errorf("moderation: violation kaydedilemedi: %w", err)
	}

	// Şeffaflık (İlke 3): kısıt users tablosuna da yansır ki mevcut auth/UI
	// kodu (is_banned/ban_expires_at) bunu zaten okuyabilsin. Uyarı (tier 1)
	// hiçbir erişimi kısıtlamaz, sadece kayda geçer.
	if action != ActionWarning {
		if _, err := db.ExecContext(ctx,
			`UPDATE users SET is_banned = 1, ban_expires_at = ? WHERE did = ?`,
			expiresAt.String, userDID,
		); err != nil {
			return "", fmt.Errorf("moderation: kullanıcı kısıtlanamadı: %w", err)
		}
	}

	return action, nil
}

// ─── Brigading koruması (Bölüm 4) ───────────────────────────────────────────

// BrigadingThreshold / BrigadingWindow are tunable (doc's own "ince ayar"
// note applies here too — these are a conservative starting point).
const (
	BrigadingThreshold = 5
	BrigadingWindow    = 1 * time.Hour
)

// IsBrigading reports whether targetDID has received BrigadingThreshold or
// more spam_reports within BrigadingWindow. Callers must NOT auto-punish when
// this is true — route to review_queue instead (Bölüm 4: "otomatik ceza
// uygulanmaz, elle/kurul inceleme kuyruğuna alınır").
func IsBrigading(ctx context.Context, db dbi.Querier, targetDID string) (bool, error) {
	if db == nil {
		return false, fmt.Errorf("moderation: IsBrigading nil db")
	}
	cutoff := time.Now().UTC().Add(-BrigadingWindow).Format(time.RFC3339)
	var count int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM spam_reports WHERE reported_did = ? AND created_at >= ?`,
		targetDID, cutoff,
	).Scan(&count); err != nil {
		return false, fmt.Errorf("moderation: brigading count: %w", err)
	}
	return count >= BrigadingThreshold, nil
}

// EnqueueReview pushes a report into the human review queue (İlke 5: system
// is a pre-filter, not a judge — anything not obvious spam, and everything
// flagged as possible brigading, lands here for a human decision).
func EnqueueReview(ctx context.Context, db dbi.Querier, reportID, reason string) error {
	if db == nil {
		return fmt.Errorf("moderation: EnqueueReview nil db")
	}
	if reportID == "" || reason == "" {
		return fmt.Errorf("moderation: EnqueueReview report_id ve reason zorunlu")
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO review_queue (id, report_id, reason, status, created_at)
		VALUES (?, ?, ?, 'pending', ?)`,
		uuid.New().String(), reportID, reason, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

// EnqueueAutoScanReview pushes an automated-scan finding (Umay, spec Bölüm
// 1.4) into the same human review queue as user complaints. Unlike
// EnqueueReview, there is no report_id — the finding did not originate from
// a spam_reports row, so report_id is left NULL and source records the
// origin. review_queue.report_id/source (migrations 143-147) exist for
// exactly this split.
// targetType/targetID identify the actual content the finding is about
// (target_type: "message" | "listing", target_id: the FULL id — not the
// truncated 8-char prefix that used to be all `reason` embedded). Admin
// review-queue resolve önceden bu ID'lere gerçek erişemiyordu (bkz.
// migration 155-159); artık kararı uygulayacak endpoint doğrudan bu
// kolonları okuyabiliyor.
func EnqueueAutoScanReview(ctx context.Context, db dbi.Querier, reason, targetType, targetID string) error {
	if db == nil {
		return fmt.Errorf("moderation: EnqueueAutoScanReview nil db")
	}
	if reason == "" {
		return fmt.Errorf("moderation: EnqueueAutoScanReview reason zorunlu")
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO review_queue (id, report_id, reason, status, source, target_type, target_id, created_at)
		VALUES (?, NULL, ?, 'pending', 'auto_scan', ?, ?, ?)`,
		uuid.New().String(), reason, targetType, targetID, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

// RepeatFalseReportThreshold: bir şikayetçinin AYNI hedefe karşı bu rapordan
// önce kaç kez haksız çıktığı bu eşiği eşit/aşarsa (yani bu üçüncü asılsız
// şikayeti oluyorsa), taciz kategorisine düşer ve doğrudan üst basamağa
// zıplar — Bölüm 4'ün global false_report sayacından ayrı, "aynı kişiyi
// hedef alma" paterni için tunable bir eşik (doc'un kendi "ince ayar" notu
// burada da geçerli).
const RepeatFalseReportThreshold = 2

// ─── Şikayetçi güvenilirliği (Bölüm 4) ──────────────────────────────────────

// RecordComplaintVerdict finalizes a report's verdict and applies the correct
// side's punishment: upheld → RecordViolation on the accused; false →
// RecordViolation on the complainant under CategoryFalseReport (Bölüm 4:
// "Yalan şikayet, ihlaldir"). Also updates the complainant's credibility
// history either way.
func RecordComplaintVerdict(ctx context.Context, db dbi.Querier, reportID string, upheld bool) error {
	if db == nil {
		return fmt.Errorf("moderation: RecordComplaintVerdict nil db")
	}

	var reporterDID, reportedDID, category string
	if err := db.QueryRowContext(ctx,
		`SELECT reporter_did, reported_did, category FROM spam_reports WHERE id = ?`, reportID,
	).Scan(&reporterDID, &reportedDID, &category); err != nil {
		return fmt.Errorf("moderation: rapor bulunamadı: %w", err)
	}

	verdict := "false"
	if upheld {
		verdict = "upheld"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.ExecContext(ctx,
		`UPDATE spam_reports SET verdict = ?, reviewed_at = ?, status = 'reviewed' WHERE id = ?`,
		verdict, now, reportID,
	); err != nil {
		return fmt.Errorf("moderation: verdict kaydedilemedi: %w", err)
	}

	if upheld {
		if _, err := RecordViolation(ctx, db, reportedDID, category, reportID, isSevereCategory(category)); err != nil {
			return err
		}
		// Kredi katmanı (Madde 4) — user_violations'tan (Bölüm 3, ban/kısıtlama
		// ekseni) AYRI bir eksen olan credit_score'u etkiler. Sadece kapalı
		// kategori listesindeki spam/dolandırıcılık (spec'in "fraud"ı kodun
		// CategoryScam'ine karşılık gelir) için — diğer kategoriler (taciz,
		// telif, vb.) spec §7.1 kredi matrisinde ayrı bir davranış değil.
		switch category {
		case CategorySpam:
			if err := credit.AddEvent(reportedDID, credit.EventSpamReceived, "Onaylanmış spam şikayeti: "+reportID); err != nil {
				log.Printf("⚠️ spam_received kredi olayı başarısız (did=%s, report=%s): %v", reportedDID, reportID, err)
			}
		case CategoryScam:
			if err := credit.AddEvent(reportedDID, credit.EventFraud, "Onaylanmış dolandırıcılık şikayeti: "+reportID); err != nil {
				log.Printf("⚠️ fraud kredi olayı başarısız (did=%s, report=%s): %v", reportedDID, reportID, err)
			}
		}
	} else {
		// Kredi katmanı: "spam_false" spec'te SADECE spam-kategorisi şikayetin
		// asılsız çıkmasına karşılık gelir (§7.1 "Spam raporu (verme, yanlış)"),
		// taciz/telif/vb. kategorilerin asılsız çıkması değil. Aşağıdaki
		// "category" hemen gölgeleniyor (satır ~289, RecordViolation'ın tier
		// hesaplaması için) — bu yüzden ORİJİNAL (spam_reports.category'den
		// okunan, fonksiyon başındaki) değeri gölgelenmeden ÖNCE yakalıyoruz.
		wasOriginallySpam := category == CategorySpam

		// Bölüm 4: "Aynı kişiyi tekrar tekrar asılsız şikayet eden → taciz
		// kategorisi, doğrudan üst basamak." Bu global false_report sayacından
		// AYRI bir sinyal — belirli bir HEDEFE karşı tekrarlanan asılsız
		// şikayeti tespit eder (id != ? bu raporun kendisini hariç tutar, çünkü
		// verdict sütunu bu satıra az önce yukarıda yazıldı).
		var priorFalse int
		if err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM spam_reports WHERE reporter_did = ? AND reported_did = ? AND verdict = 'false' AND id != ?`,
			reporterDID, reportedDID, reportID,
		).Scan(&priorFalse); err != nil {
			return fmt.Errorf("moderation: tekrar-asılsız-şikayet sayımı: %w", err)
		}

		category := CategoryFalseReport
		severe := false
		if priorFalse >= RepeatFalseReportThreshold {
			category = CategoryHarassment
			severe = true
		}
		if _, err := RecordViolation(ctx, db, reporterDID, category, reportID, severe); err != nil {
			return err
		}
		if wasOriginallySpam {
			if err := credit.AddEvent(reporterDID, credit.EventSpamFalse, "Asılsız spam şikayeti: "+reportID); err != nil {
				log.Printf("⚠️ spam_false kredi olayı başarısız (did=%s, report=%s): %v", reporterDID, reportID, err)
			}
		}
	}

	if err := updateCredibility(ctx, db, reporterDID, upheld); err != nil {
		return err
	}

	// Karar verildi — kuyruk satırı artık "bekliyor" değil (dangling state
	// bırakma: verdict alan bir rapor review_queue'da sonsuza 'pending' kalmasın).
	if _, err := db.ExecContext(ctx,
		`UPDATE review_queue SET status = 'resolved' WHERE report_id = ?`, reportID,
	); err != nil {
		return fmt.Errorf("moderation: review_queue kapatılamadı: %w", err)
	}

	return nil
}

// LowCredibilityWeightThreshold: bir şikayetçinin weight'i bu eşiğin altına
// düşerse (kronik asılsız şikayetçi — Bölüm 4 "ağırlığı zamanla düşer"),
// raporları artık "bariz spam" hızlı-yolundan otomatik işlenmez, her zaman
// insan incelemesine düşer — HandleSpamReport'taki gerçek karar noktası
// (aksi halde weight yazılıp hiç okunmayan ölü veri olarak kalırdı).
const LowCredibilityWeightThreshold = 0.3

// GetCredibilityWeight returns userDID's current complainant credibility
// weight (Bölüm 4). A reporter with no track record yet (never judged)
// defaults to 1.0 — Bölüm 5.3 güven varsayılanı: yeni/bilinmeyen kullanıcı
// şüpheli değil, güvenilir sayılır until proven otherwise.
func GetCredibilityWeight(ctx context.Context, db dbi.Querier, userDID string) (float64, error) {
	if db == nil {
		return 0, fmt.Errorf("moderation: GetCredibilityWeight nil db")
	}
	var weight float64
	err := db.QueryRowContext(ctx,
		`SELECT weight FROM complainant_credibility WHERE user_did = ?`, userDID,
	).Scan(&weight)
	if err == sql.ErrNoRows {
		return 1.0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("moderation: credibility okunamadı: %w", err)
	}
	return weight, nil
}

// updateCredibility upserts the complainant's track record. weight is a
// simple, explicitly tunable formula (doc's own "ince ayar" note): repeat
// false-reporters trend toward 0, consistently-correct reporters stay near 1.
// This is NOT the same as credit_score — a separate, complaint-specific trust
// signal (Bölüm 4: "Şikayetçi güvenilirliği").
func updateCredibility(ctx context.Context, db dbi.Querier, userDID string, upheld bool) error {
	var upheldCount, falseCount int
	err := db.QueryRowContext(ctx,
		`SELECT upheld_count, false_count FROM complainant_credibility WHERE user_did = ?`, userDID,
	).Scan(&upheldCount, &falseCount)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("moderation: credibility okunamadı: %w", err)
	}

	if upheld {
		upheldCount++
	} else {
		falseCount++
	}
	weight := float64(upheldCount+1) / float64(upheldCount+falseCount+2)

	_, err = db.ExecContext(ctx, `
		INSERT INTO complainant_credibility (user_did, upheld_count, false_count, weight, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_did) DO UPDATE SET
			upheld_count = excluded.upheld_count,
			false_count  = excluded.false_count,
			weight       = excluded.weight,
			updated_at   = excluded.updated_at`,
		userDID, upheldCount, falseCount, weight, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("moderation: credibility güncellenemedi: %w", err)
	}
	return nil
}
