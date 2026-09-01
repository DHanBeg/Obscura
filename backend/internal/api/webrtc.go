package api

// WebRTC sinyalizasyon sunucusu — P2P sesli/görüntülü arama
//
// Mimarisi:
//   - İstemciler WebSocket üzerinden /v1/rtc/signal endpoint'ine bağlanır
//   - Offer/Answer/ICE Candidate'lar relay edilir
//   - Gerçek medya trafiği P2P (coturn üzerinden NAT traversal)
//
// Mesaj formatı:
//   {"type": "offer"|"answer"|"ice"|"call"|"hangup", "to": "did:...", "data": {...}}

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"obscura.network/core/internal/auth"
	"obscura.network/core/internal/logredact"
	"obscura.network/core/internal/secrets"
)

// turnSharedSecret — coturn'daki static-auth-secret ile aynı olmalı.
// (C10 fail-open kökü kapatıldı — secrets.Require, bkz. internal/secrets.)
var turnSharedSecret = secrets.Require("TURN_SECRET", "TURN_SHARED_SECRET")

// turnHost — TURN_HOST env var ile override edilebilir (default: localhost).
var turnHost = func() string {
	if h := os.Getenv("TURN_HOST"); h != "" {
		return h
	}
	return "localhost"
}()

// ─── Sinyal Mesaj Tipleri ─────────────────────────────────────────────────────

type SignalType string

const (
	SignalOffer     SignalType = "offer"
	SignalAnswer    SignalType = "answer"
	SignalICE       SignalType = "ice"
	SignalCall      SignalType = "call"      // Gelen arama bildirimi
	SignalHangup    SignalType = "hangup"    // Arama sonlandı
	SignalBusy      SignalType = "busy"      // Hat meşgul
	SignalReject    SignalType = "reject"    // Arama reddedildi
	SignalCandidate SignalType = "candidate" // ICE candidate
)

// SignalMessage — WebSocket üzerinden gönderilen sinyal paketi
type SignalMessage struct {
	Type   SignalType      `json:"type"`
	From   string          `json:"from"`    // Gönderenin DID'i (sunucu doldurur)
	To     string          `json:"to"`      // Alıcının DID'i
	CallID string          `json:"call_id"` // Oturum ID
	Data   json.RawMessage `json:"data"`    // SDP veya ICE candidate
	TS     int64           `json:"ts"`      // Unix timestamp ms
}

// ─── Sinyal Hub ────────────────────────────────────────────────────────────────

// RTCHub — aktif WebRTC sinyalizasyon bağlantıları
type RTCHub struct {
	mu      sync.RWMutex
	clients map[string]*RTCClient // DID → client
}

// RTCClient — tek bir WebRTC sinyalizasyon bağlantısı
type RTCClient struct {
	DID  string
	Conn *websocket.Conn
	Send chan []byte
}

// GlobalRTCHub — process-wide tekil hub
var GlobalRTCHub = &RTCHub{
	clients: make(map[string]*RTCClient),
}

func (h *RTCHub) Register(client *RTCClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	// Aynı DID'in eski bağlantısını kapat
	if old, ok := h.clients[client.DID]; ok {
		close(old.Send)
	}
	h.clients[client.DID] = client
}

func (h *RTCHub) Unregister(did string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, did)
}

func (h *RTCHub) SendTo(to string, msg []byte) bool {
	h.mu.RLock()
	client, ok := h.clients[to]
	h.mu.RUnlock()
	if !ok {
		return false
	}
	select {
	case client.Send <- msg:
		return true
	default:
		return false
	}
}

func (h *RTCHub) IsOnline(did string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.clients[did]
	return ok
}

// ─── WebRTC Handler ───────────────────────────────────────────────────────────

var rtcUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:    func(r *http.Request) bool { return true },
}

// GET /v1/rtc/signal
// WebSocket tabanlı WebRTC sinyalizasyon.
//
// Token kabul sırası (METADATA FIX 1 — N10 regresyonu kapatıldı: query-param
// dalı TAMAMEN KALDIRILDI, deprecate değil silindi — bkz. main.go:730-753'teki
// aynı gerekçe, nginx access.log $request'in tam URL'i logladığı bulgusu):
//  1. Sec-WebSocket-Protocol subprotocol: new WebSocket(url, ['obscura-signal', token])
//  2. Authorization: Bearer <token> (server-to-server veya native istemciler için)
// Query'de token gelirse SESSİZCE YOK SAYILIR (okunmaz, loglanmaz).
func HandleRTCSignal(w http.ResponseWriter, r *http.Request) {
	// 1. Sec-WebSocket-Protocol subprotocol
	var tokenStr string
	for _, p := range websocket.Subprotocols(r) {
		if p != "obscura-signal" {
			tokenStr = p
			break
		}
	}
	// 2. Authorization header
	if tokenStr == "" {
		if ah := r.Header.Get("Authorization"); strings.HasPrefix(ah, "Bearer ") {
			tokenStr = strings.TrimPrefix(ah, "Bearer ")
		}
	}

	if tokenStr == "" {
		http.Error(w, "Token gerekli", 401)
		return
	}

	claims, err := auth.ValidateToken(tokenStr)
	if err != nil {
		http.Error(w, "Geçersiz token", 401)
		return
	}

	// Accepted subprotocol header — browser WebSocket API requires this to match.
	responseHeader := http.Header{}
	responseHeader.Set("Sec-WebSocket-Protocol", "obscura-signal")
	conn, err := rtcUpgrader.Upgrade(w, r, responseHeader)
	if err != nil {
		log.Printf("RTC WS yükseltme hatası: %v", err)
		return
	}

	client := &RTCClient{
		DID:  claims.DID,
		Conn: conn,
		Send: make(chan []byte, 64),
	}

	GlobalRTCHub.Register(client)
	defer func() {
		GlobalRTCHub.Unregister(claims.DID)
		conn.Close()
	}()

	// Write goroutine
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case msg, ok := <-client.Send:
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if !ok {
					conn.WriteMessage(websocket.CloseMessage, []byte{})
					return
				}
				if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
					return
				}
			case <-ticker.C:
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	// Read pump
	conn.SetReadLimit(32 * 1024) // 32KB max sinyal mesajı
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("RTC WS okuma hatası (%s): %v", logredact.DID(claims.DID), err)
			}
			break
		}
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		var msg SignalMessage
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			log.Printf("RTC mesaj parse hatası: %v", err)
			continue
		}

		// Göndereni ve timestamp'i doldur
		msg.From = claims.DID
		msg.TS = time.Now().UnixMilli()

		// Güvenlik: To boş olamaz, kendine gönderemez
		if msg.To == "" || msg.To == claims.DID {
			continue
		}

		// Relay et
		relayBytes, _ := json.Marshal(msg)
		delivered := GlobalRTCHub.SendTo(msg.To, relayBytes)

		if !delivered {
			// Alıcı çevrimiçi değil
			errMsg := SignalMessage{
				Type:   SignalHangup,
				From:   "server",
				To:     claims.DID,
				CallID: msg.CallID,
				Data:   json.RawMessage(`{"reason":"user_offline"}`),
				TS:     time.Now().UnixMilli(),
			}
			errBytes, _ := json.Marshal(errMsg)
			select {
			case client.Send <- errBytes:
			default:
			}
		}
	}
}

// GET /v1/rtc/turn-credentials
// HMAC-SHA1 short-term TURN credentials üret
//
// coturn static-auth-secret mekanizması:
//   username = <expire_timestamp>:<user_did>
//   password = Base64(HMAC-SHA1(shared_secret, username))
func HandleGetTURNCredentials(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	// 1 saatlik credential
	expireAt := time.Now().Add(time.Hour).Unix()
	username := fmt.Sprintf("%d:%s", expireAt, user.DID)

	// HMAC-SHA1(secret, username)
	mac := hmac.New(sha1.New, []byte(turnSharedSecret))
	mac.Write([]byte(username))
	password := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	respond(w, 200, map[string]interface{}{
		"urls": []string{
			fmt.Sprintf("turn:%s:3478?transport=udp", turnHost),
			fmt.Sprintf("turn:%s:3478?transport=tcp", turnHost),
			fmt.Sprintf("stun:%s:3478", turnHost),
		},
		"username":   username,
		"credential": password,
		"ttl":        3600,
	}, "")
}
