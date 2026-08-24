// #30 Faz 0 — paylaşılan tema token katmanı, TEK KAYNAK (mobile + web).
// Kapsam: SADECE marketplace ekranları + logo bu katmanı kullanır (bkz.
// mobile/lib/theme.ts — 1:1/genel UI'ın mevcut token dosyası, buradan
// TÜRETİLMEDİ, kasıtlı olarak ayrı kalıyor; toplu migrate bu oturumun işi
// değil).
//
// accent (marka/altın, #C9A24B) ile positive (işlevsel yeşil, #30D158)
// AYRI tutulur — biri kimlik, diğeri durum (çevrimiçi/şifreli) anlamı taşır,
// ikisini karıştırmak "marka rengi neden bazen yeşil bazen altın" karışıklığı
// yaratır.
export const tokens = {
  color: {
    void: "#0d0d14",
    ground: "#12121c",
    surface: "#181826",
    raised: "#1e1e30",
    border: "rgba(255,255,255,0.07)",
    muted: "#252540",
    dim: "rgba(232,232,240,0.52)",
    sub: "rgba(232,232,240,0.58)",
    body: "rgba(232,232,240,0.6)",
    head: "#e8e8f0",
    white: "#ffffff",
    accent: "#C9A24B",
    accentDim: "rgba(201,162,75,0.25)",
    accentDeep: "rgba(201,162,75,0.08)",
    // İşlevsel durum rengi — çevrimiçi/şifreli göstergeleri. accent'ten
    // BİLEREK ayrı; #30 kapsamında marka rengi değişti, durum rengi değişmedi.
    positive: "#30D158",
    red: "#ef4444",
    amber: "#f59e0b",
  },
  spacing: {
    xs: 4,
    sm: 8,
    md: 16,
    lg: 24,
    xl: 32,
    xxl: 48,
  },
  radius: {
    sm: 8,
    md: 12,
    lg: 16,
    xl: 20,
    xxl: 28,
    full: 9999,
  },
  typography: {
    xs: 11,
    sm: 13,
    base: 15,
    md: 17,
    lg: 20,
    xl: 24,
    xxl: 28,
    xxxl: 34,
  },
} as const;

export type Tokens = typeof tokens;

function toKebabCase(key: string): string {
  return key.replace(/([a-z0-9])([A-Z])/g, "$1-$2").toLowerCase();
}

/** Web tarafı için CSS custom property bloğu üretir. `selector` varsayılan
 * `:root` DEĞİL — global cascade'e sızmasın diye çağıran kapsamını (örn.
 * marketplace ekranlarını saran `.mp-theme`) açıkça vermeli. Tam site
 * migrasyonu istenirse ":root" geçilebilir, ama #30 kapsamında BİLEREK
 * kullanılmıyor (toplu migrate yok). */
export function cssVars(t: Tokens = tokens, selector: string = ":root"): string {
  const lines: string[] = [];
  for (const [k, v] of Object.entries(t.color)) lines.push(`  --color-${toKebabCase(k)}: ${v};`);
  for (const [k, v] of Object.entries(t.spacing)) lines.push(`  --spacing-${toKebabCase(k)}: ${v}px;`);
  for (const [k, v] of Object.entries(t.radius)) lines.push(`  --radius-${toKebabCase(k)}: ${v}px;`);
  for (const [k, v] of Object.entries(t.typography)) lines.push(`  --text-${toKebabCase(k)}: ${v}px;`);
  return `${selector} {\n${lines.join("\n")}\n}`;
}
