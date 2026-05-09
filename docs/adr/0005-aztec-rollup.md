# ADR 0005: Choose Aztec for zk-Rollup over zkSync/StarkNet

Date: 2026-05-09
Status: Proposed
Decider: TBD
Spec ref: Bölüm 8.2 (zk-Rollup mimarisi)

## Context

FAZ 2 introduces OBS token with shielded transfers. Spec lists three options:
- StarkNet (Cairo, STARK)
- zkSync Era (Solidity, SNARK)
- Aztec (Noir, PLONK + native privacy)

Spec recommendation: "Aztec veya zkSync Era" — Aztec for native privacy, zkSync for EVM compat.

## Options considered

### Option A: zkSync Era (Solidity)
- Pros: Solidity = vast tooling (Hardhat, Foundry, OpenZeppelin), EVM-compat, easy hire
- Cons: Privacy is opt-in, not native; "shielded" requires custom contracts
- Maturity: High (mainnet, audited)
- Privacy: Partial

### Option B: StarkNet (Cairo)
- Pros: STARK proofs (quantum-resistant), high throughput
- Cons: Cairo niche language, smaller ecosystem, Cairo 1.0 still evolving
- Maturity: Medium-high
- Privacy: Partial

### Option C: Aztec (Noir) — chosen tentatively
- Pros: Native privacy (every transaction shielded by default), aligned with Obscura's ethos
- Cons: Newer, smaller ecosystem, Noir is younger than Cairo
- Maturity: Medium (Aztec sandbox + testnet, mainnet pending)
- Privacy: Native (best fit for Obscura)

## Decision (proposed)

**Option C: Aztec**.

## Rationale

- Obscura is privacy-first; Aztec is privacy-first. Match.
- Native privacy means we don't have to design custom shielded contracts (a huge attack surface).
- Noir's syntax is inspired by Rust, which fits team's preferred languages.
- Aztec sandbox supports local development without testnet.
- Risk: Aztec mainnet timing; if delayed, fallback to zkSync.

## Risks

| Risk | Mitigation |
|------|------------|
| Aztec mainnet delayed | Build contracts in Noir but keep abstraction; can port to zkSync if needed |
| Smaller ecosystem | Heavier in-house tooling investment; budget for it |
| Noir language churn | Track Aztec release notes, plan periodic Noir version bumps |
| Auditor scarcity | Engage Trail of Bits / Zellic early; budget premium |

## Consequences

- **Positive**: Best native privacy, less custom shielded contract code, aligned ethos
- **Negative**: Smaller hiring pool for Noir, longer time to mainnet
- **Tech debt**: If we need EVM bridge, must build through Aztec's L1 contracts

## Implementation plan

1. Set up Aztec sandbox locally (`aztec-cli`)
2. Write OBS token contract in Noir (`contracts/aztec/obs_token.nr`)
3. Write tests in Noir
4. Deploy to Aztec testnet
5. Build Go RPC bridge (`backend/internal/blockchain/aztec.go`)
6. Audit before mainnet (multiple firms)

## Open questions

- Does Aztec L1 bridge support our needs for cross-chain in FAZ 3?
- What's the gas cost per shielded transfer?
- How does Aztec handle network fees during congestion?

## References

- Aztec docs: https://docs.aztec.network/
- Noir lang: https://noir-lang.org/
- zkSync vs Aztec: https://blog.aztec.network/aztec-vs-zksync/
- Spec: docs/spec/obscura_spec_v3.txt Bölüm 8.2
