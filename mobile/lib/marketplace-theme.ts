// #30 Faz 0 — marketplace ekranlarının token katmanı. `lib/theme.ts`'e
// (1:1/genel UI, mevcut) BİLEREK dokunulmadı — kapsam sınırı: bu oturumda
// sadece marketplace ekranları + logo yeni tema alır, toplu migrate yok.
// Tek kaynak: packages/theme (@obscura/theme), web ile paylaşılıyor.
import { tokens } from "@obscura/theme";

export const mpColors = tokens.color;
export const mpSpacing = tokens.spacing;
export const mpRadius = tokens.radius;
export const mpTypography = tokens.typography;
