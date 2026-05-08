export const colors = {
  void:    "#080808",
  ground:  "#0e0e0e",
  surface: "#111111",
  raised:  "#171717",
  border:  "#1f1f1f",
  muted:   "#2a2a2a",
  dim:     "#444444",
  sub:     "#888888",
  body:    "#c8c8c8",
  head:    "#f0f0f0",
  white:   "#ffffff",
  accent:  "#5ec46e",
  accentDim: "#257830",
  accentDeep: "#0d2211",
  red:     "#ef4444",
  amber:   "#f59e0b",
  tier1:   "#888888",
  tier2:   "#4a9eff",
  tier3:   "#a78bfa",
  tier4:   "#f59e0b",
  tier5:   "#5ec46e",
} as const;

export const spacing = {
  xs: 4,
  sm: 8,
  md: 16,
  lg: 24,
  xl: 32,
  xxl: 48,
} as const;

export const radius = {
  sm: 8,
  md: 12,
  lg: 16,
  xl: 20,
  xxl: 28,
  full: 9999,
} as const;

export const typography = {
  xs: 11,
  sm: 13,
  base: 15,
  md: 17,
  lg: 20,
  xl: 24,
  xxl: 28,
  xxxl: 34,
} as const;

export const shadow = {
  sm: {
    shadowColor: "#000",
    shadowOffset: { width: 0, height: 2 },
    shadowOpacity: 0.6,
    shadowRadius: 4,
    elevation: 3,
  },
  md: {
    shadowColor: "#000",
    shadowOffset: { width: 0, height: 4 },
    shadowOpacity: 0.7,
    shadowRadius: 12,
    elevation: 6,
  },
  lg: {
    shadowColor: "#000",
    shadowOffset: { width: 0, height: 8 },
    shadowOpacity: 0.8,
    shadowRadius: 24,
    elevation: 12,
  },
  accent: {
    shadowColor: "#5ec46e",
    shadowOffset: { width: 0, height: 0 },
    shadowOpacity: 0.3,
    shadowRadius: 12,
    elevation: 6,
  },
} as const;
