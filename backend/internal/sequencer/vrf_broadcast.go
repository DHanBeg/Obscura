package sequencer

// vrf_broadcast.go — ADR-0017 adım 5: her node kendi VRF proof'unu yayınlar,
// diğer node'lardan gelen proof'ları toplar/doğrular. Winner hesaplama
// (vrfSelect'in yerini alacak HRW karşılaştırması) BU dosyada DEĞİL, ayrı
// bir sonraki adımda — burası sadece yayın + toplama altyapısı.
//
// consensus.Engine ile aynı desen: publishFn/subscribeFn enjekte edilir,
// bu paket p2p'ye doğrudan bağımlı değildir (bkz. consensus/bft.go NewEngine).

import (
	"encoding/json"
	"log"
)

// VRFProofTopic — tüm node'ların epoch proof'larını yayınladığı GossipSub topic.
const VRFProofTopic = "/obscura/vrf-proofs/1.0.0"

// vrfProofMsg — topic üzerinden taşınan yayın mesajı.
type vrfProofMsg struct {
	NodeID   string `json:"node_id"`
	Epoch    uint64 `json:"epoch"`
	ProofHex string `json:"proof_hex"`
}

// maxRetainedEpochs — receivedProofs'ta kaç epoch'un tutulacağı (eskiler
// budanır, sınırsız büyümesin diye — epoch'lar sonsuza kadar tekrar eder).
const maxRetainedEpochs = 3

// vrfProofStore — alınan (ve doğrulanmış) proof'ların epoch→node_id→proof
// eşlemesi. Sequencer.mu ile korunur (ayrı kilit yok, tek noktadan yönetim).
type vrfProofStore struct {
	byEpoch map[uint64]map[string]string
}

func newVRFProofStore() *vrfProofStore {
	return &vrfProofStore{byEpoch: make(map[uint64]map[string]string)}
}

// SetVRFTransport, GossipSub publish/subscribe fonksiyonlarını enjekte eder
// ve gelen proof mesajlarını dinlemeye başlar. p2p başlatılamadıysa (dev/tek-
// node modu, main.go'da zaten tolere edilen bir durum) publishFn/subscribeFn
// nil geçilebilir — bu durumda node sadece kendi proof'unu üretir/saklar,
// yayın yapmaz (tek-node quorum=1 senaryosunda zaten yeterli).
func (s *Sequencer) SetVRFTransport(
	publishFn func(topic string, data []byte) error,
	subscribeFn func(topic string, ch chan<- []byte) error,
) error {
	s.mu.Lock()
	s.vrfPublishFn = publishFn
	if s.vrfProofs == nil {
		s.vrfProofs = newVRFProofStore()
	}
	s.mu.Unlock()

	if subscribeFn == nil {
		return nil
	}
	ch := make(chan []byte, 64)
	if err := subscribeFn(VRFProofTopic, ch); err != nil {
		return err
	}
	go func() {
		for raw := range ch {
			s.handleIncomingVRFProof(raw)
		}
	}()
	return nil
}

// PublishOwnProof, verilen epoch için bu node'un VRF proof'unu üretip
// VRFProofTopic'e yayınlar. vrfPublishFn ayarlanmadıysa (tek-node/test) no-op.
// Kendi proof'unu da doğrudan yerel store'a ekler — kendi kendine gossip
// round-trip'i beklemeye gerek yok, zaten kendi anahtarımızla ürettik.
func (s *Sequencer) PublishOwnProof(epoch uint64) error {
	proofHex, _ := s.VRFProof(epoch)

	s.mu.Lock()
	if s.vrfProofs == nil {
		s.vrfProofs = newVRFProofStore()
	}
	s.storeProofLocked(epoch, s.selfID, proofHex)
	publish := s.vrfPublishFn
	s.mu.Unlock()

	if publish == nil {
		return nil
	}
	data, err := json.Marshal(vrfProofMsg{NodeID: s.selfID, Epoch: epoch, ProofHex: proofHex})
	if err != nil {
		return err
	}
	return publish(VRFProofTopic, data)
}

// handleIncomingVRFProof, gossip'ten gelen ham mesajı çözüp doğrular.
// Doğrulama İDDİA EDİLEN node_id'nin (candidates listesinden bilinen)
// vrf_pubkey'i ile yapılır — kendi vrfKey'imizle DEĞİL (bu tam olarak eski
// bug'ın tersi: her node kendi anahtarıyla DEĞİL, karşı tarafın gerçek
// public key'iyle doğrular). Geçersiz/bilinmeyen node'dan gelen proof'lar
// sessizce (log'lanarak) atılır — hatalı/kötü niyetli ağ girdisi normal
// akışı bozmamalı (BFT engine'in bozuk oy/blok'ları atma deseniyle aynı).
func (s *Sequencer) handleIncomingVRFProof(raw []byte) {
	var msg vrfProofMsg
	if err := json.Unmarshal(raw, &msg); err != nil {
		return
	}
	if msg.NodeID == "" || msg.NodeID == s.selfID {
		return // kendi yayınımızı yoksay (PublishOwnProof zaten yerel ekledi)
	}

	pubkey, ok := s.vrfPubkeyForNode(msg.NodeID)
	if !ok || pubkey == "" {
		log.Printf("[sequencer] VRF proof reddedildi: %s için bilinen vrf_pubkey yok", msg.NodeID)
		return
	}

	valid, _ := VerifyVRFProof(pubkey, msg.Epoch, msg.ProofHex)
	if !valid {
		log.Printf("[sequencer] VRF proof doğrulanamadı: node=%s epoch=%d", msg.NodeID, msg.Epoch)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.vrfProofs == nil {
		s.vrfProofs = newVRFProofStore()
	}
	s.storeProofLocked(msg.Epoch, msg.NodeID, msg.ProofHex)
}

// storeProofLocked — s.mu WRITE LOCK altında çağrılmalı. Kaydeder + eski
// epoch'ları budar (maxRetainedEpochs'tan eskiler silinir).
func (s *Sequencer) storeProofLocked(epoch uint64, nodeID, proofHex string) {
	if s.vrfProofs.byEpoch[epoch] == nil {
		s.vrfProofs.byEpoch[epoch] = make(map[string]string)
	}
	s.vrfProofs.byEpoch[epoch][nodeID] = proofHex

	if epoch <= maxRetainedEpochs {
		return
	}
	for e := range s.vrfProofs.byEpoch {
		if e < epoch-maxRetainedEpochs {
			delete(s.vrfProofs.byEpoch, e)
		}
	}
}

// vrfPubkeyForNode, mevcut aday listesinde (currentNodes()) verilen
// node_id'nin vrf_pubkey'ini arar.
func (s *Sequencer) vrfPubkeyForNode(nodeID string) (string, bool) {
	for _, n := range s.currentNodes() {
		if n.NodeID == nodeID {
			return n.VRFPubkey, n.VRFPubkey != ""
		}
	}
	return "", false
}

// ProofsForEpoch, verilen epoch için ŞU ANA KADAR toplanan (doğrulanmış)
// proof'ların bir kopyasını döner (node_id → proof_hex). Bir sonraki adımda
// (vrfSelect'in yerini alacak HRW karşılaştırması) bu fonksiyon kazananı
// hesaplamak için kullanılacak.
func (s *Sequencer) ProofsForEpoch(epoch uint64) map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.vrfProofs == nil {
		return nil
	}
	src := s.vrfProofs.byEpoch[epoch]
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}
