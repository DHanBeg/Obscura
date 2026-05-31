# ADR 0014: JSON message format (proto3 deferred)

Date: 2026-05-17
Status: Accepted
Decider: project lead
Spec ref: Bölüm 14 EK A (proto3 mesaj formatı önerisi), Bölüm 17 EK B (API endpoint listesi)

## Context

The spec (EK A) proposes proto3 as the wire format for inter-node and client–server
messages, citing bandwidth efficiency and schema enforcement. The current implementation
uses JSON throughout: all HTTP handlers return `{"success": true, "data": ...}` or
`{"success": false, "error": "..."}`, WebSocket frames are JSON objects, and ZK proof
payloads (snarkJS output) are JSON by definition.

As of 2026-05-17 the codebase has 66+ HTTP handlers across eight handler files, a
full frontend (`lib/api.ts`) and mobile (`mobile/lib/api.ts`) client layer, plus
snarkJS WASM proof generation in the browser. Every layer speaks JSON.

## Options considered

### Option A: Migrate to proto3 (spec recommendation)
- Define `.proto` schemas for every message type; generate Go, TypeScript, and
  React Native stubs.
- Replace `encoding/json` with `google.golang.org/protobuf` in all handlers.
- Add a serialization adapter between snarkJS JSON proofs and proto3 bytes.
- Pros: ~3–5× smaller payloads; faster parse on constrained mobile devices;
  schema enforced at compile time.
- Cons: All 66+ handlers rewritten; frontend and mobile API clients rewritten;
  snarkJS ZK proof objects need a bespoke adapter layer; binary wire format
  requires dedicated tooling for log inspection and debugging.
- Effort: XL (estimated 4–6 weeks)
- Risk: High — regression surface spans every feature already shipped in FAZ 1 and FAZ 2.

### Option B: Hybrid — proto3 for node-to-node gossip, JSON for client-facing API
- Keep HTTP REST + WebSocket JSON for clients; use proto3 only on
  `POST /v1/internal/relay` between nodes.
- Pros: Bandwidth saving where it matters most (high-volume internal relay); client
  code untouched.
- Cons: Two serialization paths to maintain; proto3 gossip objects must be
  transcoded to JSON before forwarding to WebSocket clients — adds latency and a
  new failure surface. Complexity is high relative to current node count (5 nodes, FAZ 1).
- Effort: L
- Risk: Medium

### Option C: Keep JSON (chosen)
- No changes to wire format. All handlers, clients, and ZK paths continue to
  use `encoding/json`.
- Pros: Zero migration risk; debugging stays curl-friendly; snarkJS output slots
  directly into request bodies; CGO_ENABLED=0 constraint is trivially satisfied
  (some proto Go runtimes require CGO or bring in heavy reflection stubs).
- Cons: Larger payloads than proto3 (~3–5×); no compile-time schema enforcement
  across language boundaries.
- Effort: S (no work)
- Risk: Low

## Decision

**Option C.** JSON is the wire format for all client–server and node-to-node
communication until a dedicated migration ADR is accepted.

## Rationale

- **Migration cost is prohibitive relative to current scale.** There are no
  production users. Rewriting 66+ handlers and both client layers to save
  bandwidth at zero DAU is not a productive trade.
- **snarkJS proof objects are JSON.** Groth16 proofs produced by snarkJS
  (`pi_a`, `pi_b`, `pi_c`, `publicSignals`) are JSON arrays. A proto3 adapter
  would be a non-trivial piece of serialization glue with no spec guidance on
  the canonical encoding. The risk of a subtle encoding mismatch invalidating
  proof verification is unacceptable.
- **CGO_ENABLED=0 is a hard constraint.** The pure-Go proto runtime
  (`google.golang.org/protobuf`) is CGO-free, but some code-generation
  plugins pull in `github.com/golang/protobuf` v1 shims that historically
  had CGO paths. Auditing and locking the full proto dependency tree adds
  supply-chain risk that the current JSON approach does not have.
- **Observability.** JSON logs are readable without extra tooling. At the
  current stage (pre-production, active development), this matters more than
  wire efficiency.
- **The spec recommends, not mandates.** EK A says proto3 is "önerilen" —
  it is listed as a guideline, not a deliverable in any of the four phase
  checklists (Bölüm 12.1–12.4).

## Consequences

- **Positive**: No handler, client, or ZK adapter code changes. All FAZ 1 and
  FAZ 2 work remains valid. Debugging stays straightforward.
- **Negative**: Payload sizes remain larger than necessary. At 10k+ DAU the
  bandwidth difference becomes measurable, particularly for WebSocket message
  streams.
- **Tech debt**: If DAU crosses 10k or node count exceeds 20, revisit this
  ADR. At that point the most impactful migration is likely node-to-node gossip
  first (Option B above), not the full client-facing API.

## Migration trigger criteria

Revisit this decision when **any one** of the following is true:

| Trigger | Threshold |
|---------|-----------|
| Production DAU | > 10,000 |
| Active federation nodes | > 20 |
| p99 WebSocket message latency | > 200 ms and profiling attributes > 30% to JSON serialization |
| Mobile data usage complaints | Sustained user reports or app store reviews citing data consumption |

## References

- Spec: docs/spec/obscura_spec_v3.txt EK A, Bölüm 17 EK B
- google.golang.org/protobuf (CGO-free Go proto runtime): https://pkg.go.dev/google.golang.org/protobuf
- snarkJS proof format: https://github.com/iden3/snarkjs#generate-a-proof
- ADR 0004 (Go crypto for FAZ 1, CGO constraint): docs/adr/0004-go-crypto-faz1.md
