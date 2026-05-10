# ADR 0008: FAZ 1 (MVP) deliverable list complete

Date: 2026-05-10
Status: Accepted
Decider: project lead
Spec ref: Bölüm 12.1

## Context

Spec'in FAZ 1 (MVP) deliverable listesi (Bölüm 12.1) 10 madde içeriyordu.
Bu kararla FAZ 1'in deliverable listesi için "code-complete" durumu deklare ediliyor.

## Status of each deliverable

| # | Item | Implementation | Tests | Notes |
|---|------|----------------|-------|-------|
| 1 | 5 node kurulumu | docker-compose.yml | smoke (manual) | nginx LB önünde |
| 2 | E2EE Signal | X3DH backend + Double Ratchet TS browser | API test | Rust crate Signal kısmı FAZ 2 |
| 3 | MLS basic | openmls Rust + Go CLI subprocess + 9 endpoint | Rust 2 e2e + Go 3 e2e | Frontend WASM bridge yok |
| 4 | Flutter | DEFERRED — RN+Next+Tauri | n/a | ADR-0002 sapma |
| 5 | Phone verification | Twilio stub + OTP | API test | Üretim için gerçek SMS |
| 6 | Credit basic | Tier hesap + ZK upgrade | API + bench | ZK ile tier upgrade çalışıyor |
| 7 | ZK-ID basic | identity_proof.circom + BIP39 derivation | 5 Rust test | Recovery hazır |
| 8 | P2P call | TURN credentials | API test | coturn config var |
| 9 | Auto node selection | nginx least_conn + LB | manual | DHT FAZ 3'e |
| 10 | ZK Circom altyapı | 4 circuit + Groth16 verifier + dağıtım | 4 Go + 5 circuit | 9 ek circuit FAZ 2 |

## Performance success criteria (Bölüm 12.1)

| Criterion | Target | Result | Pass? |
|-----------|--------|--------|-------|
| ZK proof gen | <3s | 494ms (browser) | ✅ |
| ZK proof verify | <500ms | ~5ms | ✅ |
| ZK throughput | ≥100/s | 205/s single, 827/s parallel | ✅ |
| MLS encrypt 1000 mem | <100ms | 0.13ms | ✅ |
| MLS decrypt | <50ms | 0.109ms | ✅ |
| Node uptime 7d | %99.9 | n/a — prod yok | ⏳ |
| 10k user smoke | yes | n/a — prod yok | ⏳ |

Code-level metrics ALL PASS. Production ortamı (10k user, 7-day smoke) ayrı bir milestone — FAZ 1 GA.

## Accepted deviations (per ADRs 0002, 0003, 0004)

These are deliberate FAZ 1 shortcuts that DO NOT block FAZ 1 declaration:

- **Flutter**: ADR-0002 — Next.js + RN + Tauri instead. Permanent.
- **libp2p P2P**: ADR-0003 — HTTP gossip in FAZ 1. libp2p in FAZ 3.
- **Rust crypto crate**: ADR-0004 — Signal Protocol crypto in Go for FAZ 1. Move to Rust crate in FAZ 2.
  *Note*: openmls Rust crate for MLS is implemented (this FAZ 1 work), but X3DH/Double Ratchet still in Go.

## Decision

**FAZ 1 deliverable list is CODE-COMPLETE.**

Next milestone: **FAZ 1 GA** = production deploy + 7-day uptime + 10k user smoke.

Items NOT in FAZ 1 GA scope (but recommended before public launch):
- Multi-party trusted setup ceremony (replace dev .zkey files)
- Real SMS provider (Twilio/Netgsm)
- Real FCM service account
- SSL certificates (Let's Encrypt) on all domains
- nginx rate limiting tuning under realistic load
- Distributed tracing (OpenTelemetry)
- Backup/restore runbook tested
- Penetration test (external audit)

Move to FAZ 2 after these GA items.

## Consequences

- **Positive**: All FAZ 1 code-level gates met, ahead of spec on performance
- **Positive**: ZK pipeline + MLS foundation + multi-device + recovery all working
- **Negative**: Mobile + Frontend MLS WASM bridge still TODO (Rust crate only callable from backend currently)
- **Tech debt**:
  - mls_basic.rs deprecated — should be removed
  - Mobile RN doesn't have MLS yet (needs WASM or native bridge)
  - Frontend doesn't have MLS UI yet (handlers ready, UI not)
  - 8 ZK circuits still missing per spec Bölüm 17 (FAZ 2 work)

## Implementation summary

| Layer | LOC added (approx) | Status |
|-------|-------------------|--------|
| Rust crypto (mls + mnemonic + cli) | ~800 | Tests pass |
| Go backend (mls client + handlers + cross-sign + credit upgrade + zk verifier) | ~1500 | Build clean, tests pass |
| ZK circuits | ~80 (storage_proof) | Compiles + e2e verify |
| Migrations (DB) | ~13 new schemas | Idempotent |
| Routes registered | 16 new endpoints | Wired to main.go |

## References

- Spec FAZ 1: docs/spec/obscura_spec_v3.txt Bölüm 12.1
- ADR-0002 (Flutter sapma)
- ADR-0003 (libp2p gossip)
- ADR-0004 (Go crypto)
- ADR-0007 (openmls)
- Session: docs/sessions/2026-05-10-faz1-completion.md
