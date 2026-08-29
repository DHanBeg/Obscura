// Package gossip — Federatif node iletişimi
//
// Kullanıcı bu node'da çevrimiçi değilse, mesaj peer node'lara iletilir.
// NODE_PEERS env: "node-2:8082,node-3:8083,..."
//
// Protokol: HTTP POST /v1/internal/relay (basit, authenticated)
// İnter-node şifreleme için: HMAC-SHA256 ile shared secret doğrulama
package gossip

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"obscura.network/core/internal/secrets"
)

// ─── Yapılandırma ─────────────────────────────────────────────────────────────

// internalSecret — node'lar arası kimlik doğrulama (env: NODE_INTERNAL_SECRET).
// (C10 fail-open kökü kapatıldı — secrets.Require, bkz. internal/secrets.)
var internalSecret = secrets.Require("NODE_INTERNAL_SECRET")

// ─── Relay Mesajı ─────────────────────────────────────────────────────────────

type RelayMessage struct {
	TargetDID string      `json:"target_did"`
	MsgType   string      `json:"msg_type"`
	Payload   interface{} `json:"payload"`
	SentAt    int64       `json:"sent_at"`
	NodeID    string      `json:"node_id"` // Gönderen node
}

// ─── Peer Listesi ─────────────────────────────────────────────────────────────

var (
	peers []string
	once  sync.Once
)

func getPeers() []string {
	once.Do(func() {
		raw := os.Getenv("NODE_PEERS")
		if raw == "" {
			return
		}
		for _, p := range strings.Split(raw, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				peers = append(peers, "http://"+p)
			}
		}
		log.Printf("🔗 Gossip peer'ları: %v", peers)
	})
	return peers
}

// ─── Relay ─────────────────────────────────────────────────────────────────────

// RelayToPeers — hedef kullanıcı bu node'da yoksa peer node'lara ilet
// Tüm peer'lara paralel olarak gönderilir (fire-and-forget)
func RelayToPeers(targetDID, msgType string, payload interface{}) {
	peerList := getPeers()
	if len(peerList) == 0 {
		return
	}

	msg := RelayMessage{
		TargetDID: targetDID,
		MsgType:   msgType,
		Payload:   payload,
		SentAt:    time.Now().UnixMilli(),
		NodeID:    os.Getenv("NODE_ID"),
	}

	body, err := json.Marshal(msg)
	if err != nil {
		log.Printf("⚠️ Gossip marshal hatası: %v", err)
		return
	}

	// Tüm peer'lara paralel gönder
	for _, peer := range peerList {
		go sendToPeer(peer, body)
	}
}

// nodeMAC computes HMAC-SHA256(secret, ts+body) for inter-node auth.
// Prevents plaintext secret exposure and replay attacks (timestamp included).
func nodeMAC(body []byte) (ts, sig string) {
	ts = strconv.FormatInt(time.Now().UnixMilli(), 10)
	mac := hmac.New(sha256.New, []byte(internalSecret))
	mac.Write([]byte(ts))
	mac.Write(body)
	return ts, hex.EncodeToString(mac.Sum(nil))
}

func sendToPeer(peerURL string, body []byte) {
	url := peerURL + "/v1/internal/relay"
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return
	}

	ts, sig := nodeMAC(body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Node-Ts", ts)
	req.Header.Set("X-Node-Sig", sig)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		// Peer erişilemez — normal, loglama yapma (ağ partisyonları beklenir)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Printf("📡 Relay başarılı → %s", peerURL)
	}
}

// ─── Internal Relay Handler ─────────────────────────────────────────────────────

// verifyRelayHMAC checks HMAC-SHA256(secret, ts+body) for inter-node auth.
// Rejects |now - ts| > 30 s to prevent replay attacks (N11 fix).
func verifyRelayHMAC(tsStr, sigHex string, body []byte) bool {
	if tsStr == "" || sigHex == "" {
		return false
	}
	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return false
	}
	diff := time.Now().UnixMilli() - ts
	if diff < -30_000 || diff > 30_000 {
		return false
	}
	mac := hmac.New(sha256.New, []byte(internalSecret))
	mac.Write([]byte(tsStr))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(sigHex), []byte(expected))
}

// RelayHandler — POST /v1/internal/relay
// Diğer node'lardan gelen mesajları alır ve yerel WebSocket hub'a iletir
// Bu handler'ın gerçek mesajlaşma hub'ına entegre edilmesi gerekir
func BuildRelayHandler(onRelay func(targetDID, msgType string, payload interface{})) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Body'yi oku — HMAC doğrulama ve JSON parse için aynı baytlar kullanılır.
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			fmt.Fprint(w, `{"success":false,"error":"Body okunamadı"}`)
			return
		}

		// HMAC-SHA256 doğrula — replay koruması dahil (±30 s pencere)
		if !verifyRelayHMAC(r.Header.Get("X-Node-Ts"), r.Header.Get("X-Node-Sig"), body) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(401)
			fmt.Fprint(w, `{"success":false,"error":"Yetkisiz"}`)
			return
		}

		var msg RelayMessage
		if err := json.Unmarshal(body, &msg); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			fmt.Fprint(w, `{"success":false,"error":"Geçersiz JSON"}`)
			return
		}

		// Loop önleme: kendi NODE_ID'miz ise atla
		if msg.NodeID == os.Getenv("NODE_ID") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			fmt.Fprint(w, `{"success":true,"data":{"skipped":true}}`)
			return
		}

		targetPrefix := msg.TargetDID
		if len(targetPrefix) > 12 {
			targetPrefix = targetPrefix[:12]
		}
		log.Printf("📨 Relay alındı: %s → %s (kaynak: %s)", msg.MsgType, targetPrefix, msg.NodeID)

		// Yerel hub'a ilet
		if onRelay != nil {
			go onRelay(msg.TargetDID, msg.MsgType, msg.Payload)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{"success":true,"data":{"ok":true}}`)
	}
}

// ─── Sağlık Kontrolü ──────────────────────────────────────────────────────────

// PeerHealth — tüm peer'ların durumunu kontrol et
func PeerHealth() map[string]bool {
	peerList := getPeers()
	result := make(map[string]bool, len(peerList))

	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, peer := range peerList {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			client := &http.Client{Timeout: 3 * time.Second}
			resp, err := client.Get(p + "/v1/node/status")
			healthy := err == nil && resp != nil && resp.StatusCode == 200
			mu.Lock()
			result[p] = healthy
			mu.Unlock()
		}(peer)
	}

	wg.Wait()
	return result
}
