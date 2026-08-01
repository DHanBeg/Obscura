// Package consensus — Byzantine Fault Tolerant consensus (Tendermint-style, FAZ 3)
//
// 3f+1 node gerektirir, f adet Bizans hatası tolere eder.
// Round: Propose → Prevote → Precommit → Commit
// Bu implementasyon single-node veya multi-node modda çalışır.
// libp2p GossipSub üzerinden mesaj taşır (p2p.Publish/Subscribe).
package consensus

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"obscura.network/core/internal/sequencer"
)

// ─── Tipler ──────────────────────────────────────────────────────────────────

type Phase string

const (
	PhasePropose   Phase = "PROPOSE"
	PhasePrevote   Phase = "PREVOTE"
	PhasePrecommit Phase = "PRECOMMIT"
	PhaseCommit    Phase = "COMMIT"
)

const (
	TopicConsensus = "obscura/consensus/v1"
	RoundTimeout   = 8 * time.Second
)

// Block — konsensüs bloğu
type Block struct {
	Height     uint64 `json:"height"`
	Round      uint32 `json:"round"`
	ParentHash string `json:"parent_hash"`
	TxRoot     string `json:"tx_root"`  // merkle root of transactions
	Proposer   string `json:"proposer"` // peer ID
	Timestamp  int64  `json:"timestamp"`
	Hash       string `json:"hash"`
	// Ops, bu bloğa dahil edilen ledger operasyon ID'lerinin ham listesi
	// (bkz. ADIM 7 / ADR-0017 "sonradan-tasdik" deseni). TxRoot bunun Merkle
	// kökü — alıcı taraf handleMsg'de TxRoot'un Ops'tan üretilebildiğini
	// doğrular (bkz. aşağıda). onCommit bu listeyi audit-log'a yazar,
	// BAKİYEYE DOKUNMAZ — token.Transfer/Mint zaten senkron uygulanmıştır.
	Ops []string `json:"ops,omitempty"`
}

// Vote — bir node'un oyu
type Vote struct {
	Phase     Phase  `json:"phase"`
	Height    uint64 `json:"height"`
	Round     uint32 `json:"round"`
	BlockHash string `json:"block_hash"`
	NodeID    string `json:"node_id"`
	Sig       string `json:"sig"` // Ed25519 (stub)
}

// ConsensusMsg — GossipSub mesajı
type ConsensusMsg struct {
	Type  string `json:"type"` // "block_proposal" | "vote"
	Block *Block `json:"block,omitempty"`
	Vote  *Vote  `json:"vote,omitempty"`
}

// CommitCallback — blok commit olduğunda çağrılır
type CommitCallback func(block Block)

// ─── Engine ──────────────────────────────────────────────────────────────────

// Engine — BFT konsensüs motoru
type Engine struct {
	mu           sync.Mutex
	selfID       string
	height       uint64
	round        uint32
	phase        Phase
	currentBlock *Block
	prevotes     map[string]Vote
	precommits   map[string]Vote
	quorum       int // 2f+1
	onCommit     CommitCallback

	// proposerFn, mevcut round için hangi node'un blok önermeye yetkili
	// olduğunu döndürür (node ID). nil ise (test/geriye uyumluluk) proposer
	// kontrolü ATLANIR — herkesin önerisi kabul edilir (ADIM 1 öncesi davranış).
	// Üretimde main.go bunu sequencer.Global.ActiveSequencer()'a bağlar
	// (bkz. ADR-0017) — Engine kendi leader-election'ını YAPMAZ.
	proposerFn func() string

	// parentHashFn, height H için önerilecek bloğun parent hash'ini döner
	// (height-1'in gerçek commit edilmiş hash'i, consensus_blocks tablosundan
	// — bkz. store.go, ADIM 6). nil ise (test/geriye uyumluluk) eski sahte
	// "parent_%d" placeholder davranışına düşer.
	parentHashFn func(height uint64) string

	publishFn   func(topic string, data []byte) error
	subscribeFn func(topic string, ch chan<- []byte) error
	msgCh       chan []byte
}

// NewEngine — yeni konsensüs motoru
// quorum: gerekli oy sayısı (tipik olarak 2f+1 = ceil(2/3 * N) + 1)
// proposerFn: mevcut proposer'ın node ID'sini döndürür (bkz. ADR-0017 —
// sequencer.Global.ActiveSequencer() üzerine kurulu). nil geçilebilir
// (proposer kontrolü atlanır — testler için).
// parentHashFn: height için parent hash döndürür (bkz. store.go). nil
// geçilebilir (eski sahte placeholder davranışına düşer — testler için).
func NewEngine(
	selfID string,
	quorum int,
	onCommit CommitCallback,
	publishFn func(string, []byte) error,
	subscribeFn func(string, chan<- []byte) error,
	proposerFn func() string,
	parentHashFn func(height uint64) string,
) *Engine {
	return &Engine{
		selfID:       selfID,
		height:       1,
		phase:        PhasePropose,
		quorum:       quorum,
		onCommit:     onCommit,
		publishFn:    publishFn,
		subscribeFn:  subscribeFn,
		proposerFn:   proposerFn,
		parentHashFn: parentHashFn,
		msgCh:        make(chan []byte, 256),
		prevotes:     make(map[string]Vote),
		precommits:   make(map[string]Vote),
	}
}

// IsProposer — bu node, proposerFn'e göre mevcut round'un öneriyi yapması
// gereken node'u mu? proposerFn nil ise (kontrol atlanıyorsa) true döner —
// eski (kontrolsüz) davranışla geriye uyumlu.
func (e *Engine) IsProposer() bool {
	if e.proposerFn == nil {
		return true
	}
	return e.proposerFn() == e.selfID
}

// Start — arka planda konsensüs döngüsü başlat
func (e *Engine) Start() error {
	if err := e.subscribeFn(TopicConsensus, e.msgCh); err != nil {
		return fmt.Errorf("consensus subscribe: %w", err)
	}
	go e.loop()
	log.Printf("🗳️  BFT konsensüs başladı — quorum=%d, height=%d", e.quorum, e.height)
	return nil
}

func (e *Engine) loop() {
	ticker := time.NewTicker(RoundTimeout)
	for {
		select {
		case raw := <-e.msgCh:
			e.handleMsg(raw)
		case <-ticker.C:
			e.mu.Lock()
			e.advanceRound()
			e.mu.Unlock()
		}
	}
}

// ProposeBlock — yeni blok öner. ops, bu bloğa dahil edilecek ledger
// operasyon ID'lerinin ham listesi (bkz. ADIM 7) — txRoot buradan
// (sequencer.ComputeMerkleRoot ile) hesaplanır, ayrıca parametre alınmaz.
// Çağıran bu round'un proposer'ı değilse hata döner (bkz. proposerFn/
// ADR-0017) — savunma amaçlı ikinci kontrol, asıl kontrol handleMsg'de
// karşı taraf mesajları için yapılıyor.
func (e *Engine) ProposeBlock(ops []string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.proposerFn != nil {
		if got := e.proposerFn(); got != e.selfID {
			return fmt.Errorf("bu node proposer değil (height=%d): beklenen=%s, self=%s", e.height, got, e.selfID)
		}
	}

	b := &Block{
		Height:     e.height,
		Round:      e.round,
		ParentHash: e.parentHash(),
		TxRoot:     sequencer.ComputeMerkleRoot(ops),
		Proposer:   e.selfID,
		Timestamp:  time.Now().UnixMilli(),
		Ops:        ops,
	}
	b.Hash = blockHash(b)
	e.currentBlock = b

	msg := ConsensusMsg{Type: "block_proposal", Block: b}
	return e.broadcast(msg)
}

func (e *Engine) handleMsg(raw []byte) {
	var msg ConsensusMsg
	if err := json.Unmarshal(raw, &msg); err != nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	switch msg.Type {
	case "block_proposal":
		if msg.Block == nil || msg.Block.Height != e.height {
			return
		}
		if e.proposerFn != nil && msg.Block.Proposer != e.proposerFn() {
			log.Printf("⚠️  BFT: yetkisiz proposer'dan blok reddedildi — height=%d proposer=%s beklenen=%s",
				e.height, msg.Block.Proposer, e.proposerFn())
			return
		}
		// Bütünlük kontrolü (ADIM 7): TxRoot, verilen Ops listesinden gerçekten
		// üretilebiliyor mu? Proposer Ops'u değiştirip TxRoot'u eski/sahte
		// bırakamaz (ya da tersi) — tutmuyorsa blok reddedilir.
		if got := sequencer.ComputeMerkleRoot(msg.Block.Ops); got != msg.Block.TxRoot {
			log.Printf("⚠️  BFT: TxRoot Ops listesiyle uyuşmuyor, blok reddedildi — height=%d got=%s want=%s",
				e.height, got, msg.Block.TxRoot)
			return
		}
		e.currentBlock = msg.Block
		e.phase = PhasePrevote
		// Otomatik prevote (basit: her geçerli bloğa oy ver)
		_ = e.broadcast(ConsensusMsg{Type: "vote", Vote: &Vote{
			Phase: PhasePrevote, Height: e.height, Round: e.round,
			BlockHash: msg.Block.Hash, NodeID: e.selfID,
		}})

	case "vote":
		if msg.Vote == nil || msg.Vote.Height != e.height {
			return
		}
		e.collectVote(*msg.Vote)
	}
}

func (e *Engine) collectVote(v Vote) {
	switch v.Phase {
	case PhasePrevote:
		e.prevotes[v.NodeID] = v
		if len(e.prevotes) >= e.quorum && e.phase == PhasePrevote {
			e.phase = PhasePrecommit
			_ = e.broadcast(ConsensusMsg{Type: "vote", Vote: &Vote{
				Phase: PhasePrecommit, Height: e.height, Round: e.round,
				BlockHash: v.BlockHash, NodeID: e.selfID,
			}})
		}
	case PhasePrecommit:
		e.precommits[v.NodeID] = v
		if len(e.precommits) >= e.quorum && e.phase == PhasePrecommit {
			e.commit()
		}
	}
}

func (e *Engine) commit() {
	if e.currentBlock == nil {
		return
	}
	log.Printf("✅ BFT Commit — height=%d hash=%s", e.height, e.currentBlock.Hash[:8])
	if e.onCommit != nil {
		go e.onCommit(*e.currentBlock)
	}
	e.height++
	e.round = 0
	e.phase = PhasePropose
	e.prevotes = make(map[string]Vote)
	e.precommits = make(map[string]Vote)
	e.currentBlock = nil
}

// advanceRoundLogInterval — boşta (mempool boş, hiç öneri gelmiyor) round
// sınırsız artabilir; bu ORİJİNAL "if false" kapatma sebebiydi (25dk'da
// round 200, log-spam). Round-ilerletme mantığı DEĞİŞMEDİ, sadece bu eşiğin
// katları dışında log basılmıyor — canlıda gözlemlenip eklendi (ADIM 8).
const advanceRoundLogInterval = 20

func (e *Engine) advanceRound() {
	if e.phase == PhaseCommit {
		return
	}
	if e.round == 0 || (e.round+1)%advanceRoundLogInterval == 0 {
		log.Printf("⏱️  BFT round timeout — height=%d, round=%d→%d", e.height, e.round, e.round+1)
	}
	e.round++
	e.phase = PhasePropose
	e.prevotes = make(map[string]Vote)
	e.precommits = make(map[string]Vote)
	e.currentBlock = nil
}

func (e *Engine) broadcast(msg ConsensusMsg) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return e.publishFn(TopicConsensus, data)
}

// Height — mevcut konsensüs yüksekliği
func (e *Engine) Height() uint64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.height
}

func (e *Engine) parentHash() string {
	if e.height == 1 {
		return GenesisParentHash
	}
	if e.parentHashFn != nil {
		return e.parentHashFn(e.height)
	}
	// parentHashFn enjekte edilmemişse (testler) — eski sahte placeholder.
	return fmt.Sprintf("parent_%d", e.height-1)
}

func blockHash(b *Block) string {
	data, _ := json.Marshal(b)
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
