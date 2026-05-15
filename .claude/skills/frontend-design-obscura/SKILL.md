---
name: frontend-design-obscura
description: Frontend design principles applied to Obscura — dark-only theme, Telegram/Element references, security-feeling minimal. Use for any frontend/ or mobile/ UI work. Wraps anthropics/frontend-design + Obscura design tokens.
---

# Frontend Design — Obscura Edition

## Source library
- `E:\obscura\.claude\skills\external\anthropics\skills\frontend-design\SKILL.md` — base principles
- `E:\obscura\.claude\skills\external\vercel-agent-skills\skills\web-design-guidelines\SKILL.md`

Always read the source library SKILL.md when starting a UI task — it covers hierarchy, typography, spacing, contrast.

## Obscura-specific overrides

### Theme
- **Dark-only.** Never add a light mode. Spec mandate.
- Background: pure dark (#0A0A0F or similar) — not gray-blue
- Surfaces: layered with subtle border (1px, low-alpha white)
- Accent: green derived from the eagle logo (#22C55E family)
- Danger: red (#EF4444 family) — only for destructive + critical errors

### Tokens (locked, never hardcode)
- `lib/theme.ts` (frontend) + `lib/theme.ts` (mobile) — colors, spacing, radius, typography
- Spacing scale: 4/8/12/16/24/32/48 px
- Radius scale: 4/8/12/16/24/full
- Type scale: 11/12/14/16/20/24/32 px

### References
Telegram + Element Matrix:
- Compact list density (chat list rows ~64px)
- Avatar + name + last message + timestamp pattern
- Bottom tab bar on mobile, left sidebar on desktop
- Sheets/modals slide from bottom on mobile, center on desktop

### Security-feeling cues
- Lock icon on every E2EE-encrypted surface
- Tier color ring around avatars (bronze/silver/gold/platinum/diamond)
- "Verified" badge for cross-signed devices

## Required for every screen
1. Loading skeleton (matching final layout)
2. Empty state (not blank — guide the user)
3. Error state with retry CTA
4. Mobile + tablet + desktop layouts
5. Keyboard navigation (Tab order, ESC closes, Enter submits)
6. Accessible (ARIA labels on icon-only buttons, semantic HTML)

## When to apply this skill
- Creating a new page/screen
- Reviewing existing UI before "done"
- User reports something "feels off"
- Designing tier badges, avatars, message bubbles, modals

## When NOT to apply
- Backend-only changes
- ZK circuit work
- DevOps / infra

## Companion skills
- `motion-principles-obscura` for animation
- `impeccable-obscura` for polish/critique pass
