package main

// A4 — iki-node trustless doğrulama harness'i. GEÇİCİ, test-sonunda silinir.
//
// FİDELİTE KURALI (kullanıcı onayı): consensus/imza/federation/sequencer/p2p
// wiring'i cmd/node/main.go ile BİREBİR AYNI olmalı. Aşağıdaki blok
// main.go:163-316'nın DOĞRUDAN KOPYASI — satır satır referans yorumlarla.
// SAPMALAR (hepsi trustless-dışı, açıkça işaretli):
//   1. p2p.SetNodeProofVerifier / ZK node_proof (main.go:169-172) — ATLANDI.
//      BFT imza/konsensüs ile alakasız ayrı bir alt-sistem (peer-bağlantı
//      seviyesinde ZK-insanlık kanıtı, P2P_ZK_AUTH=false varsayılan zaten
//      kapalı, hiç devreye girmiyor).
//   2. p2p.StartHubBridge (main.go:177-183) — ATLANDI. GossipSub↔WebSocket
//      sohbet köprüsü, BFT'yle alakasız (messaging.GlobalHub bu harness'te
//      hiç yok).
//   3. token.SetOpRecorder(bftMempool.Add) (main.go:297) — ATLANDI, kullanıcı
//      onayıyla. Yerine harness kendi TEST_MEMPOOL_OP'unu doğrudan
//      bftMempool.Add() ile ekliyor — gerçek ekonomik tx YOK, mempool içeriği
//      BFT/imza doğrulamasını hiç etkilemiyor (sadece TxRoot'un neyin
//      merkle-kökü olduğunu belirliyor).
//   4. Federation/sequencer CROSS-NODE kaydı (main.go'da YOK — main.go
//      sadece KENDİ node'unu self-register ediyor, main.go:315). Bu,
//      A4 Faz 0 raporunda bulunan gerçek prod açığı (bkz. Master-Liste
//      "trustless kimlik dağıtımı" maddesi). Harness bunu dosya-tabanlı
//      elle-köprüleme ile aşıyor (aşağıda, ayrı bölüm) — federation.Register
//      ve sequencer.Global.Register GERÇEK, DEĞİŞTİRİLMEMİŞ fonksiyonlar;
//      sadece çağıranı main.go'nun HTTP handler'ı değil, bu harness.

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"obscura.network/core/internal/consensus"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/federation"
	"obscura.network/core/internal/p2p"
	"obscura.network/core/internal/sequencer"
	"obscura.network/core/internal/staking"
)

// selfConfig — bu node'un kimlik bilgileri, PEER'e dosya üzerinden aktarılır.
type selfConfig struct {
	NodeID        string                       `json:"node_id"`
	DID           string                       `json:"did"`
	P2PAddr       string                       `json:"p2p_addr"`
	VRFPubkeyHex  string                       `json:"vrf_pubkey_hex"`
	FederationReq federation.RegisterRequest   `json:"federation_req"`
}

func main() {
	label := os.Getenv("NODE_LABEL")
	dataDir := os.Getenv("DATA_DIR")
	selfID := os.Getenv("NODE_ID")
	selfDID := os.Getenv("SELF_DID")
	selfConfigPath := os.Getenv("SELF_CONFIG_PATH")
	peerConfigPath := os.Getenv("PEER_CONFIG_PATH")
	testMempoolOp := os.Getenv("TEST_MEMPOOL_OP")
	runSeconds := 60
	if v := os.Getenv("RUN_SECONDS"); v != "" {
		fmt.Sscanf(v, "%d", &runSeconds)
	}

	// ── main.go:51-53 (db.Init) — VERBATIM ─────────────────────────────────
	if err := db.Init(dataDir); err != nil {
		log.Fatalf("[%s] db.Init: %v", label, err)
	}

	// ── main.go:103-109 (federation.Init) — VERBATIM ───────────────────────
	if err := federation.Init(db.DB); err != nil {
		log.Printf("[%s] ⚠️ Federation başlatılamadı: %v", label, err)
	}

	// ── main.go:151-152 (sequencer stake lookup + epoch rotation) — VERBATIM
	sequencer.SetStakeLookup(staking.NodeOperatorStakeOBS)
	sequencer.Global.StartEpochRotation(context.Background())

	// ── main.go:166-167 (p2p config) — VERBATIM ─────────────────────────────
	p2pCfg := p2p.ConfigFromEnv()
	p2pCfg.DB = db.DB
	if !p2pCfg.Enabled {
		log.Fatalf("[%s] P2P_ENABLED=false — bu harness sadece P2P açıkken anlamlı", label)
	}

	// ── main.go:173 (p2p.Start) — VERBATIM ──────────────────────────────────
	if err := p2p.Start(context.Background(), p2pCfg); err != nil {
		log.Fatalf("[%s] p2p.Start: %v", label, err)
	}
	log.Printf("[%s] P2P hazır — SelfID=%s addrs=%v", label, p2p.SelfID(), p2p.SelfAddrs())

	// ── main.go:196-210 (selfID/quorum hesaplama) — VERBATIM ───────────────
	if selfID == "" {
		selfID = "node-1"
	}
	peerCount := 0
	if peers := strings.TrimSpace(os.Getenv("NODE_PEERS")); peers != "" {
		peerCount = len(strings.Split(peers, ","))
	}
	totalNodes := peerCount + 1
	quorum := (2*totalNodes)/3 + 1
	if quorum < 1 {
		quorum = 1
	}

	// ── main.go:216-243 (bftSignFn/bftVerifyFn) — VERBATIM ──────────────────
	bftSignFn := func(payload []byte) (string, error) {
		sig, err := p2p.SignWithIdentity(payload)
		if err != nil {
			return "", err
		}
		return hex.EncodeToString(sig), nil
	}
	bftVerifyFn := func(nodeID string, payload []byte, sigHex string) error {
		rec, err := federation.Get(nodeID)
		if err != nil || rec == nil {
			return fmt.Errorf("nodeID %q federation registry'sinde kayıtlı değil", nodeID)
		}
		pubBytes, err := hex.DecodeString(rec.Pubkey)
		if err != nil {
			return fmt.Errorf("nodeID %q pubkey hex çözülemedi: %w", nodeID, err)
		}
		sigBytes, err := hex.DecodeString(sigHex)
		if err != nil {
			return fmt.Errorf("imza hex çözülemedi: %w", err)
		}
		if len(pubBytes) != ed25519.PublicKeySize || len(sigBytes) != ed25519.SignatureSize {
			return fmt.Errorf("anahtar/imza uzunluğu geçersiz")
		}
		if !ed25519.Verify(ed25519.PublicKey(pubBytes), payload, sigBytes) {
			return fmt.Errorf("imza doğrulanamadı")
		}
		return nil
	}

	// ── main.go:245-286 (consensus.NewEngine) — VERBATIM ────────────────────
	bftEngine := consensus.NewEngine(
		selfID,
		quorum,
		func(b consensus.Block) {
			now := time.Now().UTC().Format(time.RFC3339)
			if err := consensus.SaveBlock(db.DB, b, now); err != nil {
				log.Printf("[%s] ⚠️  BFT blok kaydedilemedi (height=%d): %v", label, b.Height, err)
			}
			if err := consensus.SaveBlockOps(db.DB, b.Height, b.Ops, now); err != nil {
				log.Printf("[%s] ⚠️  BFT blok op'ları kaydedilemedi (height=%d): %v", label, b.Height, err)
			}
			log.Printf("[%s] 🧱 BFT blok commit edildi — height=%d hash=%s tx_root=%s op_sayisi=%d",
				label, b.Height, b.Hash, b.TxRoot, len(b.Ops))
		},
		p2p.Publish,
		p2p.Subscribe,
		func() string {
			if c := sequencer.Global.ActiveSequencer(); c != nil {
				return c.NodeID
			}
			return ""
		},
		func(height uint64) string {
			_, hash, err := consensus.LatestBlockHash(db.DB)
			if err != nil {
				return consensus.GenesisParentHash
			}
			return hash
		},
		bftSignFn,
		bftVerifyFn,
	)
	if err := bftEngine.Start(); err != nil {
		log.Fatalf("[%s] BFT konsensüs başlatılamadı: %v", label, err)
	}
	log.Printf("[%s] 🗳️  BFT konsensüs aktif — selfID=%s totalNodes=%d quorum=%d", label, selfID, totalNodes, quorum)

	// ── main.go:292-293 (mempool + proposer loop) — VERBATIM, token.SetOpRecorder HARİÇ (sapma #3) ──
	bftMempool := consensus.NewMempool()
	consensus.StartProposerLoop(context.Background(), bftEngine, bftMempool)

	// ── main.go:304 (VRF transport) — VERBATIM ──────────────────────────────
	if err := sequencer.SetVRFTransport(p2p.Publish, p2p.Subscribe); err != nil {
		log.Printf("[%s] ⚠️  VRF proof transport kurulamadı: %v", label, err)
	} else {
		log.Printf("[%s] 🔑 VRF proof yayını aktif — pubkey=%s", label, sequencer.VRFPublicKeyHex())
	}

	// ── main.go:701-745 (selfRegisterFederation) — VERBATIM KOPYA, aşağıda ──
	req, ok := selfRegisterFederationCopy(selfID, label)
	if !ok {
		log.Fatalf("[%s] federation self-register başarısız — harness devam edemez", label)
	}

	// ── HARNESS-ONLY GLUE (sapma #4): kendi candidate kaydı + self-config yaz ──
	sequencer.Global.Register(sequencer.SequencerCandidate{
		DID: selfDID, NodeID: selfID, Stake: 0, VRFPubkey: sequencer.VRFPublicKeyHex(),
	})
	self := selfConfig{
		NodeID: selfID, DID: selfDID, P2PAddr: p2p.SelfAddrs()[0],
		VRFPubkeyHex: sequencer.VRFPublicKeyHex(), FederationReq: req,
	}
	if selfConfigPath != "" {
		data, _ := json.MarshalIndent(self, "", "  ")
		if err := os.WriteFile(selfConfigPath, data, 0644); err != nil {
			log.Fatalf("[%s] self-config yazılamadı: %v", label, err)
		}
		log.Printf("[%s] self-config yazıldı: %s", label, selfConfigPath)
	}

	// ── HARNESS-ONLY GLUE (sapma #4): peer'i cross-register et ──────────────
	// Peer'in self-config'i henüz yazılmamış olabilir (başlatma sırası
	// önemsiz olsun diye) — dosya belirene kadar poll edilir, tek seferlik
	// zorunlu senkron bekleme YOK. federation.Register/sequencer.Global.Register
	// GERÇEK, değiştirilmemiş fonksiyonlar — imza doğrulaması dahil.
	if peerConfigPath != "" {
		go func() {
			deadline := time.Now().Add(120 * time.Second)
			for time.Now().Before(deadline) {
				data, err := os.ReadFile(peerConfigPath)
				if err != nil {
					time.Sleep(1 * time.Second)
					continue
				}
				var peer selfConfig
				if err := json.Unmarshal(data, &peer); err != nil {
					time.Sleep(1 * time.Second)
					continue
				}
				// Fatalf YOK: bu arka plan goroutine'i, ana node sürecini
				// öldürmemeli — peer-config gecikse/hiç gelmese bile node
				// (P2P+BFT) çalışmaya devam etmeli (tıpkı selfRegisterFederation'ın
				// kendisi gibi, main.go:706-708 "yumuşak geçiş" ilkesi).
				if _, err := federation.Register(peer.FederationReq); err != nil {
					log.Printf("[%s] ⚠️  peer federation.Register başarısız (GERÇEK imza doğrulaması): %v", label, err)
					return
				}
				log.Printf("[%s] peer federation kaydı GERÇEK imza doğrulamasıyla kabul edildi — peer_node_id=%s", label, peer.NodeID)
				sequencer.Global.Register(sequencer.SequencerCandidate{
					DID: peer.DID, NodeID: peer.NodeID, Stake: 0, VRFPubkey: peer.VRFPubkeyHex,
				})
				log.Printf("[%s] peer sequencer candidate olarak kaydedildi — peer_node_id=%s", label, peer.NodeID)
				return
			}
			log.Printf("[%s] ⚠️  peer-config 120s içinde belirmedi (%s)", label, peerConfigPath)
		}()
	}

	// ── HARNESS-ONLY GLUE: test-only mempool op (sapma #3) ──────────────────
	if testMempoolOp != "" {
		time.Sleep(10 * time.Second)
		bftMempool.Add(testMempoolOp)
		log.Printf("[%s] TEST mempool op eklendi: %s (gerçek tx DEĞİL, sadece ProposeBlock tetikleyici)", label, testMempoolOp)
	}

	deadline := time.Now().Add(time.Duration(runSeconds) * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(3 * time.Second)
		log.Printf("[%s] durum — PeerCount=%d height=%d", label, p2p.PeerCount(), bftEngine.Height())
	}
}

// selfRegisterFederationCopy — main.go:709-745 selfRegisterFederation'ın
// BİREBİR KOPYASI, tek fark: req'i (imzalanmış hâliyle) çağırana da
// döndürüyor (main.go'daki orijinali sadece log basıp dönüyor) — harness
// bunu SELF_CONFIG_PATH'e yazıp peer'e aktarmak için kullanıyor.
func selfRegisterFederationCopy(nodeID, label string) (federation.RegisterRequest, bool) {
	addrs := p2p.SelfAddrs()
	if len(addrs) == 0 {
		log.Printf("[%s] ⚠️  Federation self-register atlandı: P2P adresi yok", label)
		return federation.RegisterRequest{}, false
	}
	pubkeyHex, err := p2p.IdentityPubkeyHex()
	if err != nil {
		log.Printf("[%s] ⚠️  Federation self-register atlandı: pubkey alınamadı: %v", label, err)
		return federation.RegisterRequest{}, false
	}
	req := federation.RegisterRequest{
		NodeID:    nodeID,
		PeerAddr:  addrs[0],
		Pubkey:    pubkeyHex,
		Version:   "a4-harness",
		VRFPubkey: sequencer.VRFPublicKeyHex(),
		Timestamp: time.Now().UTC().Unix(),
	}
	sig, err := p2p.SignWithIdentity(federation.SignaturePayload(req))
	if err != nil {
		log.Printf("[%s] ⚠️  Federation self-register atlandı: imzalama başarısız: %v", label, err)
		return federation.RegisterRequest{}, false
	}
	req.Sig = hex.EncodeToString(sig)
	if _, err := federation.Register(req); err != nil {
		log.Printf("[%s] ⚠️  Federation self-register başarısız: %v", label, err)
		return federation.RegisterRequest{}, false
	}
	log.Printf("[%s] 📡 Federation self-register tamamlandı — node_id=%s pubkey=%s", label, nodeID, pubkeyHex)
	return req, true
}
