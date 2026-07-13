import "react-native-get-random-values";
import { x25519 } from "@noble/curves/ed25519.js";
import { u8ToBase64, u8ToHex, hexToU8 } from "./crypto";
import { secureStore, KeyValueStore } from "./keyValueStore";
import { signBytes } from "./identity";

// Signed PreKey (SPK) yönetimi — X3DH'nin yarı-statik anahtarı.
// Rotasyon: aktif SPK 7 günden eskiyse yeni SPK üretilir, eskisi "retired"
// işaretlenir. Retired SPK'lar 30 gün private key'i saklanmaya devam eder
// (o SPK ile başlamış ama henüz tamamlanmamış X3DH oturumlarını çözebilmek
// için — hemen silinirse in-flight handshake'ler kalıcı olarak kırılır),
// 30 gün sonra private key silinir.
const REGISTRY_KEY = "obscura_spk_registry";
const ROTATION_INTERVAL_MS = 7 * 24 * 60 * 60 * 1000;
const RETENTION_MS = 30 * 24 * 60 * 60 * 1000;

export interface SPKRegistryEntry {
  id: number;
  createdAt: number; // epoch ms
  retiredAt: number | null;
  publicKey: string; // base64, X25519
  signature: string; // base64, Ed25519 — identity.ts'in signing key'iyle publicKey üzerinde
}

function privKeyStoreKey(id: number): string {
  return `obscura_spk_private_${id}`;
}

export function generateSignedPreKeyPair(): { privateKey: Uint8Array; publicKey: Uint8Array } {
  const { secretKey: privateKey, publicKey } = x25519.keygen();
  return { privateKey, publicKey };
}

async function loadRegistry(store: KeyValueStore): Promise<SPKRegistryEntry[]> {
  const raw = await store.getItem(REGISTRY_KEY);
  if (!raw) return [];
  return JSON.parse(raw) as SPKRegistryEntry[];
}

async function saveRegistry(store: KeyValueStore, registry: SPKRegistryEntry[]): Promise<void> {
  await store.setItem(REGISTRY_KEY, JSON.stringify(registry));
}

async function createNewSPK(
  store: KeyValueStore,
  registry: SPKRegistryEntry[],
  now: number
): Promise<SPKRegistryEntry> {
  const nextId = registry.reduce((max, e) => Math.max(max, e.id), -1) + 1;
  const { privateKey, publicKey } = generateSignedPreKeyPair();
  const signature = await signBytes(publicKey, store);
  const entry: SPKRegistryEntry = {
    id: nextId,
    createdAt: now,
    retiredAt: null,
    publicKey: u8ToBase64(publicKey),
    signature: u8ToBase64(signature),
  };
  await store.setItem(privKeyStoreKey(nextId), u8ToHex(privateKey));
  registry.push(entry);
  return entry;
}

function pruneExpired(registry: SPKRegistryEntry[], now: number): SPKRegistryEntry[] {
  return registry.filter((e) => e.retiredAt === null || now - e.retiredAt < RETENTION_MS);
}

// Aktif SPK'yı döndürür — yoksa üretir, 7 günden eskiyse rotasyon yapar,
// 30 günden eski retired SPK'ların private key'lerini siler.
export async function rotateIfNeeded(
  store: KeyValueStore = secureStore,
  now: number = Date.now()
): Promise<SPKRegistryEntry> {
  const registry = await loadRegistry(store);
  let active = registry.find((e) => e.retiredAt === null);

  if (!active) {
    active = await createNewSPK(store, registry, now);
  } else if (now - active.createdAt > ROTATION_INTERVAL_MS) {
    active.retiredAt = now;
    active = await createNewSPK(store, registry, now);
  }

  const pruned = pruneExpired(registry, now);
  const keptIds = new Set(pruned.map((e) => e.id));
  for (const e of registry) {
    if (!keptIds.has(e.id)) {
      await store.deleteItem(privKeyStoreKey(e.id));
    }
  }
  await saveRegistry(store, pruned);
  return active;
}

export async function getSignedPreKeyBundle(
  store: KeyValueStore = secureStore,
  now: number = Date.now()
): Promise<{ signedPrekey: string; signedPrekeyId: number; signedPrekeySig: string }> {
  const active = await rotateIfNeeded(store, now);
  return {
    signedPrekey: active.publicKey,
    signedPrekeyId: active.id,
    signedPrekeySig: active.signature,
  };
}

export async function getSignedPreKeyPrivate(
  id: number,
  store: KeyValueStore = secureStore
): Promise<Uint8Array | null> {
  const hex = await store.getItem(privKeyStoreKey(id));
  return hex ? hexToU8(hex) : null;
}

// Alıcı (Bob) tarafı: v2 zarfın x3dh.spk alanındaki PUBLIC key ile kendi SPK
// kayıt defterinden ilgili PRIVATE key'i bulur. Backend GET /v1/keys/{did}
// yanıtı signed_prekey_id DÖNDÜRMEDİĞİ için gönderen id'yi bilemez — public
// key eşleşmesi tek güvenilir çözümleme yolu. Registry retired SPK'ları
// 30 gün tuttuğu için rotasyona rağmen in-flight handshake'ler çözülebilir.
export async function findSignedPreKeyPrivateByPublicKey(
  publicKeyBase64: string,
  store: KeyValueStore = secureStore
): Promise<Uint8Array | null> {
  const registry = await loadRegistry(store);
  const entry = registry.find((e) => e.publicKey === publicKeyBase64);
  if (!entry) return null;
  return getSignedPreKeyPrivate(entry.id, store);
}
