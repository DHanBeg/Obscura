package main

// N10 fix (C10 launch-blocker) kanıt testleri — /v1/stream artık aynı
// öncelik sırasını kullanıyor webrtc.go'nun HandleRTCSignal'ı gibi:
// Sec-WebSocket-Protocol > Authorization header > query (deprecated+log).
//
// Üç kanıt, üç test:
//  1. TestStreamWSHandler_HeaderAuth_OpensWithoutQuery — gerçek JWT ile
//     header-auth üzerinden, query'siz OPEN oluyor.
//  2. TestStreamWSHandler_QueryAuth_StillWorksButWarns — query-param'lı
//     bağlantı hâlâ açılıyor (geri uyum) AMA deprecation WARN log basıyor.
//  3. TestStreamWSHandler_HeaderTakesPriorityOverQuery — header VE query
//     birlikte gönderilirse header kazanır (mobile reconnect'in — api.ts
//     createWS, hep header gönderir, hiç query'ye düşmez — davranışının
//     backend tarafından da doğru önceliklendirildiğinin kanıtı).
//
// N10-b (web client, frontend/lib/api.ts): tarayıcı WebSocket API'si custom
// header göndermeyi desteklemediği için web ['obscura-stream', token]
// Sec-WebSocket-Protocol'ünü kullanıyor. Bu dosyadaki
// TestStreamWSHandler_SubprotocolAuth_OpensWithoutQuery, gerçek gorilla
// dialer'ın Subprotocols alanıyla AYNI tel-seviyesi handshake'i (Sec-WebSocket-
// Protocol header, custom Authorization header YOK) üretir — bir tarayıcının
// `new WebSocket(url, protocols)` çağrısıyla ürettiği HTTP handshake byte'ları
// ile ayırt edilemez. Bu yüzden bu test, web client'ın backend'e karşı
// dayandığı sözleşmenin canlı kanıtıdır.
//
// TestStreamWSHandler_SubprotocolAuth_RealtimePushWorks — N10-b kanıt #4:
// subprotocol-authenticated bir bağlantının GERÇEK mesaj-pompası (messaging.
// GlobalHub) üzerinden hâlâ çalıştığını kanıtlar. streamWSHandler, auth
// yolundan bağımsız olarak AYNI messaging.Client + Register + ReadPump/
// WritePump'ı kullanır — bu yüzden hub'ın her bağlantıda gönderdiği
// "connected" system mesajını gerçekten alabilmek, real-time push'un
// (mesaj/presence/call hepsi aynı Send kanalından geçer) auth taşıma
// katmanı değişikliğinden etkilenmediğinin canlı kanıtıdır.

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	gorillaws "github.com/gorilla/websocket"
	"obscura.network/core/internal/auth"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/messaging"
	"obscura.network/core/internal/models"
)

// TestMain — Hub.Run() presence lookup için gerçek DB gerektirir
// (internal/messaging/hub.go:228 isHideOnline), messaging/expiry_test.go'nun
// kullandığı AYNI db.Init(tempDir) deseni.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "obscura-ws-test-*")
	if err != nil {
		log.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(dir)
	if err := db.Init(dir); err != nil {
		log.Fatalf("db.Init: %v", err)
	}
	go messaging.GlobalHub.Run()
	os.Exit(m.Run())
}

func testJWT(t *testing.T) string {
	t.Helper()
	return testJWTFor(t, "test-user-id", "did:obs:testuser0000000000000000000")
}

func testJWTFor(t *testing.T, id, did string) string {
	t.Helper()
	tok, err := auth.GenerateToken(&models.User{
		ID:   id,
		DID:  did,
		Tier: 1,
	})
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	return tok
}

func TestStreamWSHandler_HeaderAuth_OpensWithoutQuery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(streamWSHandler))
	defer srv.Close()

	token := testJWT(t)
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/v1/stream" // no ?token=

	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)

	conn, resp, err := gorillaws.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("header-auth dial failed (expected OPEN): %v", err)
	}
	defer conn.Close()
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected 101 Switching Protocols, got %d", resp.StatusCode)
	}
}

// TestStreamWSHandler_SubprotocolAuth_OpensWithoutQuery — web client'ın
// (frontend/lib/api.ts:createWS → new WebSocket(url, ["obscura-stream",
// token])) dayandığı sözleşmenin canlı kanıtı. dialer.Subprotocols, tarayıcı
// WebSocket ctor'ının ürettiği AYNI Sec-WebSocket-Protocol header'ını üretir
// — custom Authorization header YOK (tarayıcılar bunu zaten gönderemez),
// URL'de ?token= YOK.
func TestStreamWSHandler_SubprotocolAuth_OpensWithoutQuery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(streamWSHandler))
	defer srv.Close()

	var logBuf bytes.Buffer
	prevOut := log.Writer()
	log.SetOutput(&logBuf)
	defer log.SetOutput(prevOut)

	token := testJWT(t)
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/v1/stream" // no ?token=

	dialer := *gorillaws.DefaultDialer
	dialer.Subprotocols = []string{"obscura-stream", token}

	conn, resp, err := dialer.Dial(wsURL, nil) // nil header — tarayıcı custom header GÖNDEREMEZ
	if err != nil {
		t.Fatalf("subprotocol-auth dial failed (expected OPEN — this is the exact contract the web client relies on): %v", err)
	}
	defer conn.Close()
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected 101 Switching Protocols, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Sec-WebSocket-Protocol"); got != "obscura-stream" {
		t.Fatalf("expected server to echo back Sec-WebSocket-Protocol: obscura-stream, got %q", got)
	}
	if strings.Contains(logBuf.String(), "deprecated") {
		t.Fatalf("subprotocol auth must not fall through to the deprecated query path. log: %q", logBuf.String())
	}
}

func TestStreamWSHandler_QueryAuth_StillWorksButWarns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(streamWSHandler))
	defer srv.Close()

	var logBuf bytes.Buffer
	prevOut := log.Writer()
	log.SetOutput(&logBuf)
	defer log.SetOutput(prevOut)

	token := testJWT(t)
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/v1/stream?token=" + token

	conn, resp, err := gorillaws.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("query-auth dial failed (must stay backward-compatible): %v", err)
	}
	defer conn.Close()
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected 101 Switching Protocols, got %d", resp.StatusCode)
	}

	if !strings.Contains(logBuf.String(), "deprecated") {
		t.Fatalf("expected deprecation WARN log for query-param token, got log: %q", logBuf.String())
	}
}

func TestStreamWSHandler_HeaderTakesPriorityOverQuery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(streamWSHandler))
	defer srv.Close()

	var logBuf bytes.Buffer
	prevOut := log.Writer()
	log.SetOutput(&logBuf)
	defer log.SetOutput(prevOut)

	validToken := testJWT(t)
	// query taşıyor ama GEÇERSİZ bir token — eğer handler query'yi
	// önceliklendirseydi bu bağlantı 401 ile reddedilirdi.
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/v1/stream?token=garbage-invalid-token"

	header := http.Header{}
	header.Set("Authorization", "Bearer "+validToken)

	conn, resp, err := gorillaws.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("header+query dial failed — header should have taken priority over invalid query token: %v", err)
	}
	defer conn.Close()
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected 101 Switching Protocols, got %d", resp.StatusCode)
	}
	// Header geçerliyse query hiç okunmamalı → deprecation logu OLMAMALI.
	if strings.Contains(logBuf.String(), "deprecated") {
		t.Fatalf("query fallback was reached even though a valid header was present — priority order broken. log: %q", logBuf.String())
	}
}

func TestStreamWSHandler_SubprotocolAuth_RealtimePushWorks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(streamWSHandler))
	defer srv.Close()

	did := "did:obs:realtimepush000000000000000"
	token := testJWTFor(t, "test-user-realtime", did)
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/v1/stream"

	dialer := *gorillaws.DefaultDialer
	dialer.Subprotocols = []string{"obscura-stream", token}

	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("subprotocol dial failed: %v", err)
	}
	defer conn.Close()

	// Hub, Register üzerine HER bağlantıya "connected" system mesajı gönderir
	// (hub.go:150-155) — bu, gerçek mesaj/presence/call push'unun da aktığı
	// AYNI Send kanalı. Bunu alabilmek = real-time push regresyonsuz.
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("expected the hub's post-register push over the subprotocol-authenticated connection, got error: %v", err)
	}

	var msg struct {
		Type    string `json:"type"`
		Payload struct {
			DID string `json:"did"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatalf("could not parse pushed message %q: %v", raw, err)
	}
	if msg.Type != "connected" {
		t.Fatalf("expected type=connected, got %q (raw=%s)", msg.Type, raw)
	}
	if msg.Payload.DID != did {
		t.Fatalf("expected pushed message for our own DID %q, got %q", did, msg.Payload.DID)
	}
}
