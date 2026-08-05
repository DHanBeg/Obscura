package api_test

// GET /v1/admin/review-queue testleri.

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/moderation"
)

type reviewQueueListResp struct {
	Items []struct {
		ID               string `json:"id"`
		ReportID         string `json:"report_id"`
		Reason           string `json:"reason"`
		Status           string `json:"status"`
		Source           string `json:"source"`
		TargetType       string `json:"target_type"`
		TargetID         string `json:"target_id"`
		ReporterDID      string `json:"reporter_did"`
		ReportedDID      string `json:"reported_did"`
		Category         string `json:"category"`
		EvidenceVerified bool   `json:"evidence_verified"`
	} `json:"items"`
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// seedSpamReport, gerçek bir spam_reports satırı ekler (HandleSpamReport'un
// TIER-A kanıt doğrulama akışını bypass eder — burada sadece review_queue
// endpoint'inin JOIN'ini test ediyoruz, kanıt doğrulamayı değil).
func seedSpamReport(t *testing.T, reporterDID, reportedDID, category string, evidenceVerified bool) string {
	t.Helper()
	id := uuid.New().String()
	ev := 0
	if evidenceVerified {
		ev = 1
	}
	_, err := db.DB.Exec(`
		INSERT INTO spam_reports (id, reporter_did, reported_did, reason, status, category, evidence_verified, created_at)
		VALUES (?, ?, ?, 'test şikayet', 'pending', ?, ?, datetime('now'))`,
		id, reporterDID, reportedDID, category, ev,
	)
	if err != nil {
		t.Fatalf("seedSpamReport: %v", err)
	}
	return id
}

func TestAdminReviewQueue_UnauthorizedAccess_Returns403(t *testing.T) {
	token := loginAndRegister(t, "+905559994001", "arq_unauth")
	did := currentUserDID(t, token)
	withAdminEnv(t, "did:obs:someoneelse") // token'ın DID'i sette değil
	_ = did

	_, code := get(t, "/v1/admin/review-queue", token)
	if code != 403 {
		t.Fatalf("yetkisiz erişimde 403 bekleniyordu, alınan=%d", code)
	}
}

func TestAdminReviewQueue_ListsMixedPendingSources(t *testing.T) {
	adminToken := loginAndRegister(t, "+905559994002", "arq_admin1")
	adminDID := currentUserDID(t, adminToken)
	withAdminEnv(t, adminDID)

	reporterDID := "did:obs:arq-reporter1"
	reportedDID := "did:obs:arq-reported1"
	reportID := seedSpamReport(t, reporterDID, reportedDID, "spam", true)
	if err := moderation.EnqueueReview(context.Background(), db.DB, reportID, "kanıtlı şikayet - inceleme gerekli"); err != nil {
		t.Fatalf("EnqueueReview: %v", err)
	}

	msgID := uuid.New().String()
	if err := moderation.EnqueueAutoScanReview(context.Background(), db.DB, "umay: scam (confidence=0.55)", "message", msgID); err != nil {
		t.Fatalf("EnqueueAutoScanReview: %v", err)
	}

	resp, code := get(t, "/v1/admin/review-queue", adminToken)
	if code != 200 || !resp.Success {
		t.Fatalf("listeleme başarısız: %d %s", code, resp.Error)
	}
	var list reviewQueueListResp
	if err := json.Unmarshal(resp.Data, &list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	var foundUserReport, foundAutoScan bool
	for _, it := range list.Items {
		if it.ReportID == reportID {
			foundUserReport = true
			if it.Source != "user_report" {
				t.Errorf("user_report kaydında source=%q bekleniyordu 'user_report'", it.Source)
			}
			if it.ReporterDID != reporterDID || it.ReportedDID != reportedDID {
				t.Errorf("JOIN yanlış: reporter=%q reported=%q, beklenen %q/%q",
					it.ReporterDID, it.ReportedDID, reporterDID, reportedDID)
			}
			if it.Category != "spam" || !it.EvidenceVerified {
				t.Errorf("JOIN category/evidence_verified yanlış: category=%q verified=%v", it.Category, it.EvidenceVerified)
			}
		}
		if it.TargetID == msgID {
			foundAutoScan = true
			if it.Source != "auto_scan" {
				t.Errorf("auto_scan kaydında source=%q bekleniyordu 'auto_scan'", it.Source)
			}
			if it.TargetType != "message" {
				t.Errorf("target_type=%q bekleniyordu 'message'", it.TargetType)
			}
			if it.ReportID != "" {
				t.Errorf("auto_scan kaydında report_id boş olmalıydı, alınan=%q", it.ReportID)
			}
			if it.ReporterDID != "" || it.Category != "" {
				t.Errorf("auto_scan kaydında JOIN alanları boş olmalıydı (report_id=NULL), reporter=%q category=%q", it.ReporterDID, it.Category)
			}
		}
	}
	if !foundUserReport {
		t.Error("user_report kaydı listede bulunamadı")
	}
	if !foundAutoScan {
		t.Error("auto_scan kaydı listede bulunamadı")
	}
}

func TestAdminReviewQueue_Pagination(t *testing.T) {
	adminToken := loginAndRegister(t, "+905559994003", "arq_admin2")
	adminDID := currentUserDID(t, adminToken)
	withAdminEnv(t, adminDID)

	// Bu testin kendi filtrelenebilir dilimini garanti etmek için ayırt
	// edici bir target_type kullan, source=auto_scan filtresiyle sorgula.
	targetType := "pagination_probe"
	for i := 0; i < 3; i++ {
		reason := fmt.Sprintf("pagination test kaydı #%d", i)
		if err := moderation.EnqueueAutoScanReview(context.Background(), db.DB, reason, targetType, uuid.New().String()); err != nil {
			t.Fatalf("EnqueueAutoScanReview #%d: %v", i, err)
		}
	}

	resp, code := get(t, "/v1/admin/review-queue?source=auto_scan&limit=2&offset=0", adminToken)
	if code != 200 {
		t.Fatalf("sayfa 1 başarısız: %d", code)
	}
	var page1 reviewQueueListResp
	json.Unmarshal(resp.Data, &page1)
	countPage1 := 0
	for _, it := range page1.Items {
		if it.TargetType == targetType {
			countPage1++
		}
	}

	resp2, code2 := get(t, "/v1/admin/review-queue?source=auto_scan&limit=2&offset=2", adminToken)
	if code2 != 200 {
		t.Fatalf("sayfa 2 başarısız: %d", code2)
	}
	var page2 reviewQueueListResp
	json.Unmarshal(resp2.Data, &page2)

	if len(page1.Items) != 2 {
		t.Errorf("sayfa 1: limit=2 iken 2 item bekleniyordu, alınan=%d", len(page1.Items))
	}
	if page1.Limit != 2 || page1.Offset != 0 {
		t.Errorf("sayfa 1: limit/offset yanlış yansıdı: limit=%d offset=%d", page1.Limit, page1.Offset)
	}
	if page2.Offset != 2 {
		t.Errorf("sayfa 2: offset=%d bekleniyordu 2", page2.Offset)
	}
	// Toplam auto_scan pending sayısı en az 3 olmalı (bu test + varsa öncekiler).
	if page1.Total < 3 {
		t.Errorf("total en az 3 olmalıydı (3 yeni auto_scan kaydı), alınan=%d", page1.Total)
	}
}
