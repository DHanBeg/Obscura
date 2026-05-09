# ADR 0007: Use openmls (RFC 9420) for group encryption, deprecate custom mls_basic

Date: 2026-05-10
Status: Accepted
Decider: project lead (per spec mandate)
Spec ref: Bölüm 2.1 MODUL C, Bölüm 6.3, Bölüm 14.1

## Context

Spec mandates MLS (Messaging Layer Security, RFC 9420) for group encryption with:
- TreeKEM key distribution
- KeyPackage per member (90-day rotation)
- Welcome/Commit messages
- Forward + post-compromise security
- Support for 10,000+ member groups

Current state: `crypto/src/mls_basic.rs` is a placeholder using HKDF + AES-GCM with epoch-based group keys. Author's TODO comment: "Üretimde openmls crate'e geçiş yapılacak (RFC 9420 tam uyum)".

## Options considered

### Option A: Continue building custom mls_basic
- Pros: No new dependency, simpler
- Cons: NOT spec-compliant, not RFC 9420, custom crypto = unaudited, won't interop with other MLS clients
- Effort: M (already started)
- Risk: HIGH (custom crypto)

### Option B: openmls (chosen)
- Pros: Reference implementation of RFC 9420, audited, used by Wire/Phoenix, formally analyzed
- Cons: Larger dependency tree (~200 deps), steeper learning curve, complex API
- Effort: L (full integration), but spec-mandated
- Risk: LOW (battle-tested)

### Option C: Wire's mls-rs (AWS)
- Pros: Also RFC 9420, performance-tuned
- Cons: AWS-influenced, less open-source momentum
- Effort: L

## Decision

**Option B: openmls**.

Spec rule "en iyisini kullan, alternatife yönelme" aligns with openmls — it's the de-facto reference implementation.

## Rationale

- Spec explicitly names `openmls` (Bölüm 14.1)
- Custom MLS = custom crypto = audit nightmare + interop loss
- Future MLS clients (third-party Obscura clients in any language) need RFC 9420 wire format
- openmls maintained by ETH Zurich + RFC authors

## Consequences

- **Positive**: Spec-compliant, audited, RFC 9420 wire format
- **Negative**: ~200 transitive dependencies, longer build, more code surface to learn
- **Tech debt**: `crypto/src/mls_basic.rs` deprecated; remove after openmls module proven

## Migration plan

### Phase 1 — openmls integration (this work)
- [ ] Add openmls to Cargo.toml
- [ ] crypto/src/mls/ new module structure:
  - mod.rs — public API
  - storage.rs — Memory + file-backed key storage
  - errors.rs — error types
- [ ] Define ciphersuite: MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519
- [ ] Implement minimal API surface:
  - `generate_credential(did) -> Credential`
  - `generate_key_package(credential) -> KeyPackage`
  - `create_group(self_did, group_name) -> Group`
  - `add_member(group, key_package) -> (Welcome, Commit)`
  - `process_welcome(welcome, key_package) -> Group`
  - `process_commit(group, commit) -> Group`
  - `encrypt_message(group, plaintext) -> MLSMessage`
  - `decrypt_message(group, ciphertext) -> plaintext`

### Phase 2 — FFI exports
- [ ] crypto/src/ffi.rs — JSON-RPC over CFFI for above functions
- [ ] All complex types serialized as base64 strings or JSON

### Phase 3 — Backend integration
- [ ] DB schema:
  - mls_key_packages (per user, rotated)
  - mls_groups (group state, encrypted blob)
  - mls_pending_proposals
- [ ] API endpoints (per spec Bölüm 17 EK B):
  - POST /v1/mls/group — create group
  - POST /v1/mls/key-package — upload self KeyPackage
  - GET /v1/mls/key-package/{did} — fetch peer KeyPackage
  - POST /v1/mls/join — process Welcome
  - POST /v1/mls/commit — submit Commit, server broadcasts to members
  - GET /v1/mls/group/{id}/state — fetch encrypted state
- [ ] WebSocket message types: mls_welcome, mls_commit, mls_message

### Phase 4 — Frontend integration
- [ ] frontend/lib/mls.ts — wraps WASM build of openmls
- [ ] mobile/lib/mls.ts — same via FFI
- [ ] Group chat UI: create, invite, send, receive

### Phase 5 — Cleanup
- [ ] Remove crypto/src/mls_basic.rs after openmls path proven
- [ ] Migrate any test groups to openmls (data migration plan)

## Ciphersuite choice

`MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519` (ID: 0x0001)

Reasons:
- All primitives match what we already use elsewhere (X25519, Ed25519, AES-GCM, SHA256)
- 128-bit security level (sufficient for FAZ 1-3)
- Widely supported across MLS implementations (interop)
- Smaller key sizes than ECDSA-P256

## Performance targets (from spec Bölüm 15.2)

- MLS group encrypt (1000 members): <100ms
- MLS group decrypt: <50ms
- 10,000+ member groups supported

## Storage size estimates (per group)

- Initial state: ~5KB per member (KeyPackage + leaf node)
- Per epoch transition: +~1KB (Commit + path)
- 1000-member group: ~5MB initial, +1KB per change
- Periodic GC of old epochs (keep last N for late delivery)

## Auditability

openmls is:
- Reference implementation by RFC 9420 authors (Raphael Robert, ETH Zurich)
- Used in Wire (commercial messenger) since 2023
- Public security audit by NCC Group (2023)
- Formal analysis ongoing (Tamarin prover)

## References

- RFC 9420 (MLS): https://datatracker.ietf.org/doc/rfc9420/
- openmls: https://github.com/openmls/openmls
- openmls book: https://book.openmls.tech/
- NCC audit: https://research.nccgroup.com/2023/06/16/public-report-mls-protocol-security-assessment/
- Spec: docs/spec/obscura_spec_v3.txt Bölüm 4.3, 6.3, 14.1
