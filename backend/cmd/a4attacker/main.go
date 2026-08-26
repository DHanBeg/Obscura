package main

// A4 Faz 2 — kötü node simülatörü. GEÇİCİ, test aracı.
//
// GERÇEK bir P2P kimliğiyle ağa katılır (p2p.Start, real GossipSub) ama
// consensus.Engine YOK — BFT motorunu hiç çalıştırmaz, sadece elle
// kurulmuş bozuk ConsensusMsg'leri doğrudan p2p.Publish ile
// "obscura/consensus/v1" topic'ine yayınlar. Amaç: dürüst node'un (gerçek
// A3 verifyFn + A5 self-vote fix'iyle çalışan cmd/a4harness_main.go)
// bunları GERÇEKTEN reddettiğini kanıtlamak — mock değil, canlı ağ.
//
// 3 saldırı modu (ATTACK_MODE):
//   - garbage_vote:   Vote, NodeID=IMPERSONATE, Sig=rastgele/sahte bytes
//                      (format doğru uzunlukta ama imza değil)
//   - wrongkey_block: Block, Proposer=IMPERSONATE, Sig=SALDIRGANIN KENDİ
//                      gerçek anahtarıyla üretilmiş (impersonation —
//                      format/uzunluk geçerli, ama YANLIŞ kimlik)
//   - wrongkey_vote:  Vote, NodeID=IMPERSONATE, Sig=SALDIRGANIN KENDİ
//                      gerçek anahtarıyla üretilmiş (aynı, oy yolu için)
//
// Payload formatları consensus/bft.go'daki voteSigningPayload (satır
// ~257-259, unexported — burada birebir aynı format taklit edilir, YORUM
// olarak referans verilir) ve Block.Sig'in Hash string'i üzerinden
// imzalanması (ProposeBlock, bkz. bft.go) ile UYUMLU tutulur — saldırının
// "format hatası"ndan değil GERÇEKTEN imza doğrulamasından reddedildiğini
// kanıtlamak için.
import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"obscura.network/core/internal/consensus"
	"obscura.network/core/internal/p2p"
)

func main() {
	label := os.Getenv("NODE_LABEL")
	dataDir := os.Getenv("DATA_DIR")
	impersonate := os.Getenv("IMPERSONATE_NODE_ID")
	attackMode := os.Getenv("ATTACK_MODE")
	targetHeightStr := os.Getenv("TARGET_HEIGHT")
	var targetHeight uint64 = 1
	fmt.Sscanf(targetHeightStr, "%d", &targetHeight)

	_ = dataDir // attacker consensus/federation DB'ye ihtiyaç duymaz — cfg.DB nil bırakılır

	cfg := p2p.ConfigFromEnv()
	if !cfg.Enabled {
		log.Fatalf("[%s] P2P_ENABLED=false — saldırgan P2P olmadan anlamsız", label)
	}
	if err := p2p.Start(context.Background(), cfg); err != nil {
		log.Fatalf("[%s] p2p.Start: %v", label, err)
	}
	log.Printf("[%s] SALDIRGAN P2P kimliği: %s (federation'a KAYITLI DEĞİL — gerekmez, kurbanın verifyFn'i IMPERSONATE_NODE_ID'nin KENDİ pubkey'ine bakar)", label, p2p.SelfID())

	// Bağlantı kurulması için kısa bekleme.
	time.Sleep(8 * time.Second)
	log.Printf("[%s] PeerCount=%d — saldırı başlıyor (mode=%s, impersonate=%s, height=%d)",
		label, p2p.PeerCount(), attackMode, impersonate, targetHeight)

	switch attackMode {
	case "garbage_vote":
		v := consensus.Vote{
			Phase: consensus.PhasePrevote, Height: targetHeight, Round: 0,
			BlockHash: "attacker-fake-block-hash", NodeID: impersonate,
			Sig: hex.EncodeToString(make([]byte, ed25519.SignatureSize)), // format doğru, içerik SAHTE (sıfır byte)
		}
		publishVote(label, v)

	case "wrongkey_block":
		fakeHash := "attacker-fake-proposal-hash-" + time.Now().Format(time.RFC3339Nano)
		// SALDIRGANIN KENDİ gerçek anahtarıyla imzalıyor — ama Proposer
		// alanında BAŞKA bir node'un kimliğini iddia ediyor (impersonation).
		sig, err := p2p.SignWithIdentity([]byte(fakeHash))
		if err != nil {
			log.Fatalf("[%s] kendi imzam üretilemedi: %v", label, err)
		}
		b := &consensus.Block{
			Height: targetHeight, Round: 0, ParentHash: consensus.GenesisParentHash,
			TxRoot: "attacker-fake-txroot", Proposer: impersonate,
			Timestamp: time.Now().UnixMilli(), Hash: fakeHash,
			Sig: hex.EncodeToString(sig),
		}
		raw, _ := json.Marshal(consensus.ConsensusMsg{Type: "block_proposal", Block: b})
		if err := p2p.Publish(consensus.TopicConsensus, raw); err != nil {
			log.Printf("[%s] ⚠️  publish hatası (peer yoksa beklenir): %v", label, err)
		}
		log.Printf("[%s] 🎭 SAHTE PROPOSER BLOĞU yayınlandı — Proposer=%q (taklit), gerçek imza SALDIRGANIN kendi anahtarıyla, height=%d hash=%s",
			label, impersonate, targetHeight, fakeHash)

	case "wrongkey_vote":
		v := consensus.Vote{
			Phase: consensus.PhasePrevote, Height: targetHeight, Round: 0,
			BlockHash: "attacker-fake-block-hash", NodeID: impersonate,
		}
		// consensus.voteSigningPayload (bft.go:257-259) ile BİREBİR AYNI format —
		// unexported olduğu için burada taklit ediliyor, referans o satırlar.
		payload := []byte(fmt.Sprintf("%s|%d|%d|%s|%s", v.Phase, v.Height, v.Round, v.BlockHash, v.NodeID))
		sig, err := p2p.SignWithIdentity(payload) // SALDIRGANIN KENDİ gerçek anahtarı
		if err != nil {
			log.Fatalf("[%s] kendi imzam üretilemedi: %v", label, err)
		}
		v.Sig = hex.EncodeToString(sig)
		publishVote(label, v)

	default:
		log.Fatalf("[%s] bilinmeyen ATTACK_MODE: %s", label, attackMode)
	}

	time.Sleep(5 * time.Second)
	log.Printf("[%s] saldırgan çıkıyor", label)
}

func publishVote(label string, v consensus.Vote) {
	raw, _ := json.Marshal(consensus.ConsensusMsg{Type: "vote", Vote: &v})
	if err := p2p.Publish(consensus.TopicConsensus, raw); err != nil {
		log.Printf("[%s] ⚠️  publish hatası (peer yoksa beklenir): %v", label, err)
	}
	log.Printf("[%s] 🎭 SAHTE OY yayınlandı — NodeID=%q (taklit) phase=%s height=%d sig=%s...",
		label, v.NodeID, v.Phase, v.Height, v.Sig[:16])
}
