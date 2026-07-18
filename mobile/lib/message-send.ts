import { sealAndEncryptMessage, E2EStores } from "./e2e";
import { api } from "./api";

// Madde 15, Adım 11a.2 — chat/[id].tsx'in TÜM gönderim call site'larının
// (text/image/video/file/location/voice) kullanacağı TEK karar noktası.
//
// Öncesi: her call site kendi başına encryptMessage() + api.sendMessage()
// çağırıyordu, encryption_type hiç gönderilmiyordu → sunucu her zaman
// "signal" varsayılanına düşüyordu (EffectiveEncryptionType, models.go) →
// from_did HER ZAMAN plaintext saklanıyordu, sealed-sender kütüphanesi
// (e2e.ts sealAndEncryptMessage/receiveMessage) yazılıp test edilmiş
// olmasına rağmen hiç kullanılmıyordu (bkz. ADR-0016, "kod var, tel bağlı
// değil"). Bu dosya o teli bağlıyor: her 1:1 gönderim artık sealAndEncrypt
// zarfı üretir VE encryption_type:"sealed" alanını taşır.
//
// Kapsam dışı (kasıtlı): grup mesajları — chat/[id].tsx'teki tüm call
// site'lar zaten `conv?.peer_did` gate'li (1:1), bu fonksiyon da sadece
// peerDid alıyor, grup fanout'una hiç uygulanmıyor (sealed zarf tek-alıcı
// X25519 DH'ına dayanır, mimari olarak grup'a uygulanamaz — bkz. ADR-0016
// Adım 10 eki).
export interface SendSealedResult {
  id: string;
  conv_id: string;
  status: string;
  // Sunucu ciphertext'i geri yansıtmıyor ("Server only echoes back { id,
  // conv_id, status }", bkz. chat/[id].tsx eski yorum) — çağıran taraf
  // (optimistic UI local echo) gönderdiği zarfı burada geri alır.
  ciphertext: string;
}

export interface SendSealedExtra {
  reply_to_id?: string;
  self_destruct_seconds?: number | null;
  media_url?: string;
}

export async function sendSealedMessage(
  peerDid: string,
  plaintext: string,
  type: string,
  stores: E2EStores = {},
  extra: SendSealedExtra = {}
): Promise<SendSealedResult> {
  const ciphertext = await sealAndEncryptMessage(plaintext, peerDid, stores);
  const sent = await api.sendMessage({
    to_id: peerDid,
    ciphertext,
    type,
    encryption_type: "sealed",
    ...extra,
  });
  return { id: sent.id, conv_id: sent.conv_id, status: sent.status, ciphertext };
}
