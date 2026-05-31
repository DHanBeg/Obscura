// Package p2p — bootstrap peer discovery (Spec Bölüm 3.3)
//
// DiscoverBootstrapPeers aşağıdaki öncelik sırası ile peer adresleri toplar:
//  1. Hardcoded peer listesi (NODE_PEERS / BOOTSTRAP_PEERS env)
//  2. DNS TXT lookup: bootstrap.obscura.network
//  3. ENS stub: resolveENS("obscura.eth")
//  4. SQLite peer_cache: son bağlanılan peer'lar
//
// Her kaynak başarılı peer katkısında durmaz; tüm kaynaklar tüketilir
// ve tekilleştirilmiş bir liste döner. Bu sayede node çeşitli arıza
// senaryolarında (DNS arızası, ENS yokluğu) diğer kaynaklardan beslenir.
package p2p

import (
	"context"
	"database/sql"
	"log"
	"net"
	"os"

	"github.com/multiformats/go-multiaddr"
)

// DiscoveryConfig — peer discovery yapılandırması
type DiscoveryConfig struct {
	// HardcodedPeers: BOOTSTRAP_PEERS veya NODE_PEERS'den türetilen adresler.
	// multiaddr formatında olmak zorunda değil — ham host:port kabul edilir
	// ve /dns4/host/tcp/port formatına dönüştürülür.
	HardcodedPeers []string

	// DNSBootstrapHost: TXT kaydı sorgulanan domain.
	// Boşsa "bootstrap.obscura.network" kullanılır.
	DNSBootstrapHost string

	// ENSName: çözümlenecek ENS ismi. Boşsa "obscura.eth" kullanılır.
	ENSName string

	// DB: peer_cache için SQLite bağlantısı. nil ise cache devre dışı.
	DB *sql.DB
}

// DiscoverBootstrapPeers dört kaynaktan peer adreslerini toplar.
// Hata döndürmez — her kaynak başarısız olursa uyarı loglanır ve
// bir sonraki kaynağa geçilir. Sonuç listesi boş olabilir.
func DiscoverBootstrapPeers(ctx context.Context, cfg DiscoveryConfig) ([]multiaddr.Multiaddr, error) {
	// peer_cache migration'ını tetikle (idempotent)
	if cfg.DB != nil {
		if err := AddPeerCacheMigration(cfg.DB); err != nil {
			log.Printf("P2P discovery: peer_cache migration hatası: %v", err)
		}
	}

	seen := make(map[string]struct{})
	var results []multiaddr.Multiaddr

	addAddr := func(source, addrStr string) {
		if _, dup := seen[addrStr]; dup {
			return
		}
		ma, err := multiaddr.NewMultiaddr(addrStr)
		if err != nil {
			log.Printf("P2P discovery [%s]: geçersiz multiaddr %q: %v", source, addrStr, err)
			return
		}
		seen[addrStr] = struct{}{}
		results = append(results, ma)
		log.Printf("P2P discovery [%s]: %s", source, addrStr)
	}

	// ── 1. Hardcoded / env peer listesi ──────────────────────────────────────
	for _, p := range cfg.HardcodedPeers {
		addAddr("env", p)
	}

	// NODE_PEERS env'den türetilen HTTP peer'ları P2P adrese çevir
	for _, derived := range deriveP2PPeersFromHTTP() {
		addAddr("NODE_PEERS", derived)
	}

	// ── 2. DNS TXT lookup ────────────────────────────────────────────────────
	bootstrapHost := cfg.DNSBootstrapHost
	if bootstrapHost == "" {
		bootstrapHost = "bootstrap.obscura.network"
	}
	dnsPeers := resolveDNSTXT(ctx, bootstrapHost)
	for _, p := range dnsPeers {
		addAddr("DNS-TXT", p)
	}

	// ── 3. ENS stub ──────────────────────────────────────────────────────────
	ensName := cfg.ENSName
	if ensName == "" {
		ensName = "obscura.eth"
	}
	if ensAddr, err := resolveENS(ensName); err != nil {
		log.Printf("P2P discovery [ENS]: %v", err)
	} else {
		addAddr("ENS", ensAddr)
	}

	// ── 4. SQLite peer_cache ─────────────────────────────────────────────────
	if cfg.DB != nil {
		cached := LoadPeers(cfg.DB)
		for _, p := range cached {
			addAddr("peer_cache", p)
		}
	}

	log.Printf("P2P discovery: toplam %d bootstrap adresi bulundu", len(results))
	return results, nil
}

// resolveDNSTXT verilen domain'e ait TXT kayıtlarını sorgular.
// Her TXT kaydının tek bir multiaddr stringi içerdiği varsayılır.
// Ağ erişimi yoksa veya kayıt yoksa boş liste döner.
//
// Beklenen TXT kayıt formatı:
//
//	bootstrap.obscura.network. TXT "/ip4/1.2.3.4/tcp/9000/p2p/Qm..."
//	bootstrap.obscura.network. TXT "/dns4/node-1.obscura.network/tcp/9000"
func resolveDNSTXT(ctx context.Context, domain string) []string {
	// net.Resolver context-aware sorgu için kullanılır
	resolver := &net.Resolver{}
	txts, err := resolver.LookupTXT(ctx, domain)
	if err != nil {
		// DNS erişimi yoksa veya kayıt yoksa sessizce devam et
		if !os.IsTimeout(err) {
			log.Printf("P2P discovery [DNS-TXT]: %s sorgusu başarısız: %v", domain, err)
		}
		return nil
	}

	var addrs []string
	for _, txt := range txts {
		if txt != "" {
			addrs = append(addrs, txt)
		}
	}
	return addrs
}
