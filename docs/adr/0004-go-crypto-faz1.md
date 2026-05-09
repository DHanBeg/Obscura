# ADR 0004: Crypto in Go for FAZ 1, migrate to Rust crate later

Date: 2026-05-03
Status: Accepted
Decider: project lead
Spec ref: Bölüm 2.1 MODUL C (Crypto Layer Rust), Bölüm 14.1

## Context

Spec mandates a Rust crate `obscura-crypto` for all cryptographic operations. Reasoning:
- Memory safety
- Performance for ZK ops
- Mature crypto ecosystem (libsignal-protocol-rust, openmls, arkworks)
- FFI to all clients

For FAZ 1 MVP we used Go's stdlib + small Go crypto helpers because:
- Backend is Go anyway, no FFI bridge needed
- Faster initial development
- All client crypto already happens in browser/RN via TypeScript libs (libsignal-protocol-typescript)

## Options considered

### Option A: Build Rust crate first, Go FFI from day 1
- Spec-compliant
- Pros: No migration debt
- Cons: 4-6 weeks just on crate + FFI before any feature; FFI bugs are hard
- Effort: XL

### Option B: Go-native crypto for FAZ 1, Rust crate in FAZ 2 (chosen)
- Use Go stdlib for current minimal server-side crypto (mostly key handling, not core E2EE)
- Real E2EE happens in clients via TS libs
- Pros: Ships fast, defers complexity until E2EE on server actually needed
- Cons: Will need rewrite when MLS or server-side crypto verification grows

### Option C: Skip Rust entirely
- Pros: Less complexity
- Cons: Spec violation, performance ceiling for ZK ops, no MLS Go impl ready

## Decision

**Option B**: Go for FAZ 1, Rust crate `obscura-crypto` in FAZ 2 alongside MLS group support.

## Rationale

- Server today does very little crypto — mostly stores public material (prekeys, signatures) and verifies them
- Heavy crypto (Signal Protocol session, ratchet, MLS) lives on clients
- Rust + FFI is justified when adding ZK proof generation server-side and MLS group state management
- Premature Rust + FFI adds complexity without value while server crypto is light

## Consequences

- **Positive**: FAZ 1 ships faster
- **Negative**: Code-reviewer must flag any new Go crypto code growing scope; spec-checker reports this as known deviation
- **Tech debt**: ~3-4 weeks to build Rust crate when needed (FAZ 2)

## Triggers to migrate

Migrate to Rust crate when ANY of these hits:
- MLS group encryption needed server-side (group state management)
- ZK proof generation needed server-side (currently client-only)
- Signal Protocol session storage needs server validation
- Performance bottleneck identified in Go crypto

## Migration plan

1. Create `crypto/` Rust crate with workspace + features
2. Implement first feature in Rust (likely MLS group state)
3. Build FFI: subprocess + JSON RPC (avoids CGO; matches CGO_ENABLED=0 rule)
4. Move Signal Protocol server-side helpers gradually
5. Eventually: client SDKs also share the Rust core via WASM/JNI/etc.

## References

- libsignal-protocol-rust: https://github.com/signalapp/libsignal
- openmls: https://github.com/openmls/openmls
- Spec: docs/spec/obscura_spec_v3.txt Bölüm 2 MODUL C
