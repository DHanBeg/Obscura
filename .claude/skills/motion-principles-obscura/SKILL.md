---
name: motion-principles-obscura
description: Animation + motion design principles for Obscura. Easing, duration, choreography, reduced-motion. Use when adding any transition, animation, or interactive feedback in frontend/, mobile/, or desktop/.
---

# Motion Principles — Obscura Edition

Motion communicates state, hierarchy, and causality. Bad motion is worse than no motion.

## Hard rules

1. **Respect `prefers-reduced-motion`.** All non-essential animations OFF when the user opts out.
2. **Never exceed 500ms.** Longer feels broken. Most UI motion is 150-300ms.
3. **Easing has direction.** Things entering use `ease-out`, leaving use `ease-in`, both use `ease-in-out`.
4. **One motion at a time per surface.** Don't animate multiple unrelated things simultaneously.
5. **Animate transform + opacity only** for 60fps. Avoid width/height/top/left animations.

## Duration scale (locked)

| Use | Duration | Easing |
|---|---|---|
| Micro (hover, focus, tap feedback) | 150ms | ease-out |
| Macro (modal open, page transition, drawer) | 300ms | cubic-bezier(0.4, 0, 0.2, 1) |
| Welcome (one-time intro, success celebration) | 500ms | cubic-bezier(0.16, 1, 0.3, 1) |
| Loading skeleton shimmer | 1500ms | linear, infinite |

## Easing functions

```ts
// frontend/lib/motion.ts (create if missing)
export const EASE = {
  standard: 'cubic-bezier(0.4, 0, 0.2, 1)',      // most things
  decelerate: 'cubic-bezier(0, 0, 0.2, 1)',      // entering
  accelerate: 'cubic-bezier(0.4, 0, 1, 1)',      // leaving
  spring: 'cubic-bezier(0.16, 1, 0.3, 1)',       // delight
};

export const DURATION = {
  micro: 150,
  macro: 300,
  welcome: 500,
};
```

## Choreography rules

- **Stagger lists by 30-50ms** when entering (first item 0ms, second 40ms, etc.)
- **Cap stagger total at 300ms** — don't make user wait through 20 items
- **Page transitions: outgoing fades + scales down (0.98), incoming fades + scales up (1.02→1)**
- **Modal: scrim fades 200ms, modal slides + fades 300ms, slight delay so scrim leads**
- **Sheet on mobile: spring from bottom, 350ms, ease-out**
- **Toast: slide+fade from top 200ms, dismiss 150ms**

## Per-surface motion catalog

### Message bubble appears
- Translate Y from +8px to 0
- Opacity 0 → 1
- Duration: 200ms
- Easing: decelerate

### Tier badge upgrade (celebration)
- Scale 0.8 → 1.1 → 1
- Glow pulse (box-shadow expand + fade)
- Duration: 500ms total
- Easing: spring

### Loading skeleton
- 1500ms linear shimmer (background gradient slide)
- Never longer than the actual load — replace immediately when data arrives

### Tab switch (mobile bottom bar)
- Indicator slides between tabs: 250ms decelerate
- Content cross-fades: 150ms

### ZK proof generation feedback
- Subtle pulse on the trigger button (1500ms loop)
- Progress bar fills 0-100% (proof gen ~500ms in browser)
- Success: brief checkmark scale-in (200ms spring)

### Call connecting
- Radial pulse from avatar
- 800ms loop until connected
- On connect: pulse stops, avatar borders highlight

## Anti-patterns (NEVER)

- Bouncing icons that don't communicate anything
- Animations that block the user's next action (cannot interrupt)
- Page transitions longer than 400ms (feels sluggish)
- Animating during scroll (jank)
- Multiple competing animations on the same screen
- Animations that move things you're trying to click (jumping targets)
- Disabled animations that don't have a `prefers-reduced-motion` fallback

## Implementation patterns

### React (Framer Motion)
```tsx
import { motion } from "framer-motion";
import { DURATION, EASE } from "@/lib/motion";

<motion.div
  initial={{ opacity: 0, y: 8 }}
  animate={{ opacity: 1, y: 0 }}
  exit={{ opacity: 0 }}
  transition={{ duration: DURATION.macro / 1000, ease: EASE.standard }}
>
```

### React Native (Reanimated)
```tsx
import Animated, { withSpring, withTiming, Easing } from "react-native-reanimated";

const opacity = useSharedValue(0);
useEffect(() => {
  opacity.value = withTiming(1, { duration: 300, easing: Easing.bezier(0.4, 0, 0.2, 1) });
}, []);
```

### CSS
```css
.button {
  transition: transform 150ms cubic-bezier(0, 0, 0.2, 1),
              background-color 150ms cubic-bezier(0, 0, 0.2, 1);
}
.button:hover { transform: scale(1.02); }

@media (prefers-reduced-motion: reduce) {
  .button { transition: none; }
}
```

## When to use this skill

- Adding any new transition or animation
- Reviewing UI before "done" (after impeccable critique)
- User reports "feels janky" or "too slow"
- Designing celebration / feedback moments

## When NOT to use

- Backend-only work
- Static documentation pages
- Performance-critical hot paths where animation would block work

## Companion skills

- `frontend-design-obscura` — visual design tokens
- `impeccable-obscura` — design excellence critique pass
- `external/anthropics/skills/canvas-design` — for any algorithmic / generative visual
