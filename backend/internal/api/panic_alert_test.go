package api_test

// ADIM 3 — Panik mesaj tipi testleri (Madde 13, Bölüm 6).
//
// MsgPanicAlert diğer mesaj tipleri gibi genel HandleSendMessage/HandleGetMessages
// borusundan geçer — özel bir gate yok (kasıtlı: aciliyet gerekçesiyle, sunucu
// zaten from_did/to_did/type görüyor, yeni bir sızıntı açılmıyor — bkz. proje notu
// "Obscura Sealed-Sender Bağlı Değil"). Bu testler ciphertext içindeki payload'ın
// (kaba grid_id) backend tarafından hiç PARSE EDİLMEDİĞİNİ, yalnızca opak blob
// olarak taşındığını doğrular.

import (
	"encoding/json"
	"testing"
)

// TestSendPanicAlertMessageType — panic_alert tipiyle gönderilen mesaj normal
// akıştan (201, DB'ye yazma, WS) geçmeli, type alanı aynen saklanmalı.
func TestSendPanicAlertMessageType(t *testing.T) {
	_, senderToken := registerUserDirect(t, "+905559990101", "panic_sender_001")
	receiverDID, _ := registerUserDirect(t, "+905559990102", "panic_trusted_001")

	sendResp, code := post(t, "/v1/messages", map[string]interface{}{
		"to_id":      receiverDID,
		"ciphertext": "ENCRYPTED(grid_id=4557:3224)", // opak — backend içeriğini asla çözmez
		"type":       "panic_alert",
	}, senderToken)
	if code != 201 || !sendResp.Success {
		t.Fatalf("Panik mesajı gönderilemedi (code=%d): %s", code, sendResp.Error)
	}

	var sendData struct {
		ID     string `json:"id"`
		ConvID string `json:"conv_id"`
	}
	json.Unmarshal(sendResp.Data, &sendData)
	if sendData.ID == "" {
		t.Fatal("Mesaj ID boş")
	}

	histResp, histCode := get(t, "/v1/conversations/"+sendData.ConvID+"/messages", senderToken)
	if histCode != 200 || !histResp.Success {
		t.Fatalf("Geçmiş alınamadı (code=%d): %s", histCode, histResp.Error)
	}
	var msgs []struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		Ciphertext string `json:"ciphertext"`
	}
	json.Unmarshal(histResp.Data, &msgs)

	var found *struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		Ciphertext string `json:"ciphertext"`
	}
	for i := range msgs {
		if msgs[i].ID == sendData.ID {
			found = &msgs[i]
			break
		}
	}
	if found == nil {
		t.Fatal("Gönderilen panik mesajı geçmişte bulunamadı")
	}
	if found.Type != "panic_alert" {
		t.Errorf("type = %q, beklenen \"panic_alert\"", found.Type)
	}
	// Ciphertext AYNEN (opak) döner — backend içindeki grid_id'yi hiç parse etmez.
	if found.Ciphertext != "ENCRYPTED(grid_id=4557:3224)" {
		t.Errorf("ciphertext beklenmedik şekilde değişti: %q — backend içeriği parse ediyor olabilir", found.Ciphertext)
	}
}

// TestPanicAlertConversationPreviewDoesNotLeakContent — konuşma önizlemesi
// panik mesajının içeriğini (grid_id) sızdırmamalı, jenerik "[Şifreli mesaj]"
// kalmalı (yalnızca MsgText özel bir önizleme metni alıyor, diğer her tip gibi).
func TestPanicAlertConversationPreviewDoesNotLeakContent(t *testing.T) {
	_, senderToken := registerUserDirect(t, "+905559990103", "panic_sender_002")
	receiverDID, _ := registerUserDirect(t, "+905559990104", "panic_trusted_002")

	_, code := post(t, "/v1/messages", map[string]interface{}{
		"to_id":      receiverDID,
		"ciphertext": "ENCRYPTED(grid_id=1234:5678)",
		"type":       "panic_alert",
	}, senderToken)
	if code != 201 {
		t.Fatalf("Panik mesajı gönderilemedi (code=%d)", code)
	}

	convResp, convCode := get(t, "/v1/conversations", senderToken)
	if convCode != 200 || !convResp.Success {
		t.Fatalf("Konuşma listesi alınamadı (code=%d)", convCode)
	}
	var convs []struct {
		LastMsgText string `json:"last_msg_text"`
	}
	json.Unmarshal(convResp.Data, &convs)

	for _, c := range convs {
		if c.LastMsgText != "" && c.LastMsgText != "[Şifreli mesaj]" && c.LastMsgText != "🔒 Şifreli" {
			t.Errorf("last_msg_text beklenmedik içerik taşıyor: %q — grid_id sızmış olabilir", c.LastMsgText)
		}
	}
}
