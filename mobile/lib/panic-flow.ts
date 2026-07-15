// Panik butonu orkestrasyonu — Madde 13 Adım 5. Ekranın (app/(main)/panic.tsx)
// tüm karar mantığı burada, saf/test edilebilir; ekran yalnızca bu fonksiyonu
// gerçek expo-location/lib/panic bağımlılıklarıyla çağıran ince bir kabuk.

export type PermissionResult = "granted" | "denied" | "blocked";

export interface PanicCoords {
  latitude: number;
  longitude: number;
}

export interface PanicSendDeps {
  // "denied" = yeniden sorulabilir (kullanıcı henüz kalıcı reddetmedi).
  // "blocked" = kalıcı reddedilmiş (canAskAgain=false) — tekrar sormanın
  // anlamı yok, kullanıcı cihaz ayarlarına yönlendirilmeli.
  requestPermission: () => Promise<PermissionResult>;
  getCurrentPosition: () => Promise<PanicCoords>;
  send: (trustedContactDid: string, coords: PanicCoords) => Promise<{ id: string; conv_id: string }>;
}

export type PanicSendOutcome =
  | { status: "sent"; id: string; conv_id: string }
  | { status: "permission_denied" }
  | { status: "permission_blocked" }
  | { status: "error"; message: string };

export async function triggerPanicAlert(
  trustedContactDid: string,
  deps: PanicSendDeps
): Promise<PanicSendOutcome> {
  try {
    const perm = await deps.requestPermission();
    if (perm === "denied") return { status: "permission_denied" };
    if (perm === "blocked") return { status: "permission_blocked" };

    const coords = await deps.getCurrentPosition();
    const result = await deps.send(trustedContactDid, coords);
    return { status: "sent", id: result.id, conv_id: result.conv_id };
  } catch (e: any) {
    return { status: "error", message: e?.message ?? "Bilinmeyen hata" };
  }
}

// canShowImSafeButton — Madde 13 Adım 7: "İyiyim" butonu yalnızca panik
// başarıyla gönderildikten SONRA görünür (mantıklı akış: önce panik, sonra
// güvendeyim onayı) — izin reddi/blok/hata durumlarında gösterilmez.
export function canShowImSafeButton(panicOutcome: PanicSendOutcome | null): boolean {
  return panicOutcome?.status === "sent";
}

// ── "Buluştum, iyiyim" — konum/izin adımı YOK, doğrudan gönderim ──────────

export type ImSafeOutcome =
  | { status: "sent"; id: string; conv_id: string }
  | { status: "error"; message: string };

export interface ImSafeSendDeps {
  send: (trustedContactDid: string) => Promise<{ id: string; conv_id: string }>;
}

export async function triggerImSafeAlert(
  trustedContactDid: string,
  deps: ImSafeSendDeps
): Promise<ImSafeOutcome> {
  try {
    const result = await deps.send(trustedContactDid);
    return { status: "sent", id: result.id, conv_id: result.conv_id };
  } catch (e: any) {
    return { status: "error", message: e?.message ?? "Bilinmeyen hata" };
  }
}
