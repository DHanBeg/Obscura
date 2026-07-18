// Panik butonu — Madde 13 (Bölüm 6: Panik Butonu ve Buluşma Güvenliği).
//
// Mekanizma: kaba konum (1km grid, bkz. lib/location-grid.ts) X3DH/ratchet
// ile şifrelenip güven kişisine sıradan bir mesaj gibi gönderilir. Backend
// ciphertext'i hiç çözmez, yalnızca opak blob olarak taşır.
//
// DÜRÜST SINIR (güncel — bkz. ADR-0016): sealed-sender Madde 15 Adım 5'te
// gerçek gönderim akışına bağlandı, ANCAK sendPanicAlert() aşağıda
// api.sendMessage() çağrısına encryption_type: "sealed" GEÇMİYOR — yani panik
// mesajları bilinçli olarak sealed yoluna girmiyor, sunucu from_did/to_did/
// type/sent_at'i normal mesajlarda olduğu gibi düz görür VE kalıcı saklar.
// İÇERİK (konum) gizli kalır, ama panik sinyali gönderildiği OLGUSU ve KİME
// gönderildiği gizli DEĞİLDİR. Ayrıca ADR-0016 uyarınca: sealed olsaydı bile
// bu "sunucu bilmez" anlamına gelmezdi, yalnızca "sunucu kalıcı saklamaz/
// iletmez" anlamına gelirdi. UI metni bunu asla "sunucu hiçbir şey bilmez"
// diye yazmamalı.

import { api } from "./api";
import { encryptMessage } from "./e2e";
import { gridCell } from "./location-grid";

export const PANIC_MESSAGE_TYPE = "panic_alert";
export const IM_SAFE_MESSAGE_TYPE = "im_safe";

export const PANIC_PRIVACY_NOTE =
  "Konumun şifrelenir, yalnızca güven kişin çözebilir — sunucu konumu göremez. " +
  "Ancak panik sinyali gönderdiğini ve kime gönderdiğini sunucu görür " +
  "(mesajlaşmada da böyle, panik butonuna özel değil).";

interface PanicCoords {
  latitude: number;
  longitude: number;
}

interface PanicAlertPayload {
  grid_id: string;
  sent_at: string;
}

interface SendPanicAlertResult {
  id: string;
  conv_id: string;
}

// sendPanicAlert — kaba grid_id'yi güven kişisine şifreli mesaj olarak
// gönderir. Ham lat/lon asla payload'a girmez, asla sunucuya gitmez.
export async function sendPanicAlert(
  trustedContactDid: string,
  coords: PanicCoords
): Promise<SendPanicAlertResult> {
  const payload: PanicAlertPayload = {
    grid_id: gridCell(coords.latitude, coords.longitude),
    sent_at: new Date().toISOString(),
  };

  const ciphertext = await encryptMessage(JSON.stringify(payload), trustedContactDid);

  const result = await api.sendMessage({
    to_id: trustedContactDid,
    ciphertext,
    type: PANIC_MESSAGE_TYPE,
  });

  return { id: result.id, conv_id: result.conv_id };
}

interface ImSafePayload {
  sent_at: string;
}

// sendImSafe — "Buluştum, iyiyim" onayı. KONUM YOK: bu bir yer bildirimi
// değil, salt güvendeyim sinyali (Bölüm 6.1) — fonksiyon imzası bilerek
// coords parametresi almıyor, konum eklemek isteyen biri buraya kolayca
// sızdıramaz.
export async function sendImSafe(trustedContactDid: string): Promise<SendPanicAlertResult> {
  const payload: ImSafePayload = { sent_at: new Date().toISOString() };

  const ciphertext = await encryptMessage(JSON.stringify(payload), trustedContactDid);

  const result = await api.sendMessage({
    to_id: trustedContactDid,
    ciphertext,
    type: IM_SAFE_MESSAGE_TYPE,
  });

  return { id: result.id, conv_id: result.conv_id };
}
