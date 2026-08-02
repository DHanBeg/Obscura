# Architecture Decision Records

Sequential 4-digit-numbered records of significant architectural decisions.

## Index

| # | Title | Status | Date |
|---|-------|--------|------|
| 0001 | [Use modernc.org/sqlite over CGO SQLite](0001-modernc-sqlite.md) | Accepted | 2026-04-30 |
| 0002 | [Defer Flutter, ship Next.js + Expo + Tauri instead](0002-no-flutter.md) | Accepted | 2026-05-01 |
| 0003 | [HTTP gossip for FAZ 1, libp2p later](0003-http-gossip-mvp.md) | Accepted | 2026-05-02 |
| 0004 | [Crypto in Go for FAZ 1, migrate to Rust crate later](0004-go-crypto-faz1.md) | Accepted | 2026-05-03 |
| 0005 | [Choose Aztec for zk-Rollup over zkSync/StarkNet](0005-aztec-rollup.md) | Accepted | 2026-05-09 |
| 0006 | [Single-contributor trusted setup for dev, multi-party for prod](0006-dev-trusted-setup.md) | Accepted | 2026-05-10 |
| 0007 | [Use openmls (RFC 9420) for group encryption](0007-openmls-for-groups.md) | Accepted | 2026-05-10 |
| 0008 | [FAZ 1 (MVP) deliverable list complete](0008-faz1-complete.md) | Accepted | 2026-05-10 |
| 0009 | [FAZ 1 post-audit hardening (6 critical fixes)](0009-faz1-post-audit-hardening.md) | Accepted | 2026-05-10 |
| 0010 | [OBS token economics (supply, distribution, inflation, burn)](0010-obs-token-economics.md) | Proposed | 2026-05-13 |
| 0011 | [Staking and slashing parameters](0011-staking-slashing.md) | Proposed | 2026-05-13 |
| 0012 | [Governance mechanism (ZK voting, eligibility, quorum, veto)](0012-governance.md) | Proposed | 2026-05-13 |
| 0013 | [ZK-ML moderation approach (hybrid heuristic + ezkl)](0013-zkml-moderation.md) | Accepted | 2026-05-16 |
| 0014 | [JSON message format (proto3 deferred)](0014-json-over-proto3.md) | Accepted | 2026-05-17 |
| 0015 | [Continue with Groth16 (PLONK/STARK deferred)](0015-groth16-over-plonk-stark.md) | Accepted | 2026-05-17 |
| 0016 | [Sealed-sender threat model clarification](0016-sealed-sender-threat-model.md) | Accepted | 2026-07-17 |
| 0017 | [BFT consensus real scope — OBS ledger agreement, not governance](0017-bft-consensus-scope.md) | Accepted | 2026-08-02 |
| 0018 | [Federation node registration requires Ed25519 signature (soft transition)](0018-federation-registration-signature.md) | Accepted | 2026-08-02 |

## Process

See `.claude/skills/adr-template/SKILL.md` for template and rules.
