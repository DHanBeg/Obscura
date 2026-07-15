package api_test

// Madde 13 Adım 6 — panik push bildirimi entegrasyon testleri.
//
// push.Default artık bir arayüz (push.Sender) — testler gerçek FCM'e
// gitmeden çağrıları bir spy ile yakalayabiliyor (bkz. internal/push/push.go).

import (
	"context"
	"testing"
	"time"

	"obscura.network/core/internal/db"
	"obscura.network/core/internal/push"
)

type pushSpyMsg struct {
	fcmToken string
	msg      push.FCMMessage
}

type pushSpy struct {
	ch chan pushSpyMsg
}

func (s *pushSpy) Send(ctx context.Context, fcmToken string, msg push.FCMMessage) error {
	s.ch <- pushSpyMsg{fcmToken: fcmToken, msg: msg}
	return nil
}

func setFCMToken(t *testing.T, did, token string) {
	t.Helper()
	if _, err := db.DB.Exec("UPDATE users SET fcm_token = ? WHERE did = ?", token, did); err != nil {
		t.Fatalf("fcm_token güncellenemedi: %v", err)
	}
}

// TestPanicAlertMessageTriggersPanicPush — panic_alert tipiyle gönderilen
// mesaj push.PanicAlert() şablonunu tetiklemeli, Data'da konum SIZMAMALI.
func TestPanicAlertMessageTriggersPanicPush(t *testing.T) {
	spy := &pushSpy{ch: make(chan pushSpyMsg, 4)}
	orig := push.Default
	push.Default = spy
	defer func() { push.Default = orig }()

	_, senderToken := registerUserDirect(t, "+905559990301", "push_sender_001")
	receiverDID, _ := registerUserDirect(t, "+905559990302", "push_receiver_001")
	setFCMToken(t, receiverDID, "fake-fcm-token-001")

	_, code := post(t, "/v1/messages", map[string]interface{}{
		"to_id":      receiverDID,
		"ciphertext": "ENCRYPTED(grid_id=4557:3224)",
		"type":       "panic_alert",
	}, senderToken)
	if code != 201 {
		t.Fatalf("Panik mesajı gönderilemedi (code=%d)", code)
	}

	select {
	case got := <-spy.ch:
		if got.fcmToken != "fake-fcm-token-001" {
			t.Errorf("fcmToken = %q, beklenen fake-fcm-token-001", got.fcmToken)
		}
		if got.msg.Data["type"] != "panic_alert" {
			t.Errorf(`push Data["type"] = %q, beklenen "panic_alert"`, got.msg.Data["type"])
		}
		for _, key := range []string{"grid_id", "lat", "lon", "latitude", "longitude", "location", "semt"} {
			if _, exists := got.msg.Data[key]; exists {
				t.Errorf("push Data[%q] mevcut — konum push'a sızmış", key)
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("panik push'u tetiklenmedi (timeout)")
	}
}

// TestNormalMessageDoesNotTriggerPanicPush — normal metin mesajı PanicAlert
// şablonunu TETİKLEMEMELİ (kendi NewMessage push'unu tetikler, ayrı konu).
func TestNormalMessageDoesNotTriggerPanicPush(t *testing.T) {
	spy := &pushSpy{ch: make(chan pushSpyMsg, 4)}
	orig := push.Default
	push.Default = spy
	defer func() { push.Default = orig }()

	_, senderToken := registerUserDirect(t, "+905559990303", "push_sender_002")
	receiverDID, _ := registerUserDirect(t, "+905559990304", "push_receiver_002")
	setFCMToken(t, receiverDID, "fake-fcm-token-002")

	_, code := post(t, "/v1/messages", map[string]interface{}{
		"to_id":      receiverDID,
		"ciphertext": "normal_payload",
		"type":       "text",
	}, senderToken)
	if code != 201 {
		t.Fatalf("Mesaj gönderilemedi (code=%d)", code)
	}

	select {
	case got := <-spy.ch:
		if got.msg.Data["type"] == "panic_alert" {
			t.Error("normal metin mesajı PanicAlert push şablonunu tetikledi")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("normal mesajın kendi push'u hiç tetiklenmedi (beklenmeyen ayrı regresyon)")
	}
}
