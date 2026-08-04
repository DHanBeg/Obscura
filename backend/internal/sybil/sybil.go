// Package sybil holds the Sybil-resistance primitives originally built for
// internal/airdrop (design doc: airdrop.go's package comment) and now shared
// across any flow that needs "one phone-verified identity = one action":
// airdrop claims today, token-earning/marketplace participation next
// (Marketplace tasarım kararı a — Bölüm 8.2).
//
//  1. VerifyIdentityProof — the claimer proves ownership of an Obscura DID
//     without revealing the underlying secret/phone, via the identity_proof
//     ZK circuit.
//  2. ComputeNullifier — namespace-scoped nullifier
//     (sha256(namespace||"|"||did)), meant to be stored UNIQUE per namespace
//     so a second attempt for the same (namespace, identity) is rejected at
//     the DB layer. namespace replaces airdrop's campaignID — callers define
//     their own namespace scheme (e.g. "campaign:<id>", "marketplace_listing:<id>").
//  3. CallerTier — tier lookup, DID with no users row treated as tier 0
//     (below every threshold) rather than an error.
package sybil

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"

	"obscura.network/core/internal/db"
	"obscura.network/core/internal/zk"
)

// ComputeNullifier derives a namespace-scoped nullifier:
// sha256(namespace || "|" || did). The separator stops
// (namespaceA, didB) from colliding with (namespaceAdidB-prefix, "").
func ComputeNullifier(namespace, did string) string {
	sum := sha256.Sum256([]byte(namespace + "|" + did))
	return hex.EncodeToString(sum[:])
}

// VerifyIdentityProof is the seam through which callers verify the
// identity_proof ZK proof. In production it points at zk.VerifyGroth16.
// Tests override it via SetVerifyHookForTest (no identity_proof smoke
// fixture ships in circuits/test/, so tests mock at this boundary).
var VerifyIdentityProof = func(proofJSON []byte, publicSignals []string) error {
	return zk.VerifyGroth16(zk.CircuitIdentityProof, proofJSON, publicSignals)
}

// SetVerifyHookForTest swaps VerifyIdentityProof and returns a restore func.
// Exists ONLY for tests — production code never calls this.
func SetVerifyHookForTest(fn func(proofJSON []byte, publicSignals []string) error) func() {
	prev := VerifyIdentityProof
	VerifyIdentityProof = fn
	return func() { VerifyIdentityProof = prev }
}

// CallerTier looks up a DID's tier. A DID with no users row is treated as
// tier 0 (below every positive minimum) rather than an error — an unknown
// caller is simply ineligible.
func CallerTier(did string) (int, error) {
	var tier int
	err := db.DB.QueryRow(`SELECT tier FROM users WHERE did = ?`, did).Scan(&tier)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("tier lookup: %w", err)
	}
	return tier, nil
}
