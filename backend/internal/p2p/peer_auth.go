// Package p2p — ZK kanıt tabanlı peer yetki doğrulaması (Spec Bölüm 3.3)
//
// Peer bağlandığında bu modül:
//  1. Rastgele challenge üretir
//  2. Karşı tarafa challenge gönderir
//  3. Peer'dan identity_proof ZK kanıtı alır
//  4. zk.VerifyGroth16 ile kanıtı doğrular
//  5. Başarısızsa bağlantıyı keser
//
// İmport döngüsünü önlemek için gerçek ZK doğrulama fonksiyonu
// host.go'daki SetNodeProofVerifier ile inject edilir.
package p2p

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	// PeerAuthProtocol — ZK challenge/response protokol stream ID'si
	PeerAuthProtocol = "/obscura/peerauth/1.0.0"

	// authTimeout — challenge/response turu için maksimum bekleme süresi
	authTimeout = 15 * time.Second

	// challengeSize — rastgele challenge bayt sayısı (128 bit entropi)
	challengeSize = 16
)

// PeerAuthChallenge sunucunun peer'a gönderdiği challenge paketi.
// NodeID alanı bu node'un kimliğini bildirir; Timestamp replay koruması sağlar.
type PeerAuthChallenge struct {
	NodeID    string `json:"node_id"`
	Challenge string `json:"challenge"` // hex-encoded random bytes
	Timestamp int64  `json:"timestamp"`
	Signature string `json:"signature"` // TODO: Ed25519 imzası ile challenge'ı imzala
}

// PeerAuthResponse peer'ın challenge'a döndürdüğü yanıt paketi.
// identity_proof ZK kanıtı ve public signals içerir.
type PeerAuthResponse struct {
	// ProofJSON: snarkjs Groth16 proof JSON (doğrudan JSON — base64 değil)
	ProofJSON string `json:"proof"`
	// PublicSignals: kanıtın açık girdileri/çıktıları
	PublicSignals []string `json:"public_signals"`
	// Challenge: sunucunun gönderdiği challenge değerinin echo'su (replay guard)
	Challenge string `json:"challenge"`
}

// PeerAuthResult doğrulama sonucunu özetler.
type PeerAuthResult struct {
	PeerID    peer.ID
	Verified  bool
	Challenge string
	At        time.Time
	Err       error
}

// AuthenticatePeer uzak bir peer'dan ZK identity_proof kanıtı talep eder ve doğrular.
//
// Adımlar:
//  1. Peer'a /obscura/peerauth/1.0.0 stream açar
//  2. PeerAuthChallenge gönderir
//  3. PeerAuthResponse okur
//  4. verifyNodeProof (inject edilmiş fonksiyon) ile kanıtı doğrular
//  5. Başarısızsa peer bağlantısı kesilir ve hata döner
//
// ZK kanıt doğrulama yoksa (verifyNodeProof stub olarak bırakılmışsa)
// bu fonksiyon her peer'ı başarılı kabul eder — prod'da mutlaka inject edilmeli.
func AuthenticatePeer(ctx context.Context, peerID peer.ID) error {
	if Global == nil {
		return fmt.Errorf("p2p node başlatılmadı")
	}

	authCtx, cancel := context.WithTimeout(ctx, authTimeout)
	defer cancel()

	// Stream aç
	s, err := Global.Host.NewStream(authCtx, peerID, PeerAuthProtocol)
	if err != nil {
		return fmt.Errorf("peerauth stream açılamadı (%s): %w", peerID, err)
	}
	defer s.Close()

	// Deadline ayarla
	if err := s.SetDeadline(time.Now().Add(authTimeout)); err != nil {
		return fmt.Errorf("stream deadline: %w", err)
	}

	// Challenge üret
	challenge, err := generateChallenge()
	if err != nil {
		return fmt.Errorf("challenge üretimi: %w", err)
	}

	// Challenge gönder
	authChallenge := PeerAuthChallenge{
		NodeID:    Global.Host.ID().String(),
		Challenge: challenge,
		Timestamp: time.Now().UnixMilli(),
	}
	if err := json.NewEncoder(s).Encode(authChallenge); err != nil {
		return fmt.Errorf("challenge gönderilemedi (%s): %w", peerID, err)
	}

	// Yanıt oku
	var resp PeerAuthResponse
	if err := json.NewDecoder(s).Decode(&resp); err != nil {
		return fmt.Errorf("peerauth yanıt okunamadı (%s): %w", peerID, err)
	}

	// Challenge echo doğrula (replay protection)
	if resp.Challenge != challenge {
		disconnectPeer(peerID, "challenge uyuşmazlığı")
		return fmt.Errorf("peer %s geçersiz challenge echo döndürdü", peerID)
	}

	// ZK kanıtı doğrula
	if err := verifyNodeProof(resp.ProofJSON, resp.PublicSignals); err != nil {
		disconnectPeer(peerID, fmt.Sprintf("ZK doğrulama başarısız: %v", err))
		return fmt.Errorf("peer %s ZK kanıtı reddedildi: %w", peerID, err)
	}

	log.Printf("P2P peerauth: peer %s doğrulandı", peerID)
	return nil
}

// HandlePeerAuthStream gelen /obscura/peerauth/1.0.0 stream'ini işler.
// Bu node sunucu tarafındadır: karşı tarafın challenge'ını okur,
// kendi ZK kanıtını üretir (TODO) ve yanıt gönderir.
//
// Şu anda kendi proof'unu üretemiyor — stub yanıt gönderir.
// FAZ 3'te: node kendi node_proof kanıtını local snarkjs ile üretecek.
func HandlePeerAuthStream(s network.Stream) {
	defer s.Close()

	if err := s.SetDeadline(time.Now().Add(authTimeout)); err != nil {
		log.Printf("P2P peerauth: deadline ayarlanamadı: %v", err)
		return
	}

	// Challenge oku
	var challenge PeerAuthChallenge
	if err := json.NewDecoder(s).Decode(&challenge); err != nil {
		log.Printf("P2P peerauth: challenge okunamadı (%s): %v", s.Conn().RemotePeer(), err)
		s.Conn().Close()
		return
	}

	// Timestamp kontrolü — 30 saniyeden eski challenge'ları reddet (replay guard)
	age := time.Now().UnixMilli() - challenge.Timestamp
	if age < 0 || age > 30_000 {
		log.Printf("P2P peerauth: eski challenge (%dms) peer %s'den — reddedildi", age, s.Conn().RemotePeer())
		s.Conn().Close()
		return
	}

	// Yanıt gönder — kendi ZK kanıtımızı ekle.
	// TODO (FAZ 3): node_proof.circom ile gerçek kanıt üret.
	// Şu an stub: verifyNodeProof default stub'ı kabul eder.
	resp := PeerAuthResponse{
		ProofJSON:     `{"pi_a":[],"pi_b":[],"pi_c":[],"protocol":"groth16","curve":"bn128"}`,
		PublicSignals: []string{"1"},
		Challenge:     challenge.Challenge,
	}
	if err := json.NewEncoder(s).Encode(resp); err != nil {
		log.Printf("P2P peerauth: yanıt gönderilemedi (%s): %v", s.Conn().RemotePeer(), err)
	}
}

// PeerAuthMiddleware stream handler'ı ZK auth ile sarmalar.
// Peer doğrulanmadan stream işleme başlamaz.
// Kullanım:
//
//	h.SetStreamHandler("/obscura/myproto/1.0.0",
//	    p2p.PeerAuthMiddleware(ctx, myHandler))
func PeerAuthMiddleware(ctx context.Context, next network.StreamHandler) network.StreamHandler {
	return func(s network.Stream) {
		peerID := s.Conn().RemotePeer()

		// Auth sadece ZKAuthEnabled ise zorlanır
		if Global != nil && Global.cfg.ZKAuthEnabled {
			if err := AuthenticatePeer(ctx, peerID); err != nil {
				log.Printf("P2P middleware: peer %s auth başarısız — stream reddedildi: %v", peerID, err)
				s.Reset()
				return
			}
		}

		next(s)
	}
}

// ─── iç yardımcılar ──────────────────────────────────────────────────────────

// generateChallenge kriptografik olarak güvenli rastgele challenge üretir.
func generateChallenge() (string, error) {
	b := make([]byte, challengeSize)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// disconnectPeer peer'ı DHT'den çıkarır ve ağ bağlantısını kapatır.
func disconnectPeer(peerID peer.ID, reason string) {
	log.Printf("P2P peerauth: peer %s bağlantısı kesiliyor — %s", peerID, reason)
	if Global == nil {
		return
	}

	// DHT'den peer bilgisini temizle
	if Global.DHT != nil {
		Global.DHT.RoutingTable().RemovePeer(peerID)
	}

	// Tüm açık bağlantıları kapat
	if err := Global.Host.Network().ClosePeer(peerID); err != nil {
		log.Printf("P2P peerauth: peer %s kapatılamadı: %v", peerID, err)
	}
}
