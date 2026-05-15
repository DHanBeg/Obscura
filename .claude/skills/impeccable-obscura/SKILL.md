---
name: impeccable-obscura
description: Design excellence polish pass for Obscura UI — critique, distill, normalize, delight. Use after a feature works but before declaring "done". Wraps pbakaus/impeccable for Obscura context.
---

# Impeccable — Obscura Edition

## Source library
`E:\obscura\.claude\skills\external\pbakaus-impeccable\skill\SKILL.md`

The impeccable collection includes: polish, critique, distill, normalize, bolder, quieter, delight, extract, typeset, overdrive, arrange, teach-impeccable.

Always read the source SKILL.md for the full methodology — it's a quality bar, not a checklist.

## When to use in Obscura

After a UI feature compiles, works, and passes tests — BEFORE marking the task done:

1. **Critique** — pretend you're a first-time user. What's confusing?
2. **Distill** — what's the simplest possible version that still works?
3. **Normalize** — does this match the rest of Obscura's UI patterns?
4. **Polish** — pixel-level details (alignment, spacing, easing)
5. **Delight** — is there one small surprise that makes this feel premium?

## Obscura-specific bar

Reference apps (the bar to beat):
- **Telegram** — speed, density, polish on every interaction
- **Element Matrix** — security signaling, room/group UX
- **Signal** — minimal, no clutter, every pixel earned
- **Linear** — keyboard-first, animation discipline

If your screen looks worse than these, it's not done.

## Common violations to fix

- Inconsistent radius (one card 12px, neighbor 16px — pick one)
- Inconsistent spacing (gap-3 here, gap-4 there — use the scale)
- Hover/focus states missing on interactive elements
- Loading states that pop in instead of skeleton → content transition
- Error messages that say "Error" instead of "What went wrong + what to do"
- Empty states that say "No data" instead of "Start your first chat"
- Tap targets <44px on mobile
- Text contrast <4.5:1 on body, <3:1 on large

## Distill checklist (per screen)

- Can I remove one element without losing function? Remove it.
- Can two similar buttons become one? Combine.
- Is there a label that nobody reads? Cut.
- Is there a divider that doesn't divide? Remove.

## Process

1. Read `frontend-design-obscura/SKILL.md` first (foundation)
2. Build the feature
3. Apply impeccable critique
4. Fix findings
5. Then declare done

## When NOT to apply
- Backend-only work
- Pre-implementation (no UI to critique yet)
- When time pressure is real and feature is "good enough" — flag for later polish pass
