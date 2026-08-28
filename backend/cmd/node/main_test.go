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

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gorillaws "github.com/gorilla/websocket"
	"obscura.network/core/internal/auth"
	"obscura.network/core/internal/models"
)

func testJWT(t *testing.T) string {
	t.Helper()
	tok, err := auth.GenerateToken(&models.User{
		ID:   "test-user-id",
		DID:  "did:obs:testuser0000000000000000000",
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
