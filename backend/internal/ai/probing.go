package ai

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// StartNodeProber — bilinen federation node'larını periyodik olarak probe eder,
// gerçek latency + uptime verilerini GlobalOptimizer'a kaydeder.
// NODE_PEERS env değişkenindeki node'ları kullanır.
func StartNodeProber(ctx context.Context) {
	go func() {
		for {
			peers := parsePeers()
			for _, peer := range peers {
				go probePeer(peer)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(30 * time.Second): // 30 saniyede bir probe
			}
		}
	}()
}

// RecordMessageLoad — WebSocket hub'dan mesaj yük verisini optimizer'a aktar.
// messaging paketi tarafından her N mesajda bir çağrılır.
func RecordMessageLoad(count float64) {
	GlobalOptimizer.RecordLoad(count)
}

func probePeer(peer string) {
	url := "http://" + peer + "/v1/node/status"
	if strings.HasPrefix(peer, "http") {
		url = peer + "/v1/node/status"
	}

	start := time.Now()
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	latency := float64(time.Since(start).Milliseconds())

	nodeID := peer // node kimliği olarak adres kullan (gerçekte node_id olmalı)

	if err != nil {
		GlobalOptimizer.RecordError(nodeID, true)
		GlobalOptimizer.RecordUptime(nodeID, false)
		GlobalOptimizer.RecordLatency(nodeID, 5000) // timeout = yüksek latency
		log.Printf("🔴 Node probe başarısız (%s): %v", peer, err)
		return
	}
	defer resp.Body.Close()

	GlobalOptimizer.RecordLatency(nodeID, latency)
	GlobalOptimizer.RecordUptime(nodeID, resp.StatusCode < 500)
	GlobalOptimizer.RecordError(nodeID, resp.StatusCode >= 500)
}

func parsePeers() []string {
	raw := os.Getenv("NODE_PEERS")
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	var peers []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			peers = append(peers, p)
		}
	}
	return peers
}
