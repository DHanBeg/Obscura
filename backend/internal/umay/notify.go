package umay

// notify.go — bulgu sonrası aksiyon (spec Bölüm 1.4, "notify"). İlke 5:
// sistem ön-filtre, hakim değil — bariz olmayan HER ŞEY insan incelemesine
// düşer. Otomatik silme yalnızca yüksek-confidence bariz spam için.

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"obscura.network/core/internal/moderation"
	"obscura.network/core/internal/dbi"
)

const defaultAutoDeleteConfidence = 0.9

// autoDeleteConfidence reads OBSCURA_UMAY_AUTODELETE_CONFIDENCE. Sabit değil
// çünkü spec (Bölüm 5.2 notu) eşiklerin "kod aşamasında ince ayara tabi"
// olduğunu söylüyor — burada da aynı ilke uygulanıyor.
func autoDeleteConfidence() float64 {
	raw := os.Getenv("OBSCURA_UMAY_AUTODELETE_CONFIDENCE")
	if raw == "" {
		return defaultAutoDeleteConfidence
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v <= 0 || v > 1 {
		return defaultAutoDeleteConfidence
	}
	return v
}

// Handle applies notify's decision for one message's classification result.
//
//   - classifyErr != nil (Ollama/Groq'a hiç ulaşılamadı, devre-kesici açık,
//     vb.): sınıflandırılamayan içerik "bariz" sayılmaz — insan kuyruğuna düşer.
//   - verdict.Category == CategoryNone: hiçbir aksiyon yok.
//   - spam + confidence >= eşik: otomatik soft-delete (spec 1.4: "otomatik
//     silme yalnızca bariz spam için").
//   - diğer her şey: insan inceleme kuyruğu (review_queue, source='auto_scan').
func Handle(ctx context.Context, db dbi.Querier, msgID, fromDID string, verdict Verdict, classifyErr error) error {
	if classifyErr != nil {
		reason := fmt.Sprintf("umay: sınıflandırma başarısız (msg=%s): %v", truncate(msgID, 8), classifyErr)
		return moderation.EnqueueAutoScanReview(ctx, db, reason, "message", msgID)
	}

	if verdict.Category == "" || verdict.Category == CategoryNone {
		return nil
	}

	if verdict.Category == moderation.CategorySpam && verdict.Confidence >= autoDeleteConfidence() {
		if _, err := db.ExecContext(ctx,
			`UPDATE messages SET deleted_at = ? WHERE id = ? AND deleted_at IS NULL`,
			time.Now().UTC().Format(time.RFC3339), msgID,
		); err != nil {
			return fmt.Errorf("umay: otomatik silme hatası: %w", err)
		}
		log.Printf("[Umay] otomatik silindi: msg=%s... spam confidence=%.2f", truncate(msgID, 8), verdict.Confidence)
		return nil
	}

	reason := fmt.Sprintf("umay: %s (confidence=%.2f, msg=%s, from=%s)",
		verdict.Category, verdict.Confidence, truncate(msgID, 8), truncate(fromDID, 12))
	if err := moderation.EnqueueAutoScanReview(ctx, db, reason, "message", msgID); err != nil {
		return fmt.Errorf("umay: review_queue kuyruğa alma hatası: %w", err)
	}
	log.Printf("[Umay] insan incelemesine alındı: msg=%s... kategori=%s confidence=%.2f",
		truncate(msgID, 8), verdict.Category, verdict.Confidence)
	return nil
}

// HandleListing is Handle's marketplace-listing counterpart (spec Bölüm 1.1:
// "Taranır: ... marketplace ilanları"). Same decision tree, two differences:
// the auto-action is marketplace_listings.status='removed' instead of a
// message soft-delete. review_queue.target_type/target_id (migration 155-159)
// carry the FULL listingID now — reason still embeds a truncated copy for
// human-readable logging, but resolve-action code must use target_id, not
// parse reason (that copy is intentionally lossy, see truncate()).
func HandleListing(ctx context.Context, db dbi.Querier, listingID, sellerDID string, verdict Verdict, classifyErr error) error {
	if classifyErr != nil {
		reason := fmt.Sprintf("umay: sınıflandırma başarısız (listing=%s): %v", truncate(listingID, 8), classifyErr)
		return moderation.EnqueueAutoScanReview(ctx, db, reason, "listing", listingID)
	}

	if verdict.Category == "" || verdict.Category == CategoryNone {
		return nil
	}

	if verdict.Category == moderation.CategorySpam && verdict.Confidence >= autoDeleteConfidence() {
		if _, err := db.ExecContext(ctx,
			`UPDATE marketplace_listings SET status = 'removed', updated_at = ? WHERE id = ? AND status != 'removed'`,
			time.Now().UTC().Format(time.RFC3339), listingID,
		); err != nil {
			return fmt.Errorf("umay: ilan otomatik kaldırma hatası: %w", err)
		}
		log.Printf("[Umay] ilan otomatik kaldırıldı: listing=%s... spam confidence=%.2f", truncate(listingID, 8), verdict.Confidence)
		return nil
	}

	reason := fmt.Sprintf("umay: %s (confidence=%.2f, listing=%s, seller=%s)",
		verdict.Category, verdict.Confidence, truncate(listingID, 8), truncate(sellerDID, 12))
	if err := moderation.EnqueueAutoScanReview(ctx, db, reason, "listing", listingID); err != nil {
		return fmt.Errorf("umay: review_queue kuyruğa alma hatası: %w", err)
	}
	log.Printf("[Umay] ilan insan incelemesine alındı: listing=%s... kategori=%s confidence=%.2f",
		truncate(listingID, 8), verdict.Category, verdict.Confidence)
	return nil
}

// truncate — scanner.go'daki yardımcının kopyası (bilinçli — internal/umay,
// internal/scanner'a bağımlı olmamalı, iki paket bağımsız kalmalı).
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
