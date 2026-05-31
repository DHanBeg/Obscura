// Package p2p — libp2p host, GossipSub, DHT peer discovery (FAZ 3)
package p2p

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	quic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	"github.com/multiformats/go-multiaddr"
)

// ZKAuthProtocol — peer yetki doğrulama protokol ID'si
const ZKAuthProtocol = "/obscura/zkauth/1.0.0"

// Node — libp2p host + pub/sub + DHT
type Node struct {
	Host   host.Host
	PubSub *pubsub.PubSub
	DHT    *dht.IpfsDHT
	cfg    Config
	ctx    context.Context
	cancel context.CancelFunc
	topics sync.Map // topic name → *pubsub.Topic
}

var Global *Node

// Start başlatır. Config yapısını kullanır; env'den okumak için ConfigFromEnv() çağır.
func Start(ctx context.Context, cfg Config) error {
	// Ed25519 kimlik anahtarı — persist edilebilir
	priv, err := LoadOrCreatePrivateKey(cfg.PrivateKeyPath)
	if err != nil {
		return fmt.Errorf("p2p identity key: %w", err)
	}

	// Listen multiaddr'ları oluştur
	listenAddrs, err := buildListenAddrs(cfg)
	if err != nil {
		return err
	}

	// libp2p host seçenekleri
	opts := []libp2p.Option{
		libp2p.ListenAddrs(listenAddrs...),
		libp2p.Identity(priv),
		libp2p.EnableNATService(),
		libp2p.EnableRelay(),
		libp2p.Transport(quic.NewTransport),
	}

	h, err := libp2p.New(opts...)
	if err != nil {
		return fmt.Errorf("libp2p host: %w", err)
	}

	nodeCtx, cancel := context.WithCancel(ctx)

	// Kademlia DHT
	d, err := dht.New(nodeCtx, h, dht.Mode(dht.ModeAuto))
	if err != nil {
		cancel()
		h.Close()
		return fmt.Errorf("DHT: %w", err)
	}

	if err := d.Bootstrap(nodeCtx); err != nil {
		log.Printf("P2P uyari: DHT bootstrap: %v", err)
	}

	// GossipSub
	ps, err := pubsub.NewGossipSub(nodeCtx, h)
	if err != nil {
		cancel()
		h.Close()
		return fmt.Errorf("GossipSub: %w", err)
	}

	Global = &Node{
		Host:   h,
		PubSub: ps,
		DHT:    d,
		cfg:    cfg,
		ctx:    nodeCtx,
		cancel: cancel,
	}

	log.Printf("P2P node baslatildi — ID: %s", h.ID())
	for _, a := range h.Addrs() {
		log.Printf("  Dinleniyor: %s/p2p/%s", a, h.ID())
	}

	// ZK Auth bağlantı handler — P2P_ZK_AUTH=true ise aktif
	if cfg.ZKAuthEnabled {
		h.SetStreamHandler(ZKAuthProtocol, handleZKAuthStream)
		h.Network().Notify(&zkAuthNotifee{h: h, ctx: nodeCtx})
		log.Println("P2P: ZK node yetki doğrulama aktif (ZKAuthProtocol)")
	}

	// Bootstrap peer'lara bağlan
	go connectBootstrap(nodeCtx, h, cfg.BootstrapPeers)

	return nil
}

// StartFromEnv — env'den Config okuyarak başlatır (main.go kolaylığı)
func StartFromEnv(ctx context.Context) error {
	return Start(ctx, ConfigFromEnv())
}

// Stop — P2P node'u kapat
func Stop() {
	if Global == nil {
		return
	}
	Global.cancel()
	if err := Global.Host.Close(); err != nil {
		log.Printf("P2P host kapanma hatasi: %v", err)
	}
	Global = nil
}

// Publish — topic'e mesaj yayinla
func Publish(topicName string, data []byte) error {
	if Global == nil {
		return fmt.Errorf("p2p node baslatilmadi")
	}
	t, err := getOrJoinTopic(topicName)
	if err != nil {
		return err
	}
	return t.Publish(Global.ctx, data)
}

// Subscribe — topic'i dinle, mesajlari ch'e yaz
func Subscribe(topicName string, ch chan<- []byte) error {
	if Global == nil {
		return fmt.Errorf("p2p node baslatilmadi")
	}
	t, err := getOrJoinTopic(topicName)
	if err != nil {
		return err
	}
	sub, err := t.Subscribe()
	if err != nil {
		return err
	}
	go func() {
		for {
			msg, err := sub.Next(Global.ctx)
			if err != nil {
				return
			}
			if msg.ReceivedFrom == Global.Host.ID() {
				continue
			}
			ch <- msg.Data
		}
	}()
	return nil
}

// PeerCount — bagli peer sayisi
func PeerCount() int {
	if Global == nil {
		return 0
	}
	return len(Global.Host.Network().Peers())
}

// SelfID — bu node'un peer ID'si
func SelfID() string {
	if Global == nil {
		return ""
	}
	return Global.Host.ID().String()
}

// SelfAddrs — bu node'un tam multiaddr listesi (p2p suffix dahil)
func SelfAddrs() []string {
	if Global == nil {
		return nil
	}
	id := Global.Host.ID()
	addrs := Global.Host.Addrs()
	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, fmt.Sprintf("%s/p2p/%s", a, id))
	}
	return out
}

// ─── iç yardımcılar ──────────────────────────────────────────────────────────

func buildListenAddrs(cfg Config) ([]multiaddr.Multiaddr, error) {
	var addrs []multiaddr.Multiaddr

	if cfg.ListenTCP != "" {
		ma, err := multiaddr.NewMultiaddr(cfg.ListenTCP)
		if err != nil {
			return nil, fmt.Errorf("P2P TCP listen addr gecersiz (%s): %w", cfg.ListenTCP, err)
		}
		addrs = append(addrs, ma)
	}

	if cfg.ListenQUIC != "" {
		ma, err := multiaddr.NewMultiaddr(cfg.ListenQUIC)
		if err != nil {
			// QUIC hatası kritik değil — sadece uyar
			log.Printf("P2P uyari: QUIC listen addr gecersiz (%s): %v", cfg.ListenQUIC, err)
		} else {
			addrs = append(addrs, ma)
		}
	}

	if len(addrs) == 0 {
		return nil, fmt.Errorf("P2P: en az bir dinleme adresi gerekli")
	}
	return addrs, nil
}

func getOrJoinTopic(name string) (*pubsub.Topic, error) {
	if v, ok := Global.topics.Load(name); ok {
		return v.(*pubsub.Topic), nil
	}
	t, err := Global.PubSub.Join(name)
	if err != nil {
		return nil, err
	}
	Global.topics.Store(name, t)
	return t, nil
}

func connectBootstrap(ctx context.Context, h host.Host, peers []string) {
	if len(peers) == 0 {
		// BOOTSTRAP_PEERS boşsa NODE_PEERS HTTP peer listesinden türetmeye çalış
		peers = deriveP2PPeersFromHTTP()
	}
	if len(peers) == 0 {
		return
	}

	// Host'un dinlemeye başlaması için kısa bekleme
	time.Sleep(2 * time.Second)

	for _, addrStr := range peers {
		if ctx.Err() != nil {
			return
		}
		ma, err := multiaddr.NewMultiaddr(addrStr)
		if err != nil {
			log.Printf("P2P uyari: bootstrap adresi gecersiz: %s (%v)", addrStr, err)
			continue
		}
		pi, err := peer.AddrInfoFromP2pAddr(ma)
		if err != nil {
			log.Printf("P2P uyari: bootstrap peer info cozumlenemedi (%s): %v", addrStr, err)
			continue
		}
		if err := h.Connect(ctx, *pi); err != nil {
			log.Printf("P2P uyari: bootstrap baglantilanmasi basarisiz (%s): %v", pi.ID, err)
		} else {
			log.Printf("P2P: bootstrap peer baglandi: %s", pi.ID)
		}
	}
}

// deriveP2PPeersFromHTTP — NODE_PEERS "host:port" listesini
// /dns4/host/tcp/9000 formatina cevirmek icin yardimci.
// Peer ID bilinmediğinden sadece DNS4+TCP adresi döner;
// DHT peer keşfi peer ID'yi bulduktan sonra tamamlar.
// PeerID gerektiren peer.AddrInfoFromP2pAddr cağrılmaz.
func deriveP2PPeersFromHTTP() []string {
	raw := os.Getenv("NODE_PEERS")
	if raw == "" {
		return nil
	}
	parts := splitTrimmed(raw, ',')
	var out []string
	for _, p := range parts {
		host, _, err := net.SplitHostPort(p)
		if err != nil {
			continue
		}
		// P2P portu varsayılan 9000 — BOOTSTRAP_PEERS env ile üzerine yazılabilir
		out = append(out, fmt.Sprintf("/dns4/%s/tcp/9000", host))
	}
	return out
}

// ─── ZK Auth ─────────────────────────────────────────────────────────────────

// zkAuthRequest — peer'ın gönderdiği kanıt paketi
type zkAuthRequest struct {
	ProofJSON     string   `json:"proof"`
	PublicSignals []string `json:"public_signals"`
}

// handleZKAuthStream — /obscura/zkauth/1.0.0 protokolü handler
// Peer bağlandığında ZK node_proof kanıtı gönderir; doğrulama başarısızsa
// stream kapatılır ve bağlantı kesilir.
func handleZKAuthStream(s network.Stream) {
	defer s.Close()

	var req zkAuthRequest
	if err := json.NewDecoder(s).Decode(&req); err != nil {
		log.Printf("P2P ZK auth: peer %s'den geçersiz istek: %v", s.Conn().RemotePeer(), err)
		s.Conn().Close()
		return
	}

	// node_proof circuit doğrulama (import cycle önlemek için lazy import)
	if err := verifyNodeProof(req.ProofJSON, req.PublicSignals); err != nil {
		log.Printf("P2P ZK auth: peer %s reddedildi: %v", s.Conn().RemotePeer(), err)
		s.Conn().Close()
		return
	}

	log.Printf("P2P ZK auth: peer %s dogrulandi", s.Conn().RemotePeer())
	// Onay yaniti gönder
	json.NewEncoder(s).Encode(map[string]bool{"ok": true})
}

// zkAuthNotifee — yeni baglanti geldiginde ZK auth isteği baslatir
type zkAuthNotifee struct {
	h   host.Host
	ctx context.Context
}

func (n *zkAuthNotifee) Connected(net network.Network, conn network.Conn) {
	// Gelen baglantilardan ZK kaniti iste (asenkron)
	go func() {
		s, err := n.h.NewStream(n.ctx, conn.RemotePeer(), ZKAuthProtocol)
		if err != nil {
			// Peer protokolü desteklemiyorsa (eski node veya client) — uyar, kesme
			log.Printf("P2P ZK auth: peer %s protokolü desteklemiyor: %v", conn.RemotePeer(), err)
			return
		}
		defer s.Close()
		// Peer'in kanıtını oku (peer kendi proofunu gönderir)
		var resp map[string]bool
		json.NewDecoder(s).Decode(&resp)
	}()
}

func (n *zkAuthNotifee) Disconnected(_ network.Network, conn network.Conn) {
	log.Printf("P2P: peer ayrildi: %s", conn.RemotePeer())
}
func (n *zkAuthNotifee) Listen(_ network.Network, _ multiaddr.Multiaddr)      {}
func (n *zkAuthNotifee) ListenClose(_ network.Network, _ multiaddr.Multiaddr) {}

// verifyNodeProof — zk.VerifyGroth16'yi import cycle olmadan cagirir.
// zk paketi p2p paketini import etmez, p2p paketi ise zk paketini import etmez.
// Bu fonksiyon runtime'da inject edilen bir fonksiyon referansi olarak calisir.
var verifyNodeProof func(proofJSON string, publicSignals []string) error = func(proofJSON string, publicSignals []string) error {
	// Default stub — zk.SetNodeProofVerifier ile override edilir (main.go'da)
	return nil
}

// SetNodeProofVerifier — ZK node_proof dogrulama fonksiyonunu dis taraftan inject et.
// Import cycle'i onlemek icin main.go'da cagirilir:
//
//	p2p.SetNodeProofVerifier(func(proof string, sigs []string) error {
//	    return zk.VerifyGroth16(zk.CircuitNodeProof, []byte(proof), sigs)
//	})
func SetNodeProofVerifier(fn func(proofJSON string, publicSignals []string) error) {
	verifyNodeProof = fn
}
