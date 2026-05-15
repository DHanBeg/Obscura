# ADR 0013: ZK-ML Content Moderation Approach

Date: 2026-05-16
Status: Accepted
Decider: user
Spec ref: Bölüm 12.2 (ZK-ML içerik filtreleme — temel)

## Context

Obscura is end-to-end encrypted. The server never reads plaintext. Yet spam,
abuse, and coordinated harassment are real failure modes for any messaging
platform; if the server cannot moderate plaintext, *some* signal must still
exist to throttle the obvious cases (mass-fanout spam, repeated reports against
the same DID, ciphertext anomalies that betray non-message payloads).

The spec (Bölüm 12.2) calls for "ZK-ML içerik filtreleme (temel)": a model
runs client-side, and the client submits a zero-knowledge proof that the model
classified its plaintext as non-spam. The server verifies the proof, never the
plaintext. This is the right destination, but it is research-grade today.

This ADR locks the path from "no ML at all" (FAZ 1) to "ZK-proven client-side
inference" (FAZ 3), with a defensible intermediate stage.

## Options considered

### Option A: ezkl (Python + Rust, ONNX → halo2)
- Pros: Production-oriented, ONNX import, growing ecosystem, halo2 backend has
  good tooling.
- Cons: Proof generation is seconds-to-minutes per inference even for tiny
  models. Per-message inference at MVP scale (target ~10 msg/s/node) is not
  remotely feasible. Circuit size scales with model size; quantization is
  required and lossy.
- Maturity: Research → early production. Used by Worldcoin, Modulus Labs.

### Option B: RISC0 (general zkVM)
- Pros: Run arbitrary Rust including ONNX runtimes; no per-model circuit
  authoring.
- Cons: Even higher prover overhead than ezkl for ML workloads (general zkVM
  is the wrong shape for matrix multiplies). Worse fit for the specific task.
- Maturity: Production zkVM, but ML usage is exploratory.

### Option C: TF-Encrypted / homomorphic inference
- Pros: Server-side inference on ciphertext, no proof needed.
- Cons: FHE inference is orders of magnitude slower than ZK-ML and requires
  the server to hold a shared key, which **breaks our threat model** (server
  must not be able to decrypt). Hard pass.

### Option D: Heuristic baseline only (ciphertext metadata)
- Pros: Zero new dependencies, runs in microseconds, works today.
- Cons: Not "ML" in any meaningful sense; easy to evade once attackers know
  the heuristics. Useful as a baseline floor, not as the long-term answer.

### Option E (chosen): Hybrid — Option D now, Option A later

Server-side heuristic baseline on **ciphertext metadata only** (length,
Shannon entropy, repetition rate, fanout) gates the obvious abuse cases for
FAZ 2. The `moderation.Score` interface is shaped so the heuristic
implementation can be swapped for a ZK-verifier call without touching
callers. Client-side ONNX inference ships in FAZ 2 GA. ZK-proof of inference
(ezkl) layers in during FAZ 3 once prover performance is acceptable for the
target throughput, or once we accept a sampling regime ("prove 1 in N
messages").

## Decision

**Option E.** Heuristic-on-ciphertext-metadata baseline now, behind a
`moderation.Score` interface that returns a `float64` in `[0.0, 1.0]`. ZK-ML
deferred to FAZ 3.

## Rationale

- ezkl per-message proving is infeasible at MVP throughput. Shipping it as
  the only path would block the moderation feature on a research dependency.
- Heuristics on ciphertext metadata cannot read plaintext but *can* detect
  the failure modes that matter most at MVP scale: a single DID fanning out
  identical 32-byte payloads to thousands of recipients is spam regardless of
  what the payload decrypts to.
- The interface boundary (`Score(ctx, ciphertext, metadata) → float64`) is
  the same whether the implementation is a heuristic, a server-side ML
  model running on metadata, or a ZK-verifier checking a client-supplied
  proof. Migration cost is contained.
- Client-side ONNX inference (Phase 2) gives us the user-experience benefit
  ("your message looks like spam, are you sure?") without any server trust
  shift, and lets us collect telemetry on false-positive rates before
  committing to a ZK circuit.

## Migration plan

| Phase | When | What |
|-------|------|------|
| 0 (now) | FAZ 2 | Heuristic baseline on ciphertext metadata. `Score()` returns entropy/length/repetition composite. Wired into `HandleSpamReport`. |
| 1 | FAZ 2 GA | Ship ONNX model in client (web + mobile). Client warns user before sending. Server still uses heuristic only. Collect labelled false-positive data. |
| 2 | FAZ 3 | Add ezkl ZK-proof-of-inference path. Client submits proof alongside ciphertext for a sampled fraction of messages. Server verifies. |
| 3 | FAZ 3 GA | If prover perf allows, mandate ZK-proof for all messages from low-reputation DIDs. Heuristic remains as fallback for legacy clients. |

## Risks

| Risk | Mitigation |
|------|------------|
| Heuristic false-positive rate too high | Threshold is configurable; default 0.7 conservative. Score is advisory, not blocking, in Phase 0. |
| ezkl never reaches production perf | Fall back to client-side ONNX + reputation gating; ZK proof becomes opt-in trust signal, not mandatory. |
| Attacker reverse-engineers heuristic | Heuristic is a floor, not the only line of defense. Combined with rate limits, reputation, and (later) ZK-ML. |
| Model drift / adversarial inputs | Phase 1 telemetry feeds retraining. Model versioning baked into client release cadence. |

## Consequences

- **Positive**: Moderation surface exists today; spec promise (Bölüm 12.2) is
  partially fulfilled at FAZ 2. Interface is stable; downstream callers don't
  change when implementation evolves.
- **Negative**: "ZK-ML" label on the FAZ 2 deliverable is aspirational — what
  actually ships is heuristic + interface scaffold. Document this honestly.
- **Tech debt**: When ZK-ML lands, the heuristic stays as the fast-path
  pre-filter (cheap check first; only run expensive proof verification on
  borderline cases). Not removed.

## References

- ezkl: https://github.com/zkonduit/ezkl
- RISC0: https://www.risczero.com/
- Spec: docs/spec/obscura_spec_v3.txt Bölüm 12.2
- Modulus Labs benchmarks (ZK-ML perf reality check): https://medium.com/@ModulusLabs/
