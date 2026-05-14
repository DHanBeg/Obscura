# ADR 0010: OBS Token Economics (supply, distribution, inflation, burn)

Date: 2026-05-13
Status: Proposed
Decider: user
Spec ref: Bölüm 8 (Token Economics), Bölüm 8.1–8.4

## Context

FAZ 2 introduces the OBS token as Obscura's economic primitive: payment for shielded transfers and mini-app deploys, staking collateral for node operators, governance weight, and incentive carrier for the trust/reputation tier system. Before any Solidity/Noir code is written or any genesis state is committed, we must lock five interdependent parameters: **total supply, distribution split, inflation rate, fee burn rate, and decimal precision**. These five values shape every downstream economic decision (staking APY math, governance quorum, slash sizing, node operator breakeven) and are effectively immutable once mainnet launches — changing them post-launch requires either a hard fork or a governance proposal that supermajority holders are economically incentivized to reject.

The spec (Bölüm 8) prescribes concrete numbers (1B supply, 40/20/15/15/10 split, 2% inflation, 50% burn). This ADR's job is not to relitigate those numbers but to **justify each one against comparable systems** (Bitcoin, Ethereum, Filecoin, Zcash, Tornado Cash, Aztec) and document the trade-offs we accept by adopting them. We also lock token mechanics that the spec leaves implicit: **18 decimals** (EVM/Aztec standard), **symbol "OBS"**, and **shielded-by-default transfer mode** with optional transparent escape hatch for KYC/compliance flows.

Constraints: (a) target chain is Aztec (per ADR-0005), so we inherit EVM-equivalent ERC-20 semantics and 18-decimal expectations; (b) the token must work in both shielded (zk-rollup note) and transparent (public ledger) modes for compliance escape hatches; (c) the economic model must remain coherent under both bull (90% staked, high fees, deflationary) and bear (low staking, low fees, mildly inflationary) scenarios.

## Options considered

### Option A: Adopt spec numbers verbatim (1B / 40-20-15-15-10 / 2% / 50% burn / 18 dec)

Use the parameters as written in Bölüm 8. Total supply 1,000,000,000 OBS, 18 decimals, 2%/year inflation funding staking rewards, 50% of all transaction fees burned, the rest of fees distributed to node operators and treasury. Vesting: team 20% over 4 years with 1-year cliff, investors 15% similar.

- Pros:
  - Zero spec deviation; no governance churn at launch
  - 1B supply is the modal choice for L2 / privacy-coin launches in 2023–2026 (matches user mental model)
  - 2% inflation is below the staking yield ceiling, leaving room for fee-driven deflation in steady state
  - 50% burn is aggressive enough to make the token plausibly deflationary at moderate fee volume
- Cons:
  - "Spec said so" is not a justification; we still owe future readers the *why*
  - 1B supply means per-token price stays low for years, which some treat as a signaling negative (psychological "penny stock" effect)
- Effort: S (parameter lock only, no code yet)
- Risk: Low — parameters are conservative and well-precedented

### Option B: Bitcoin-style fixed supply, no inflation (21M cap, 0% inflation, 100% fee burn)

Mirror Bitcoin/Zcash: hard cap, no new issuance, stakers/node operators paid entirely from fees. Total supply 100M–1B (still our choice), but `mint` function permanently disabled after genesis.

- Pros:
  - Maximally credible scarcity narrative
  - No "inflation tax" on non-stakers
  - Simpler economic model (one knob fewer)
- Cons:
  - **Staking security collapses at low fee volume**: if fees can't pay node operators, they unstake, network security drops, fees drop further — death spiral. Ethereum explicitly rejected this for the same reason post-Merge.
  - No flexibility to bootstrap network effects (early stakers get nothing if early users don't pay fees)
  - Forces extreme fee levels in early years, hostile to growth
- Effort: S
- Risk: **High** — empirically observed failure mode in low-fee L2s (early Optimism, pre-bridge Arbitrum subsidized sequencers from treasury for this reason)

### Option C: Ethereum-style uncapped supply, dynamic burn (no cap, ~0.5–1% net inflation, EIP-1559-style base fee burn)

No max supply; issuance floats to hit a target staking ratio (e.g., 30% of supply staked → APY auto-adjusts). Burn rate floats with congestion (base fee scales with demand).

- Pros:
  - Self-balancing: high demand → high burn → deflationary; low demand → mild inflation funds security
  - Proven at scale (Ethereum post-EIP-1559 + Merge)
- Cons:
  - Far more complex to implement and audit (dynamic issuance curve, base fee oracle)
  - Loses the "1B fixed cap" narrative that helps OTC and exchange listings
  - Adds new attack surface (issuance curve manipulation via governance)
- Effort: L (custom issuance curve, dynamic fee module, audit cost ~2x)
- Risk: Medium — works for Ethereum but Ethereum has 7+ years of in-the-wild data; we don't

## Decision

We chose **Option A** (adopt spec numbers verbatim) because the spec parameters fall inside the proven safe zone of comparable systems and the marginal benefit of bespoke economics does not justify the audit and complexity cost for a Year-1 token launch.

## Rationale

**Total supply: 1,000,000,000 OBS.** The 1B figure sits in the middle of the comparable-launch distribution: Bitcoin 21M (scarcity-maxi), Zcash 21M (Bitcoin mimicry), Tornado Cash 10M (deliberately tiny to make per-token governance weight expensive to acquire), Filecoin ~2B, Polkadot ~1.5B, Aztec testnet ~10B placeholder. 1B gives us (a) enough granularity for micro-fees (0.01 OBS file share is still meaningful at 18 decimals), (b) low enough per-token nominal that early users can hold whole tokens (avoids Bitcoin's psychological "fractional ownership" friction), and (c) a round number that fits exchange listing conventions. 100M would have forced per-token prices high early and complicated micro-fees; 10B would have signaled inflationary mindset. **1B is the Schelling point.**

**18 decimals.** Non-negotiable on Aztec/EVM: ERC-20 ecosystem tooling (wallets, indexers, block explorers, the entire DeFi composability stack) assumes 18. Using a non-18 value (e.g., USDC's 6) saves nothing on Aztec (gas is paid in field elements, not bytes) and breaks every off-the-shelf integration. **The decision is "use the platform default."**

**Distribution 40/20/15/15/10.** Community 40% is the upper-quartile choice (Filecoin 70%, Polkadot 30%, Optimism 19%, Arbitrum 56%) — biased toward community because Obscura's value comes from network effects of trust-tier users, not from a tight founding team. Team 20% with 4-year linear vest + 1-year cliff matches the Silicon Valley YC-standard founder equity schedule and the Ethereum/Solana team allocations; shorter vests (Filecoin 6mo cliff) have historically produced post-cliff dumps that we explicitly want to avoid. Investors 15% on the same schedule keeps team and investors economically aligned. Ecosystem fund 15% funds grants, audits, bug bounties — sized so we can run a $10M+ bug bounty program at $0.50/OBS without governance vote. Reserve 10% is the multi-sig emergency buffer (security incidents, exchange listing market-making, treaty payments to integration partners).

**Inflation 2%/year.** Funds staking rewards. Below the 5–15% APY ceiling means in steady state most reward comes from *fee redistribution* not new issuance — i.e., we are an L2 fees protocol with a small inflation top-up, not a high-inflation chain. Compare: Cosmos ~7–20%, Polkadot ~10%, Ethereum ~0.5–1% net, Bitcoin 0% post-tail-emission. 2% is closer to Ethereum's model and reflects our belief that fee revenue will carry security long-term. The 2% is a *ceiling* — actual issuance can be lower if staked ratio is below target.

**Burn 50% of fees.** Splits between deflation pressure (50% burned) and node operator revenue (30% to operators, 20% to treasury). 100% burn (Tornado) makes operators unprofitable; 0% burn (early Cosmos) makes the token purely inflationary. 50% is the EIP-1559-inspired middle: enough burn to plausibly hit net-deflationary at moderate volume, enough operator revenue to keep nodes profitable at minimum stake.

**Shielded by default, transparent opt-in.** Privacy is Obscura's product. A transparent-by-default transfer mode would leak the social graph (sender, receiver, amount) and defeat the entire point. Transparent mode exists only as an escape hatch for exchanges, compliance reporting, and audits — users who need to *prove* a transfer happened (KYC withdrawal, tax filing) can opt-in per-transfer. This matches Zcash's design (shielded `z-addrs` vs. transparent `t-addrs`) but inverts the default — Zcash's transparent default was a critical adoption mistake we will not repeat.

## Consequences

- **Positive**: Spec-conformant; auditable against well-studied comparable systems; conservative parameter choices reduce governance churn risk in Year 1; aligns operator and holder incentives via fee burn + revenue share.
- **Negative**: 2% inflation is a perpetual tax on non-stakers, which may push casual holders into custodial staking services (centralization vector — mitigated by low minimum stake in ADR-0011). Fixed 1B supply means we cannot respond to sudden demand by issuing more (counterargument: this is the *point*).
- **Neutral**: 18 decimals locks us into EVM tooling assumptions, which is fine on Aztec but would be an issue if we ever migrated to a non-EVM L1 (we won't — ADR-0005).
- **Tech debt incurred**: None at this layer; the genesis allocation script and ERC-20 contract are net-new code, not legacy.

## Implementation plan

1. Write `contracts/OBS.sol` (or Noir equivalent) — ERC-20 with `mint` callable only by `InflationController`, capped at 2%/year, and `burn` callable by `FeeCollector`. Files: `contracts/token/OBS.sol`, `contracts/token/InflationController.sol`, `contracts/token/FeeCollector.sol`.
2. Write `contracts/vesting/TeamVesting.sol` — 4-year linear vest, 1-year cliff, beneficiary list immutable post-deploy. Files: `contracts/vesting/TeamVesting.sol`, `contracts/vesting/InvestorVesting.sol`.
3. Write `scripts/genesis.ts` — mints initial 1B to community/team/investors/ecosystem/reserve addresses per the 40/20/15/15/10 split. File: `scripts/genesis.ts`.
4. Document the shielded/transparent dual mode in `docs/economics/privacy-modes.md`.
5. Verify via: (a) unit tests that `totalSupply()` at genesis equals exactly 1e9 * 1e18; (b) inflation simulation showing year-1 issuance ≤ 2% even under adversarial staking ratios; (c) burn accounting test showing 50% of every fee txn ends in the zero address.

## Spec deviation (if applicable)

None. This ADR adopts spec Bölüm 8 verbatim and adds justification + implicit parameters (decimals, symbol, default privacy mode).

## References

- Spec: docs/spec/obscura_spec_v3.txt Bölüm 8.1–8.4
- Related ADRs: ADR-0005 (Aztec rollup choice), ADR-0011 (staking/slashing), ADR-0012 (governance)
- External:
  - Bitcoin whitepaper §6 (incentive / fixed supply rationale)
  - EIP-1559 (base fee burn mechanism)
  - Buterin, "Endgame" (2021) — burn vs. issuance balance
  - Zcash protocol spec §4.7 (shielded/transparent dual mode)
  - Filecoin token economics paper (CoinList 2020) — 4-year vest precedent
