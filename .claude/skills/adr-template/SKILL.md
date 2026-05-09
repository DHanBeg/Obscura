---
name: adr-template
description: Architecture Decision Record template and process. Use whenever a non-trivial design decision is made or a deviation from spec is accepted.
---

# Architecture Decision Records (ADR)

## Why

Memory of why we chose X over Y. Future Claude (or human) reads these to avoid relitigating decisions.

## File location

`docs/adr/NNNN-short-title.md` where NNNN is sequential 4-digit (0001, 0002, ...).

## Template

```markdown
# ADR NNNN: [Decision title in active voice]

Date: YYYY-MM-DD
Status: Proposed | Accepted | Deprecated | Superseded by ADR-XXXX
Decider: [name or "user"]
Spec ref: [Bölüm X.Y, line Z]

## Context

What problem are we solving? What constraints exist? Why now?

[2-4 paragraphs]

## Options considered

### Option A: [name]

[1 paragraph description]

- Pros:
  - ...
- Cons:
  - ...
- Effort: S | M | L | XL
- Risk: Low | Medium | High
- Cost (if relevant): $X/month, N hours

### Option B: [name]

...

### Option C: [name]

...

## Decision

We chose **Option X** because [primary reason in 1-2 sentences].

## Rationale

[2-4 paragraphs explaining why this option won and why others lost]

## Consequences

- **Positive**: ...
- **Negative**: ...
- **Neutral**: ...
- **Tech debt incurred**: [explicit, with estimated remediation cost]

## Implementation plan

1. Step 1 — files affected
2. Step 2 — files affected
3. Verify via: [test, metric, manual check]

## Spec deviation (if applicable)

If this decision deviates from spec:
- Spec says: [quote]
- We're doing: [our approach]
- Why this deviation is acceptable: [justification]
- When/how we'll close the gap: [or "permanent — won't close"]

## References

- Spec: docs/spec/obscura_spec_v3.txt:NNNN
- Related ADRs: ADR-XXXX, ADR-YYYY
- External: [paper, RFC, library docs]
- Discussion: [PR link, session log link]
```

## Status meanings

| Status | Meaning |
|--------|---------|
| Proposed | Drafted, awaiting review |
| Accepted | Approved, in effect |
| Deprecated | Still in code but should not be used for new work |
| Superseded | Replaced by another ADR; cite the new one |

## When to write an ADR

YES, write one:
- Choosing between competing libraries (libp2p-go vs raw QUIC)
- Adopting a deviation from spec
- Changing protocol or wire format
- Significant data model change
- Picking infrastructure (Postgres vs SQLite, k8s vs Compose)
- Security model changes (auth method, key storage)

NO, don't write one:
- Variable rename
- Bug fix
- New endpoint that follows existing patterns
- UI tweak

## Numbering

- Sequential, never reuse
- Even if proposed and rejected, number is consumed
- Find next number: `ls docs/adr/ | sort -n | tail -1`

## Index

Maintain `docs/adr/README.md` with table of all ADRs:

```markdown
# ADR Index

| # | Title | Status | Date |
|---|-------|--------|------|
| 0001 | Use modernc.org/sqlite over CGO | Accepted | 2026-04-30 |
| 0002 | Defer Flutter, ship RN+Next.js+Tauri | Accepted | 2026-05-01 |
| 0003 | HTTP gossip MVP, libp2p later | Accepted | 2026-05-02 |
```

## Writing tips

- Active voice ("We chose X" not "X was chosen")
- Cite primary sources for crypto / security decisions (RFC, paper)
- Quantify when possible ("3x faster", "$X/month savings")
- Don't pad with options you'd never pick — show real consideration
- "Negative consequences" must include real tradeoffs, not just "more code"
