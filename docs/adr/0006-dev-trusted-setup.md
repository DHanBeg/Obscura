# ADR 0006: Single-contributor trusted setup for dev, multi-party for prod

Date: 2026-05-10
Status: Accepted
Decider: project lead
Spec ref: Bölüm 4.4, Bölüm 7.3 (ZK proof üretim akışı), Bölüm 13.8 (ZK altyapısı)

## Context

Groth16 ZK proofs require a "trusted setup" ceremony to generate proving + verification keys. The setup uses random "toxic waste" entropy that, if leaked, allows forging proofs.

Two-phase ceremony:
1. **Phase 1 (Powers of Tau)**: Universal setup, reusable across many circuits with the same constraint count power. Hermez Network published a public Powers of Tau ceremony for BN254.
2. **Phase 2**: Per-circuit, takes the Phase 1 output + R1CS, produces .zkey for that specific circuit.

In both phases, multiple contributors add randomness sequentially. As long as ONE contributor is honest and destroys their entropy, the setup is secure.

Single-contributor setup = trust that one party. Acceptable for development, NOT for production.

## Options considered

### Option A: Use existing Hermez ptau download (recommended in build.sh original)
- Pros: No local Phase 1, faster builds
- Cons: Hermez S3 link returned 263-byte error in our test (link may be dead). Reliance on external host.
- Effort: S
- Risk: Low (Hermez ceremony itself was multi-party + audited)

### Option B: Locally generate Phase 1 single-contributor for dev (chosen)
- Pros: Self-contained, no network dependency, deterministic
- Cons: Single point of trust (acceptable for dev only)
- Effort: S
- Risk: Acceptable for dev, NOT for production

### Option C: Multi-party ceremony today
- Pros: Production-ready
- Cons: Months of coordination, premature for current state
- Effort: XL

## Decision

**Option B for development; Option C before production.**

For dev (current):
- `circuits/build.sh` runs `snarkjs powersoftau new bn128 14` locally, contributes once, prepares phase2
- Output: `circuits/build/pot14_final.ptau` (~36MB)
- Per-circuit Phase 2 also single-contributor with random openssl entropy

For production (FAZ 1 GA):
- Coordinate multi-party Powers of Tau ceremony
  - Min 5 contributors from independent organizations
  - Each uses hardware RNG (not /dev/urandom)
  - Contribution attestation published with hash + name
  - Toxic waste destroyed (RAM only, dd if=/dev/urandom of=/dev/sda after)
- Phase 2 ceremony per circuit with same contributors
- Final zkey hashes published in release notes
- All artifacts signed by ceremony coordinator

## Rationale

- We need working ZK end-to-end NOW for development. Single-contributor is sufficient for "does the math work" testing.
- Production trust is earned via ceremony, not assumed via library choice.
- Bumping circuit constraints later requires re-ceremony anyway, so investing in production ceremony before circuits stabilize is wasteful.

## Consequences

- **Positive**: ZK pipeline works today. Can build features against verified circuits.
- **Negative**: All current .zkey artifacts MUST be discarded before production launch.
- **Tech debt**: Multi-party ceremony work item for FAZ 1 GA milestone.

## Implementation status

✅ Done:
- `circuits/build.sh` — single-contributor dev Phase 1 + Phase 2 per circuit
- `circuits/distribute.sh` — copies .wasm + .zkey to clients, vkey to backend
- `backend/internal/zk/verifier.go` — Go Groth16 verifier (iden3/go-rapidsnark)
- `backend/internal/zk/keys/*.json` — vkey.json embedded via go:embed
- 3 working circuits: credit_threshold, identity_proof, message_integrity
- End-to-end test: snarkjs prove → Go verify → PASS; tampered proof rejected

⬜ TODO before production:
- Multi-party Powers of Tau ceremony for BN254 power 14+
- Multi-party Phase 2 per circuit
- Discard dev .zkey files; replace with production
- Publish ceremony transcript with all contributor attestations
- Sign ceremony output with project release key

## Constraint counts (current circuits)

Measured 2026-05-10:
| Circuit | non-linear | linear | wires | Phase1 power needed |
|---------|-----------|--------|-------|---------------------|
| credit_threshold | 270 | 284 | 554 | 12 (4096) |
| identity_proof | TBD | TBD | TBD | 12 |
| message_integrity | TBD | TBD | TBD | 12 |

We use power 14 (16384) for headroom.

## References

- snarkjs trusted setup docs: https://github.com/iden3/snarkjs#7-prepare-phase-2
- Hermez Powers of Tau ceremony: https://github.com/iden3/snarkjs/blob/master/README.md#guide
- go-rapidsnark: https://github.com/iden3/go-rapidsnark
- Original ZK security whitepaper (Groth16): https://eprint.iacr.org/2016/260.pdf
- Spec: docs/spec/obscura_spec_v3.txt Bölüm 13.8
