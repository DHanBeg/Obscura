export const colors = {
  void:    "#0d0d14",
  ground:  "#12121c",
  surface: "#181826",
  raised:  "#1e1e30",
  border:  "rgba(255,255,255,0.07)",
  muted:   "#252540",
  // dim/sub metin için kullanılan opaklıklar WCAG AA (4.5:1) hedefine göre
  // kalibre edilmiş — void (#0d0d14) zemine karşı: dim ~4.9:1, sub ~5.8:1,
  // body ~6.1:1 (değişmedi) — sıralama (dim < sub < body) korunuyor.
  // Eski değerler (0.18/0.35) sırasıyla ~1.7:1 ve ~2.8:1 idi, küçük fontla
  // (10-11px) pratikte okunamıyordu.
  dim:     "rgba(232,232,240,0.52)",
  sub:     "rgba(232,232,240,0.58)",
  body:    "rgba(232,232,240,0.6)",
  head:    "#e8e8f0",
  white:   "#ffffff",
  accent:  "#4ade80",
  accentDim: "rgba(74,222,128,0.25)",
  accentDeep: "rgba(74,222,128,0.08)",
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

// Özel font dosyası yüklenmiyor (bkz. app/_layout.tsx) — sistem varsayılan
// fontuna düşer, kalınlık ayrımı fontWeight ile yapılır.
export const font = {
  regular: undefined,
  bold: undefined,
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
