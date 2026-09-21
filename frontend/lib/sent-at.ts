// Mesaj zamanı normalizasyonu.
//
// WS `new_message` yükündeki `sent_at` Unix SANİYESİ (sayı; backend
// handlers.go msgPayload: now.Unix()). REST yanıtları (GET messages) ise RFC3339
// string. `new Date(<saniye>)` bunu milisaniye sayıp mesajı 1970'e koyuyordu →
// canlı gelen mesaj listenin en başına düşüyor, tarih ayırıcıları bozuluyordu.

const MS_THRESHOLD = 1e12; // bunun altındaki sayı saniye (bugün ~1.7e9), üstü ms (~1.7e12)

/** number (saniye/ms), sayısal string, RFC3339 string → epoch ms. Çözülemezse 0. */
export function sentAtToMs(value: unknown): number {
  if (typeof value === "number") {
    if (!Number.isFinite(value) || value <= 0) return 0;
    return value < MS_THRESHOLD ? value * 1000 : value;
  }
  if (typeof value === "string") {
    const trimmed = value.trim();
    if (/^\d+(\.\d+)?$/.test(trimmed)) return sentAtToMs(Number(trimmed));
    const parsed = Date.parse(trimmed);
    return Number.isNaN(parsed) ? 0 : parsed;
  }
  return 0;
}

/** Store'a girecek ISO string. Zaman çözülemezse şimdiki an (mesaj listenin sonuna düşer). */
export function sentAtToIso(value: unknown): string {
  const ms = sentAtToMs(value);
  return new Date(ms > 0 ? ms : Date.now()).toISOString();
}
