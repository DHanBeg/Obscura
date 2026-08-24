// E1 — chat/[id].tsx'in grup gönder/al yolunun saf (UI'sız) akışı.
// createGroupFlow.ts/joinGroupFlow.ts ile AYNI desen: local-önce, sıralı,
// plaintext-fallback yok. Yeni kripto YOK — group.ts/mlsApi.ts/mls-store.ts'i
// bağlar.
import { getMlsCiphersuiteImpl, encryptGroupMessage, decryptApplicationMessageWireWithState } from "./group";
import { loadGroupState, saveGroupState, type MlsStores } from "./mls-store";
import { sendGroupMessage, getGroupMessages, type SendGroupMessageResult } from "./mlsApi";
import { cachePlaintext, getCachedPlaintext, type CacheStores } from "../plaintext-cache";

async function ensureGroupClientState(groupId: string, stores: MlsStores) {
  const state = await loadGroupState(groupId, stores);
  if (!state) {
    throw new Error(
      `ensureGroupClientState: local grup state yok (${groupId}) — davet henüz kabul edilmemiş olabilir (bkz. mls-invites.tsx)`
    );
  }
  return state;
}

/** Grup için tek bir metin mesajı şifreler, backend'e yollar, yeni state'i
 * (ilerleyen ratchet generation) AĞDAN ÖNCE kaydeder — bkz. group.ts
 * EncryptedGroupMessage.newState notu (persist edilmezse nonce/key reuse).
 * stores — chat/[id].tsx gerçek cihazda hiç geçmez (varsayılan gerçek
 * SecureStore/AsyncStorage); sadece tek process'te birden çok "cihaz"
 * simüle eden testler (bkz. mls-store.ts DI deseni) enjekte eder. */
export async function sendGroupTextMessage(
  groupId: string,
  plaintext: string,
  stores: MlsStores = {}
): Promise<SendGroupMessageResult> {
  const cs = await getMlsCiphersuiteImpl();
  const state = await ensureGroupClientState(groupId, stores);
  const encrypted = await encryptGroupMessage(state, plaintext, cs);
  await saveGroupState(groupId, encrypted.newState, stores);
  return sendGroupMessage(groupId, encrypted.ciphertextWireB64, encrypted.epoch);
}

export interface DecryptedGroupMessage {
  id: string;
  sender_did: string;
  epoch: number;
  created_at: string;
  plaintext: string;
}

/** Grup mesaj kuyruğunu çeker, sadece 'application' kayıtlarını (commit'ler
 * DEĞİL, bkz. mls-e2e-mock-relay.test.ts) alıcı tarafın kendi state'iyle
 * SIRAYLA (created_at artan) çözer.
 *
 * BULUNDU (canlı smoke, E1) — İKİ KATMANLI:
 * 1) ts-mls'in processMessage'ı state referansının secretTree'sini YERİNDE
 *    mutasyona uğratıyor — aynı state referansını ikinci bir mesaj için
 *    TEKRAR kullanmak "aes/gcm: invalid ghash tag" ile başarısız olur.
 *    Fix: state döngü içinde AÇIKÇA ileri taşınır
 *    (decryptApplicationMessageWireWithState.newState).
 * 2) chat/[id].tsx her poll'da TÜM geçmişi tekrar çeker (sinceEpoch yok) —
 *    forward-secret ratchet'te aynı mesajı İKİNCİ KEZ decrypt etmeye
 *    çalışmak (state artık o generation'ın ÖNÜNDE) retention penceresi
 *    dışındaysa başarısız olur — bu YANLIŞ bir davranış değil, forward
 *    secrecy'nin ta kendisi (eski anahtar kasıtlı olarak erişilemez hale
 *    gelir). Fix: 1:1'in zaten kullandığı plaintext-cache.ts (aynı desen,
 *    messageId ile şifreli local önbellek) — bir mesaj BİR KEZ decrypt
 *    edilir, sonraki fetch'ler ciphertext'e hiç dokunmadan önbellekten okur.
 *
 * Çözülemeyen (önbellekte yok VE retention dışı) tek bir mesaj tüm listeyi
 * düşürmesin diye sessizce atlanır. */
export async function fetchAndDecryptGroupMessages(
  groupId: string,
  sinceEpoch?: number,
  stores: MlsStores = {},
  cacheStores: CacheStores = {}
): Promise<DecryptedGroupMessage[]> {
  const cs = await getMlsCiphersuiteImpl();
  let cursorState: Awaited<ReturnType<typeof ensureGroupClientState>> | null = null;
  const res = await getGroupMessages(groupId, sinceEpoch);
  const appMessages = res.messages
    .filter((m) => m.content_type === "application")
    .sort((a, b) => a.created_at.localeCompare(b.created_at));

  const out: DecryptedGroupMessage[] = [];
  let advanced = false;
  for (const m of appMessages) {
    const cached = await getCachedPlaintext(m.id, cacheStores);
    if (cached !== null) {
      out.push({ id: m.id, sender_did: m.sender_did, epoch: m.epoch, created_at: m.created_at, plaintext: cached });
      continue;
    }
    try {
      if (cursorState === null) cursorState = await ensureGroupClientState(groupId, stores);
      const { plaintext, newState } = await decryptApplicationMessageWireWithState(cursorState, m.ciphertext_b64, cs);
      cursorState = newState;
      advanced = true;
      await cachePlaintext(m.id, plaintext, cacheStores);
      out.push({ id: m.id, sender_did: m.sender_did, epoch: m.epoch, created_at: m.created_at, plaintext });
    } catch {
      // çözülemeyen tek mesaj (önbellekte yok + retention dışı) diğerlerini
      // engellemesin, state o mesaj için ilerletilmez.
    }
  }
  if (advanced && cursorState !== null) await saveGroupState(groupId, cursorState, stores);
  return out;
}
