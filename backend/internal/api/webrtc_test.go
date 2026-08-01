package api

// Tests for internal/api/webrtc.go (WebRTC signaling: RTCHub registry,
// HandleRTCSignal WebSocket relay, HandleGetTURNCredentials). Previously
// zero test coverage on this file.

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"obscura.network/core/internal/auth"
	"obscura.network/core/internal/models"
)

// ─── RTCHub — Register/Unregister/SendTo/IsOnline (pure, no HTTP) ──────────

func newTestHub() *RTCHub {
	return &RTCHub{clients: make(map[string]*RTCClient)}
}

func TestRTCHub_RegisterAndIsOnline(t *testing.T) {
	hub := newTestHub()
	c := &RTCClient{DID: "did:obs:alice", Send: make(chan []byte, 4)}
	hub.Register(c)
	if !hub.IsOnline("did:obs:alice") {
		t.Fatal("register sonrası IsOnline true olmalıydı")
	}
	if hub.IsOnline("did:obs:bob") {
		t.Fatal("kayıtlı olmayan DID için IsOnline false olmalıydı")
	}
}

func TestRTCHub_RegisterSameDIDClosesOldConnection(t *testing.T) {
	hub := newTestHub()
	oldClient := &RTCClient{DID: "did:obs:alice", Send: make(chan []byte, 4)}
	hub.Register(oldClient)

	newClient := &RTCClient{DID: "did:obs:alice", Send: make(chan []byte, 4)}
	hub.Register(newClient)

	// Eski client'ın Send kanalı kapatılmış olmalı (aynı DID yeniden bağlandı).
	_, ok := <-oldClient.Send
	if ok {
		t.Fatal("aynı DID yeniden register olunca eski bağlantının Send kanalı kapatılmalıydı")
	}
	// Yeni client hâlâ aktif olmalı.
	if !hub.IsOnline("did:obs:alice") {
		t.Fatal("yeni client register sonrası IsOnline true olmalıydı")
	}
}

func TestRTCHub_Unregister(t *testing.T) {
	hub := newTestHub()
	hub.Register(&RTCClient{DID: "did:obs:alice", Send: make(chan []byte, 4)})
	hub.Unregister("did:obs:alice")
	if hub.IsOnline("did:obs:alice") {
		t.Fatal("unregister sonrası IsOnline false olmalıydı")
	}
}

func TestRTCHub_UnregisterNonexistentDoesNotPanic(t *testing.T) {
	hub := newTestHub()
	hub.Unregister("did:obs:hic-yok") // panic etmemeli
}

func TestRTCHub_SendToDeliversToRegisteredClient(t *testing.T) {
	hub := newTestHub()
	c := &RTCClient{DID: "did:obs:bob", Send: make(chan []byte, 4)}
	hub.Register(c)

	ok := hub.SendTo("did:obs:bob", []byte(`{"type":"offer"}`))
	if !ok {
		t.Fatal("kayıtlı client'a SendTo true dönmeliydi")
	}
	select {
	case msg := <-c.Send:
		if string(msg) != `{"type":"offer"}` {
			t.Fatalf("mesaj içeriği yanlış: %s", msg)
		}
	default:
		t.Fatal("mesaj client'ın Send kanalına ulaşmadı")
	}
}

func TestRTCHub_SendToReturnsFalseForUnknownDID(t *testing.T) {
	hub := newTestHub()
	if hub.SendTo("did:obs:hic-yok", []byte("x")) {
		t.Fatal("kayıtlı olmayan DID için SendTo false dönmeliydi")
	}
}

func TestRTCHub_SendToReturnsFalseWhenChannelFull(t *testing.T) {
	hub := newTestHub()
	c := &RTCClient{DID: "did:obs:dolu", Send: make(chan []byte, 1)}
	hub.Register(c)
	if !hub.SendTo("did:obs:dolu", []byte("1")) {
		t.Fatal("ilk mesaj kanala sığmalıydı")
	}
	if hub.SendTo("did:obs:dolu", []byte("2")) {
		t.Fatal("kanal doluyken SendTo false dönmeliydi (non-blocking select/default)")
	}
}

// ─── HandleGetTURNCredentials ───────────────────────────────────────────────

func TestHandleGetTURNCredentials_Unauthorized(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/rtc/turn-credentials", nil)
	w := httptest.NewRecorder()
	HandleGetTURNCredentials(w, req)
	if w.Code != 401 {
		t.Fatalf("user yokken 401 beklenirdi, got=%d", w.Code)
	}
}

func TestHandleGetTURNCredentials_Success(t *testing.T) {
	user := &models.User{ID: "u1", DID: "did:obs:turntest"}
	req := withUser(httptest.NewRequest(http.MethodGet, "/v1/rtc/turn-credentials", nil), user)
	w := httptest.NewRecorder()
	HandleGetTURNCredentials(w, req)

	if w.Code != 200 {
		t.Fatalf("200 beklenirdi, got=%d body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			URLs       []string `json:"urls"`
			Username   string   `json:"username"`
			Credential string   `json:"credential"`
			TTL        int      `json:"ttl"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response parse: %v", err)
	}
	if !resp.Success {
		t.Fatal("success=true beklenirdi")
	}
	if len(resp.Data.URLs) != 3 {
		t.Fatalf("3 URL (turn/udp, turn/tcp, stun) beklenirdi, got=%d", len(resp.Data.URLs))
	}
	if resp.Data.TTL != 3600 {
		t.Fatalf("ttl=3600 beklenirdi, got=%d", resp.Data.TTL)
	}

	// username formatı: "<expire_unix>:<did>"
	parts := strings.SplitN(resp.Data.Username, ":", 2)
	if len(parts) != 2 || !strings.Contains(parts[1], user.DID) {
		t.Fatalf("username formatı beklenmedik: %s", resp.Data.Username)
	}

	// credential = base64(HMAC-SHA1(turnSharedSecret, username)) — bağımsız
	// yeniden hesaplayıp karşılaştır (gerçek kripto doğru mu diye).
	mac := hmac.New(sha1.New, []byte(turnSharedSecret))
	mac.Write([]byte(resp.Data.Username))
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if resp.Data.Credential != want {
		t.Fatalf("credential HMAC doğrulanamadı: got=%s want=%s", resp.Data.Credential, want)
	}
}

func TestHandleGetTURNCredentials_DifferentUsersGetDifferentCredentials(t *testing.T) {
	u1 := &models.User{ID: "u1", DID: "did:obs:aaa"}
	u2 := &models.User{ID: "u2", DID: "did:obs:bbb"}

	w1 := httptest.NewRecorder()
	HandleGetTURNCredentials(w1, withUser(httptest.NewRequest(http.MethodGet, "/v1/rtc/turn-credentials", nil), u1))
	w2 := httptest.NewRecorder()
	HandleGetTURNCredentials(w2, withUser(httptest.NewRequest(http.MethodGet, "/v1/rtc/turn-credentials", nil), u2))

	if w1.Body.String() == w2.Body.String() {
		t.Fatal("farklı kullanıcılar aynı TURN credential'ı almamalı")
	}
}

// ─── HandleRTCSignal — gerçek WebSocket bağlantısıyla uçtan uca ────────────

func mustToken(t *testing.T, did string) string {
	t.Helper()
	tok, err := auth.GenerateToken(&models.User{ID: did, DID: did, Tier: 1})
	if err != nil {
		t.Fatalf("token üretilemedi: %v", err)
	}
	return tok
}

// dialSignal, HandleRTCSignal'a Sec-WebSocket-Protocol üzerinden token
// geçirerek bağlanır (handler'ın birincil token kabul yolu).
func dialSignal(t *testing.T, wsURL, token string) *websocket.Conn {
	t.Helper()
	dialer := websocket.Dialer{Subprotocols: []string{"obscura-signal", token}}
	conn, resp, err := dialer.Dial(wsURL, nil)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("WS dial başarısız (http status=%d): %v", status, err)
	}
	return conn
}

func TestHandleRTCSignal_RejectsMissingToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(HandleRTCSignal))
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("token olmadan bağlantı başarılı oldu, reddedilmesi beklenirdi")
	}
	if resp == nil || resp.StatusCode != 401 {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("401 beklenirdi, got=%d", status)
	}
}

func TestHandleRTCSignal_RejectsInvalidToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(HandleRTCSignal))
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	dialer := websocket.Dialer{Subprotocols: []string{"obscura-signal", "gecersiz-token"}}
	_, resp, err := dialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("geçersiz token ile bağlantı başarılı oldu")
	}
	if resp == nil || resp.StatusCode != 401 {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("401 beklenirdi, got=%d", status)
	}
}

func TestHandleRTCSignal_RelaysOfferToCorrectRecipient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(HandleRTCSignal))
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	aliceDID := "did:obs:alice-rtc"
	bobDID := "did:obs:bob-rtc"
	aliceConn := dialSignal(t, wsURL, mustToken(t, aliceDID))
	defer aliceConn.Close()
	bobConn := dialSignal(t, wsURL, mustToken(t, bobDID))
	defer bobConn.Close()

	// Hub'a register'ın işlenmesi için kısa bir an tanı.
	time.Sleep(50 * time.Millisecond)

	offer := SignalMessage{Type: SignalOffer, To: bobDID, CallID: "call-1", Data: json.RawMessage(`{"sdp":"v=0..."}`)}
	if err := aliceConn.WriteJSON(offer); err != nil {
		t.Fatalf("offer gönderilemedi: %v", err)
	}

	bobConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var got SignalMessage
	if err := bobConn.ReadJSON(&got); err != nil {
		t.Fatalf("bob mesajı okuyamadı: %v", err)
	}
	if got.Type != SignalOffer {
		t.Fatalf("type=offer beklenirdi, got=%s", got.Type)
	}
	if got.From != aliceDID {
		t.Fatalf("From sunucu tarafından alice'in DID'i olarak doldurulmalıydı, got=%s", got.From)
	}
	if got.To != bobDID {
		t.Fatalf("To=bob beklenirdi, got=%s", got.To)
	}
	if got.CallID != "call-1" {
		t.Fatalf("call_id korunmalıydı, got=%s", got.CallID)
	}
}

func TestHandleRTCSignal_OfflineRecipientGetsFallbackHangup(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(HandleRTCSignal))
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	aliceDID := "did:obs:alice-offline-test"
	aliceConn := dialSignal(t, wsURL, mustToken(t, aliceDID))
	defer aliceConn.Close()
	time.Sleep(50 * time.Millisecond)

	offer := SignalMessage{Type: SignalOffer, To: "did:obs:hic-boyle-biri-yok", CallID: "call-x"}
	if err := aliceConn.WriteJSON(offer); err != nil {
		t.Fatalf("offer gönderilemedi: %v", err)
	}

	aliceConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var got SignalMessage
	if err := aliceConn.ReadJSON(&got); err != nil {
		t.Fatalf("alice fallback mesajını okuyamadı: %v", err)
	}
	if got.Type != SignalHangup {
		t.Fatalf("çevrimdışı alıcı için hangup fallback beklenirdi, got=%s", got.Type)
	}
	if got.From != "server" {
		t.Fatalf("fallback From=server beklenirdi, got=%s", got.From)
	}
}

func TestHandleRTCSignal_SelfSendIsDropped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(HandleRTCSignal))
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	aliceDID := "did:obs:alice-self-test"
	aliceConn := dialSignal(t, wsURL, mustToken(t, aliceDID))
	defer aliceConn.Close()
	time.Sleep(50 * time.Millisecond)

	// Kendine gönderim — sessizce görmezden gelinmeli (ne relay ne fallback).
	self := SignalMessage{Type: SignalOffer, To: aliceDID, CallID: "self-call"}
	if err := aliceConn.WriteJSON(self); err != nil {
		t.Fatalf("mesaj gönderilemedi: %v", err)
	}

	// Ardından gerçek bir karşı taraf mesajı gönderip İLK OKUNAN mesajın bu
	// olduğunu doğruluyoruz — self-send için ayrıca bir mesaj gelmediğinin
	// dolaylı kanıtı.
	bobDID := "did:obs:bob-self-test"
	bobConn := dialSignal(t, wsURL, mustToken(t, bobDID))
	defer bobConn.Close()
	time.Sleep(50 * time.Millisecond)

	marker := SignalMessage{Type: SignalOffer, To: bobDID, CallID: "marker-call"}
	if err := aliceConn.WriteJSON(marker); err != nil {
		t.Fatalf("marker mesajı gönderilemedi: %v", err)
	}
	bobConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var got SignalMessage
	if err := bobConn.ReadJSON(&got); err != nil {
		t.Fatalf("bob marker mesajını okuyamadı: %v", err)
	}
	if got.CallID != "marker-call" {
		t.Fatalf("marker-call beklenirdi, got=%s", got.CallID)
	}

	// Alice kendi bağlantısında hiçbir şey almamış olmalı (self-send +
	// marker kendisine yönelik değildi).
	aliceConn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if _, _, err := aliceConn.ReadMessage(); err == nil {
		t.Fatal("alice'in kendi bağlantısında beklenmedik bir mesaj geldi")
	}
}
