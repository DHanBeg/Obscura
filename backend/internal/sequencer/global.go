package sequencer

// global.go — package-level facade that faz4_handlers.go and main.go use.
//
// The core Sequencer struct uses internal NodeInfo + Batch types.
// This file provides:
//   - SequencerCandidate  (handler-facing type, mapped to NodeInfo)
//   - GlobalSequencer      wraps *Sequencer with the handler-expected API
//   - Global               singleton *GlobalSequencer (init'd in init())
//   - SetStakeLookup       package-level convenience (called from main.go)
//
// StartEpochRotation is exposed with a context.Context parameter (main.go
// passes context.Background(); the context is only used for future
// cancellation and not wired into the ticker yet).

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"sync"
	"time"

	"obscura.network/core/internal/logredact"
)

// StakingFn is the signature of staking.NodeOperatorStakeOBS — returns the
// total staked OBS (as float64 whole units) for a given DID. Used by
// SetStakeLookup to derive per-node stake when the sequencer rotates.
type StakingFn func(ctx context.Context, did string) (float64, error)

// SequencerCandidate is the external representation of a sequencer node.
// Handlers decode JSON request bodies into this type.
type SequencerCandidate struct {
	DID       string `json:"did"`
	NodeID    string `json:"node_id"`
	Stake     uint64 `json:"stake"`
	PubKey    string `json:"pub_key,omitempty"`    // Ed25519 or Dilithium pubkey (hex) — oy imzalama (ADR-0017 adım 4, ayrı iş)
	VRFPubkey string `json:"vrf_pubkey,omitempty"` // sequencer.Sequencer.VRFPublicKeyHex() — ADR-0017 adım 5
}

// ActiveCandidate augments SequencerCandidate with runtime state.
type ActiveCandidate struct {
	SequencerCandidate
	IsActive bool   `json:"is_active"`
	EpochWon uint64 `json:"epoch_won,omitempty"`
}

// GlobalSequencer wraps *Sequencer and exposes the richer API the handlers expect.
type GlobalSequencer struct {
	mu         sync.RWMutex
	inner      *Sequencer
	candidates []SequencerCandidate // registered candidates (DID-keyed)
}

func newGlobalSequencer() *GlobalSequencer {
	nodeID := os.Getenv("NODE_ID")
	if nodeID == "" {
		nodeID = "node-1"
	}
	return &GlobalSequencer{
		inner: NewSequencer(nodeID, 4*time.Second),
	}
}

// Global is the process-wide singleton.
var Global = newGlobalSequencer()

// SetStakeLookup injects the staking oracle (called from main.go).
// fn is staking.NodeOperatorStakeOBS — takes (ctx, did) and returns the
// total staked OBS for that DID. The GlobalSequencer wraps it to build a
// []NodeInfo from registered candidates on each epoch rotation.
func SetStakeLookup(fn StakingFn) {
	Global.mu.Lock()
	defer Global.mu.Unlock()
	// Build an adapter: iterate the registered candidates and look up each one.
	Global.inner.SetStakeLookup(func() []NodeInfo {
		Global.mu.RLock()
		cands := make([]SequencerCandidate, len(Global.candidates))
		copy(cands, Global.candidates)
		Global.mu.RUnlock()

		nodes := make([]NodeInfo, 0, len(cands))
		ctx := context.Background()
		for _, c := range cands {
			stake, err := fn(ctx, c.DID)
			if err != nil {
				log.Printf("[sequencer] stake lookup error (did=%s): %v", logredact.DID(c.DID), err)
				stake = 0
			}
			nodes = append(nodes, NodeInfo{
				NodeID:    c.NodeID,
				Stake:     uint64(stake),
				VRFPubkey: c.VRFPubkey,
			})
		}
		return nodes
	})
}

// SetVRFTransport injects the GossipSub publish/subscribe functions used for
// ADR-0017 adım 5 (VRF proof yayını/toplama). Called from main.go after p2p
// starts successfully — see p2p.Publish/p2p.Subscribe. If p2p never starts
// (dev/single-node fallback), main.go simply doesn't call this; the inner
// Sequencer then only tracks its own proof locally (publishFn stays nil).
func SetVRFTransport(publishFn func(string, []byte) error, subscribeFn func(string, chan<- []byte) error) error {
	return Global.inner.SetVRFTransport(publishFn, subscribeFn)
}

// VRFPublicKeyHex — bu node'un VRF public key'i (hex). Operatör bu node'u
// sequencer adayı olarak /v1/sequencer/register'a kaydederken vrf_pubkey
// alanına bunu koymalı — koymazsa bu node'un proof'ları diğer node'larca
// doğrulanamaz (bilinen pubkey yok), leader-election'a katılamaz.
func VRFPublicKeyHex() string {
	return Global.inner.VRFPublicKeyHex()
}

// StartEpochRotation starts the epoch ticker.
// ctx is accepted for API symmetry with callers; unused until the ticker is
// wired to context cancellation (FAZ 4 TODO).
func (g *GlobalSequencer) StartEpochRotation(_ context.Context) {
	g.inner.StartEpochRotation()
}

// Register adds or updates a candidate.
func (g *GlobalSequencer) Register(c SequencerCandidate) error {
	if c.DID == "" {
		return fmt.Errorf("did zorunlu")
	}
	if c.NodeID == "" {
		return fmt.Errorf("node_id zorunlu")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for i, existing := range g.candidates {
		if existing.DID == c.DID {
			g.candidates[i] = c
			// Sync inner NodeInfo.
			g.syncInner()
			return nil
		}
	}
	g.candidates = append(g.candidates, c)
	g.syncInner()
	return nil
}

// syncInner rebuilds the inner Sequencer's node list from g.candidates.
// Must be called with g.mu held.
func (g *GlobalSequencer) syncInner() {
	nodes := make([]NodeInfo, 0, len(g.candidates))
	for _, c := range g.candidates {
		nodes = append(nodes, NodeInfo{NodeID: c.NodeID, Stake: c.Stake, VRFPubkey: c.VRFPubkey})
	}
	g.inner.mu.Lock()
	g.inner.nodes = nodes
	g.inner.mu.Unlock()
}

// Candidates returns all registered candidates sorted by stake descending.
func (g *GlobalSequencer) Candidates() []ActiveCandidate {
	g.mu.RLock()
	defer g.mu.RUnlock()

	activeNodeID := g.inner.CurrentSequencer()

	out := make([]ActiveCandidate, 0, len(g.candidates))
	for _, c := range g.candidates {
		out = append(out, ActiveCandidate{
			SequencerCandidate: c,
			IsActive:           c.NodeID == activeNodeID,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Stake != out[j].Stake {
			return out[i].Stake > out[j].Stake
		}
		return out[i].NodeID < out[j].NodeID
	})
	return out
}

// ActiveSequencer returns the currently active SequencerCandidate, or nil.
func (g *GlobalSequencer) ActiveSequencer() *SequencerCandidate {
	activeNodeID := g.inner.CurrentSequencer()
	if activeNodeID == "" {
		return nil
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	for _, c := range g.candidates {
		if c.NodeID == activeNodeID {
			cp := c
			return &cp
		}
	}
	return nil
}

// RecentBatches returns up to n most-recent batches (newest first).
func (g *GlobalSequencer) RecentBatches(n int) []Batch {
	all := g.inner.Batches()
	if len(all) <= n {
		// Return copy reversed (newest first)
		out := make([]Batch, len(all))
		for i, b := range all {
			out[len(all)-1-i] = b
		}
		return out
	}
	// Last n elements, reversed
	tail := all[len(all)-n:]
	out := make([]Batch, n)
	for i, b := range tail {
		out[n-1-i] = b
	}
	return out
}

// SubmitBatch allows the active sequencer to commit a batch by providing
// stateRoot and txCount (handler-friendly version). txHashes are synthesised
// as [stateRoot] for now (FAZ 4 TODO: real tx hash list from mempool).
func (g *GlobalSequencer) SubmitBatch(nodeID, stateRoot string, txCount int) (*Batch, error) {
	// Generate synthetic tx hashes for the merkle root (stateRoot is the rollup root).
	txHashes := []string{stateRoot}
	return g.inner.SubmitBatch(txHashes)
}
