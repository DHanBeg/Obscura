// B10 Faz 1 — mobile/lib/plaintext-cache.ts'nin portu. MANTIK DEĞİŞMEDİ:
// çözülmüş grup mesajı düz metinleri AES-256-GCM ile şifrelenip webAsyncStore'a
// yazılır (forward-secret MLS ratchet'te bir mesaj sadece BİR KEZ çözülebilir —
// bkz. groupChat.ts fetchAndDecryptGroupMessages).
"use client";
import { gcm } from "@noble/ciphers/aes.js";
import { webSecureStore, webAsyncStore, type KeyValueStore } from "./webStore";
import { hexToU8, u8ToHex } from "./cryptoBlob";

const MASTER_KEY_STORE_KEY = "obscura_msgcache_master_key";
const CACHE_KEY_PREFIX = "obscura_msgcache_";

export interface CacheStores {
  secure?: KeyValueStore;
  async?: KeyValueStore;
}

function cacheStoreKey(messageId: string): string {
  return `${CACHE_KEY_PREFIX}${messageId}`;
}

async function getOrCreateMasterKey(secStore: KeyValueStore): Promise<Uint8Array> {
  const hex = await secStore.getItem(MASTER_KEY_STORE_KEY);
  if (hex) return hexToU8(hex);
  const key = new Uint8Array(32);
  crypto.getRandomValues(key);
  await secStore.setItem(MASTER_KEY_STORE_KEY, u8ToHex(key));
  return key;
}

export async function cachePlaintext(messageId: string, plaintext: string, stores: CacheStores = {}): Promise<void> {
  const secStore = stores.secure ?? webSecureStore;
  const asyStore = stores.async ?? webAsyncStore;

  const masterKey = await getOrCreateMasterKey(secStore);
  const nonce = new Uint8Array(12);
  crypto.getRandomValues(nonce);
  const ct = gcm(masterKey, nonce).encrypt(new TextEncoder().encode(plaintext));
  const blob = new Uint8Array(12 + ct.length);
  blob.set(nonce, 0);
  blob.set(ct, 12);

  await asyStore.setItem(cacheStoreKey(messageId), u8ToHex(blob));
}

export async function getCachedPlaintext(messageId: string, stores: CacheStores = {}): Promise<string | null> {
  const secStore = stores.secure ?? webSecureStore;
  const asyStore = stores.async ?? webAsyncStore;

  const hex = await asyStore.getItem(cacheStoreKey(messageId));
  if (!hex) return null;

  try {
    const masterKey = await getOrCreateMasterKey(secStore);
    const blob = hexToU8(hex);
    if (blob.length < 28) return null;
    const plain = gcm(masterKey, blob.slice(0, 12)).decrypt(blob.slice(12));
    return new TextDecoder().decode(plain);
  } catch {
    return null;
  }
}

export async function purgePlaintext(messageId: string, stores: CacheStores = {}): Promise<void> {
  const asyStore = stores.async ?? webAsyncStore;
  await asyStore.deleteItem(cacheStoreKey(messageId));
}
