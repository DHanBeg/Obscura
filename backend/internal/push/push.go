// Package push — Mobil push bildirimleri (FCM HTTP v1 API)
//
// Env değişkenleri:
//   FCM_PROJECT_ID   — Firebase proje kimliği
//   FCM_SERVICE_KEY  — Service account JSON içeriği (base64)
//                      veya dosya yolu için FCM_SERVICE_KEY_FILE
//
// Development'ta sadece loglama yapılır (FCM_PROJECT_ID boşsa).
package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

// ─── Mesaj Tipleri ────────────────────────────────────────────────────────────

type Notification struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type AndroidConfig struct {
	Priority string `json:"priority"` // "high" | "normal"
	TTL      string `json:"ttl"`      // "3600s"
}

type APNSConfig struct {
	Headers map[string]string `json:"headers"`
}

type FCMMessage struct {
	Token        string        `json:"token"`                  // Cihaz FCM token
	Notification *Notification `json:"notification,omitempty"` // Görünür bildirim
	Data         map[string]string `json:"data,omitempty"`     // Sessiz veri
	Android      *AndroidConfig    `json:"android,omitempty"`
	APNS         *APNSConfig       `json:"apns,omitempty"`
}

type fcmRequest struct {
	Message FCMMessage `json:"message"`
}

// ─── Push Servisi ─────────────────────────────────────────────────────────────

type Service struct {
	projectID string
	client    *http.Client
}

var Default = &Service{
	projectID: os.Getenv("FCM_PROJECT_ID"),
	client:    &http.Client{Timeout: 10 * time.Second},
}

// Send — FCM mesajı gönder
// fcmToken: alıcının cihaz FCM token'ı
func (s *Service) Send(ctx context.Context, fcmToken string, msg FCMMessage) error {
	if s.projectID == "" {
		// Development: sadece logla
		log.Printf("📲 [PUSH-DEV] → token=%s...  title=%q body=%q",
			truncate(fcmToken, 20),
			notifTitle(msg),
			notifBody(msg),
		)
		return nil
	}

	msg.Token = fcmToken
	payload := fcmRequest{Message: msg}
	body, _ := json.Marshal(payload)

	url := fmt.Sprintf(
		"https://fcm.googleapis.com/v1/projects/%s/messages:send",
		s.projectID,
	)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("FCM istek oluşturma: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// OAuth2 token (Service Account) — prod'da google.golang.org/api ile
	// Şimdilik basit env-key flow
	if apiKey := os.Getenv("FCM_LEGACY_KEY"); apiKey != "" {
		req.Header.Set("Authorization", "key="+apiKey)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("FCM HTTP: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var errBody map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&errBody)
		return fmt.Errorf("FCM hata %d: %v", resp.StatusCode, errBody)
	}

	log.Printf("✅ Push gönderildi: %s...", truncate(fcmToken, 16))
	return nil
}

// ─── Hazır Bildirim Şablonları ────────────────────────────────────────────────

// NewMessage — yeni mesaj push bildirimi
func NewMessage(senderName, preview, convID string) FCMMessage {
	return FCMMessage{
		Notification: &Notification{
			Title: senderName,
			Body:  preview,
		},
		Data: map[string]string{
			"type":    "new_message",
			"conv_id": convID,
		},
		Android: &AndroidConfig{
			Priority: "high",
			TTL:      "3600s",
		},
		APNS: &APNSConfig{
			Headers: map[string]string{
				"apns-priority": "10",
				"apns-topic":    "network.obscura.app",
			},
		},
	}
}

// IncomingCall — gelen arama push bildirimi
func IncomingCall(callerName, callID string) FCMMessage {
	return FCMMessage{
		Notification: &Notification{
			Title: "📞 " + callerName + " arıyor",
			Body:  "Obscura aramayı yanıtlamak için dokununuz",
		},
		Data: map[string]string{
			"type":    "incoming_call",
			"call_id": callID,
		},
		Android: &AndroidConfig{
			Priority: "high",
			TTL:      "30s", // 30 saniyeden sonra geçersiz
		},
		APNS: &APNSConfig{
			Headers: map[string]string{
				"apns-priority":    "10",
				"apns-push-type":   "voip",
				"apns-topic":       "network.obscura.app.voip",
				"apns-expiration":  "30",
			},
		},
	}
}

// ─── Yardımcılar ──────────────────────────────────────────────────────────────

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func notifTitle(m FCMMessage) string {
	if m.Notification != nil {
		return m.Notification.Title
	}
	return ""
}

func notifBody(m FCMMessage) string {
	if m.Notification != nil {
		return m.Notification.Body
	}
	return ""
}
