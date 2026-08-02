# ADR 0018: Federation node registration requires Ed25519 signature (soft transition)

Date: 2026-08-02
Status: Accepted
Decider: user
Spec ref: spec 12.3 (permissionless node registration)

## Context

`federation.RegisterRequest.Sig` existed as a field since the permissionless
registration endpoint was built, documented as "Ed25519 signature of
node_id+peer_addr (hex)" — but `Register()` never read it, and the only
existing client (the manual frontend form at `frontend/app/nodes/page.tsx`)
never populated it. Anyone could `POST /v1/nodes/register` claiming any
`node_id`/`pubkey`/`vrf_pubkey` with zero proof of control over that key.

This became urgent after the VRF fairness fix (ADR-0017 adım 5):
`sequencer.handleIncomingVRFProof` trusts the `vrf_pubkey` recorded in
federation for a given `node_id` when verifying incoming VRF proofs. Without
signature verification on registration, an attacker could register a fake
`vrf_pubkey` for a victim's `node_id` (or simply squat an unclaimed one),
undermining the VRF integrity the previous ADR just fixed.

## Options considered

### Option A: Soft transition (chosen)

Verify `Sig` when present (reject invalid signatures outright); accept empty
`Sig` for now with a deprecation warning logged. Node itself self-registers
using its P2P identity key (Ed25519, already used for libp2p PeerID) at
startup, signed.

- Pros: doesn't break the 2 already-registered live nodes or the existing
  manual frontend form; ships incrementally; self-register makes the manual
  form mostly unnecessary going forward without removing it.
- Cons: unsigned registrations remain possible until a hard cutover decision
  is made later — the vulnerability isn't fully closed, only made optional
  to close.
- Effort: M
- Risk: Low

### Option B: Hard cutover

Require `Sig` immediately; reject any request without it.

- Pros: closes the gap completely, immediately.
- Cons: the 2 live Railway nodes were registered manually (no key ever
  signed anything) — they'd need manual re-registration with a real key
  before this ships, and the frontend form (raw text inputs, no signing UI)
  would need to be pulled or reworked first. No safe rollback path if
  something in the new self-register flow is subtly wrong.
- Effort: M (same core work) + coordination overhead
- Risk: Medium (live node drop-off if self-register has a bug)

## Decision

We chose **Option A** because the two live nodes and the existing frontend
form can't sign anything today — a hard cutover would immediately deregister
production nodes with no fallback.

## Rationale

The signature payload is `node_id|peer_addr|pubkey|vrf_pubkey|timestamp`.
`vrf_pubkey` is deliberately included: leaving it out would let an attacker
take a victim's *validly signed* `(node_id, peer_addr, pubkey)` registration
and swap in their own `vrf_pubkey` before submitting it — signature still
technically present, but for a different claim — silently defeating the
VRF proof-verification trust chain from ADR-0017. Including it in the signed
payload makes any post-signing tampering (of `vrf_pubkey`, or any other
field) invalidate the signature. `timestamp` is included with a ±5 minute
window to prevent replaying an old captured request.

We reuse the existing P2P identity key (`p2p/identity.go`,
`P2P_KEY_PATH`-backed Ed25519) rather than the VRF key or a new key: it's
already the node's cryptographic identity (determines its libp2p PeerID),
already persisted, and `NodeRecord.Pubkey`'s original doc comment
("node imza anahtarı, Ed25519") already assumed this key type. The VRF key
is P-256/ECDSA and serves a deliberately separate purpose (leader-election
randomness) — reusing it for identity signing would re-blur the exact
separation ADR-0017 adım 0 introduced.

## Consequences

- **Positive**: new/self-registering nodes now prove key ownership;
  `vrf_pubkey` can no longer be silently swapped onto someone else's
  registration; replay of old registration requests is bounded to 5 minutes.
- **Negative**: unsigned registration is still accepted — the endpoint is
  not yet fully permissionless-but-verified, only optionally so.
- **Neutral**: the manual frontend form still exists (now labeled as
  legacy/lower-trust) for third-party/external node operators who aren't
  running this codebase's `cmd/node` binary and thus have no self-register
  flow of their own.
- **Tech debt incurred**: hard cutover (rejecting empty `Sig`) is
  intentionally deferred. Remediation: once the 2 live nodes have gone
  through at least one self-registering restart (confirmed via logs —
  `📡 Federation self-register tamamlandı`), flip `Register()` to reject
  empty `Sig`. Estimated: small (one conditional removed), but requires a
  live-nodes check first.

## Implementation plan

1. `p2p/host.go` — `IdentityPubkeyHex()` + `SignWithIdentity()`, reusing the
   existing P2P identity key.
2. `federation/federation.go` — `RegisterRequest.Timestamp`,
   `SignaturePayload()`, `verifyRegistrationSignature()` (not yet wired).
3. `federation/federation.go` `Register()` — wire soft-transition check.
4. `federation/federation_test.go` — valid/invalid signer, tampered
   `vrf_pubkey` (the ADR-0017-bypass scenario), tampered other fields, old/
   future timestamp, missing timestamp, malformed pubkey/sig, empty-Sig
   legacy path, `SignaturePayload` field-sensitivity.
5. `cmd/node/main.go` — `selfRegisterFederation()`, called after P2P + VRF
   transport are both up.
6. `frontend/app/nodes/page.tsx` — manual form kept, labeled as legacy/
   lower-trust, points at automatic self-registration as the primary path.
7. This ADR.

Verify via: `go build ./...`, `go vet ./...`,
`go test ./internal/federation/... ./internal/sequencer/... ./internal/p2p/...`
(all green, 0 skips, no regressions to the ADR-0017 VRF fix work).

## References

- Related ADRs: ADR-0017 (BFT/VRF scope — the `vrf_pubkey` trust chain this
  protects)
- Spec: `docs/spec/obscura_spec_v3.txt:1280` ("Acik node kaydi (permissionless)")
