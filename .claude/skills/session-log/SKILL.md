---
name: session-log
description: Maintain per-session logs in docs/sessions/YYYY-MM-DD.md for cross-session memory. Use at end of every session that produces meaningful changes.
---

# Session Log

## Why

Claude conversations have context windows. Summaries lose detail. Session log = persistent memory across sessions.

## File location

`docs/sessions/YYYY-MM-DD.md` — one per calendar day. Append to existing if same day.

## Template

```markdown
# Session YYYY-MM-DD

## Summary
[One sentence what this session accomplished]

## Tasks completed
- [task] (commit: SHA or "uncommitted")
- [task]

## Tasks in progress
- [task] — next step is X

## Decisions made
- [decision] — see ADR-NNNN for rationale

## Files changed
- backend/internal/api/handlers.go — added X
- frontend/lib/api.ts — added Y

## Spec gaps closed
- Bölüm 6.3 group messaging — partially done (basic create works)

## Spec gaps remaining (from this work area)
- Bölüm 6.3 — MLS encryption not implemented
- Bölüm 13.X — Y still missing

## CLAUDE.md updates needed
- Move "MLS basic" from ❌ to 🟡

## Open questions for next session
- Should we use openmls or roll our own?
- Is the FCM stub OK for staging or do we need real creds?

## Notes
- Found a bug in X, fixed in commit SHA
- Discovered that Y doesn't work with Z — see commit SHA
```

## When to update

- End of each productive session
- After any significant decision (link to ADR)
- When discovering something not previously documented
- When changing the CLAUDE.md feature matrix

## Reading session logs

At session start:
1. Read CLAUDE.md (always)
2. Read most recent 3 session logs (if exist)
3. Search for keyword if user references prior work: `grep -r "MLS" docs/sessions/`

## Rules

- Append-only — don't edit prior sessions (except typo fixes)
- Date format ISO: 2026-05-09, never localized
- One file per day, multiple sessions same day = sections inside
- No PII (no real phone numbers, no JWTs, no real user data)
- Keep entries factual — opinions go in ADRs
