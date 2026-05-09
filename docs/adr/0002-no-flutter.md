# ADR 0002: Defer Flutter, ship Next.js + Expo + Tauri

Date: 2026-05-01
Status: Accepted
Decider: project lead
Spec ref: Bölüm 9.1 (Önerilen client: Flutter 3.19+)

## Context

Spec recommends Flutter for unified web/mobile/desktop client. We chose to ship three separate clients: Next.js (web), React Native via Expo (mobile), Tauri 2.x (desktop wrapping the web client).

This is a deliberate deviation from spec.

## Options considered

### Option A: Flutter (per spec)
- Single codebase, all platforms
- Dart language
- Excellent perf
- Cons:
  - flutter_rust_bridge for FFI is mature but adds build complexity
  - Web target is heavyweight (large bundle)
  - Team has zero Flutter experience
  - Plugin ecosystem for Signal Protocol immature

### Option B: Three separate clients (chosen)
- Web: Next.js 14 (App Router) — best web ecosystem
- Mobile: React Native + Expo — better RN devx than vanilla
- Desktop: Tauri 2.x wrapping the Next.js build
- Pros:
  - Each platform uses best-in-class tooling
  - Existing TypeScript expertise in team
  - libsignal-protocol-typescript works in browser + RN
  - Tauri reuses 100% of Next.js code
- Cons:
  - Three codebases (mitigated by shared `packages/`)
  - More test surface
  - Some duplication of business logic

### Option C: React Native everywhere (RN Web)
- Pros: Truly unified
- Cons: RN Web is brittle, web devx degraded

## Decision

**Option B**: Three clients with shared TypeScript packages.

## Rationale

- Team can ship faster with familiar tools
- Tauri 2.x reuses ~100% web code, so desktop is "free" once web is done
- Expo is the de facto standard for RN; SecureStore + Notifications + Router are mature
- Spec rule "use the best, don't fall back to lesser" — best web stack is Next.js, best RN stack is Expo, best lightweight desktop is Tauri 2.x. Flutter is good but not best per platform.
- Shared `packages/` (api, store, e2ee) keeps logic DRY

## Consequences

- **Positive**: Faster shipping, leveraging existing TS skills, top-tier per-platform UX
- **Negative**: 3 build pipelines, 3 dependency trees, some duplication
- **Tech debt**: Per-platform polish drift over time (mitigate via shared design tokens, screenshot tests per platform)

## Revisit trigger

Reconsider Flutter if:
- Team grows to need cross-platform parity at all costs
- Bundle size on web becomes prohibitive (>500KB initial)
- Mobile parity drift >2 weeks behind web

## References

- Flutter pros/cons: https://docs.flutter.dev/resources/faq
- Tauri 2.x architecture: https://tauri.app/concept/architecture/
- Spec: docs/spec/obscura_spec_v3.txt Bölüm 9
