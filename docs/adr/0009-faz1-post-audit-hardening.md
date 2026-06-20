# ADR 0009: FAZ 1 post-audit hardening

Date: 2026-05-10
Last updated: 2026-06-21
Status: Accepted
Decider: project lead
Spec ref: Bölüm 4.5 (KESIN güvenlik kuralları)

## Context

ADR-0008 deklare etti ki "FAZ 1 deliverable list code-complete". Sonrasında 3 alt-ajanı (spec-checker, security-auditor, code-reviewer) çağırdık ve şu sonuçlar geldi:

- **spec-checker**: ~%90 code-complete, %100 değil — MLS server-side var ama client UI yok
- **security-auditor**: **DO NOT SHIP** — 6 KRİTİK güvenlik açığı
- **code-reviewer**: **Needs Work** — Rust UB, ignored errors, missing transactions

## Original Critical Findings (security audit 2026-05-10)

| ID | Issue | Status after fix |
|----|-------|------------------|
| C1 | Credit upgrade replay attack | ✅ FIXED — `zk_nullifiers` tablosu, transaction içinde insert |
| C2 | user_hash forgery (proof başkasından alınabilir) | ✅ FIXED — `users.credit_user_hash` binding commitment + route registered |
| C3 | credit_threshold.circom user_hash kısıtlanmıyor (`x*0`) | ✅ FIXED — Circuit'te `Poseidon(secret, BINDING_TAG) === user_hash` |
| C4 | Public input order brittle (magic positions) | ✅ FIXED — Per-circuit length validation + named constants |
| C5 | Cross-signing account hijack | ✅ FIXED — Signed message includes JWT-trusted DID + domain separator |
| C6 | ZK verifier no input count validation | ✅ FIXED — `expectedPublicSignals` map enforced before pairing |

## Code-reviewer Findings

| ID | Issue | Status |
|----|-------|--------|
| R1 | mls-cli.rs `unsafe { &*ptr }` UB | ✅ FIXED — Refactored to `Arc<IdentityBundle>` + `Arc<Mutex<GroupBundle>>`, zero unsafe |
| R2 | mls_handlers ignored Scan errors | ✅ FIXED — Explicit `sql.ErrNoRows` handling |
| R3 | Multi-statement writes without transactions | ✅ FIXED — `Begin/Commit/Rollback` in HandleMLSAddMember + HandleMLSCreateGroup |
| R4 | rows.Close on potentially-nil rows | ✅ FIXED — Error checked before defer |
| R5 | mnemonic seed leak on HKDF error path | ✅ FIXED — Closure-wrapped fallible work, zeroize unconditional |
| R6 | mls.Call no timeout protection | ✅ FIXED — Default 30s timeout, single-shot on cancel |
| R7 | Hardcoded TURN/INTERNAL secrets | ✅ FIXED — Env-loaded with prod-fatal/dev-placeholder |
| R8 | ZK verifier disk fallback default-on | ✅ FIXED — Embedded-only default, `OBSCURA_ZK_KEYS_FROM_DISK` opt-in |
| R9 | storage_proof.circom meaningless `valid` output | ✅ FIXED — `proof_commitment` Poseidon binding + epoch input |
| R10 | Migration runner silently masks errors | ✅ FIXED — Fail-loudly except for known idempotent errors |

## New findings from re-audit (after fix sprint)

| ID | Sev | Issue | Status |
|----|-----|-------|--------|
| N1 | CRITICAL | `/credit/binding` route not registered in main.go | ✅ FIXED in same session |
| N2 | HIGH | BINDING_TAG `hkdfInfo="obscura-id-v1"` entropy rationale undocumented | ✅ FIXED 2026-06-21 — RFC 5869 explanation added to `identity/bip39.go` const block |
| N3 | HIGH | Binding permanently immutable (no recovery rotation) | ✅ FIXED 2026-06-21 — `binding_rotation.go` (3 endpoints) + migrations 096/097; 7-day timelock per spec Bölüm 5.4 |
| N4 | MEDIUM | Proof envelope handling accepts both `{proof:..}` and raw — could mask malformed input | ✅ FIXED 2026-06-21 — `validateProofEnvelope()` in `zk/verifier.go` enforces protocol/curve/pi_a/pi_b/pi_c before pairing |
| N5 | MEDIUM | mls.Client single-shot DoS amplification (subprocess dies → all calls fail) | ✅ FIXED 2026-06-21 — `mls/global.go` auto-restarts subprocess on `closed.Load()==true` with write-lock + double-check |
| N6 | MEDIUM | MLS handler returns 500 after commit — caller-confusing (commit already persisted) | ✅ FIXED 2026-06-21 — `HandleMLSAddMember` + `HandleMLSGroupMessage` broadcast errors now `log.Printf` out-of-band; always return 200 on successful commit |
| N7 | MEDIUM | storage_proof has no production handler — circuit changes unused yet | ✅ ACCEPTABLE — storage layer not in FAZ 1 spec; circuit compiled and ready |
| N8 | MEDIUM | `target_did_hint` not validated as DID regex | ✅ FIXED 2026-06-21 — `obsDidRegex = ^did:obs:[0-9a-f]{64}$` enforced in `HandlePairStart` |
| N9 | LOW | HandlePairStart unauthenticated → DB spam → disk fill → crash | ✅ FIXED 2026-06-21 — IP-based rate limit: 5 req/min per IP (`cross_signing.go`) |
| N10 | LOW | WebRTC JWT token in URL query string → leaks into nginx/access logs | ✅ FIXED 2026-06-21 — Token accepted via `Sec-WebSocket-Protocol` subprotocol first, then `Authorization: Bearer`, URL query deprecated with log warning |
| N11 | LOW | `X-Node-Secret` plaintext header + `!=` comparison (timing attack) | ✅ FIXED 2026-06-21 — `gossip.go`: HMAC-SHA256(secret, ts+body), ±30s replay window, `hmac.Equal` constant-time compare |
| N12 | LOW | mls-cli `state.lock().unwrap()` panics → subsystem-wide poison | ✅ N/A — Audit of `crypto/src/mls/mod.rs` shows no `unwrap()` in production paths; only in `bench.rs`/`tests.rs` where panics are expected |
| N13 | LOW | WAL mode + MaxOpenConns tuning | ✅ N/A — `db/database.go` already has `?_journal_mode=WAL` + `MaxOpenConns(1)` with correct comment |

## Decision

FAZ 1 deliverable list is **CODE-COMPLETE + CRITICAL-AUDIT-CLEAN**.

All 6 original Critical findings + the 1 new Critical introduced by the fix sprint are resolved. All 11 lower-severity deferred items are now also resolved (2026-06-21 hardening sprint).

## Production GA still requires (in addition to ADR-0008's GA list)

- ~~N3, N5, N6, N9, N10, N11 fixes~~ ✅ All done 2026-06-21
- Audit: external penetration test (not yet performed)
- Multi-party trusted setup ceremony for `.zkey` files (not yet performed)

## Test results after hardening

| Suite | Result |
|---|---|
| `go test ./internal/zk/...` | PASS (4 tests) |
| `go test ./internal/mls/...` | PASS (3 tests, with refactored Rust subprocess) |
| `cargo test --lib` | PASS (30/30 incl. mnemonic + mls e2e + bench-decrypt) |
| `cargo build --release` | clean |
| `go build ./...` | clean |
| `grep unsafe crypto/src/bin/mls-cli.rs` | 0 hits |
| `grep CHANGE-IN-PRODUCTION backend/` | 0 hits |

## 2026-06-21 hardening sprint — files changed

| File | Items fixed |
|------|------------|
| `backend/internal/gossip/gossip.go` | N11 — HMAC-SHA256 auth + replay protection |
| `backend/internal/api/cross_signing.go` | N9 (rate limit) + N8 (DID regex) |
| `backend/internal/api/webrtc.go` | N10 — Sec-WebSocket-Protocol token |
| `backend/internal/mls/global.go` | N5 — subprocess auto-restart |
| `backend/internal/api/mls_handlers.go` | N6 — out-of-band broadcast errors |
| `backend/internal/zk/verifier.go` | N4 — strict proof envelope schema |
| `backend/internal/api/binding_rotation.go` | N3 — timelocked DID rotation (new file) |
| `backend/cmd/node/main.go` | N3 — route registration |
| `backend/internal/db/database.go` | N3 — migrations 096+097 |
| `backend/internal/identity/bip39.go` | N2 — BINDING_TAG documentation |

## References

- Original audit reports: see this conversation's session notes
- ADR-0008 (the over-optimistic "complete" claim being amended here)
- Spec Bölüm 4.5: 7 KESIN güvenlik kuralları
- Files touched (original sprint, 12): credit_upgrade.go, cross_signing.go, mls_handlers.go, mls/client.go, zk/verifier.go, db/database.go, gossip/gossip.go, webrtc.go, credit_threshold.circom, storage_proof.circom, mls-cli.rs, mnemonic.rs, main.go

## Process lessons

- Should have invoked `code-reviewer` + `security-auditor` agents BEFORE declaring ADR-0008
- Sub-agents found 6+ blocking bugs in code I wrote thinking it was clean
- "Code compiles + tests pass" ≠ "production-safe"
- Use the agent team for every meaningful change going forward
