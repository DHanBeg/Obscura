# ADR 0005: Choose Aztec for zk-Rollup over zkSync/StarkNet

Date: 2026-05-09 (updated 2026-05-16)
Status: Accepted
Decider: user
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

Phased so Obscura is never blocked on Aztec mainnet timing.

### Phase 0 (FAZ 2): In-house Merkle UTXO shielded transfer

Bridge-independent. A separate work stream (companion agent) ships shielded
OBS transfers via an in-house Merkle tree + Circom proof
(`circuits/token_balance.circom`). Settlement is to our own SQLite ledger; no
external rollup involved. This unblocks the "shielded transfer" product
feature without taking the Aztec dependency.

### Phase 1 (FAZ 2 GA): Aztec sandbox local devnet

1. Stand up Aztec sandbox locally (`aztec-cli sandbox`).
2. Implement OBS token in Noir at `contracts/aztec/obs_token.nr` — `mint`,
   `transfer`, shielded `ShieldedBalance` note. (Stub committed 2026-05-16.)
3. Noir test suite covering mint/transfer/fee invariants.
4. Wire Go RPC bridge `backend/internal/blockchain/aztec.go` against the
   sandbox JSON-RPC (`node_getStatus`, `submitTx`). (Stub committed
   2026-05-16; `SubmitProof` returns `ErrNotImplemented` until this phase.)
5. Run shielded transfers end-to-end client → bridge → sandbox.

### Phase 2 (FAZ 3): Aztec testnet deploy + L1 bridge

1. Deploy `obs_token.nr` to public Aztec testnet.
2. Implement L1 bridge contract (Solidity) for OBS deposit/withdraw against
   Aztec's portal pattern.
3. Audit firm #1 reviews Noir contract and bridge (engaged at Phase 1 start).

### Phase 3 (FAZ 3 GA): Mainnet

1. Aztec mainnet deploy when Aztec network itself goes mainnet.
2. Audit firm #2 confirmatory review.
3. Phase 0 in-house Merkle UTXO becomes the migration source: balances move
   over via a one-time deposit flow.

### Failover plan: pivot to zkSync Era

Aztec mainnet is not on a published date. If the Aztec mainnet schedule slips
past our FAZ 3 GA window, or the network is paused/degraded at switchover, we
fall back to zkSync Era:

- The Noir contract is rewritten as a Solidity contract using OpenZeppelin's
  ERC20 + a custom shielded-pool implementation (the same shape as Tornado
  Cash, audited). We lose *native* privacy but keep *opt-in* privacy via the
  shielded pool.
- The Go bridge's `SubmitProof` interface is unchanged; only the wire format
  of `proof` and `publicInputs` differs (PLONK on Aztec, SNARK-Groth16 on
  zkSync). The interface boundary in `backend/internal/blockchain/aztec.go`
  is shaped so a `ZKSyncBridge` can be a drop-in.
- Phase 0 in-house Merkle UTXO stays as-is — it does not depend on either
  choice and remains the privacy floor.

Decision gate for the pivot: if Aztec mainnet has no confirmed date 60 days
before our FAZ 3 GA target, switch.

## Open questions

- Does Aztec L1 bridge support our needs for cross-chain in FAZ 3?
- What's the gas cost per shielded transfer?
- How does Aztec handle network fees during congestion?

## References

- Aztec docs: https://docs.aztec.network/
- Noir lang: https://noir-lang.org/
- zkSync vs Aztec: https://blog.aztec.network/aztec-vs-zksync/
- Spec: docs/spec/obscura_spec_v3.txt Bölüm 8.2
