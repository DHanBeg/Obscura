package api

// admin_handlers.go — İlke 5 (docs/spec/obscura_denetim_topluluk_katmani.md
// Bölüm 0: "Sistem hakim değil, ön eleyicidir. AI/otomasyon içeriği
// işaretler, ciddi cezalarda kararı insan (operatör/kurul) verir.") gereği
// review_queue'yu bir insanın görüp karar verebileceği admin yüzeyi.
//
// GET /v1/admin/review-queue — AdminMiddleware ile korunur (bkz. main.go).

import (
	"net/http"
	"strconv"

	"obscura.network/core/internal/db"
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
