package api_test

// sealed_read_receipt_test.go — Madde 15, Adım 6b: sealed mesajlarda okundu-
// bilgisi, sunucu-taraflı SendReadReceipt (from_did'e dayanır) yerine ALICININ
// GÖNDERDİĞİ ayrı bir sealed mesaj (type: "read_receipt") olarak modellenir.
// Bu test SADECE sunucu tarafını doğrular: read_receipt tipi de her mesaj gibi
// from_did'i opak taşımalı (Adım 5 ile tutarlı), AMA konuşma önizlemesini/
// okunmamış sayacını EZMEMELİ (bir meta-sinyal, "gerçek mesaj" değil).

import (
	"encoding/json"
	"testing"

	"obscura.network/core/internal/db"
)

func TestSealedReadReceiptDoesNotLeakFromDIDOrOverwritePreview(t *testing.T) {
	aliceDID, aliceToken := registerUserDirect(t, "+905550000201", "rr_alice")
	bobDID, bobToken := registerUserDirect(t, "+905550000202", "rr_bob")

	// 1) Alice → Bob normal sealed mesaj (konuşma önizlemesi bunun metnini almalı).
	firstMsg, code := post(t, "/v1/messages", map[string]interface{}{
		"to_id":           bobDID,
		"ciphertext":      "b64:sealed-first-message==",
		"type":            "text",
		"encryption_type": "sealed",
	}, aliceToken)
	if (code != 200 && code != 201) || !firstMsg.Success {
		t.Fatalf("İlk mesaj gönderilemedi: %d %s", code, firstMsg.Error)
	}
	var firstData struct {
		ID     string `json:"id"`
		ConvID string `json:"conv_id"`
	}
	json.Unmarshal(firstMsg.Data, &firstData)
	if firstData.ID == "" {
		t.Fatal("İlk mesaj ID bulunamadı")
	}

	var previewBefore string
	if err := db.DB.QueryRow(`SELECT last_msg_text FROM conversations WHERE id = ?`, firstData.ConvID).
		Scan(&previewBefore); err != nil {
		t.Fatalf("Konuşma önizlemesi okunamadı: %v", err)
	}
	var unreadBefore int
	if err := db.DB.QueryRow(`SELECT unread_count FROM conv_members WHERE conv_id = ? AND user_did = ?`,
		firstData.ConvID, aliceDID).Scan(&unreadBefore); err != nil {
		t.Fatalf("unread_count okunamadı: %v", err)
	}

	// 2) Bob → Alice sealed read_receipt (Adım 6b'nin yeni mekanizması).
	rrResp, code := post(t, "/v1/messages", map[string]interface{}{
		"to_id":           aliceDID,
		"ciphertext":      "b64:sealed-read-receipt-envelope==",
		"type":            "read_receipt",
		"encryption_type": "sealed",
	}, bobToken)
	if (code != 200 && code != 201) || !rrResp.Success {
		t.Fatalf("Read receipt gönderilemedi: %d %s", code, rrResp.Error)
	}
	var rrData struct {
		ID string `json:"id"`
	}
	json.Unmarshal(rrResp.Data, &rrData)
	if rrData.ID == "" {
		t.Fatal("Read receipt mesaj ID bulunamadı")
	}

	// from_did read_receipt'te de opak olmalı (Adım 5 ile tutarlı).
	var fromDID, toDID, encType string
	if err := db.DB.QueryRow(`SELECT from_did, to_did, encryption_type FROM messages WHERE id = ?`, rrData.ID).
		Scan(&fromDID, &toDID, &encType); err != nil {
		t.Fatalf("Read receipt satırı okunamadı: %v", err)
	}
	if fromDID != "" {
		t.Errorf("read_receipt'te de from_did opak (boş) OLMALI, alınan: %q", fromDID)
	}
	if toDID != aliceDID {
		t.Errorf("read_receipt to_did beklenen %q, alınan %q", aliceDID, toDID)
	}
	if encType != "sealed" {
		t.Errorf("read_receipt encryption_type 'sealed' olmalı, alınan %q", encType)
	}

	// 3) Konuşma önizlemesi ve Alice'in okunmamış sayacı read_receipt'ten
	// ETKİLENMEMİŞ olmalı — meta-sinyal, gerçek mesaj gibi davranmamalı.
	var previewAfter string
	db.DB.QueryRow(`SELECT last_msg_text FROM conversations WHERE id = ?`, firstData.ConvID).Scan(&previewAfter)
	if previewAfter != previewBefore {
		t.Errorf("read_receipt konuşma önizlemesini EZMİŞ: önce %q, sonra %q", previewBefore, previewAfter)
	}

	var unreadAfter int
	db.DB.QueryRow(`SELECT unread_count FROM conv_members WHERE conv_id = ? AND user_did = ?`,
		firstData.ConvID, aliceDID).Scan(&unreadAfter)
	if unreadAfter != unreadBefore {
		t.Errorf("read_receipt Alice'in okunmamış sayacını ARTIRMIŞ: önce %d, sonra %d", unreadBefore, unreadAfter)
	}
}

// TestNormalMessageStillIncrementsUnreadAndPreview — read_receipt için eklenen
// `req.Type != models.MsgReadReceipt` koruması, DİĞER tüm mesaj tipleri için
// unread_count/önizleme güncellemesini YANLIŞLIKLA atlamamalı — regresyon kilidi.
func TestNormalMessageStillIncrementsUnreadAndPreview(t *testing.T) {
	_, aliceToken := registerUserDirect(t, "+905550000203", "rr_legacy_alice")
	bobDID, _ := registerUserDirect(t, "+905550000204", "rr_legacy_bob")

	firstMsg, code := post(t, "/v1/messages", map[string]interface{}{
		"to_id":      bobDID,
		"ciphertext": "eski_client_mesaji",
		"type":       "text",
	}, aliceToken)
	if (code != 200 && code != 201) || !firstMsg.Success {
		t.Fatalf("İlk mesaj gönderilemedi: %d %s", code, firstMsg.Error)
	}
	var firstData struct {
		ConvID string `json:"conv_id"`
	}
	json.Unmarshal(firstMsg.Data, &firstData)

	var previewAfterFirst string
	db.DB.QueryRow(`SELECT last_msg_text FROM conversations WHERE id = ?`, firstData.ConvID).Scan(&previewAfterFirst)
	if previewAfterFirst == "" {
		t.Error("normal mesaj sonrası konuşma önizlemesi boş kalmış — güncelleme atlanmış olabilir")
	}

	// unread_count ALICI (Bob) için artar — gönderen (Alice) için değil
	// (UPDATE ... WHERE user_did != gönderen).
	var unreadAfterFirst int
	db.DB.QueryRow(`SELECT unread_count FROM conv_members WHERE conv_id = ? AND user_did = ?`,
		firstData.ConvID, bobDID).Scan(&unreadAfterFirst)
	if unreadAfterFirst < 1 {
		t.Errorf("normal mesaj Bob'un unread_count'unu artırmamış: %d", unreadAfterFirst)
	}
}
