---
name: obscura-spec-conformance
description: Verify Obscura code matches spec v3.0 deliverables. Use whenever evaluating "is feature X done" or before claiming any phase completion.
---

# Obscura Spec Conformance Check

## When to use

- Before claiming "FAZ N is done"
- Before merging a feature branch
- During session start to validate prior state
- When user asks "what's actually finished"

## Process

### Step 1: Locate the spec section

Open `docs/spec/obscura_spec_v3.txt` and find:
- **Feature definitions**: Bölüm 1-11 (architecture, modules, messaging, credit, token, client, mini app, physical)
- **Phase deliverables**: Bölüm 12.1-12.4 (per-phase checklists)
- **API contract**: Bölüm 17 EK B (endpoint catalog)
- **Test criteria**: Bölüm 15

### Step 2: List required deliverables

For phase claims, copy the exact `[x]` list from the spec phase section. Example FAZ 1 (Bölüm 12.1):

```
[x] 5 node kurulumu (Türkiye)
[x] E2EE mesajlaşma (Signal)
[x] MLS grup mesajlaşma (temel)
[x] Flutter client (5 platform)
[x] Telefon doğrulama
[x] Kredi puanı sistemi (temel)
[x] ZK-ID kimlik sistemi (temel)
[x] P2P sesli arama
[x] Otomatik node seçimi
[x] ZK proof altyapısı (Circom devreye)
```

### Step 3: For each deliverable, verify

Run grep + read files:

```bash
# Find handler implementation
grep -r "MLS\|mls" D:/obscura/backend/internal/

# Check route registration
grep "v1/mls" D:/obscura/backend/cmd/node/main.go

# Check if test exists
find D:/obscura -name "*_test.go" | xargs grep -l "TestMLS"

# Check if wired to UI
grep -r "createMLSGroup\|mls" D:/obscura/frontend/lib/

# Check if not stubbed
grep -A5 "func.*MLS" D:/obscura/backend/**/*.go | grep -E "TODO|panic|unimplemented"
```

### Step 4: Mark each deliverable

| Status | Meaning |
|--------|---------|
| ✅ Done | Code present, route wired, tests exist, not stubbed |
| 🟡 Partial | Skeleton/stub exists OR only one platform OR no tests |
| ❌ Missing | No code, no route, no test |

### Step 5: Output format

```markdown
## Spec Conformance Check: FAZ N (Bölüm 12.N)

| # | Deliverable | Status | Evidence |
|---|-------------|--------|----------|
| 1 | E2EE Signal | ✅ | backend/internal/api/keys.go:42, frontend/lib/e2ee.ts |
| 2 | MLS basic | ❌ | no match for "openmls" or TreeKEM |
| 3 | Flutter client | ❌ | mobile/ uses React Native (DEVIATION, not Flutter) |
| 4 | ZK Circom | 🟡 | 3 of 12 spec circuits exist, .zkey artifacts not built |

## Phase Completion: 4/10 deliverables done = 40%
## Verdict: NOT COMPLETE — do not mark FAZ 1 as done
## Required to ship FAZ 1
- Implement MLS (Bölüm 6.3 group flow + Bölüm 17 EK A MLSMessage)
- Either: (a) build Flutter client, OR (b) accept deviation in CLAUDE.md
- Build remaining ZK circuits: token_balance, vote_proof, storage_proof, age, activity, node, msg_count, endorsement, streak
- Run trusted setup ceremony for all circuits
- Achieve spec performance targets (Bölüm 15.2): ZK proof gen <3s, verify <500ms
```

## Accepted deviations (don't flag as bugs)

These are documented in CLAUDE.md "Accepted deviations":

- Crypto in Go instead of Rust crate `obscura-crypto` (FAZ 1 deviation)
- HTTP gossip instead of libp2p+GossipSub+DHT (FAZ 1 deviation)
- React Native + Next.js + Tauri instead of Flutter (multi-client deviation)
- modernc.org/sqlite instead of CGO SQLite (CGO_ENABLED=0 hard rule)

NEW deviations not in this list = findings.

## Rules

- Never grade generously — "looks similar" ≠ Done
- Stub or TODO = Partial at best
- One platform of N required = Partial
- Cite file:line for every claim
- Distinguish accepted deviations from new drift
- If spec is ambiguous, surface the ambiguity to user — don't assume
