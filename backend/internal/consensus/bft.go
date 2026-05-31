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
)

// ─── Tipler ──────────────────────────────────────────────────────────────────

type Phase string

const (
	PhasePropose    Phase = "PROPOSE"
	PhasePrevote    Phase = "PREVOTE"
	PhasePrecommit  Phase = "PRECOMMIT"
	PhaseCommit     Phase = "COMMIT"
)

const (
	TopicConsensus = "obscura/consensus/v1"
	RoundTimeout   = 8 * time.Second
)

// Block — konsensüs bloğu
type Block struct {
	Height    uint64 `json:"height"`
	Round     uint32 `json:"round"`
	ParentHash string `json:"parent_hash"`
	TxRoot    string `json:"tx_root"`    // merkle root of transactions
	Proposer  string `json:"proposer"`   // peer ID
	Timestamp int64  `json:"timestamp"`
	Hash      string `json:"hash"`
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
	Type  string      `json:"type"` // "block_proposal" | "vote"
	Block *Block      `json:"block,omitempty"`
	Vote  *Vote       `json:"vote,omitempty"`
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

	publishFn   func(topic string, data []byte) error
	subscribeFn func(topic string, ch chan<- []byte) error
	msgCh       chan []byte
}

// NewEngine — yeni konsensüs motoru
// quorum: gerekli oy sayısı (tipik olarak 2f+1 = ceil(2/3 * N) + 1)
func NewEngine(
	selfID string,
	quorum int,
	onCommit CommitCallback,
	publishFn func(string, []byte) error,
	subscribeFn func(string, chan<- []byte) error,
) *Engine {
	return &Engine{
		selfID:      selfID,
		height:      1,
		phase:       PhasePropose,
		quorum:      quorum,
		onCommit:    onCommit,
		publishFn:   publishFn,
		subscribeFn: subscribeFn,
		msgCh:       make(chan []byte, 256),
		prevotes:    make(map[string]Vote),
		precommits:  make(map[string]Vote),
	}
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

// ProposeBlock — yeni blok öner (bu node proposer ise)
func (e *Engine) ProposeBlock(txRoot string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	b := &Block{
		Height:     e.height,
		Round:      e.round,
		ParentHash: e.parentHash(),
		TxRoot:     txRoot,
		Proposer:   e.selfID,
		Timestamp:  time.Now().UnixMilli(),
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

func (e *Engine) advanceRound() {
	if e.phase == PhaseCommit {
		return
	}
	log.Printf("⏱️  BFT round timeout — height=%d, round=%d→%d", e.height, e.round, e.round+1)
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
		return "0000000000000000000000000000000000000000000000000000000000000000"
	}
	// Üretimde: zincirin son commit edilen bloğunun hash'i
	return fmt.Sprintf("parent_%d", e.height-1)
}

func blockHash(b *Block) string {
	data, _ := json.Marshal(b)
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
