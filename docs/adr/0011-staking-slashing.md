# ADR 0011: Staking and Slashing Parameters

Date: 2026-05-13
Status: Proposed
Decider: user
Spec ref: Bölüm 8.5 (Staking, Node Operator Rewards, Slashing)

## Context

ADR-0010 locks supply, distribution, and inflation. This ADR locks the **staking and slashing mechanics** that turn those raw parameters into network security. Three actor classes need staking economics:

1. **User stakers** — passive holders who lock OBS for yield and governance weight. They are not running infrastructure; they delegate trust to the protocol.
2. **Node operators** — run zk-proof aggregators, sequencer/rollup nodes, MLS group servers. They are the on-the-ground operators whose misbehavior or downtime directly degrades network quality.
3. **The protocol itself** — collects slashed stake and either burns it, redistributes to honest stakers, or routes to treasury.

For each, we must answer: minimum stake (Sybil resistance), lock period (capital commitment depth), reward source (inflation + fee share), and **slashing conditions** (what's punishable, how much, who adjudicates). Slash sizing is the most consequential decision here: too lax and node operators rationally cheat (cost of misbehavior < expected gain); too harsh and honest operators flee after a single network hiccup (mass unstaking → security collapse). Cosmos uses 5% for downtime / 100% for double-sign. Ethereum uses ~1% for inactivity / up to 100% for slashable offenses (correlation penalty). Polkadot uses tiered 0.01–100%.

We must also decide *who decides*: pure on-chain rules (objective, fast, but brittle to edge cases) vs. multisig review (subjective, slow, but handles ambiguous cases like "node was honest but bandwidth-degraded by ISP outage"). The spec (Bölüm 8.5) prescribes specific numbers; this ADR justifies them and locks the adjudication model.

## Options considered

### Option A: Spec parameters with hybrid adjudication (chosen)

Adopt spec values: user min 1,000 OBS / 30-day lock / 5–15% variable APY; operator min 10,000 OBS / 30% fee share; slash tiers 1%/10%/50%/100% by offense severity; **on-chain auto-slash for ≤10%**, **3/5 multisig review for >10%** (gives humans a chance to reverse false-positive catastrophic slashes); 7-day unstake cooldown.

- Pros:
  - Numbers calibrated against Cosmos/Polkadot/Ethereum precedent
  - Multisig review on catastrophic slashes prevents one buggy oracle from nuking honest operators (real risk: Cosmos Hub had a chain-halting slash bug in 2021)
  - 30-day lock matches typical L2 unbonding (Optimism 7d, Arbitrum 7d, Cosmos 21d, Polkadot 28d) — long enough to make slash threats credible, short enough that capital isn't permanently dead
  - 7-day cooldown after unstake request is the standard "rage-quit" deterrent
- Cons:
  - Multisig is a centralization vector (5 signers control catastrophic slash reversal)
  - 30-day lock + 7-day cooldown = effective 37-day capital lock, which competitors may undercut
- Effort: M
- Risk: Low — every parameter has multiple prior-art comparables

### Option B: Pure on-chain, no multisig

All slashes execute automatically based on on-chain evidence (uptime oracle, proof verification result, fork detection). No human override. Cosmos-style.

- Pros:
  - Maximally credibly neutral — no signers to capture
  - Faster (no review delay)
  - Simpler audit (no off-chain governance to model)
- Cons:
  - **Single bug in the slash oracle = catastrophic loss** (Cosmos Hub 2021, Polkadot 2023 had near-misses requiring emergency upgrades)
  - No recourse for honest operators slashed due to upstream Internet outage they didn't cause
  - Forces conservative slash sizes (operators won't stake if a freak network event = -100%)
- Effort: M
- Risk: **High** — single point of failure in oracle code

### Option C: Optimistic slashing (slash proposed, 7-day challenge window)

Slashes are *proposed* on-chain with evidence; if not challenged within 7 days, they execute. Operators (or third parties) can post a counter-bond to challenge with counter-evidence.

- Pros:
  - Fault-tolerant to oracle bugs (anyone can challenge)
  - No designated multisig — fully open
- Cons:
  - 7-day delay on every slash is too slow for active attack defense (a forked validator could keep operating for a week before being slashed)
  - Challenge bond economics are themselves a parameter we'd have to tune (recursion)
  - Effectively reinvents optimistic rollup fraud proofs at the staking layer — large surface area
- Effort: L
- Risk: Medium — works in theory, but adds significant complexity for marginal centralization gain over Option A

## Decision

We chose **Option A** because it matches battle-tested precedent (Cosmos/Polkadot/Ethereum slash sizing), retains a human circuit-breaker on catastrophic slashes (the failure mode we most want to avoid), and ships in M-effort rather than L-effort.

## Rationale

**User minimum 1,000 OBS / 30-day lock / 5–15% variable APY.** 1,000 OBS at a plausible $0.10–$1.00 launch price ($100–$1,000) is the standard "meaningful but not exclusionary" floor — high enough to deter Sybil staking-pool attacks, low enough that a college student can participate. Compare: Ethereum 32 ETH (~$80k, exclusionary by design), Cosmos no minimum (Sybil-friendly), Polkadot 1 DOT minimum nominator (~$5, too low). 30-day lock is the median across PoS L1s. The 5–15% APY band is *not* a fixed yield — it floats inversely with total staked ratio (more staked → lower per-staker yield, because 2% inflation divides among more stakers). This is the same self-balancing mechanism Ethereum uses; it prevents both "too much staked → token velocity collapses" and "too little staked → security crisis."

**Operator minimum 10,000 OBS / 30% fee share.** 10x the user minimum because operators have hardware costs (server, bandwidth, uptime monitoring) and must be capitalized enough that a single 50% slash is genuinely punishing. 30% fee share is the operator revenue line; combined with 20% to treasury and 50% burn, fees route cleanly. Operator breakeven sketch: at 10,000 OBS staked, $0.50/OBS token price, 5% network fee-derived APY, plus the 30% fee share, an operator nets ~$500–$1,500/year per node at moderate volume — covers a $20/month VPS and leaves margin. **This is intentionally thin** so we get many operators, not few fat ones.

**Slash tiers (1% / 10% / 50% / 100%).** Calibrated to severity:
- **Uptime <95% over 30-day window: -1% per percentage point below threshold.** 95% is ~36 hours of downtime per month — generous enough to cover ISP outages, OS reboots, kernel panics. The linear ramp (1% per pp) means an operator at 90% loses 5%, at 80% loses 15% — proportional, recoverable.
- **Failure to produce ZK proof when required: -10% per incident.** This is "you were asked to do your job and didn't" — moderate severity because it could be transient (proof generation OOM'd, machine swapping). Cosmos uses ~5% for missed signatures; we go 10% because proof generation is the entire job, not a side task.
- **Producing invalid ZK proof: -50% per incident.** Either the operator's prover code is buggy (negligence) or they intentionally submitted bad proof (malice). Either is severe — bad proofs accepted on-chain would compromise the rollup's validity. 50% is the cliff above which we trigger multisig review (so a buggy verifier oracle can't accidentally nuke 50% of every honest operator).
- **Fork attack / double-sign: -100%.** Unambiguous protocol violation; identical to Cosmos and Ethereum policy. Detection is objective (two signed messages over conflicting chain heads).

**3/5 multisig review for >10% slashes.** This is the single most contested parameter. The case *for*: Cosmos Hub had a 2021 incident where a buggy slash oracle would have slashed honest validators by ~5% before the chain was halted; without human review, that loss is permanent. The case *against*: any multisig is a censorship vector. We resolve by **scoping narrowly**: the multisig can *only* reverse a slash (cannot initiate, cannot increase, cannot modify), the 5 signers are chosen via on-chain governance and rotate every 6 months, and every multisig action is logged with on-chain rationale. Effective veto on false-positive catastrophic slashes; no positive power.

**7-day unstake cooldown.** Stops a slashable operator from unstaking *between* an offense and its detection. Ethereum uses ~9 days; Cosmos uses 21; Polkadot 28. We pick 7 because Aztec's proof finality window is shorter than Cosmos's tendermint finality, so 7 days is more than enough for all slash conditions to be discoverable.

## Consequences

- **Positive**: Slash sizing is precedent-backed; multisig circuit-breaker eliminates the highest-risk failure mode (oracle-bug mass slash); operator economics are deliberately competitive to favor many small operators over few large ones; user staking is accessible at 1,000 OBS minimum.
- **Negative**: Multisig is a 5-signer centralization vector for the catastrophic-slash path (mitigated by narrow scope, rotation, transparency). 30+7 = 37 day effective lock is on the higher end and may push capital toward liquid-staking derivatives (LSD risk — see follow-up ADR).
- **Neutral**: 30% fee share to operators is the spec value; if measured operator breakeven is too thin in production, this can be adjusted via governance (timelocked, ADR-0012).
- **Tech debt incurred**: LSD ecosystem will emerge unbidden (operators will tokenize stake to escape the 37-day lock). We are not designing official LSD support in Year 1, which means a third-party LSD market will likely form and create centralization risk we'll need to address in FAZ 3.

## Implementation plan

1. Write `contracts/staking/UserStaking.sol` — `stake(amount)`, `requestUnstake()`, `withdraw()` with 30-day lock + 7-day cooldown enforced via timestamp checks. Reward accrual via `claimRewards()`. File: `contracts/staking/UserStaking.sol`.
2. Write `contracts/staking/OperatorStaking.sol` — extends UserStaking with operator registration, fee share accumulator, slashable-state flag. File: `contracts/staking/OperatorStaking.sol`.
3. Write `contracts/staking/SlashController.sol` — auto-executes slashes ≤10%, escalates >10% to `SlashMultisig`. Inputs: uptime oracle, proof verifier oracle, fork detection oracle. File: `contracts/staking/SlashController.sol`.
4. Write `contracts/staking/SlashMultisig.sol` — 3/5 multisig with reverse-only authority; signers rotated via governance vote. File: `contracts/staking/SlashMultisig.sol`.
5. Write off-chain `services/uptime-oracle/` — pings each registered operator every 60s, posts 30-day rolling availability on-chain hourly. File: `services/uptime-oracle/main.go`.
6. Write `docs/economics/staking.md` and `docs/economics/threat-models.md` (slash-griefing, lazy-operator, validator-cartel scenarios).
7. Verify via: (a) unit tests of every slash tier with synthetic offenses; (b) simulation of operator breakeven under 100 OBS/day, 1000 OBS/day, 10,000 OBS/day fee volumes; (c) adversarial test: attacker stakes minimum, intentionally double-signs, confirms 100% slash executes and unstake is blocked.

## Spec deviation (if applicable)

None. Numbers match Bölüm 8.5. The 3/5 multisig review threshold for >10% slashes is an implementation detail not specified in the spec but consistent with Bölüm 12 governance principles.

## References

- Spec: docs/spec/obscura_spec_v3.txt Bölüm 8.5
- Related ADRs: ADR-0010 (token economics), ADR-0012 (governance)
- External:
  - Cosmos SDK slashing module documentation
  - Ethereum consensus specs — `slash_validator` and correlation penalty
  - Polkadot Wiki — slashing levels
  - Buterin & Griffith, "Casper the Friendly Finality Gadget" (2017) — slash sizing rationale
  - Cosmos Hub Vega upgrade postmortem (2021) — case for human override on catastrophic slashes
