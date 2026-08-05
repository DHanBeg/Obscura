package api_test

// Mesaj Durum Sistemi Testleri (Spec Bölüm 6.4)
//
// Test senaryoları:
//   1. Mesaj gönder → durum 'sent' (alıcı offline)
//   2. Mesaj gönder → durum 'delivered' (alıcı online)
//   3. GET /v1/messages/{id}/status — gönderen sorgulayabilir
//   4. POST /v1/messages/{id}/read — alıcı okundu işaretler
//   5. POST /v1/messages/{id}/read — gönderen kendi mesajını okundu işaretleyemez (403)
//   6. GET /v1/messages/{id}/status — yetkisiz üçüncü kişi erişemez (403)
//   7. POST /v1/messages/{id}/read — olmayan mesaj (404)

import (
	"encoding/json"
	"fmt"
	"testing"
)

// TestMessageStatusSentWhenOffline — offline alıcıya mesaj gönderilince status='sent' olmalı.
func TestMessageStatusSentWhenOffline(t *testing.T) {
	senderToken := loginAndRegister(t, "+905550000201", "status_sender_201")
	_ = loginAndRegister(t, "+905550000202", "status_receiver_202") // offline — WS bağlantısı yok

	// Alıcının DID'ini bul
	r, _ := get(t, "/v1/users/search?q=status_receiver_202", senderToken)
	if !r.Success {
		t.Fatalf("Kullanıcı araması başarısız: %s", r.Error)
	}
	var searchResult struct {
		Users []struct {
			DID string `json:"did"`
		} `json:"users"`
	}
	json.Unmarshal(r.Data, &searchResult)
	users := searchResult.Users
	if len(users) == 0 {
		t.Fatal("Alıcı bulunamadı")
	}
	receiverDID := users[0].DID

	// Mesaj gönder
	sendResp, code := post(t, "/v1/messages", map[string]interface{}{
		"to_id":      receiverDID,
		"ciphertext": "test_ciphertext_offline",
		"type":       "text",
	}, senderToken)
	if code != 201 || !sendResp.Success {
		t.Fatalf("Mesaj gönderilemedi (code=%d): %s", code, sendResp.Error)
	}

	var sendData struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	json.Unmarshal(sendResp.Data, &sendData)
	if sendData.ID == "" {
		t.Fatal("Mesaj ID boş")
	}
	if sendData.Status != "sent" {
		t.Errorf("Beklenen durum='sent', alınan='%s'", sendData.Status)
	}

	// GET /v1/messages/{id}/status — gönderen sorgulayabilir
	statusResp, statusCode := get(t, fmt.Sprintf("/v1/messages/%s/status", sendData.ID), senderToken)
	if statusCode != 200 || !statusResp.Success {
		t.Fatalf("Durum sorgulanamadı (code=%d): %s", statusCode, statusResp.Error)
	}
	var statusData struct {
		MsgID  string `json:"msg_id"`
		Status string `json:"status"`
	}
	json.Unmarshal(statusResp.Data, &statusData)
	if statusData.Status != "sent" {
		t.Errorf("Beklenen durum='sent', alınan='%s'", statusData.Status)
	}
}

// TestMarkMessageRead — alıcı okundu işaretleyebilir; durum 'read' olmalı.
func TestMarkMessageRead(t *testing.T) {
	senderToken := loginAndRegister(t, "+905550000203", "status_sender_203")
	receiverToken := loginAndRegister(t, "+905550000204", "status_receiver_204")

	// Alıcının DID'ini bul
	r, _ := get(t, "/v1/users/search?q=status_receiver_204", senderToken)
	var searchResult struct {
		Users []struct {
			DID string `json:"did"`
		} `json:"users"`
	}
	json.Unmarshal(r.Data, &searchResult)
	users := searchResult.Users
	if len(users) == 0 {
		t.Fatal("Alıcı bulunamadı")
	}
	receiverDID := users[0].DID

	// Mesaj gönder
	sendResp, _ := post(t, "/v1/messages", map[string]interface{}{
		"to_id":      receiverDID,
		"ciphertext": "test_e2ee_payload",
		"type":       "text",
	}, senderToken)
	if !sendResp.Success {
		t.Fatalf("Mesaj gönderilemedi: %s", sendResp.Error)
	}
	var sendData struct {
		ID string `json:"id"`
	}
	json.Unmarshal(sendResp.Data, &sendData)
	msgID := sendData.ID

	// Alıcı okundu işaretler
	readResp, readCode := post(t, fmt.Sprintf("/v1/messages/%s/read", msgID), nil, receiverToken)
	if readCode != 200 || !readResp.Success {
		t.Fatalf("Okundu işaretlenemedi (code=%d): %s", readCode, readResp.Error)
	}
	var readData struct {
		Status string `json:"status"`
	}
	json.Unmarshal(readResp.Data, &readData)
	if readData.Status != "read" {
		t.Errorf("Beklenen status='read', alınan='%s'", readData.Status)
	}

	// GET /v1/messages/{id}/status — durumu doğrula
	statusResp, _ := get(t, fmt.Sprintf("/v1/messages/%s/status", msgID), senderToken)
	var statusData struct {
		Status  string `json:"status"`
		ReadAt  string `json:"read_at"`
	}
	json.Unmarshal(statusResp.Data, &statusData)
	if statusData.Status != "read" {
		t.Errorf("Beklenen status='read', alınan='%s'", statusData.Status)
	}
	if statusData.ReadAt == "" {
		t.Error("read_at boş olmamalı")
	}
}

// TestMarkMessageReadForbiddenForSender — gönderen kendi mesajını okundu işaretleyemez.
func TestMarkMessageReadForbiddenForSender(t *testing.T) {
	senderToken := loginAndRegister(t, "+905550000205", "status_sender_205")
	_ = loginAndRegister(t, "+905550000206", "status_receiver_206")

	r, _ := get(t, "/v1/users/search?q=status_receiver_206", senderToken)
	var searchResult struct {
		Users []struct {
			DID string `json:"did"`
		} `json:"users"`
	}
	json.Unmarshal(r.Data, &searchResult)
	users := searchResult.Users
	if len(users) == 0 {
		t.Fatal("Alıcı bulunamadı")
	}

	sendResp, _ := post(t, "/v1/messages", map[string]interface{}{
		"to_id":      users[0].DID,
		"ciphertext": "another_payload",
		"type":       "text",
	}, senderToken)
	var sendData struct {
		ID string `json:"id"`
	}
	json.Unmarshal(sendResp.Data, &sendData)
	msgID := sendData.ID

	// Gönderen kendi mesajını "read" yapamamalı → 403
	_, forbidCode := post(t, fmt.Sprintf("/v1/messages/%s/read", msgID), nil, senderToken)
	if forbidCode != 403 {
		t.Errorf("Beklenen HTTP 403, alınan %d", forbidCode)
	}
}

// TestGetMessageStatusForbiddenForThirdParty — mesaja dahil olmayan kullanıcı sorgulayamaz.
func TestGetMessageStatusForbiddenForThirdParty(t *testing.T) {
	senderToken := loginAndRegister(t, "+905550000207", "status_sender_207")
	thirdToken := loginAndRegister(t, "+905550000208", "status_third_208")
	_ = loginAndRegister(t, "+905550000209", "status_receiver_209")

	r, _ := get(t, "/v1/users/search?q=status_receiver_209", senderToken)
	var searchResult struct {
		Users []struct {
			DID string `json:"did"`
		} `json:"users"`
	}
	json.Unmarshal(r.Data, &searchResult)
	users := searchResult.Users
	if len(users) == 0 {
		t.Fatal("Alıcı bulunamadı")
	}

	sendResp, _ := post(t, "/v1/messages", map[string]interface{}{
		"to_id":      users[0].DID,
		"ciphertext": "third_party_test",
		"type":       "text",
	}, senderToken)
	var sendData struct {
		ID string `json:"id"`
	}
	json.Unmarshal(sendResp.Data, &sendData)
	msgID := sendData.ID

	// Üçüncü taraf status sorgulayamaz → 403
	_, forbidCode := get(t, fmt.Sprintf("/v1/messages/%s/status", msgID), thirdToken)
	if forbidCode != 403 {
		t.Errorf("Beklenen HTTP 403, alınan %d", forbidCode)
	}
}

// TestMarkMessageReadNotFound — olmayan mesaj 404 döner.
func TestMarkMessageReadNotFound(t *testing.T) {
	token := loginAndRegister(t, "+905550000210", "status_user_210")
	_, code := post(t, "/v1/messages/nonexistent-msg-id-12345/read", nil, token)
	if code != 404 {
		t.Errorf("Beklenen HTTP 404, alınan %d", code)
	}
}

// TestGetMessageStatusNotFound — olmayan mesaj durumu 404 döner.
func TestGetMessageStatusNotFound(t *testing.T) {
	token := loginAndRegister(t, "+905550000211", "status_user_211")
	_, code := get(t, "/v1/messages/nonexistent-msg-id-99999/status", token)
	if code != 404 {
		t.Errorf("Beklenen HTTP 404, alınan %d", code)
	}
}
