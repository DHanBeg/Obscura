// Package token — Shielded note Merkle tree (spec Bölüm 8.3, FAZ 3).
//
// Depth=20 binary Merkle tree; Poseidon hash at every internal node.
// Backed by SQLite (shielded_notes tablosu leaf'leri saklar).
// Thread-safe: callers hold a *sql.Tx while calling methods.
//
// Node naming convention (level, index):
//   level 0  = leaves  (index = leaf_index in DB)
//   level 20 = root    (single node)
//
// Empty subtree roots are pre-computed from EMPTY_LEAF so that
// GetProof can return a valid proof for any inserted leaf without
// storing O(2^depth) nodes.

package token

import (
	"database/sql"
	"fmt"
	"math/big"

	"github.com/iden3/go-iden3-crypto/poseidon"
)

const MerkleDepth = 20 // 2^20 = 1,048,576 leaves

// emptyLeaf — zero field element, used for unoccupied leaves.
var emptyLeaf = new(big.Int)

// emptyRoots[i] = Poseidon root of a subtree of depth i filled with emptyLeaf.
// Pre-computed once at startup.
var emptyRoots [MerkleDepth + 1]*big.Int

func init() {
	emptyRoots[0] = new(big.Int).Set(emptyLeaf)
	for i := 1; i <= MerkleDepth; i++ {
		h, err := poseidon.Hash([]*big.Int{emptyRoots[i-1], emptyRoots[i-1]})
		if err != nil {
			panic("merkle: emptyRoots init: " + err.Error())
		}
		emptyRoots[i] = h
	}
}

// poseidonNode computes Poseidon(left, right).
func poseidonNode(left, right *big.Int) (*big.Int, error) {
	return poseidon.Hash([]*big.Int{left, right})
}

// parseFieldElem parses a base-10 string into *big.Int; rejects negatives.
func parseFieldElem(s string) (*big.Int, error) {
	if s == "0" || s == "" {
		return new(big.Int), nil
	}
	v, ok := new(big.Int).SetString(s, 10)
	if !ok || v.Sign() < 0 {
		return nil, fmt.Errorf("merkle: invalid field element %q", s)
	}
	return v, nil
}

// MerkleProofResult holds the sibling path for a leaf.
type MerkleProofResult struct {
	Root         string   `json:"root"`
	LeafIndex    int      `json:"leaf_index"`
	PathElements []string `json:"path_elements"` // MerkleDepth siblings, base-10
	PathIndices  []int    `json:"path_indices"`  // 0=leaf is left, 1=leaf is right
}

// leafAt returns the commitment stored at leafIndex, or emptyLeaf if none.
func leafAt(tx *sql.Tx, leafIndex int) (*big.Int, error) {
	var commitment string
	err := tx.QueryRow(`SELECT commitment FROM shielded_notes WHERE leaf_index = ?`, leafIndex).Scan(&commitment)
	if err == sql.ErrNoRows {
		return new(big.Int).Set(emptyLeaf), nil
	}
	if err != nil {
		return nil, fmt.Errorf("merkle: leafAt(%d): %w", leafIndex, err)
	}
	return parseFieldElem(commitment)
}

// nodeAt computes the subtree root at (level, index) by walking down to leaves.
// Uses cached stored nodes when available, falls back to emptyRoots.
// level=0 are leaves; level=MerkleDepth is the tree root.
func nodeAt(tx *sql.Tx, level, index int) (*big.Int, error) {
	if level == 0 {
		return leafAt(tx, index)
	}
	left, err := nodeAt(tx, level-1, index*2)
	if err != nil {
		return nil, err
	}
	right, err := nodeAt(tx, level-1, index*2+1)
	if err != nil {
		return nil, err
	}
	// Optimization: if both sides are empty, return pre-computed emptyRoots.
	if left.Sign() == 0 && right.Sign() == 0 {
		return new(big.Int).Set(emptyRoots[level]), nil
	}
	return poseidonNode(left, right)
}

// ComputeRoot recomputes the Merkle root from DB leaves (no caching).
// Call after inserting a leaf to get the new root.
func ComputeRoot(tx *sql.Tx) (*big.Int, error) {
	return nodeAt(tx, MerkleDepth, 0)
}

// GetProof returns the Merkle sibling path for the leaf at leafIndex.
// The proof is valid against ComputeRoot(tx).
func GetProof(tx *sql.Tx, leafIndex int) (*MerkleProofResult, error) {
	pathElements := make([]string, MerkleDepth)
	pathIndices := make([]int, MerkleDepth)

	idx := leafIndex
	for level := 0; level < MerkleDepth; level++ {
		siblingIdx := idx ^ 1 // flip last bit = sibling index at this level
		sibling, err := nodeAt(tx, level, siblingIdx)
		if err != nil {
			return nil, fmt.Errorf("merkle: GetProof level=%d: %w", level, err)
		}
		pathElements[level] = sibling.String()
		pathIndices[level] = idx & 1 // 0=leaf is left child, 1=leaf is right child
		idx >>= 1
	}

	root, err := ComputeRoot(tx)
	if err != nil {
		return nil, fmt.Errorf("merkle: ComputeRoot: %w", err)
	}

	return &MerkleProofResult{
		Root:         root.String(),
		LeafIndex:    leafIndex,
		PathElements: pathElements,
		PathIndices:  pathIndices,
	}, nil
}
