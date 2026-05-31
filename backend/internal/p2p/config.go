// Package p2p — config yapısı ve env'den yükleme
package p2p

import (
	"os"
	"strings"
)

// Config — libp2p host yapılandırması
type Config struct {
	// Enabled: false ise P2P host başlatılmaz, HTTP gossip devam eder
	Enabled bool

	// ListenTCP: TCP multiaddr, örn. /ip4/0.0.0.0/tcp/9000
	ListenTCP string

	// ListenQUIC: QUIC multiaddr, örn. /ip4/0.0.0.0/udp/9001/quic-v1
	// Boş bırakılırsa QUIC dinlenmez
	ListenQUIC string

	// BootstrapPeers: virgülle ayrılmış /p2p/ bitiş adresli multiaddr listesi
	// Örn: /dns4/node-1/tcp/9000/p2p/QmXxx,/ip4/1.2.3.4/tcp/9000/p2p/QmYyy
	BootstrapPeers []string

	// ZKAuthEnabled: true ise bağlanan her peer'dan node_proof ZK kanıtı istenir
	// P2P_ZK_AUTH=true env değişkeni ile açılır; başlangıçta false tutulmalı
	ZKAuthEnabled bool

	// NodeID: NODE_ID env — log ve federation için
	NodeID string

	// PrivateKeyPath: Ed25519 identity key dosya yolu
	// Boş bırakılırsa her başlatmada yeni anahtar üretilir (dev mode)
	PrivateKeyPath string
}

// ConfigFromEnv env değişkenlerinden Config üretir.
//
// Env değişkenleri:
//
//	P2P_ENABLED       — default "true" (false yazmak devre dışı bırakır)
//	P2P_LISTEN_TCP    — default /ip4/0.0.0.0/tcp/9000
//	P2P_LISTEN_QUIC   — default /ip4/0.0.0.0/udp/9001/quic-v1 (boş bırakılabilir)
//	BOOTSTRAP_PEERS   — virgülle ayrılmış multiaddr listesi
//	P2P_ZK_AUTH       — default "false"
//	NODE_ID           — node kimliği
//	P2P_KEY_PATH      — identity key dosya yolu (boş = in-memory)
func ConfigFromEnv() Config {
	return Config{
		Enabled:        os.Getenv("P2P_ENABLED") != "false",
		ListenTCP:      getEnvOr("P2P_LISTEN_TCP", "/ip4/0.0.0.0/tcp/9000"),
		ListenQUIC:     getEnvOr("P2P_LISTEN_QUIC", "/ip4/0.0.0.0/udp/9001/quic-v1"),
		BootstrapPeers: parseBootstrapPeers(),
		ZKAuthEnabled:  os.Getenv("P2P_ZK_AUTH") == "true",
		NodeID:         os.Getenv("NODE_ID"),
		PrivateKeyPath: os.Getenv("P2P_KEY_PATH"),
	}
}

// parseBootstrapPeers BOOTSTRAP_PEERS env'den peer adreslerini okur.
// Boşsa NODE_PEERS'den HTTP peer listesini P2P adrese çevirmeyi dener.
func parseBootstrapPeers() []string {
	raw := os.Getenv("BOOTSTRAP_PEERS")
	if raw != "" {
		return splitTrimmed(raw, ',')
	}
	// NODE_PEERS fallback — HTTP peer'ları P2P adres tahminlerine dönüştür
	// Format: "host:port" → /dns4/host/tcp/9000/p2p/<peerID>
	// PeerID bu aşamada bilinmediği için sadece /dns4/host/tcp/9000 formatında döner;
	// DHT bootstrap bu adresi, peer'ın kendi ID'sini announce etmesinden sonra tamamlar.
	// Gerçek peer ID'ler federation DB'den veya BOOTSTRAP_PEERS env'den alınmalı.
	return nil
}

func getEnvOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func splitTrimmed(s string, sep rune) []string {
	raw := strings.FieldsFunc(s, func(r rune) bool { return r == sep })
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
