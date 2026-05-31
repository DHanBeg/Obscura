# ADR 0015: Continue with Groth16 (PLONK/STARK deferred)

Date: 2026-05-17
Status: Accepted
Decider: project lead
Spec ref: Bölüm 12.3 (FAZ 3 deliverables — "PLONK veya STARK"), Bölüm 4.4 (ZK proof akışı), Bölüm 13.8 (ZK altyapısı)

## Context

The spec (Bölüm 12.3) lists "PLONK veya STARK (trusted setup gerektirmez)" as a
FAZ 3 deliverable, implying a migration away from Groth16. The motivation is
eliminating the trusted-setup requirement: Groth16 requires a per-circuit
Powers of Tau ceremony; PLONK (URS-based) and STARK (hash-based) are
transparent or universal-setup systems.

As of 2026-05-17 the project has 10 deployed Groth16 circuits:

| Circuit | Constraints (non-linear) | Status |
|---------|--------------------------|--------|
| credit_threshold | 270 | ✅ production dev-key |
| identity_proof | ~200 | ✅ production dev-key |
| message_integrity | ~180 | ✅ production dev-key |
| token_balance | 944 | ✅ production dev-key |
| vote_proof | 733 | ✅ production dev-key |
| storage_proof | ~300 | ✅ production dev-key |
| age_proof | TBD | ✅ circuit complete, key generated |
| activity_proof | TBD | ✅ circuit complete, key generated |
| msg_count_proof | TBD | ✅ circuit complete, key generated |
| recursive_proof | TBD | ✅ circuit complete, key generated |

All circuits are Circom 2.1.6, proven with snarkJS WASM in the browser, and
verified server-side by `iden3/go-rapidsnark` (BN254 pairing check). The
end-to-end pipeline is tested and passing.

Switching proof systems would require: rewriting all circuits in a different
language or DSL (halo2/PLONKish, Cairo, Noir, RISC Zero guest), replacing the
WASM prover, replacing the Go verifier, and redoing key distribution.

## Options considered

### Option A: Migrate to PLONK (e.g., Noir + Barretenberg)
- Pros: Universal setup — no per-circuit trusted ceremony; Aztec's Barretenberg
  has a Go verifier and browser WASM prover; Noir DSL is more ergonomic than
  Circom for complex logic.
- Cons: Prover is ~5× slower than Groth16 for comparable constraint counts.
  Browser/WASM prover support for Barretenberg is experimental as of 2026-Q2.
  All 10 existing circuits must be rewritten in Noir. The existing
  `go-rapidsnark` verifier is replaced by a Barretenberg Go binding that
  currently requires CGO — violating our CGO_ENABLED=0 constraint.
- Effort: XL
- Risk: High

### Option B: Migrate to STARKs (e.g., StarkNet Cairo, RISC Zero)
- Pros: Transparent setup (no ceremony at all); post-quantum secure (hash-based
  commitments); RISC Zero allows arbitrary Rust guest programs.
- Cons: Proof size is ~100 KB–1 MB vs ~800 bytes for Groth16. Mobile data cost
  per proof submission is 100–1000× higher. WASM prover for STARKs is not
  production-ready for browser use. Prover time is 10–50× Groth16 for similar
  tasks. Completely different constraint system — all circuits rewritten.
- Effort: XL
- Risk: Very high (mobile feasibility not demonstrated)

### Option C: Migrate to halo2 (PSE / Zcash)
- Pros: No trusted setup per circuit (IPA commitment scheme); used by Zcash,
  Scroll, Polygon zkEVM — production track record; Rust-native DSL.
- Cons: No production-ready Go verifier; browser/WASM prover is not available
  for general halo2 circuits; requires Rust FFI in the verifier path, conflicting
  with CGO_ENABLED=0; circuit authoring is significantly more complex than Circom.
- Effort: XL
- Risk: High

### Option D: Stay with Groth16 + pursue multi-party ceremony (chosen)
- Keep snarkJS + Circom + go-rapidsnark. Conduct multi-party trusted setup
  ceremony before production GA.
- Pros: Pipeline proven end-to-end. No code change in circuits, frontend, mobile,
  or backend verifier. Multi-party ceremony adequately addresses the trust concern
  as long as ≥ 1 contributor is honest.
- Cons: Ceremony coordination overhead; .zkey artifacts must be replaced before
  production; future circuits each need a per-circuit Phase 2.
- Effort: S (retain) + M (ceremony coordination before prod)
- Risk: Low (technical risk); Medium (ceremony logistics)

## Decision

**Option D.** Groth16 with snarkJS + go-rapidsnark is retained for all current
and near-term circuits. A multi-party trusted setup ceremony is scheduled before
FAZ 1 GA (already tracked in ADR-0006). PLONK or halo2 may be revisited for
net-new circuits after FAZ 4 GA.

## Rationale

- **The snarkJS WASM prover is the only mature browser ZK prover.** Obscura
  generates proofs client-side (browser + mobile). No competing proof system
  has a production-grade, bundle-size-acceptable WASM prover as of 2026-Q2.
  Shipping "ZK in the browser" is the spec's UX requirement; switching proof
  systems would block that requirement, not satisfy it.
- **Proof size matters on mobile.** Groth16 produces ~800-byte proofs.
  STARK proofs are 100 KB to 1 MB. At the planned message rates (spec Bölüm 15.2:
  10 msg/s/node), STARK proof overhead per message would be 10–100 MB/s of
  additional upload traffic on mobile connections — not acceptable.
- **10 circuits are already complete.** Rewriting all circuits in Noir or Cairo
  for a system with zero production users is pure cost with no user benefit
  today. The correct time to switch is when adding a circuit whose logic
  benefits substantially from the target system's abstractions.
- **CGO_ENABLED=0 is a hard blocker for alternatives.** The best available Go
  verifier for PLONK (Barretenberg) and halo2 both require CGO. The pure-Go
  `go-rapidsnark` BN254 verifier works today without CGO.
- **"Trusted setup gerektirmez" is a property, not a deliverable.** The spec
  lists it as context for why PLONK/STARK are appealing, not as a binary
  pass/fail for FAZ 3 completion. A properly conducted multi-party Groth16
  ceremony provides equivalent practical security: one honest contributor is
  sufficient, and the ceremony transcript is public.
- **Prover performance.** Groth16 prove time is 494 ms on current circuits
  (measured 2026-05-10, spec target < 3 s). PLONK prover is ~5× slower (~2.5 s)
  and STARK is ~10–50× slower. Neither stays within the performance budget on
  mid-range mobile hardware.

## Consequences

- **Positive**: No circuit, prover, verifier, or key-distribution code changes.
  All in-flight FAZ 3 and FAZ 4 ZK work continues without disruption.
- **Negative**: Multi-party ceremony for all 10 circuits is required before
  production launch (inherits ADR-0006 obligation). Each new circuit added after
  the ceremony needs its own Phase 2 ceremony run.
- **Tech debt**: Post FAZ 4 GA, evaluate halo2 or Noir for incremental new
  circuits (e.g., GPS attestation, cross-chain bridges) where the DSL advantage
  and universal setup outweigh the prover-performance trade-off. New ADR required.

## Ceremony plan (pre-production)

Inherits the multi-party ceremony plan from ADR-0006, extended to all 10 circuits:

| Step | Owner | When |
|------|-------|------|
| Finalize circuit constraint counts | zk-circuit-engineer | FAZ 3 complete |
| Coordinate ≥ 5 independent contributors | project lead | 4 weeks before FAZ 1 GA |
| Phase 1 (Powers of Tau, BN254 power 14) | ceremony coordinator | Week 1 |
| Phase 2 per circuit × 10 | ceremony coordinator | Weeks 2–3 |
| Publish transcript + contributor attestations | project lead | Week 4 |
| Replace dev .zkey with ceremony .zkey in CI | devops-engineer | Week 4 |
| Discard dev .zkey from repo history | devops-engineer | Week 4 |

## Migration trigger criteria

Revisit this decision for new circuits when **any** of the following:

| Trigger | Threshold |
|---------|-----------|
| New circuit constraint count | > 100,000 (Groth16 ceremony cost grows super-linearly) |
| Browser WASM prover for PLONK/halo2 | Production-grade, < 5 MB bundle, < 3 s prove time on mid-range device |
| Mobile proof upload budget exceeded | p95 proof size > 50 KB |
| Post-quantum threat materialized | Groth16 BN254 security assumptions broken |

## References

- Spec: docs/spec/obscura_spec_v3.txt Bölüm 12.3, Bölüm 4.4, Bölüm 13.8
- ADR-0006 (dev trusted setup + ceremony plan): docs/adr/0006-dev-trusted-setup.md
- snarkJS: https://github.com/iden3/snarkjs
- go-rapidsnark: https://github.com/iden3/go-rapidsnark
- Groth16 original paper: https://eprint.iacr.org/2016/260.pdf
- Barretenberg (PLONK, Aztec): https://github.com/AztecProtocol/barretenberg
- halo2 (PSE): https://github.com/privacy-scaling-explorations/halo2
- ZK proof system comparison (2024): https://hackmd.io/@axiom/SJw3p-qX3
