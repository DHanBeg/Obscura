// Package p2p — config yapısı ve env'den yükleme
package p2p

import (
	"os"
	"strings"

	"obscura.network/core/internal/dbi"
)

// Config — libp2p host yapılandırması
type Config struct {
	// Enabled: false ise P2P host başlatılmaz, HTTP gossip devam eder
	Enabled bool

	// ListenQUIC: QUIC multiaddr, örn. /ip4/0.0.0.0/udp/9001/quic-v1
	// Tek transport bu — TCP kasıtlı olarak yok (bkz. ADR: TCP hiç register
	// edilmemişti, sessizce bind olmuyordu; yarım TCP yerine tek gerçek yol
	// olan QUIC'e sadeleştirildi).
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

	// DB: peer discovery (DiscoverBootstrapPeers) ve peer_cache (SavePeer)
	// için SQLite bağlantısı. nil ise cache kaynak olarak devre dışı kalır,
	// başarılı bağlantılar kaydedilmez — ConfigFromEnv bunu set ETMEZ,
	// çağıran main.go'da db.DB atamalıdır.
	DB dbi.Querier
}

// ConfigFromEnv env değişkenlerinden Config üretir.
//
// Env değişkenleri:
//
//	P2P_ENABLED       — default "true" (false yazmak devre dışı bırakır)
//	P2P_LISTEN_QUIC   — default /ip4/0.0.0.0/udp/9001/quic-v1
//	BOOTSTRAP_PEERS   — virgülle ayrılmış, PEER ID İÇEREN multiaddr listesi
//	                    (örn. /dns4/node-1/udp/9001/quic-v1/p2p/<ID>). Otomatik
//	                    türetme YOK — ilk node ayağa kalkıp log'a kendi peer
//	                    ID'sini basmadan diğer node'lar ona bootstrap OLAMAZ.
//	P2P_ZK_AUTH       — default "false"
//	NODE_ID           — node kimliği
//	P2P_KEY_PATH      — identity key dosya yolu (boş = in-memory)
func ConfigFromEnv() Config {
	return Config{
		Enabled:        os.Getenv("P2P_ENABLED") != "false",
		ListenQUIC:     getEnvOr("P2P_LISTEN_QUIC", "/ip4/0.0.0.0/udp/9001/quic-v1"),
		BootstrapPeers: parseBootstrapPeers(),
		ZKAuthEnabled:  os.Getenv("P2P_ZK_AUTH") == "true",
		NodeID:         os.Getenv("NODE_ID"),
		PrivateKeyPath: os.Getenv("P2P_KEY_PATH"),
	}
}

// parseBootstrapPeers BOOTSTRAP_PEERS env'den peer adreslerini okur.
// Otomatik NODE_PEERS türetme YOK (kaldırıldı — peer ID içermediği için
// bootstrap her zaman başarısız oluyordu, bkz. host.go connectBootstrap).
// Boş dönerse node bootstrapsız başlar; DHT/gelen bağlantılarla zamanla
// başka peer keşfedebilir ama garantili değildir.
func parseBootstrapPeers() []string {
	raw := os.Getenv("BOOTSTRAP_PEERS")
	if raw == "" {
		return nil
	}
	return splitTrimmed(raw, ',')
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
