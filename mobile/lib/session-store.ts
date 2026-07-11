import "react-native-get-random-values";
import { gcm } from "@noble/ciphers/aes.js";
import { RatchetState, SerializedRatchetState } from "./ratchet";
import { secureStore, asyncStore, KeyValueStore } from "./keyValueStore";
import { hexToU8, u8ToHex } from "./crypto";

// Ratchet oturumlarının şifreli kalıcılığı.
//
// Master key (32 byte, rastgele) SecureStore'da (küçük, cihaz keychain/
// keystore korumalı). Oturum durumu (RatchetState.serialize() — TÜM zincir
// anahtarlarını ve DH private key'i açık taşır) bu master key ile AES-256-GCM
// şifrelenip AsyncStorage'da tutulur — SecureStore'un pratik boyut sınırını
// (iOS Keychain ~2KB/item) aşabilecek kadar büyük olabilir, AsyncStorage
// bunun için var.
//
// Skip buffer sınırı: RatchetState zaten MAX_SKIP=1000 mesajla out-of-order
// buffer'ı sınırlıyor (DoS koruması), ama 1000 kayıt serialize edilince
// ~100-150KB'a çıkabilir — her mesajdan sonra AsyncStorage'a bu boyutta yazmak
// gereksiz disk/IO maliyeti. Saklanan skip buffer'ı ayrıca ~2KB'a kırpıyoruz:
// en YENİ eklenen kayıtlar tutulur (geç gelmesi en olası olan mesajlar
// bunlar), daha eskiler diskten düşer. Bellekteki (in-memory) RatchetState
// bundan etkilenmez — sadece disk'e ne yazıldığı sınırlanır.

const MASTER_KEY_STORE_KEY = "obscura_ratchet_master_key";
const SESSION_KEY_PREFIX = "obscura_ratchet_session_";
const MAX_SKIPPED_STORAGE_BYTES = 2048;

export interface SessionStores {
  secure?: KeyValueStore;
  async?: KeyValueStore;
}

function sessionStoreKey(sessionId: string): string {
  return `${SESSION_KEY_PREFIX}${sessionId}`;
}

async function getOrCreateMasterKey(secStore: KeyValueStore): Promise<Uint8Array> {
  const hex = await secStore.getItem(MASTER_KEY_STORE_KEY);
  if (hex) return hexToU8(hex);
  const key = new Uint8Array(32);
  crypto.getRandomValues(key);
  await secStore.setItem(MASTER_KEY_STORE_KEY, u8ToHex(key));
  return key;
}

// En son eklenen skipped-key kayıtlarını, toplam boyut MAX_SKIPPED_STORAGE_BYTES'ı
// aşmayacak şekilde sondan başlayarak tutar (eskiler diskten düşer).
function trimSkippedForStorage(
  mkSkipped: SerializedRatchetState["mkSkipped"]
): SerializedRatchetState["mkSkipped"] {
  const kept: SerializedRatchetState["mkSkipped"] = [];
  let size = 0;
  for (let i = mkSkipped.length - 1; i >= 0; i--) {
    const entry = mkSkipped[i];
    const entrySize = entry.key.length + entry.mk.length + 16; // JSON alan adları + ayraçlar için pay
    if (size + entrySize > MAX_SKIPPED_STORAGE_BYTES) break;
    kept.unshift(entry);
    size += entrySize;
  }
  return kept;
}

function encryptBlob(masterKey: Uint8Array, plaintext: Uint8Array): Uint8Array {
  const nonce = new Uint8Array(12);
  crypto.getRandomValues(nonce);
  const ct = gcm(masterKey, nonce).encrypt(plaintext);
  const out = new Uint8Array(12 + ct.length);
  out.set(nonce, 0);
  out.set(ct, 12);
  return out;
}

function decryptBlob(masterKey: Uint8Array, blob: Uint8Array): Uint8Array {
  if (blob.length < 28) throw new Error("Oturum verisi bozuk (çok kısa)");
  const nonce = blob.slice(0, 12);
  const ct = blob.slice(12);
  try {
    return gcm(masterKey, nonce).decrypt(ct);
  } catch {
    throw new Error("Oturum şifresi çözülemedi — bütünlük bozulmuş veya master key yanlış");
  }
}

// Ratchet oturumunu şifreli olarak sakla. `sessionId` genelde peer DID'i
// (1:1 sohbet) — çağıran taraf belirler, bu modül anlam yüklemiyor.
export async function saveSession(sessionId: string, state: RatchetState, stores: SessionStores = {}): Promise<void> {
  const secStore = stores.secure ?? secureStore;
  const asyStore = stores.async ?? asyncStore;

  const masterKey = await getOrCreateMasterKey(secStore);
  const serialized = state.serialize();
  serialized.mkSkipped = trimSkippedForStorage(serialized.mkSkipped);

  const plaintext = new TextEncoder().encode(JSON.stringify(serialized));
  const blob = encryptBlob(masterKey, plaintext);

  await asyStore.setItem(sessionStoreKey(sessionId), u8ToHex(blob));
}

// Şifreli oturumu yükle. Kayıt yoksa null döner.
export async function loadSession(sessionId: string, stores: SessionStores = {}): Promise<RatchetState | null> {
  const secStore = stores.secure ?? secureStore;
  const asyStore = stores.async ?? asyncStore;

  const hex = await asyStore.getItem(sessionStoreKey(sessionId));
  if (!hex) return null;

  const masterKey = await getOrCreateMasterKey(secStore);
  const plaintext = decryptBlob(masterKey, hexToU8(hex));
  const serialized = JSON.parse(new TextDecoder().decode(plaintext)) as SerializedRatchetState;
  return RatchetState.deserialize(serialized);
}

export async function deleteSession(sessionId: string, stores: SessionStores = {}): Promise<void> {
  const asyStore = stores.async ?? asyncStore;
  await asyStore.deleteItem(sessionStoreKey(sessionId));
}
