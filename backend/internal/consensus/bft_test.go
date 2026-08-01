package consensus

import (
	"encoding/json"
	"testing"

	"obscura.network/core/internal/sequencer"
)

func noopPublish(string, []byte) error          { return nil }
func noopSubscribe(string, chan<- []byte) error { return nil }

// ─── IsProposer / proposerFn wiring (ADIM 1) ───────────────────────────────

func TestIsProposer_NilProposerFnAlwaysTrue(t *testing.T) {
	e := NewEngine("node-1", 1, nil, noopPublish, noopSubscribe, nil, nil)
	if !e.IsProposer() {
		t.Fatal("proposerFn nil iken IsProposer() true dönmeli (geriye uyumluluk)")
	}
}

func TestIsProposer_MatchesSelfID(t *testing.T) {
	e := NewEngine("node-1", 1, nil, noopPublish, noopSubscribe, func() string { return "node-1" }, nil)
	if !e.IsProposer() {
		t.Fatal("proposerFn selfID ile eşleşiyor, IsProposer() true dönmeli")
	}
}

func TestIsProposer_DoesNotMatchSelfID(t *testing.T) {
	e := NewEngine("node-1", 1, nil, noopPublish, noopSubscribe, func() string { return "node-2" }, nil)
	if e.IsProposer() {
		t.Fatal("proposerFn selfID ile eşleşmiyor, IsProposer() false dönmeli")
	}
}

func TestProposeBlock_FailsWhenNotProposer(t *testing.T) {
	e := NewEngine("node-1", 1, nil, noopPublish, noopSubscribe, func() string { return "node-2" }, nil)
	if err := e.ProposeBlock([]string{"op-1"}); err == nil {
		t.Fatal("proposer olmayan node ProposeBlock çağırdı, hata beklenirdi")
	}
}

func TestProposeBlock_SucceedsWhenProposer(t *testing.T) {
	e := NewEngine("node-1", 1, nil, noopPublish, noopSubscribe, func() string { return "node-1" }, nil)
	if err := e.ProposeBlock([]string{"op-1"}); err != nil {
		t.Fatalf("proposer olan node ProposeBlock çağırdı, hata BEKLENMEZDİ: %v", err)
	}
}

func TestProposeBlock_SucceedsWhenProposerFnNil(t *testing.T) {
	e := NewEngine("node-1", 1, nil, noopPublish, noopSubscribe, nil, nil)
	if err := e.ProposeBlock([]string{"op-1"}); err != nil {
		t.Fatalf("proposerFn nil iken ProposeBlock hata vermemeli: %v", err)
	}
}

// ─── handleMsg proposer doğrulaması (ADIM 1) ───────────────────────────────

func TestHandleMsg_RejectsBlockFromWrongProposer(t *testing.T) {
	e := NewEngine("node-1", 2, nil, noopPublish, noopSubscribe, func() string { return "node-2" }, nil)

	msg := ConsensusMsg{Type: "block_proposal", Block: &Block{
		Height: 1, Proposer: "node-3", Hash: "deadbeef",
	}}
	raw, _ := json.Marshal(msg)
	e.handleMsg(raw)

	if e.currentBlock != nil {
		t.Fatal("yetkisiz proposer'dan gelen blok kabul edilmemeliydi (currentBlock nil kalmalı)")
	}
	if e.phase != PhasePropose {
		t.Fatalf("yetkisiz blok reddedilince phase değişmemeliydi, got=%s", e.phase)
	}
}

func TestHandleMsg_AcceptsBlockFromCorrectProposer(t *testing.T) {
	e := NewEngine("node-1", 2, nil, noopPublish, noopSubscribe, func() string { return "node-2" }, nil)

	msg := ConsensusMsg{Type: "block_proposal", Block: &Block{
		Height: 1, Proposer: "node-2", Hash: "deadbeef",
	}}
	raw, _ := json.Marshal(msg)
	e.handleMsg(raw)

	if e.currentBlock == nil {
		t.Fatal("doğru proposer'dan gelen blok kabul edilmeliydi")
	}
	if e.phase != PhasePrevote {
		t.Fatalf("geçerli blok sonrası phase PREVOTE olmalıydı, got=%s", e.phase)
	}
}

func TestHandleMsg_NilProposerFnAcceptsAnyProposer(t *testing.T) {
	e := NewEngine("node-1", 2, nil, noopPublish, noopSubscribe, nil, nil)

	msg := ConsensusMsg{Type: "block_proposal", Block: &Block{
		Height: 1, Proposer: "herhangi-bir-node", Hash: "deadbeef",
	}}
	raw, _ := json.Marshal(msg)
	e.handleMsg(raw)

	if e.currentBlock == nil {
		t.Fatal("proposerFn nil iken herhangi bir proposer'dan blok kabul edilmeliydi")
	}
}

// ─── TxRoot/Ops bütünlük kontrolü (ADIM 7) ─────────────────────────────────

func TestProposeBlock_SetsMatchingTxRootAndOps(t *testing.T) {
	e := NewEngine("node-1", 1, nil, noopPublish, noopSubscribe, func() string { return "node-1" }, nil)
	ops := []string{"op-1", "op-2"}
	if err := e.ProposeBlock(ops); err != nil {
		t.Fatalf("ProposeBlock: %v", err)
	}
	want := sequencer.ComputeMerkleRoot(ops)
	if e.currentBlock.TxRoot != want {
		t.Fatalf("TxRoot Ops'tan üretilen kökle eşleşmiyor: got=%s want=%s", e.currentBlock.TxRoot, want)
	}
	if len(e.currentBlock.Ops) != 2 {
		t.Fatalf("Block.Ops beklenen 2 elemanı taşımıyor, got=%v", e.currentBlock.Ops)
	}
}

func TestHandleMsg_RejectsBlockWithMismatchedTxRootAndOps(t *testing.T) {
	e := NewEngine("node-1", 2, nil, noopPublish, noopSubscribe, nil, nil)

	// Ops ile uyuşmayan sahte/bozuk bir TxRoot — proposer (veya bir MITM)
	// Ops'u değiştirip TxRoot'u eski bırakmış gibi simüle eder.
	msg := ConsensusMsg{Type: "block_proposal", Block: &Block{
		Height: 1, Proposer: "node-2", Hash: "deadbeef",
		Ops:    []string{"op-1", "op-2"},
		TxRoot: "bu-hic-dogru-merkle-koku-degil",
	}}
	raw, _ := json.Marshal(msg)
	e.handleMsg(raw)

	if e.currentBlock != nil {
		t.Fatal("TxRoot/Ops uyuşmayan blok reddedilmeliydi (currentBlock nil kalmalı)")
	}
	if e.phase != PhasePropose {
		t.Fatalf("reddedilen blok sonrası phase değişmemeliydi, got=%s", e.phase)
	}
}

func TestHandleMsg_AcceptsBlockWithMatchingTxRootAndOps(t *testing.T) {
	e := NewEngine("node-1", 2, nil, noopPublish, noopSubscribe, nil, nil)
	ops := []string{"op-1", "op-2"}

	msg := ConsensusMsg{Type: "block_proposal", Block: &Block{
		Height: 1, Proposer: "node-2", Hash: "deadbeef",
		Ops:    ops,
		TxRoot: sequencer.ComputeMerkleRoot(ops),
	}}
	raw, _ := json.Marshal(msg)
	e.handleMsg(raw)

	if e.currentBlock == nil {
		t.Fatal("TxRoot/Ops uyuşan blok kabul edilmeliydi")
	}
}
