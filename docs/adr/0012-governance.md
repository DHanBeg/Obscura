# ADR 0012: Governance Mechanism (ZK voting, eligibility, quorum, veto)

Date: 2026-05-13
Status: Proposed
Decider: user
Spec ref: Bölüm 12.2 (Governance), Bölüm 8.5 (governance eligibility tie-in)

## Context

ADRs 0010 and 0011 lock the token and staking layer. Governance is the layer that lets those parameters evolve — fee schedules, slash sizes, ecosystem grants, protocol upgrades. Done wrong, governance is the attack surface that captures the protocol: token-weighted votes empower whale plutocracy (Compound, MakerDAO have spent years fighting this), and public vote choices enable on-chain bribery markets (the "dark DAO" attack — see Daian et al. 2018).

Obscura's threat model demands two governance-specific properties that most chains lack:

1. **Vote privacy**: choices must be hidden during voting (and ideally forever — see "vote receipts" below) so that bribery is structurally impossible. A briber cannot pay for what they cannot verify.
2. **Reputation-gated eligibility**: pure token-weight is plutocratic. Obscura already has a reputation/tier system (Platinum, Diamond from FAZ 1); governance should require *both* economic stake *and* social proof, so a $10M whale who just bought in cannot immediately rewrite the protocol.

The spec (Bölüm 12.2) is explicit on most numbers: 5,000 OBS staked + Platinum tier (80+ score) to propose/vote, 100 OBS burned to propose (anti-spam), 7-day vote, 48h timelock on execution, 10% quorum, 51% pass for parameters / 67% for protocol changes, 1/5 Diamond-tier veto with timelocked appeal. This ADR justifies each number and locks the ZK voting circuit interface (`vote_proof.circom`).

## Options considered

### Option A: ZK-voted, tier-gated governance per spec (chosen)

Adopt Bölüm 12.2 parameters. Eligibility = 5,000 OBS staked AND Platinum (80+). Proposal cost 100 OBS burned. 7-day vote, 48h timelock, 10% quorum, 51%/67% thresholds. Diamond veto with appeal. Votes submitted as ZK proofs (`vote_proof.circom`): the proof asserts "I am eligible, my stake is N, my vote is V" but reveals only N (weight) and the aggregate tally — never the per-voter choice. Vote receipts are not provable: even the voter cannot later prove how they voted, defeating bribery.

- Pros:
  - Bribery-resistant by construction (you can't pay for unprovable behavior)
  - Tier gating defangs whale plutocracy without forfeiting economic skin-in-the-game
  - Timelock + Diamond veto give two layers of "wait, are we sure?" before a malicious or rushed proposal executes
  - Quorum and threshold values are well-precedented (Compound 4% quorum, Uniswap 4%, MakerDAO ~10% — we pick the strict end)
- Cons:
  - ZK voting circuit is non-trivial to build and audit (~2 person-months)
  - Tier gating means low-reputation users have zero governance voice, which is philosophically uncomfortable (mitigation: low tier still earns yield, just can't vote on protocol changes)
  - Diamond veto is a centralization vector if the Diamond cohort is small at launch
- Effort: L
- Risk: Medium — ZK circuit is new code; everything else is precedent

### Option B: Plain on-chain token-weighted vote (Compound-style)

Public votes, weighted by staked OBS, no tier gate. Simple, fast to ship.

- Pros:
  - Trivial to implement (Compound governor contracts are open-source)
  - Standard tooling (Tally, Snapshot) works out-of-the-box
- Cons:
  - **Bribery markets emerge immediately** (this is observed, not theoretical: Curve, Convex, MakerDAO all have active vote-buying markets)
  - Plutocratic by construction
  - Public votes leak political alignment of every voter (anti-Obscura ethos)
- Effort: S
- Risk: **High** for a privacy chain — this is a category violation of our core value prop

### Option C: Off-chain Snapshot vote + on-chain execution

Voting happens on Snapshot (gasless, off-chain, signature-based); winning proposals are manually executed on-chain by a multisig.

- Pros:
  - Zero gas cost to voters
  - Familiar UX (every major DAO uses some Snapshot variant)
- Cons:
  - The multisig is the actual government (vote is advisory) — unacceptable centralization
  - No vote privacy (Snapshot is public)
  - Doesn't solve the bribery problem
- Effort: S
- Risk: High — degenerates into multisig rule

## Decision

We chose **Option A** because vote privacy is non-negotiable for a privacy-first protocol, and tier-gated eligibility solves the whale-capture problem that has captured every comparable token-governed protocol.

## Rationale

**Eligibility: 5,000 OBS staked AND Platinum tier (80+ score).** 5,000 OBS is 5x the user staking minimum — meaningful enough that a flash-loan-style governance attack costs real capital, low enough that ~tens-of-thousands of users qualify at maturity. The Platinum tier requirement is the load-bearing innovation: it means governance power requires *behavior*, not just *money*. A whale who buys $100k of OBS yesterday cannot vote until they accrue an 80+ reputation score, which takes months of legitimate network participation. This is structurally identical to Sybil-resistant identity systems (BrightID, Worldcoin) but bootstrapped from our own reputation graph rather than requiring an external trust anchor.

**Proposal cost 100 OBS, burned.** Anti-spam, not anti-participation. 100 OBS at any plausible launch price is enough that frivolous proposals are expensive but legitimate ones are trivially affordable. Burning (vs. treasury deposit) prevents the "submit-spam-to-deplete-treasury" attack and aligns proposal cost with the protocol's deflationary mechanism.

**Voting period 7 days.** Standard governance window. Long enough that EU + US + APAC time zones all get full attention; short enough that decisions don't drag. Compound uses 3 days (too short for global participation, IMO), MakerDAO uses ~14 (too long, momentum dies). 7 is the median.

**Timelock 48 hours.** After a proposal passes, execution is delayed 48h. This is the standard "exit window" — if users hate the outcome, they have two days to unstake and exit before the change applies. Compound uses 48h; Uniswap uses 48h; MakerDAO uses 48h. There is no good argument against 48h.

**Quorum 10%.** Of *staked* OBS, not total supply. Compound uses 4% (consistently failing to hit quorum), Uniswap 4% (same issue). 10% is the strict end — high enough that decisions reflect real consensus, low enough to be hittable. Calculated against staked supply rather than total because non-stakers have already opted out of active participation.

**Pass thresholds: 51% parameter / 67% protocol.** Two-tier threshold. Parameter changes (fee schedule, slash sizes, ecosystem grant size) require simple majority — these are reversible and low-stakes. Protocol changes (upgrading the OBS contract, changing the rollup verifier key, changing governance itself) require 2/3 — irreversible, high-stakes. This is Ethereum-foundation-style ("rough consensus" for parameters, supermajority for forks).

**Diamond veto (1/5 of Diamond-tier voters can veto, with timelocked appeal).** The escape valve for the rare case where a 51% vote is technically legitimate but catastrophically wrong (e.g., a coordinated brigade votes to drain the ecosystem fund). 1/5 of Diamond holders is intentionally low — it's a fire alarm, not a co-decision-maker. Veto triggers a *timelocked appeal*: the proposal is paused 14 days, during which the original proposers can post counter-evidence and the full electorate re-votes. If the re-vote passes at 67%, the veto is overridden. So the veto can delay but not unilaterally kill; the cost of veto-griefing is reputational (Diamond identities are stable; serial vetoers lose tier).

**ZK vote via `vote_proof.circom`.** The circuit asserts: (a) the prover is in the eligible set (membership proof over the Platinum+5000-OBS merkle root, snapshotted at proposal creation), (b) their stake weight is correctly computed, (c) their vote is in {YES, NO, ABSTAIN}, (d) they haven't already voted (nullifier check). The public output is the vote weight + the homomorphically encrypted choice; only the aggregate tally is decrypted after the vote closes. **Receipt non-provability** is enforced by deterministically deriving the nullifier from the proposal ID + voter secret key — the voter cannot construct a second proof showing the same nullifier with a different choice, so any "receipt" they show after the fact is non-verifiable.

## Consequences

- **Positive**: Bribery-resistant; whale-capture-resistant; aligns with Obscura's privacy-first identity; timelock + veto provide defense-in-depth against rushed bad proposals.
- **Negative**: ZK voting circuit adds ~2 person-months of net-new code and an audit dependency. Tier gating may be perceived as elitist (counter: every user can earn Platinum through behavior; the bar is participation, not money). Diamond veto could be abused for griefing (mitigation: serial vetoers lose tier via existing reputation system).
- **Neutral**: 10% quorum may not be hit early when staked supply is small (acceptable — early-stage protocols should have *less* governance, not more; until quorum is reliably met, the founding team operates under existing multisig).
- **Tech debt incurred**: The vote-aggregation oracle (decrypts tallies post-vote, posts results on-chain) is a trusted off-chain component in v1; long-term we want fully on-chain MPC decryption (FAZ 3 work).

## Implementation plan

1. Write `circuits/vote_proof.circom` — eligibility membership + stake weight + nullifier + encrypted choice. Files: `circuits/vote_proof.circom`, `circuits/vote_proof_test.ts`.
2. Write `contracts/governance/Governor.sol` — proposal lifecycle (propose → vote → tally → timelock → execute). Implements both threshold tiers (51%/67%) based on proposal type tag. File: `contracts/governance/Governor.sol`.
3. Write `contracts/governance/Veto.sol` — Diamond-tier veto with 14-day appeal window. File: `contracts/governance/Veto.sol`.
4. Write `contracts/governance/TallyOracle.sol` — accepts encrypted tally inputs, releases plaintext result after vote close + threshold-of-decryptors. File: `contracts/governance/TallyOracle.sol`.
5. Write `services/tally-decryptor/` — off-chain threshold decryption service (3/5 of decryptors must agree on tally to publish). File: `services/tally-decryptor/main.go`.
6. Write `docs/economics/governance.md` documenting the full lifecycle with diagrams.
7. Verify via: (a) circuit test — invalid eligibility merkle proof rejected; (b) circuit test — double-vote with same nullifier rejected; (c) end-to-end test — pass a parameter change at 51%, confirm timelock + execute; (d) end-to-end test — Diamond veto pauses proposal, re-vote at 67% overrides, proposal executes; (e) adversarial test — voter attempts to construct receipt proving their vote, must fail.

## Spec deviation (if applicable)

None. Numbers match Bölüm 12.2 and Bölüm 8.5. The 14-day appeal window on Diamond veto and the threshold-decryption tally mechanism are implementation details not specified verbatim; both are consistent with spec principles (privacy + timelocks).

## References

- Spec: docs/spec/obscura_spec_v3.txt Bölüm 12.2, Bölüm 8.5
- Related ADRs: ADR-0010 (token), ADR-0011 (staking/slashing), ADR-0007 (MLS — Diamond-tier membership proofs)
- External:
  - Daian et al., "On-Chain Vote Buying and the Rise of Dark DAOs" (2018) — bribery threat model
  - Buterin, "Moving beyond coin voting governance" (2021) — case against pure plutocracy
  - Tornado Cash governance vote post-mortem (2023) — case study in token-weighted capture
  - Aragon ZK Voting research (zk-voting.org)
  - Compound Governor Bravo contracts (precedent for 48h timelock, quorum mechanics)
