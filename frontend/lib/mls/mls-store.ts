// B10 Faz 1 — mobile/lib/mls/mls-store.ts'nin birebir portu. MANTIK
// DEĞİŞMEDİ (AES-256-GCM master-key şifreleme, aynı storage key prefix'leri,
// aynı fonksiyon imzaları) — SADECE store injection: mobile'ın
// secureStore/asyncStore'u (expo-secure-store/AsyncStorage) yerine
// webSecureStore/webAsyncStore (localStorage/IndexedDB, webStore.ts).
"use client";
import {
  encodeGroupState,
  decodeGroupState,
  defaultKeyRetentionConfig,
  defaultLifetimeConfig,
  defaultKeyPackageEqualityConfig,
  defaultPaddingConfig,
  defaultAuthenticationService,
  type ClientState,
  type ClientConfig,
} from "ts-mls";
import { webSecureStore, webAsyncStore, type KeyValueStore } from "./webStore";
import { getOrCreateMasterKey, encryptBlob, decryptBlob, hexToU8, u8ToHex } from "./cryptoBlob";
import type { RawPrivateKeyPackage } from "./group";

const MLS_MASTER_KEY_STORE_KEY = "obscura_mls_master_key";
const GROUP_STORE_KEY_PREFIX = "obscura_mls_group_";
const KEYPKG_STORE_KEY_PREFIX = "obscura_mls_keypkg_";

// mobile ile AYNI değer (MLS_RETAIN_EPOCHS) — üyeler arası PCS penceresi
// tutarsızlığı doğmasın diye iki platform da 2 kullanmalı.
export const MLS_RETAIN_EPOCHS = 2;

export function mlsClientConfig(): ClientConfig {
  return {
    keyRetentionConfig: {
      ...defaultKeyRetentionConfig,
      retainKeysForEpochs: MLS_RETAIN_EPOCHS,
    },
    lifetimeConfig: defaultLifetimeConfig,
    keyPackageEqualityConfig: defaultKeyPackageEqualityConfig,
    paddingConfig: defaultPaddingConfig,
    authService: defaultAuthenticationService,
  };
}

export interface MlsStores {
  secure?: KeyValueStore;
  async?: KeyValueStore;
}

function groupStoreKey(groupId: string): string {
  return `${GROUP_STORE_KEY_PREFIX}${groupId}`;
}

function keyPackageStoreKey(did: string): string {
  return `${KEYPKG_STORE_KEY_PREFIX}${did}`;
}

// ─── Grup state'i (ClientState) ────────────────────────────────────────────

export async function saveGroupState(groupId: string, state: ClientState, stores: MlsStores = {}): Promise<void> {
  const secStore = stores.secure ?? webSecureStore;
  const asyStore = stores.async ?? webAsyncStore;

  const masterKey = await getOrCreateMasterKey(secStore, MLS_MASTER_KEY_STORE_KEY);
  const plaintext = encodeGroupState(state);
  const blob = encryptBlob(masterKey, plaintext);

  await asyStore.setItem(groupStoreKey(groupId), u8ToHex(blob));
}

export async function loadGroupState(groupId: string, stores: MlsStores = {}): Promise<ClientState | null> {
  const secStore = stores.secure ?? webSecureStore;
  const asyStore = stores.async ?? webAsyncStore;

  const hex = await asyStore.getItem(groupStoreKey(groupId));
  if (!hex) return null;

  const masterKey = await getOrCreateMasterKey(secStore, MLS_MASTER_KEY_STORE_KEY);
  const plaintext = decryptBlob(masterKey, hexToU8(hex));
  const decoded = decodeGroupState(plaintext, 0);
  if (!decoded) throw new Error("loadGroupState: grup state'i decode edilemedi (bozuk kayıt)");
  const [groupState] = decoded;

  return { ...groupState, clientConfig: mlsClientConfig() };
}

export async function deleteGroupState(groupId: string, stores: MlsStores = {}): Promise<void> {
  const asyStore = stores.async ?? webAsyncStore;
  await asyStore.deleteItem(groupStoreKey(groupId));
}

// ─── Kendi KeyPackage private-state'i (local-only, backend'e gitmez) ───────

export interface StoredOwnKeyPackage {
  keyPackageWireB64: string;
  privateKeyPackage: RawPrivateKeyPackage;
}

export async function saveOwnKeyPackage(did: string, kp: StoredOwnKeyPackage, stores: MlsStores = {}): Promise<void> {
  const secStore = stores.secure ?? webSecureStore;
  const asyStore = stores.async ?? webAsyncStore;

  const masterKey = await getOrCreateMasterKey(secStore, MLS_MASTER_KEY_STORE_KEY);
  const plaintext = new TextEncoder().encode(JSON.stringify(kp));
  const blob = encryptBlob(masterKey, plaintext);

  await asyStore.setItem(keyPackageStoreKey(did), u8ToHex(blob));
}

export async function loadOwnKeyPackage(did: string, stores: MlsStores = {}): Promise<StoredOwnKeyPackage | null> {
  const secStore = stores.secure ?? webSecureStore;
  const asyStore = stores.async ?? webAsyncStore;

  const hex = await asyStore.getItem(keyPackageStoreKey(did));
  if (!hex) return null;

  const masterKey = await getOrCreateMasterKey(secStore, MLS_MASTER_KEY_STORE_KEY);
  const plaintext = decryptBlob(masterKey, hexToU8(hex));
  return JSON.parse(new TextDecoder().decode(plaintext)) as StoredOwnKeyPackage;
}

export async function deleteOwnKeyPackage(did: string, stores: MlsStores = {}): Promise<void> {
  const asyStore = stores.async ?? webAsyncStore;
  await asyStore.deleteItem(keyPackageStoreKey(did));
}

// ─── "Backend'e yüklendi mi" bayrağı ────────────────────────────────────────

const KEYPKG_UPLOADED_FLAG_PREFIX = "obscura_mls_keypkg_uploaded_";

function keyPackageUploadedFlagKey(did: string): string {
  return `${KEYPKG_UPLOADED_FLAG_PREFIX}${did}`;
}

export async function markKeyPackageUploaded(did: string, stores: MlsStores = {}): Promise<void> {
  const asyStore = stores.async ?? webAsyncStore;
  await asyStore.setItem(keyPackageUploadedFlagKey(did), "1");
}

export async function hasUploadedKeyPackage(did: string, stores: MlsStores = {}): Promise<boolean> {
  const asyStore = stores.async ?? webAsyncStore;
  return (await asyStore.getItem(keyPackageUploadedFlagKey(did))) === "1";
}
