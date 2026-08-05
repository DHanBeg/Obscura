package api_test

// POST /v1/admin/review-queue/{id}/resolve testleri. Bu adım en riskli olan
// (gerçek silme/ceza tetikliyor) — her aksiyon için DB'den doğrudan kontrol.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/marketplace"
	"obscura.network/core/internal/moderation"
)

// seedAutoScanQueueItem, gerçek moderation.EnqueueAutoScanReview yazma
// yoluyla bir review_queue satırı ekler ve id'sini döner.
func seedAutoScanQueueItem(t *testing.T, reason, targetType, targetID string) string {
	t.Helper()
	if err := moderation.EnqueueAutoScanReview(context.Background(), db.DB, reason, targetType, targetID); err != nil {
		t.Fatalf("seedAutoScanQueueItem: %v", err)
	}
	var id string
	if err := db.DB.QueryRow(
		`SELECT id FROM review_queue WHERE target_id = ? ORDER BY created_at DESC LIMIT 1`, targetID,
	).Scan(&id); err != nil {
		t.Fatalf("seedAutoScanQueueItem id okunamadı: %v", err)
	}
	return id
}

// sendRealMessage, gerçek POST /v1/messages ile bir mesaj gönderir, id'sini
// döner (admin resolve'un gerçekten var olan bir satırı silip silmediğini
// test etmek için gerçek bir mesaj gerekiyor).
func sendRealMessage(t *testing.T, senderToken, toDID string) string {
	t.Helper()
	resp, code := post(t, "/v1/messages", map[string]interface{}{
		"to_id":      toDID,
		"ciphertext": "resolve_test_payload",
		"type":       "text",
	}, senderToken)
	if code != 201 || !resp.Success {
		t.Fatalf("mesaj gönderilemedi: %d %s", code, resp.Error)
	}
	var data struct {
		ID string `json:"id"`
	}
	json.Unmarshal(resp.Data, &data)
	if data.ID == "" {
		t.Fatal("mesaj id boş")
	}
	return data.ID
}

func messageDeletedAt(t *testing.T, msgID string) string {
	t.Helper()
	var deletedAt string
	db.DB.QueryRow("SELECT COALESCE(deleted_at, '') FROM messages WHERE id = ?", msgID).Scan(&deletedAt)
	return deletedAt
}

func listingStatus(t *testing.T, listingID string) string {
	t.Helper()
	var status string
	db.DB.QueryRow("SELECT status FROM marketplace_listings WHERE id = ?", listingID).Scan(&status)
	return status
}

func reviewQueueRow(t *testing.T, id string) (status, resolvedBy, resolution string) {
	t.Helper()
	db.DB.QueryRow(
		"SELECT status, COALESCE(resolved_by,''), COALESCE(resolution,'') FROM review_queue WHERE id = ?", id,
	).Scan(&status, &resolvedBy, &resolution)
	return
}

func userIsBanned(t *testing.T, did string) bool {
	t.Helper()
	var banned int
	db.DB.QueryRow("SELECT COALESCE(is_banned,0) FROM users WHERE did = ?", did).Scan(&banned)
	return banned == 1
}

// ─── Yetkisiz ───────────────────────────────────────────────────────────────

func TestAdminResolve_Unauthorized_Returns403(t *testing.T) {
	nonAdminToken := loginAndRegister(t, "+905559995001", "arr_nonadmin")
	nonAdminDID := currentUserDID(t, nonAdminToken)
	withAdminEnv(t, "did:obs:someoneelse")
	_ = nonAdminDID

	qID := seedAutoScanQueueItem(t, "test", "message", uuid.New().String())

	_, code := post(t, "/v1/admin/review-queue/"+qID+"/resolve",
		map[string]interface{}{"action": "dismiss"}, nonAdminToken)
	if code != 403 {
		t.Fatalf("yetkisiz erişimde 403 bekleniyordu, alınan=%d", code)
	}
}

// ─── dismiss ──────────────────────────────────────────────────────────────

func TestAdminResolve_Dismiss_DoesNotTouchContent(t *testing.T) {
	adminToken := loginAndRegister(t, "+905559995002", "arr_admin_dismiss")
	adminDID := currentUserDID(t, adminToken)
	withAdminEnv(t, adminDID)

	senderToken := loginAndRegister(t, "+905559995003", "arr_dismiss_sender")
	receiverToken := loginAndRegister(t, "+905559995004", "arr_dismiss_receiver")
	receiverDID := currentUserDID(t, receiverToken)
	msgID := sendRealMessage(t, senderToken, receiverDID)

	qID := seedAutoScanQueueItem(t, "umay: yanlış pozitif olabilir", "message", msgID)

	resp, code := post(t, "/v1/admin/review-queue/"+qID+"/resolve",
		map[string]interface{}{"action": "dismiss", "note": "yanlış pozitif"}, adminToken)
	if code != 200 || !resp.Success {
		t.Fatalf("dismiss başarısız: %d %s", code, resp.Error)
	}

	if got := messageDeletedAt(t, msgID); got != "" {
		t.Errorf("dismiss içeriğe DOKUNMAMALIYDI, ama deleted_at=%q", got)
	}
	status, resolvedBy, resolution := reviewQueueRow(t, qID)
	if status != "resolved" || resolution != "dismissed" || resolvedBy != adminDID {
		t.Errorf("review_queue yanlış: status=%q resolution=%q resolved_by=%q (beklenen resolved/dismissed/%s)",
			status, resolution, resolvedBy, adminDID)
	}
}

// ─── confirm_remove: message ────────────────────────────────────────────────

func TestAdminResolve_ConfirmRemove_Message_DeletesMessage(t *testing.T) {
	adminToken := loginAndRegister(t, "+905559995005", "arr_admin_rmmsg")
	adminDID := currentUserDID(t, adminToken)
	withAdminEnv(t, adminDID)

	senderToken := loginAndRegister(t, "+905559995006", "arr_rmmsg_sender")
	receiverToken := loginAndRegister(t, "+905559995007", "arr_rmmsg_receiver")
	receiverDID := currentUserDID(t, receiverToken)
	msgID := sendRealMessage(t, senderToken, receiverDID)

	qID := seedAutoScanQueueItem(t, "umay: spam (confidence=0.7)", "message", msgID)

	resp, code := post(t, "/v1/admin/review-queue/"+qID+"/resolve",
		map[string]interface{}{"action": "confirm_remove"}, adminToken)
	if code != 200 || !resp.Success {
		t.Fatalf("confirm_remove başarısız: %d %s", code, resp.Error)
	}

	if got := messageDeletedAt(t, msgID); got == "" {
		t.Error("mesaj gerçekten silinmeliydi (deleted_at boş kaldı)")
	}
	status, _, resolution := reviewQueueRow(t, qID)
	if status != "resolved" || resolution != "removed" {
		t.Errorf("review_queue yanlış: status=%q resolution=%q", status, resolution)
	}
}

// ─── confirm_remove: listing ────────────────────────────────────────────────

func TestAdminResolve_ConfirmRemove_Listing_RemovesListing(t *testing.T) {
	adminToken := loginAndRegister(t, "+905559995008", "arr_admin_rmlisting")
	adminDID := currentUserDID(t, adminToken)
	withAdminEnv(t, adminDID)

	sellerPhone := "+905559995014"
	sellerToken := loginAndRegister(t, sellerPhone, "arr_rmlisting_seller")
	sellerDID := currentUserDID(t, sellerToken)
	setUserCreditScore(t, sellerPhone, 90, 5) // Elmas — access level 3, SellerAccessLevel karşılanır

	listingID, err := marketplace.CreateListing(context.Background(), sellerDID,
		"Test İlan", "açıklama", "1000000000000000000", "misc")
	if err != nil {
		t.Fatalf("CreateListing: %v", err)
	}

	qID := seedAutoScanQueueItem(t, "umay: illegal_sale (confidence=0.8)", "listing", listingID)

	resp, code := post(t, "/v1/admin/review-queue/"+qID+"/resolve",
		map[string]interface{}{"action": "confirm_remove"}, adminToken)
	if code != 200 || !resp.Success {
		t.Fatalf("confirm_remove başarısız: %d %s", code, resp.Error)
	}

	if got := listingStatus(t, listingID); got != "removed" {
		t.Errorf("ilan status=%q bekleniyordu 'removed'", got)
	}
}

// ─── confirm_warn: auto_scan'da reddedilmeli ────────────────────────────────

func TestAdminResolve_ConfirmWarn_AutoScan_Returns400(t *testing.T) {
	adminToken := loginAndRegister(t, "+905559995009", "arr_admin_warn400")
	adminDID := currentUserDID(t, adminToken)
	withAdminEnv(t, adminDID)

	qID := seedAutoScanQueueItem(t, "umay: scam (confidence=0.5)", "message", uuid.New().String())

	resp, code := post(t, "/v1/admin/review-queue/"+qID+"/resolve",
		map[string]interface{}{"action": "confirm_warn"}, adminToken)
	if code != 400 {
		t.Fatalf("auto_scan'da confirm_warn için 400 bekleniyordu, alınan=%d %s", code, resp.Error)
	}

	// Reddedilen istek kaydı ÇÖZÜMLENMEMİŞ bırakmalı.
	status, _, _ := reviewQueueRow(t, qID)
	if status != "pending" {
		t.Errorf("400 sonrası kayıt hâlâ pending kalmalıydı, alınan status=%q", status)
	}
}

// ─── tekrar-resolve → 409 ───────────────────────────────────────────────────

func TestAdminResolve_AlreadyResolved_Returns409(t *testing.T) {
	adminToken := loginAndRegister(t, "+905559995010", "arr_admin_409")
	adminDID := currentUserDID(t, adminToken)
	withAdminEnv(t, adminDID)

	qID := seedAutoScanQueueItem(t, "test", "message", uuid.New().String())

	if _, code := post(t, "/v1/admin/review-queue/"+qID+"/resolve",
		map[string]interface{}{"action": "dismiss"}, adminToken); code != 200 {
		t.Fatalf("ilk resolve başarısız olmamalıydı, alınan=%d", code)
	}

	_, code := post(t, "/v1/admin/review-queue/"+qID+"/resolve",
		map[string]interface{}{"action": "dismiss"}, adminToken)
	if code != 409 {
		t.Fatalf("ikinci resolve'da 409 bekleniyordu, alınan=%d", code)
	}
}

// ─── user_report + confirm_remove → gerçekten kademeli-ceza zincirine giriyor mu ──

func TestAdminResolve_ConfirmRemove_UserReport_TriggersViolationChain(t *testing.T) {
	adminToken := loginAndRegister(t, "+905559995011", "arr_admin_chain")
	adminDID := currentUserDID(t, adminToken)
	withAdminEnv(t, adminDID)

	reporterToken := loginAndRegister(t, "+905559995012", "arr_chain_reporter")
	accusedToken := loginAndRegister(t, "+905559995013", "arr_chain_accused")
	accusedDID := currentUserDID(t, accusedToken)
	msgID := sendRealMessage(t, accusedToken, currentUserDID(t, reporterToken))

	reportID := uuid.New().String()
	if _, err := db.DB.Exec(`
		INSERT INTO spam_reports (id, reporter_did, reported_did, reason, status, category, message_id, created_at)
		VALUES (?, ?, ?, 'taciz', 'pending', ?, ?, datetime('now'))`,
		reportID, currentUserDID(t, reporterToken), accusedDID, moderation.CategoryHarassment, msgID,
	); err != nil {
		t.Fatalf("spam_reports seed: %v", err)
	}
	if err := moderation.EnqueueReview(context.Background(), db.DB, reportID, "insan incelemesi gerekli"); err != nil {
		t.Fatalf("EnqueueReview: %v", err)
	}
	var qID string
	db.DB.QueryRow(`SELECT id FROM review_queue WHERE report_id = ?`, reportID).Scan(&qID)
	if qID == "" {
		t.Fatal("review_queue satırı bulunamadı")
	}

	resp, code := post(t, "/v1/admin/review-queue/"+qID+"/resolve",
		map[string]interface{}{"action": "confirm_remove"}, adminToken)
	if code != 200 || !resp.Success {
		t.Fatalf("confirm_remove başarısız: %d %s", code, resp.Error)
	}

	if got := messageDeletedAt(t, msgID); got == "" {
		t.Error("mesaj silinmeliydi")
	}
	// harassment CategoryHarassment isSevereCategory=true → doğrudan 30 gün kısıt.
	if !userIsBanned(t, accusedDID) {
		t.Error("harassment ihlali doğrudan 30-gün kısıt uygulamalıydı (is_banned=1), uygulanmadı")
	}
	var verdict string
	db.DB.QueryRow(`SELECT verdict FROM spam_reports WHERE id = ?`, reportID).Scan(&verdict)
	if verdict != "upheld" {
		t.Errorf("spam_reports.verdict=%q bekleniyordu 'upheld'", verdict)
	}
}
