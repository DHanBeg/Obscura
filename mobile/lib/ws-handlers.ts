import { purgePlaintext, CacheStores } from "./plaintext-cache";
import { Message } from "./store";

export interface MessageExpiredPayload {
  message_id: string;
  conv_id?: string;
  expired_at?: number;
}

// handleMessageExpired — WS "message_expired" event'i (bkz. backend
// messaging/expiry.go runExpiryPass). Backend ciphertext'i zaten sildi
// (ADIM 3); burada iki şey yapılır:
//   1. Mesajı store'da "expired" olarak işaretle (UI ikon/içerik güncellensin)
//   2. plaintext-cache'ten PURGE et — self-destruct'ın kalbi: backend
//      ciphertext'i silse bile client'ta çözülmüş metin kalırsa self-destruct
//      SAHTE olur.
export async function handleMessageExpired(
  payload: MessageExpiredPayload,
  updateMsgStatus: (msgId: string, status: Message["status"]) => void,
  stores: CacheStores = {}
): Promise<void> {
  if (!payload?.message_id) return;
  updateMsgStatus(payload.message_id, "expired");
  await purgePlaintext(payload.message_id, stores).catch(() => {});
}
