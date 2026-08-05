package api

// admin_handlers.go — İlke 5 (docs/spec/obscura_denetim_topluluk_katmani.md
// Bölüm 0: "Sistem hakim değil, ön eleyicidir. AI/otomasyon içeriği
// işaretler, ciddi cezalarda kararı insan (operatör/kurul) verir.") gereği
// review_queue'yu bir insanın görüp karar verebileceği admin yüzeyi.
//
// POST /v1/admin/review-queue/{id}/resolve — aynı şekilde korunur.

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/moderation"
)

const (
	reviewQueueDefaultLimit = 50
	reviewQueueMaxLimit     = 200
)

// ReviewQueueItem — review_queue satırı + (varsa) bağlı spam_reports
// zenginleştirmesi. source='auto_scan' kayıtlarında ReporterDID/ReportedDID/
// Category boş kalır (report_id NULL, JOIN eşleşmez) — ham reason ve
// target_type/target_id ile yetinilir.
type ReviewQueueItem struct {
	ID               string `json:"id"`
	ReportID         string `json:"report_id,omitempty"`
	Reason           string `json:"reason"`
	Status           string `json:"status"`
	Source           string `json:"source"`
	TargetType       string `json:"target_type,omitempty"`
	TargetID         string `json:"target_id,omitempty"`
	ResolvedAt       string `json:"resolved_at,omitempty"`
	ResolvedBy       string `json:"resolved_by,omitempty"`
	Resolution       string `json:"resolution,omitempty"`
	CreatedAt        string `json:"created_at"`
	ReporterDID      string `json:"reporter_did,omitempty"`
	ReportedDID      string `json:"reported_did,omitempty"`
	Category         string `json:"category,omitempty"`
	EvidenceVerified bool   `json:"evidence_verified,omitempty"`
}

// GET /v1/admin/review-queue?status=pending&source=user_report&limit=50&offset=0
func HandleAdminListReviewQueue(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	status := q.Get("status")
	if status == "" {
		status = "pending"
	}
	source := q.Get("source") // "" = user_report + auto_scan karışık

	limit := reviewQueueDefaultLimit
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > reviewQueueMaxLimit {
		limit = reviewQueueMaxLimit
	}
	offset := 0
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	queryArgs := []interface{}{status}
	sourceFilter := ""
	if source != "" {
		sourceFilter = " AND rq.source = ?"
		queryArgs = append(queryArgs, source)
	}
	queryArgs = append(queryArgs, limit, offset)

	rows, err := db.DB.Query(`
		SELECT rq.id, COALESCE(rq.report_id, ''), rq.reason, rq.status, rq.source,
		       COALESCE(rq.target_type, ''), COALESCE(rq.target_id, ''),
		       COALESCE(rq.resolved_at, ''), COALESCE(rq.resolved_by, ''), COALESCE(rq.resolution, ''),
		       rq.created_at,
		       COALESCE(sr.reporter_did, ''), COALESCE(sr.reported_did, ''),
		       COALESCE(sr.category, ''), COALESCE(sr.evidence_verified, 0)
		FROM review_queue rq
		LEFT JOIN spam_reports sr ON sr.id = rq.report_id
		WHERE rq.status = ?`+sourceFilter+`
		ORDER BY rq.created_at ASC
		LIMIT ? OFFSET ?`, queryArgs...,
	)
	if err != nil {
		respond(w, 500, nil, "İnceleme kuyruğu alınamadı")
		return
	}
	defer rows.Close()

	items := []ReviewQueueItem{}
	for rows.Next() {
		var it ReviewQueueItem
		var evidenceVerified int
		if err := rows.Scan(&it.ID, &it.ReportID, &it.Reason, &it.Status, &it.Source,
			&it.TargetType, &it.TargetID, &it.ResolvedAt, &it.ResolvedBy, &it.Resolution,
			&it.CreatedAt, &it.ReporterDID, &it.ReportedDID, &it.Category, &evidenceVerified,
		); err != nil {
			continue
		}
		it.EvidenceVerified = evidenceVerified != 0
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		respond(w, 500, nil, "İnceleme kuyruğu alınamadı")
		return
	}

	countArgs := []interface{}{status}
	countQuery := `SELECT COUNT(*) FROM review_queue rq WHERE rq.status = ?`
	if source != "" {
		countQuery += " AND rq.source = ?"
		countArgs = append(countArgs, source)
	}
	var total int
	db.DB.QueryRow(countQuery, countArgs...).Scan(&total)

	respond(w, 200, map[string]interface{}{
		"items":  items,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	}, "")
}

// ─── RESOLVE ──────────────────────────────────────────────────────────────────

// POST /v1/admin/review-queue/{id}/resolve
//
// Body: {"action": "dismiss"|"confirm_remove"|"confirm_warn", "note": "..."}
//
//   - dismiss: SADECE status/resolved_*/resolution yazılır — içeriğe,
//     ihlal zincirine, spam_reports.verdict'e dokunulmaz.
//   - confirm_remove: hedef içerik kaldırılır (mesaj soft-delete / ilan
//     status=removed — HandleDeleteMessage/HandleMarketplaceDeleteListing'in
//     AYNI SQL geçişi, o handler'ların sahiplik kontrolü admin için
//     anlamsız olduğundan doğrudan uygulanıyor). source='user_report' ise
//     ayrıca moderation.RecordComplaintVerdict(upheld=true) ile Bölüm 3'ün
//     var olan kademeli-ceza zincirine girilir.
//   - confirm_warn: içeriğe DOKUNMAZ, sadece moderation.RecordComplaintVerdict
//     (upheld=true) çağrılır — Bölüm 3'ün kademeli mantığı (1. ihlal=warning,
//     2.=7gün, 3.+=30gün, severe kategori doğrudan 30gün) zaten hangi
//     "basamağa" düşeceğine kendi karar veriyor; admin'in confirm_warn/
//     confirm_remove seçimi CEZA ŞİDDETİNİ değil, içerik kaldırılıp
//     kaldırılmayacağını belirliyor. auto_scan kaynaklı kayıtlarda insan
//     şikayetçi/report_id olmadığından 400 ile reddedilir.
//
// Zaten status != 'pending' olan bir kayıt tekrar resolve edilmeye
// çalışılırsa 409 (idempotent yanlış-kullanım koruması).
func HandleAdminResolveReviewQueue(w http.ResponseWriter, r *http.Request) {
	admin := getUser(r)
	if admin == nil {
		respond(w, 401, nil, "Yetkilendirme gerekli")
		return
	}
	id := mux.Vars(r)["id"]

	var req struct {
		Action string `json:"action"`
		Note   string `json:"note"`
	}
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz istek gövdesi")
		return
	}
	if req.Action != "dismiss" && req.Action != "confirm_remove" && req.Action != "confirm_warn" {
		respond(w, 400, nil, `action "dismiss", "confirm_remove" veya "confirm_warn" olmalı`)
		return
	}

	var status, source, reportID, targetType, targetID string
	err := db.DB.QueryRowContext(r.Context(), `
		SELECT status, source, COALESCE(report_id, ''), COALESCE(target_type, ''), COALESCE(target_id, '')
		FROM review_queue WHERE id = ?`, id,
	).Scan(&status, &source, &reportID, &targetType, &targetID)
	if err == sql.ErrNoRows {
		respond(w, 404, nil, "Kayıt bulunamadı")
		return
	}
	if err != nil {
		respond(w, 500, nil, "Kayıt okunamadı")
		return
	}
	if status != "pending" {
		respond(w, 409, nil, "Bu kayıt zaten çözümlenmiş")
		return
	}
	if req.Action == "confirm_warn" && source != "user_report" {
		respond(w, 400, nil, "confirm_warn yalnızca user_report kaynaklı kayıtlarda kullanılabilir")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	resolution := ""

	switch req.Action {
	case "dismiss":
		resolution = "dismissed"

	case "confirm_warn":
		if reportID == "" {
			respond(w, 500, nil, "Kayıtta report_id eksik (tutarsız durum)")
			return
		}
		if err := moderation.RecordComplaintVerdict(r.Context(), db.DB, reportID, true); err != nil {
			respond(w, 500, nil, "Ceza zinciri tetiklenemedi: "+err.Error())
			return
		}
		resolution = "warned"

	case "confirm_remove":
		if err := adminRemoveTargetContent(r.Context(), source, reportID, targetType, targetID); err != nil {
			respond(w, 500, nil, "İçerik kaldırılamadı: "+err.Error())
			return
		}
		if source == "user_report" {
			if reportID == "" {
				respond(w, 500, nil, "Kayıtta report_id eksik (tutarsız durum)")
				return
			}
			if err := moderation.RecordComplaintVerdict(r.Context(), db.DB, reportID, true); err != nil {
				respond(w, 500, nil, "Ceza zinciri tetiklenemedi: "+err.Error())
				return
			}
		}
		resolution = "removed"
	}

	if _, err := db.DB.ExecContext(r.Context(), `
		UPDATE review_queue SET status = 'resolved', resolved_at = ?, resolved_by = ?, resolution = ?
		WHERE id = ?`, now, admin.DID, resolution, id,
	); err != nil {
		respond(w, 500, nil, "Kayıt güncellenemedi")
		return
	}

	respond(w, 200, map[string]interface{}{
		"id":         id,
		"action":     req.Action,
		"status":     "resolved",
		"resolution": resolution,
	}, "")
}

// adminRemoveTargetContent kaldırılacak içeriği bulur ve kaldırır.
// auto_scan kaynaklı kayıtlarda target_type/target_id doğrudan kullanılır
// (Adım 1). user_report kaynaklı kayıtlarda review_queue.target_type/
// target_id BOŞTUR (EnqueueReview bunları hiç yazmıyor) — gerçek hedef
// spam_reports.message_id üzerinden çözülür. HandleSpamReport message_id'yi
// zorunlu kılıyor, bu yüzden pratikte her zaman dolu; listing_id kolonu
// (migration 154) var ama hiçbir handler onu şu an yazmıyor — yine de
// ileride yazılırsa diye defensive olarak kontrol ediliyor.
func adminRemoveTargetContent(ctx context.Context, source, reportID, targetType, targetID string) error {
	if source == "auto_scan" {
		return removeContentByType(ctx, targetType, targetID)
	}

	var msgID, listingID string
	if err := db.DB.QueryRowContext(ctx,
		"SELECT COALESCE(message_id, ''), COALESCE(listing_id, '') FROM spam_reports WHERE id = ?", reportID,
	).Scan(&msgID, &listingID); err != nil {
		return fmt.Errorf("spam_reports okunamadı: %w", err)
	}
	if msgID != "" {
		return removeContentByType(ctx, "message", msgID)
	}
	if listingID != "" {
		return removeContentByType(ctx, "listing", listingID)
	}
	return fmt.Errorf("rapor bir mesaj/ilan hedefi belirtmiyor")
}

// removeContentByType — HandleDeleteMessage / HandleMarketplaceDeleteListing
// ile AYNI durum geçişi (deleted_at set / status='removed'). O handler'lar
// sahiplik kontrolü yapıyor (mesaj sahibi / ilan satıcısı) — admin bunların
// hiçbiri olmadığından (kasıtlı olarak, moderasyon yetkisi sahiplikten
// bağımsız) o fonksiyonlar yerine aynı SQL geçişi doğrudan uygulanıyor.
func removeContentByType(ctx context.Context, targetType, targetID string) error {
	if targetID == "" {
		return fmt.Errorf("target_id boş")
	}
	switch targetType {
	case "message":
		_, err := db.DB.ExecContext(ctx,
			"UPDATE messages SET deleted_at = ?, ciphertext = '[Mesaj silindi]' WHERE id = ? AND deleted_at IS NULL",
			time.Now().UTC().Format(time.RFC3339), targetID,
		)
		return err
	case "listing":
		_, err := db.DB.ExecContext(ctx,
			"UPDATE marketplace_listings SET status = 'removed', updated_at = ? WHERE id = ? AND status != 'removed'",
			time.Now().UTC().Format(time.RFC3339), targetID,
		)
		return err
	default:
		return fmt.Errorf("bilinmeyen target_type: %q", targetType)
	}
}
