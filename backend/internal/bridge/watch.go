// Package bridge — author_submitAndWatchExtrinsic üzerinden gerçek zaman
// durum takibi (ready/broadcast/inBlock/finalized/dropped/invalid/usurped).
//
// HTTP JSON-RPC abonelik desteklemez (bkz. dot_submit.go üstündeki not:
// PolkadotRPC düz HTTPS POST) — bu yüzden bu dosya ayrı bir WebSocket
// bağlantısı açar (aynı düğüm, wss:// şeması).
package bridge

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// deriveWSURL, https/http RPC URL'sini karşılık gelen wss/ws URL'sine çevirir.
func deriveWSURL(httpURL string) (string, error) {
	switch {
	case strings.HasPrefix(httpURL, "https://"):
		return "wss://" + strings.TrimPrefix(httpURL, "https://"), nil
	case strings.HasPrefix(httpURL, "http://"):
		return "ws://" + strings.TrimPrefix(httpURL, "http://"), nil
	default:
		return "", fmt.Errorf("desteklenmeyen RPC URL şeması (wss türetilemedi): %s", httpURL)
	}
}

// SubmitAndWatchExtrinsic, author_submitAndWatchExtrinsic ile imzalı
// extrinsic'i gönderir ve abonelik bildirimlerini onUpdate ile (her durum
// değişiminde bir kez) çağırıcıya iletir. "finalized", "dropped", "invalid"
// veya "usurped" durumlarından biri gelince ya da ctx iptal olunca döner.
//
// GERİ ALINAMAZ: bu fonksiyon zincire gerçekten yazar. Çağıran kod, kullanıcı
// AÇIK ONAY vermeden bunu çağırmamalıdır (bkz. dot_submit.go, SubmitExtrinsic
// dokümantasyonu — aynı kısıt burada da geçerli).
func (c *Client) SubmitAndWatchExtrinsic(ctx context.Context, extrinsicHex string, onUpdate func(status map[string]interface{})) (map[string]interface{}, error) {
	if c.cfg.PolkadotRPC == "" {
		return nil, fmt.Errorf("DOT_RPC_URL yapılandırılmadı")
	}
	wsURL, err := deriveWSURL(c.cfg.PolkadotRPC)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(extrinsicHex, "0x") {
		extrinsicHex = "0x" + extrinsicHex
	}

	dialer := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("websocket bağlantısı (%s): %w", wsURL, err)
	}
	defer conn.Close()

	req := map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": "author_submitAndWatchExtrinsic",
		"params": []interface{}{extrinsicHex},
	}
	if err := conn.WriteJSON(req); err != nil {
		return nil, fmt.Errorf("author_submitAndWatchExtrinsic yaz: %w", err)
	}

	subID, err := readSubscriptionID(conn)
	if err != nil {
		return nil, err
	}

	// ctx iptal olunca bağlantıyı kapat — bloklanmış ReadJSON'u uyandırmanın
	// güvenli yolu bu (gorilla/websocket: bir okuma zaman aşımına uğradıktan
	// SONRA aynı bağlantıda tekrar okumak panic atar, "repeated read on
	// failed websocket connection" — bu yüzden SetReadDeadline ile
	// döngü-içi retry KULLANILMAZ).
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-done:
		}
	}()

	var lastStatus map[string]interface{}
	for {
		var msg map[string]interface{}
		err := conn.ReadJSON(&msg)
		if err != nil {
			if ctx.Err() != nil {
				return lastStatus, ctx.Err()
			}
			return lastStatus, fmt.Errorf("websocket okuma: %w", err)
		}

		params, _ := msg["params"].(map[string]interface{})
		if params == nil {
			continue
		}
		if sub, _ := params["subscription"].(string); sub != subID {
			continue
		}

		statusMap := normalizeExtrinsicStatus(params["result"])
		if statusMap == nil {
			continue
		}
		lastStatus = statusMap
		if onUpdate != nil {
			onUpdate(statusMap)
		}

		if _, ok := statusMap["finalized"]; ok {
			return statusMap, nil
		}
		if _, ok := statusMap["dropped"]; ok {
			return statusMap, fmt.Errorf("extrinsic dropped (havuzdan düştü)")
		}
		if v, ok := statusMap["invalid"]; ok {
			return statusMap, fmt.Errorf("extrinsic invalid: %v", v)
		}
		if v, ok := statusMap["usurped"]; ok {
			return statusMap, fmt.Errorf("extrinsic usurped (başka biri aynı nonce'u kullandı): %v", v)
		}
	}
}

func readSubscriptionID(conn *websocket.Conn) (string, error) {
	conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	var msg map[string]interface{}
	if err := conn.ReadJSON(&msg); err != nil {
		return "", fmt.Errorf("abonelik yanıtı okunamadı: %w", err)
	}
	if errObj, ok := msg["error"]; ok {
		return "", fmt.Errorf("author_submitAndWatchExtrinsic hata döndürdü: %v", errObj)
	}
	subID, ok := msg["result"].(string)
	if !ok || subID == "" {
		return "", fmt.Errorf("abonelik id'si alınamadı, yanıt: %+v", msg)
	}
	return subID, nil
}

// normalizeExtrinsicStatus, ExtrinsicStatus enum'ının JSON temsilini
// (bazı varyantlar düz string — "ready"/"future"/"broadcast" boşsa,
// bazıları {"inBlock":"0x.."} gibi tek-anahtarlı obje) tek bir map'e indirger.
func normalizeExtrinsicStatus(result interface{}) map[string]interface{} {
	switch v := result.(type) {
	case string:
		return map[string]interface{}{v: true}
	case map[string]interface{}:
		return v
	default:
		return nil
	}
}
