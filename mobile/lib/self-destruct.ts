// Self-destruct süre seçenekleri — backend ADIM 1 semantiğiyle birebir:
// null=kapalı, 0="okununca", >0=gönderimden N sn sonra (10/60/300/3600).
// expires_at (30 gün genel saklama TTL) İLE KARIŞMAZ, ayrı ve isteğe bağlı.
export const SELF_DESTRUCT_OPTIONS: { label: string; seconds: number | null }[] = [
  { label: "Kapalı", seconds: null },
  { label: "Okununca", seconds: 0 },
  { label: "10 saniye", seconds: 10 },
  { label: "1 dakika", seconds: 60 },
  { label: "5 dakika", seconds: 300 },
  { label: "1 saat", seconds: 3600 },
];

export function isSelfDestructMessage(msg: { self_destruct_seconds?: number | null }): boolean {
  return msg.self_destruct_seconds !== null && msg.self_destruct_seconds !== undefined;
}

export function formatSelfDestructLabel(seconds: number | null | undefined): string {
  const opt = SELF_DESTRUCT_OPTIONS.find((o) => o.seconds === (seconds ?? null));
  return opt ? opt.label : "Kapalı";
}
