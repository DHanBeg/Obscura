package api_test

// ADIM 7 — "Buluştum, iyiyim" onayı testleri (Madde 13, Bölüm 6.1).
//
// im_safe, panic_alert ile aynı genel mesaj/push borusundan geçer ama İKİ
// bilinçli farkla: (1) konum içermez — bu backend'in bilemeyeceği bir şey
// (ciphertext opak) ama push tarafında hiç location-şekilli veri geçmediğini
// yine de doğruluyoruz; (2) NORMAL push şablonunu tetikler, PanicAlert'i
// DEĞİL — "iyiyim" panik kadar acil değil, sessiz modu delmesine gerek yok.

import (
	"encoding/json"
	"testing"
	"time"

	"obscura.network/core/internal/push"
)

// TestSendImSafeMessageType — im_safe tipiyle gönderilen mesaj normal
// akıştan geçmeli, type alanı aynen saklanmalı, ciphertext opak kalmalı.
func TestSendImSafeMessageType(t *testing.T) {
	_, senderToken := registerUserDirect(t, "+905559990401", "imsafe_sender_001")
	receiverDID, _ := registerUserDirect(t, "+905559990402", "imsafe_trusted_001")

	sendResp, code := post(t, "/v1/messages", map[string]interface{}{
		"to_id":      receiverDID,
		"ciphertext": "ENCRYPTED(sent_at=2026-07-16T00:00:00Z)",
		"type":       "im_safe",
	}, senderToken)
	if code != 201 || !sendResp.Success {
		t.Fatalf("İyiyim mesajı gönderilemedi (code=%d): %s", code, sendResp.Error)
	}

	var sendData struct {
		ID     string `json:"id"`
		ConvID string `json:"conv_id"`
	}
	json.Unmarshal(sendResp.Data, &sendData)

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

	var found bool
	for _, m := range msgs {
		if m.ID == sendData.ID {
			found = true
			if m.Type != "im_safe" {
				t.Errorf("type = %q, beklenen \"im_safe\"", m.Type)
			}
			if m.Ciphertext != "ENCRYPTED(sent_at=2026-07-16T00:00:00Z)" {
				t.Errorf("ciphertext beklenmedik şekilde değişti — backend içeriği parse ediyor olabilir")
			}
		}
	}
	if !found {
		t.Fatal("Gönderilen im_safe mesajı geçmişte bulunamadı")
	}
}

// TestImSafeMessageTriggersNormalPushNotPanic — im_safe NORMAL push şablonunu
// tetiklemeli (panik kadar acil değil), PanicAlert'i DEĞİL.
func TestImSafeMessageTriggersNormalPushNotPanic(t *testing.T) {
	spy := &pushSpy{ch: make(chan pushSpyMsg, 4)}
	orig := push.Default
	push.Default = spy
	defer func() { push.Default = orig }()

	_, senderToken := registerUserDirect(t, "+905559990403", "imsafe_sender_002")
	receiverDID, _ := registerUserDirect(t, "+905559990404", "imsafe_trusted_002")
	setFCMToken(t, receiverDID, "fake-fcm-token-imsafe-002")

	_, code := post(t, "/v1/messages", map[string]interface{}{
		"to_id":      receiverDID,
		"ciphertext": "ENCRYPTED(sent_at=...)",
		"type":       "im_safe",
	}, senderToken)
	if code != 201 {
		t.Fatalf("İyiyim mesajı gönderilemedi (code=%d)", code)
	}

	select {
	case got := <-spy.ch:
		if got.msg.Data["type"] == "panic_alert" {
			t.Error("im_safe mesajı PanicAlert push şablonunu tetikledi — 'iyiyim' panik kadar acil değil")
		}
		// Konum sızıntısı kontrolü — im_safe'in KENDİSİ konum içermez, push
		// payload'ında da hiç location-şekilli anahtar bulunmamalı.
		for _, key := range []string{"grid_id", "lat", "lon", "latitude", "longitude", "location"} {
			if _, exists := got.msg.Data[key]; exists {
				t.Errorf("push Data[%q] mevcut — im_safe'te konum olmamalı", key)
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("im_safe'in normal push'u hiç tetiklenmedi (beklenmeyen ayrı regresyon)")
	}
}
