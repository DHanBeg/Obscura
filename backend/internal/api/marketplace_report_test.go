package api_test

// #36 — marketplace listing report akışı. Mevcut moderasyon zincirini
// (spam_reports/review_queue/admin resolve, handlers.go:1031 HandleSpamReport)
// listing hedefine genişletir. admin_handlers.go'nun confirm_remove yolu
// (adminRemoveTargetContent → removeContentByType "listing") spam_reports.
// listing_id'yi migration 154'ten beri okuyordu ama yazan yoktu (ölü kod,
// bkz. admin_resolve_test.go:TestAdminResolve_ConfirmRemove_Listing_RemovesListing
// — o test target_type="listing" auto_scan yolunu kullanıyor, BU dosyadaki
// testler user_report/spam_reports.listing_id yolunu kanıtlıyor).

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"obscura.network/core/internal/db"
	"obscura.network/core/internal/marketplace"
)

// createTestListing gerçek marketplace.CreateListing ile bir ilan açar —
// sellerPhone'un kredi/tier'ı SellerAccessLevel (3) karşılamalı.
func createTestListing(t *testing.T, sellerDID string) string {
	t.Helper()
	id, err := marketplace.CreateListing(context.Background(), sellerDID,
		"Rapor Test İlanı", "açıklama", "1000000000000000000", "misc")
	if err != nil {
		t.Fatalf("CreateListing: %v", err)
	}
	return id
}

func spamReportRow(t *testing.T, reportID string) (reporterDID, reportedDID, listingID, category, status string) {
	t.Helper()
	if err := db.DB.QueryRow(
		"SELECT reporter_did, reported_did, COALESCE(listing_id,''), COALESCE(category,''), status FROM spam_reports WHERE id = ?",
		reportID,
	).Scan(&reporterDID, &reportedDID, &listingID, &category, &status); err != nil {
		t.Fatalf("spam_reports satırı okunamadı (id=%s): %v", reportID, err)
	}
	return
}

// TestReportListing_HappyPath_WritesListingIDIntoSpamReports — gerçek bir
// ilanı gerçek bir üye raporlar → spam_reports satırında listing_id dolu,
// reported_did = satıcı, review_queue'ya düşer (source='user_report' default).
func TestReportListing_HappyPath_WritesListingIDIntoSpamReports(t *testing.T) {
	sellerPhone := "+905559996001"
	sellerToken := loginAndRegister(t, sellerPhone, "r36_seller1")
	sellerDID := currentUserDID(t, sellerToken)
	setUserCreditScore(t, sellerPhone, 90, 5) // Elmas — SellerAccessLevel (3) karşılanır

	listingID := createTestListing(t, sellerDID)

	reporterToken := loginAndRegister(t, "+905559996002", "r36_reporter1")
	reporterDID := currentUserDID(t, reporterToken)

	resp, code := post(t, fmt.Sprintf("/v1/marketplace/listings/%s/report", listingID), map[string]interface{}{
		"reason":   "sahte ürün",
		"category": "scam",
	}, reporterToken)
	if code != 200 || !resp.Success {
		t.Fatalf("rapor başarısız: %d %s", code, resp.Error)
	}
	var data struct {
		Status   string `json:"status"`
		ReportID string `json:"report_id"`
	}
	json.Unmarshal(resp.Data, &data)
	if data.ReportID == "" {
		t.Fatal("report_id boş döndü")
	}
	if data.Status != "reported" {
		t.Errorf("status beklenen 'reported', alınan %q", data.Status)
	}

	gotReporter, gotReported, gotListingID, gotCategory, gotStatus := spamReportRow(t, data.ReportID)
	if gotReporter != reporterDID {
		t.Errorf("reporter_did beklenen %q, alınan %q", reporterDID, gotReporter)
	}
	if gotReported != sellerDID {
		t.Errorf("reported_did beklenen %q (satıcı), alınan %q", sellerDID, gotReported)
	}
	if gotListingID != listingID {
		t.Errorf("listing_id beklenen %q, alınan %q — YAZILMADI", listingID, gotListingID)
	}
	if gotCategory != "scam" {
		t.Errorf("category beklenen 'scam', alınan %q", gotCategory)
	}
	if gotStatus != "pending" {
		t.Errorf("status beklenen 'pending', alınan %q", gotStatus)
	}

	var qCount int
	db.DB.QueryRow("SELECT COUNT(*) FROM review_queue WHERE report_id = ?", data.ReportID).Scan(&qCount)
	if qCount != 1 {
		t.Errorf("review_queue'da 1 kayıt bekleniyordu, bulunan %d", qCount)
	}
}

// TestReportListing_NonExistentListing_Returns404 — olmayan/sahte listing_id reddedilir.
func TestReportListing_NonExistentListing_Returns404(t *testing.T) {
	token := loginAndRegister(t, "+905559996003", "r36_reporter2")

	resp, code := post(t, "/v1/marketplace/listings/no-such-listing-id/report", map[string]interface{}{
		"reason":   "spam",
		"category": "spam",
	}, token)
	if code != 404 {
		t.Errorf("olmayan ilan: beklenen 404, alınan %d (%s)", code, resp.Error)
	}
}

// TestReportListing_SelfReport_Returns400 — satıcı kendi ilanını raporlayamaz.
func TestReportListing_SelfReport_Returns400(t *testing.T) {
	sellerPhone := "+905559996004"
	sellerToken := loginAndRegister(t, sellerPhone, "r36_seller2")
	sellerDID := currentUserDID(t, sellerToken)
	setUserCreditScore(t, sellerPhone, 90, 5)

	listingID := createTestListing(t, sellerDID)

	resp, code := post(t, fmt.Sprintf("/v1/marketplace/listings/%s/report", listingID), map[string]interface{}{
		"reason":   "kendi ilanım",
		"category": "spam",
	}, sellerToken)
	if code != 400 {
		t.Errorf("kendi ilanını raporlama: beklenen 400, alınan %d (%s)", code, resp.Error)
	}
}

// TestReportListing_InvalidCategory_Returns400 — kapalı kategori listesi dışı reddedilir.
func TestReportListing_InvalidCategory_Returns400(t *testing.T) {
	sellerPhone := "+905559996005"
	sellerToken := loginAndRegister(t, sellerPhone, "r36_seller3")
	sellerDID := currentUserDID(t, sellerToken)
	setUserCreditScore(t, sellerPhone, 90, 5)
	listingID := createTestListing(t, sellerDID)

	reporterToken := loginAndRegister(t, "+905559996006", "r36_reporter3")

	resp, code := post(t, fmt.Sprintf("/v1/marketplace/listings/%s/report", listingID), map[string]interface{}{
		"reason":   "?",
		"category": "not_a_real_category",
	}, reporterToken)
	if code != 400 {
		t.Errorf("geçersiz kategori: beklenen 400, alınan %d (%s)", code, resp.Error)
	}
}

// TestAdminResolve_ConfirmRemove_ListingUserReport_RemovesListingAndAppliesVerdict
// — uçtan uca: gerçek report endpoint'i → admin resolve confirm_remove →
// ilan kaldırılır + ceza zinciri tetiklenir. admin_resolve_test.go'daki
// TestAdminResolve_ConfirmRemove_UserReport_TriggersViolationChain (mesaj
// hedefli) ile birebir aynı desen, hedef listing.
func TestAdminResolve_ConfirmRemove_ListingUserReport_RemovesListingAndAppliesVerdict(t *testing.T) {
	adminToken := loginAndRegister(t, "+905559996007", "r36_admin1")
	adminDID := currentUserDID(t, adminToken)
	withAdminEnv(t, adminDID)

	sellerPhone := "+905559996008"
	sellerToken := loginAndRegister(t, sellerPhone, "r36_seller4")
	sellerDID := currentUserDID(t, sellerToken)
	setUserCreditScore(t, sellerPhone, 90, 5)
	listingID := createTestListing(t, sellerDID)

	reporterToken := loginAndRegister(t, "+905559996009", "r36_reporter4")

	reportResp, code := post(t, fmt.Sprintf("/v1/marketplace/listings/%s/report", listingID), map[string]interface{}{
		"reason":   "sahte ürün, para alıp göndermedi",
		"category": "scam", // isSevereCategory=true → doğrudan üst basamak (30 gün)
	}, reporterToken)
	if code != 200 || !reportResp.Success {
		t.Fatalf("rapor başarısız: %d %s", code, reportResp.Error)
	}
	var reportData struct {
		ReportID string `json:"report_id"`
	}
	json.Unmarshal(reportResp.Data, &reportData)

	var qID string
	db.DB.QueryRow("SELECT id FROM review_queue WHERE report_id = ?", reportData.ReportID).Scan(&qID)
	if qID == "" {
		t.Fatal("review_queue satırı bulunamadı")
	}

	resolveResp, code := post(t, "/v1/admin/review-queue/"+qID+"/resolve",
		map[string]interface{}{"action": "confirm_remove"}, adminToken)
	if code != 200 || !resolveResp.Success {
		t.Fatalf("confirm_remove başarısız: %d %s", code, resolveResp.Error)
	}

	if got := listingStatus(t, listingID); got != "removed" {
		t.Errorf("ilan status=%q bekleniyordu 'removed'", got)
	}
	if !userIsBanned(t, sellerDID) {
		t.Error("scam ihlali doğrudan 30-gün kısıt uygulamalıydı (is_banned=1), uygulanmadı")
	}
	var verdict string
	db.DB.QueryRow("SELECT verdict FROM spam_reports WHERE id = ?", reportData.ReportID).Scan(&verdict)
	if verdict != "upheld" {
		t.Errorf("spam_reports.verdict=%q bekleniyordu 'upheld'", verdict)
	}
}
