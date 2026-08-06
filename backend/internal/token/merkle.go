package token

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"sync"
	"obscura.network/core/internal/dbi"
)

// IncrementalMerkleTree — sabit derinlikli Merkle ağacı (shielded pool için).
// Boş yapraklar zero-hash ile doldurulur.
//
// Düğüm cache'i: "level:index" → hash. nodeAt önce cache'e bakar, yoksa
// hesaplayıp cache'ler. InsertLeaf sadece etkilenen path'i invalidate eder
// (O(depth)), tüm ağacı değil (O(2^depth)).

const TreeDepth = 20 // 2^20 = ~1M yaprak

type IncrementalMerkleTree struct {
	leaves    []string
	zeros     []string
	nodeCache map[string]string // "level:index" → hash
	mu        sync.RWMutex
}

// computeZeros belirli bir seviyedeki boş alt-ağacın hash'ini döner.
func computeZeros() []string {
	zeros := make([]string, TreeDepth+1)
	zeros[0] = hex.EncodeToString(make([]byte, 32))
	for i := 1; i <= TreeDepth; i++ {
		zeros[i] = hashPair(zeros[i-1], zeros[i-1])
	}
	return zeros
}

func hashPair(l, r string) string {
	data, err := hex.DecodeString(l + r)
	if err != nil || len(data) == 0 {
		// fallback: string concat
		sum := sha256.Sum256([]byte(l + r))
		return hex.EncodeToString(sum[:])
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// NewIncrementalMerkleTree yeni bir ağaç oluşturur.
func NewIncrementalMerkleTree() *IncrementalMerkleTree {
	return &IncrementalMerkleTree{
		leaves:    []string{},
		zeros:     computeZeros(),
		nodeCache: make(map[string]string),
	}
}

func cacheKey(level, index int) string {
	return fmt.Sprintf("%d:%d", level, index)
}

// InsertLeaf yeni bir yaprak ekler ve indeksini döner.
// Sadece bu yaprağın kökü etkileyen path'ini invalidate eder (O(depth)).
func (t *IncrementalMerkleTree) InsertLeaf(leaf string) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.leaves = append(t.leaves, leaf)
	index := len(t.leaves) - 1
	t.invalidatePath(index)
	return index
}

// invalidatePath bir yaprak indeksinden köke giden tüm düğümleri cache'ten siler.
// Kilit çağıran tarafından tutulmalı.
func (t *IncrementalMerkleTree) invalidatePath(leafIndex int) {
	idx := leafIndex
	for level := 0; level <= TreeDepth; level++ {
		delete(t.nodeCache, cacheKey(level, idx))
		idx /= 2
	}
}

// nodeAt belirli seviye ve indeksteki düğümün hash'ini hesaplar.
// level 0 = yapraklar. Cache'li: varsa cache'ten döner, yoksa hesaplayıp cache'ler.
// Kilit çağıran tarafından tutulmalı (en az RLock; cache yazımı için Lock gerekir,
// bu yüzden cache yazımını yalnızca yazma kilidi altındaki yollardan yapıyoruz).
func (t *IncrementalMerkleTree) nodeAt(level, index int) string {
	if level == 0 {
		if index < len(t.leaves) {
			return t.leaves[index]
		}
		return t.zeros[0]
	}
	// Tamamen boş alt-ağaç ise zero-hash (yaprak yok).
	subtreeLeaves := 1 << uint(level)
	if index*subtreeLeaves >= len(t.leaves) {
		return t.zeros[level]
	}

	key := cacheKey(level, index)
	if v, ok := t.nodeCache[key]; ok {
		return v
	}
	left := t.nodeAt(level-1, index*2)
	right := t.nodeAt(level-1, index*2+1)
	h := hashPair(left, right)
	t.nodeCache[key] = h
	return h
}

// ComputeRoot ağacın kök hash'ini döner.
func (t *IncrementalMerkleTree) ComputeRoot() string {
	t.mu.Lock() // nodeAt cache yazdığı için Lock gerekli
	defer t.mu.Unlock()
	return t.nodeAt(TreeDepth, 0)
}

// GetProof belirli bir yaprağın Merkle kanıtını döner.
func (t *IncrementalMerkleTree) GetProof(index int) ([]string, error) {
	t.mu.Lock() // nodeAt cache yazdığı için Lock gerekli
	defer t.mu.Unlock()
	if index >= len(t.leaves) {
		return nil, fmt.Errorf("index %d out of range (%d leaves)", index, len(t.leaves))
	}
	proof := make([]string, TreeDepth)
	idx := index
	for level := 0; level < TreeDepth; level++ {
		sibling := idx ^ 1
		proof[level] = t.nodeAt(level, sibling)
		idx /= 2
	}
	return proof, nil
}

// VerifyProof bir yaprağın kanıtını doğrular.
func (t *IncrementalMerkleTree) VerifyProof(leaf string, index int, proof []string, root string) bool {
	if len(proof) != TreeDepth {
		return false
	}
	cur := leaf
	idx := index
	for level := 0; level < TreeDepth; level++ {
		if idx%2 == 0 {
			cur = hashPair(cur, proof[level])
		} else {
			cur = hashPair(proof[level], cur)
		}
		idx /= 2
	}
	return cur == root
}

// ─── DB-backed package-level helpers ─────────────────────────────────────────
//
// shielded.go calls ComputeRoot(tx) and the handler calls GetProof(tx, idx)
// as package-level functions that read leaves from the DB and use
// IncrementalMerkleTree to compute the answer.

// loadLeavesFromTx reads all shielded_notes commitments ordered by leaf_index.
func loadLeavesFromTx(tx dbi.Querier) ([]string, error) {
	rows, err := tx.Query(`SELECT commitment FROM shielded_notes ORDER BY leaf_index ASC`)
	if err != nil {
		return nil, fmt.Errorf("load shielded leaves: %w", err)
	}
	defer rows.Close()

	var leaves []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, fmt.Errorf("scan leaf: %w", err)
		}
		leaves = append(leaves, c)
	}
	return leaves, rows.Err()
}

// buildTreeFromLeaves builds an IncrementalMerkleTree pre-loaded with leaves.
func buildTreeFromLeaves(leaves []string) *IncrementalMerkleTree {
	t := NewIncrementalMerkleTree()
	t.leaves = leaves
	return t
}

// ComputeRoot reads all shielded leaves from the open transaction and returns
// the Merkle root as *big.Int. Called by appendLeafTx after inserting a new
// commitment so the stored root always reflects the full tree state.
func ComputeRoot(tx dbi.Querier) (*big.Int, error) {
	leaves, err := loadLeavesFromTx(tx)
	if err != nil {
		return nil, err
	}
	tree := buildTreeFromLeaves(leaves)
	rootHex := tree.ComputeRoot()
	// Convert the hex root to *big.Int for storage as a decimal field element.
	rootBytes, err := hex.DecodeString(rootHex)
	if err != nil {
		return nil, fmt.Errorf("root hex decode: %w", err)
	}
	return new(big.Int).SetBytes(rootBytes), nil
}

// MerkleProofResult is returned by GetProof.
type MerkleProofResult struct {
	Root        string   `json:"root"`
	LeafIndex   int      `json:"leaf_index"`
	PathElements []string `json:"path_elements"`
	PathIndices []int    `json:"path_indices"`
}

// GetProof reads all shielded leaves from the open transaction and returns the
// Merkle sibling path for leafIdx. Called by the /v1/wallet/shielded/proof
// handler so the client can build the ZK witness.
func GetProof(tx dbi.Querier, leafIdx int) (*MerkleProofResult, error) {
	leaves, err := loadLeavesFromTx(tx)
	if err != nil {
		return nil, err
	}
	if leafIdx < 0 || leafIdx >= len(leaves) {
		return nil, fmt.Errorf("leaf_index %d out of range (%d leaves)", leafIdx, len(leaves))
	}
	tree := buildTreeFromLeaves(leaves)
	proof, err := tree.GetProof(leafIdx)
	if err != nil {
		return nil, err
	}

	// Build path_indices: bit-decomposition of leafIdx (0=left, 1=right)
	pathIndices := make([]int, TreeDepth)
	idx := leafIdx
	for i := 0; i < TreeDepth; i++ {
		pathIndices[i] = idx % 2
		idx /= 2
	}

	rootHex := tree.ComputeRoot()
	return &MerkleProofResult{
		Root:        rootHex,
		LeafIndex:   leafIdx,
		PathElements: proof,
		PathIndices: pathIndices,
	}, nil
}
