// B10 Faz 1 — mobile/lib/mls/groupChat.ts'nin portu. chat/[id]'nin grup
// gönder/al yolunun saf (UI'sız) akışı. Yeni kripto YOK — group.ts/
// mls-store.ts'i bağlar, api.ts'nin (canlı) mls wrapper'larını kullanır.
"use client";
import { getMlsCiphersuiteImpl, encryptGroupMessage, decryptApplicationMessageWireWithState } from "./group";
import { loadGroupState, saveGroupState, type MlsStores } from "./mls-store";
import { cachePlaintext, getCachedPlaintext, type CacheStores } from "./plaintextCache";
import { api } from "../api";

async function ensureGroupClientState(groupId: string, stores: MlsStores) {
  const state = await loadGroupState(groupId, stores);
  if (!state) {
    throw new Error(
      `ensureGroupClientState: local grup state yok (${groupId}) — davet henüz kabul edilmemiş olabilir`
    );
  }
  return state;
}

export interface SendGroupMessageResult {
  id: string;
  created_at: string;
  delivered: number;
  queued: number;
}

/** Grup için tek bir metin mesajı şifreler, backend'e yollar, yeni state'i
 * (ilerleyen ratchet generation) AĞDAN ÖNCE kaydeder — persist edilmezse
 * nonce/key reuse (bkz. group.ts EncryptedGroupMessage.newState notu). */
export async function sendGroupTextMessage(
  groupId: string,
  plaintext: string,
  stores: MlsStores = {}
): Promise<SendGroupMessageResult> {
  const cs = await getMlsCiphersuiteImpl();
  const state = await ensureGroupClientState(groupId, stores);
  const encrypted = await encryptGroupMessage(state, plaintext, cs);
  await saveGroupState(groupId, encrypted.newState, stores);
  return api.mlsSendGroupMessage(groupId, encrypted.ciphertextWireB64, encrypted.epoch);
}

export interface DecryptedGroupMessage {
  id: string;
  sender_did: string;
  epoch: number;
  created_at: string;
  plaintext: string;
}

/** Grup mesaj kuyruğunu çeker, sadece 'application' kayıtlarını (commit'ler
 * DEĞİL) alıcı tarafın kendi state'iyle SIRAYLA (created_at artan) çözer.
 * processMessage state'i YERİNDE mutasyona uğrattığı için state döngü
 * içinde AÇIKÇA ileri taşınır. Çözülmüş düz metinler plaintextCache'te
 * saklanır (forward-secret ratchet'te bir mesaj birden fazla çözülemez). */
export async function fetchAndDecryptGroupMessages(
  groupId: string,
  sinceEpoch?: number,
  stores: MlsStores = {},
  cacheStores: CacheStores = {}
): Promise<DecryptedGroupMessage[]> {
  const cs = await getMlsCiphersuiteImpl();
  let cursorState: Awaited<ReturnType<typeof ensureGroupClientState>> | null = null;
  const res = await api.mlsGetGroupMessages(groupId, sinceEpoch);
  const appMessages = (res.messages as Array<{
    id: string; sender_did: string; ciphertext_b64: string;
    content_type: "application" | "commit"; epoch: number; created_at: string;
  }>)
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
      // çözülemeyen tek mesaj diğerlerini engellemesin.
    }
  }
  if (advanced && cursorState !== null) await saveGroupState(groupId, cursorState, stores);
  return out;
}
