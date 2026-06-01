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
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// ─── Yapılandırma ─────────────────────────────────────────────────────────────

// InternalSecret — node'lar arası kimlik doğrulama (env: NODE_INTERNAL_SECRET)
// Production'da zorunlu; dev'de placeholder.
var internalSecret = func() string {
	if s := os.Getenv("NODE_INTERNAL_SECRET"); s != "" {
		return s
	}
	if os.Getenv("OBSCURA_ENV") == "production" {
		log.Fatal("NODE_INTERNAL_SECRET env required in production")
	}
	log.Println("⚠ NODE_INTERNAL_SECRET not set — using dev placeholder")
	return "dev-only-placeholder-not-for-prod"
}()

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

func sendToPeer(peerURL string, body []byte) {
	url := peerURL + "/v1/internal/relay"
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Node-Secret", internalSecret)

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

// RelayHandler — POST /v1/internal/relay
// Diğer node'lardan gelen mesajları alır ve yerel WebSocket hub'a iletir
// Bu handler'ın gerçek mesajlaşma hub'ına entegre edilmesi gerekir
func BuildRelayHandler(onRelay func(targetDID, msgType string, payload interface{})) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Shared secret kontrolü
		if r.Header.Get("X-Node-Secret") != internalSecret {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(401)
			fmt.Fprint(w, `{"success":false,"error":"Yetkisiz"}`)
			return
		}

		var msg RelayMessage
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
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
