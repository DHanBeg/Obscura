// Package sequencer — Decentralized ZK rollup sequencer (FAZ 4, spec 12.4)
//
// Aztec rollup sequencer'ını merkezi yapıdan DAO tarafından seçilen
// rotasyon tabanlı sequencer setine geçirir.
//
// Sequencer selection: VRF (Verifiable Random Function) + stake-weighted rotation.
// Batch submission: sequencer → Aztec prover → L1 verification.
package sequencer

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"
)

// SequencerStatus — sequencer durumu
type SequencerStatus string

const (
	StatusActive   SequencerStatus = "active"
	StatusStandby  SequencerStatus = "standby"
	StatusSlashed  SequencerStatus = "slashed"
	StatusRotating SequencerStatus = "rotating"
)

// EpochDuration — bir sequencer'ın aktif olduğu süre
const EpochDuration = 4 * time.Hour

// SequencerCandidate — sequencer adayı
type SequencerCandidate struct {
	NodeID    string          `json:"node_id"`
	DID       string          `json:"did"`
	Stake     float64         `json:"stake"`    // OBS stake miktarı
	PeerAddr  string          `json:"peer_addr"` // libp2p adresi
	Status    SequencerStatus `json:"status"`
	EpochID   uint64          `json:"epoch_id"`
	AddedAt   time.Time       `json:"added_at"`
}

// Batch — sequencer'ın bir batch'i
type Batch struct {
	ID          string    `json:"id"`
	SequencerID string    `json:"sequencer_id"`
	TxCount     int       `json:"tx_count"`
	StateRoot   string    `json:"state_root"`
	ProofHash   string    `json:"proof_hash"`
	SubmittedAt time.Time `json:"submitted_at"`
	L1TxHash    string    `json:"l1_tx_hash,omitempty"`
	Status      string    `json:"status"` // pending|proving|submitted|finalized
}

// Registry — sequencer kayıt defteri
type Registry struct {
	mu         sync.RWMutex
	candidates []SequencerCandidate
	active     *SequencerCandidate
	epoch      uint64
	batches    []Batch
}

var Global = &Registry{}

// StakeLookup is a function type that returns a node operator's real stake.
// Set by main via SetStakeLookup to avoid an import cycle.
var StakeLookup func(ctx context.Context, did string) (float64, error)

// SetStakeLookup wires the staking package into the sequencer without an
// import cycle. Called once from main before the first Register.
func SetStakeLookup(fn func(ctx context.Context, did string) (float64, error)) {
	StakeLookup = fn
}

// Register — sequencer adayı ekle; stake değeri on-chain staking DB'den alınır.
func (r *Registry) Register(candidate SequencerCandidate) error {
	// Query real staking data if lookup is wired up.
	if StakeLookup != nil {
		realStake, err := StakeLookup(context.Background(), candidate.DID)
		if err != nil {
			return fmt.Errorf("stake sorgulanamadı: %w", err)
		}
		if realStake < 1000 {
			return fmt.Errorf("sequencer için minimum 1000 OBS node_operator stake gerekli (mevcut: %.2f OBS)", realStake)
		}
		candidate.Stake = realStake
	} else if candidate.Stake < 1000 {
		return fmt.Errorf("sequencer için minimum 1000 OBS stake gerekli")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	candidate.AddedAt = time.Now().UTC()
	candidate.Status = StatusStandby

	for i, c := range r.candidates {
		if c.NodeID == candidate.NodeID {
			r.candidates[i] = candidate
			return nil
		}
	}
	r.candidates = append(r.candidates, candidate)
	log.Printf("🔄 Sequencer aday kaydedildi: %s (stake=%.0f OBS)", candidate.NodeID, candidate.Stake)
	return nil
}

// StartEpochRotation runs the automatic epoch rotation ticker. Call once from main.
func (r *Registry) StartEpochRotation(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(EpochDuration)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				seed := sha256.Sum256([]byte(fmt.Sprintf("epoch-rotate-%d", time.Now().UnixNano())))
				if _, err := r.Rotate(seed[:]); err != nil {
					log.Printf("⚠️  Sequencer rotasyon başarısız: %v", err)
				}
			}
		}
	}()
	log.Printf("⏱️  Sequencer otomatik rotasyon aktif (%s epoch)", EpochDuration)
}

// Rotate — epoch'u döndür, yeni active sequencer seç (VRF simülasyonu)
func (r *Registry) Rotate(seed []byte) (*SequencerCandidate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	active := r.eligibleCandidates()
	if len(active) == 0 {
		return nil, fmt.Errorf("aktif sequencer adayı yok")
	}

	// Stake-weighted VRF: her adaya stake oranında slot ver
	totalStake := 0.0
	for _, c := range active {
		totalStake += c.Stake
	}

	// VRF: seed'den deterministik 64-bit sayı üret
	h := sha256.Sum256(append(seed, toBytes(r.epoch)...))
	rndVal := float64(binary.BigEndian.Uint64(h[:8])) / float64(^uint64(0))
	target := rndVal * totalStake

	var selected *SequencerCandidate
	cumulative := 0.0
	for i := range active {
		cumulative += active[i].Stake
		if cumulative >= target {
			selected = &active[i]
			break
		}
	}
	if selected == nil {
		selected = &active[len(active)-1]
	}

	// Eski active'i standby'a al
	if r.active != nil {
		for i := range r.candidates {
			if r.candidates[i].NodeID == r.active.NodeID {
				r.candidates[i].Status = StatusStandby
			}
		}
	}

	// Yeni active
	r.epoch++
	for i := range r.candidates {
		if r.candidates[i].NodeID == selected.NodeID {
			r.candidates[i].Status = StatusActive
			r.candidates[i].EpochID = r.epoch
			r.active = &r.candidates[i]
		}
	}

	log.Printf("🔄 Sequencer rotasyon — epoch=%d, seçilen=%s", r.epoch, selected.NodeID)
	return r.active, nil
}

// SubmitBatch — sequencer bir batch sunar
func (r *Registry) SubmitBatch(sequencerID, stateRoot string, txCount int) (*Batch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.active == nil || r.active.NodeID != sequencerID {
		return nil, fmt.Errorf("aktif sequencer değil: %s", sequencerID)
	}

	b := Batch{
		ID:          fmt.Sprintf("batch_%d_%d", r.epoch, time.Now().UnixMilli()),
		SequencerID: sequencerID,
		TxCount:     txCount,
		StateRoot:   stateRoot,
		ProofHash:   generateProofHash(stateRoot, sequencerID),
		SubmittedAt: time.Now().UTC(),
		Status:      "pending",
	}
	r.batches = append(r.batches, b)
	log.Printf("📦 Batch gönderildi: %s (txCount=%d)", b.ID, txCount)
	return &b, nil
}

// ActiveSequencer — mevcut aktif sequencer
func (r *Registry) ActiveSequencer() *SequencerCandidate {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.active
}

// Candidates — tüm adaylar
func (r *Registry) Candidates() []SequencerCandidate {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]SequencerCandidate, len(r.candidates))
	copy(out, r.candidates)
	sort.Slice(out, func(i, j int) bool { return out[i].Stake > out[j].Stake })
	return out
}

// RecentBatches — son N batch
func (r *Registry) RecentBatches(n int) []Batch {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if n > len(r.batches) {
		n = len(r.batches)
	}
	return r.batches[len(r.batches)-n:]
}

func (r *Registry) eligibleCandidates() []SequencerCandidate {
	var out []SequencerCandidate
	for _, c := range r.candidates {
		if c.Status == StatusStandby || c.Status == StatusActive {
			out = append(out, c)
		}
	}
	return out
}

func toBytes(n uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, n)
	return b
}

func generateProofHash(stateRoot, sequencerID string) string {
	data, _ := json.Marshal(map[string]string{"state_root": stateRoot, "seq": sequencerID, "ts": time.Now().String()})
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
