import "react-native-get-random-values";
import { x25519 } from "@noble/curves/ed25519.js";
import { u8ToBase64, u8ToHex, hexToU8 } from "./crypto";
import { secureStore, KeyValueStore } from "./keyValueStore";

// Tek kullanımlık PreKey (OPK) havuzu — X3DH'nin opsiyonel 4. DH'i (DH4).
// SPK'nın aksine rotasyon yok: her OPK sunucuda bir kez dağıtılır (bkz.
// backend keys.go GET /v1/keys/{did}), tüketildikten sonra client tarafında
// bilinmez — hangi id'nin tüketildiğini client asla öğrenmez. Bu yüzden
// pruning burada YOK (Adım 7 kapsamı dışı, bilinen boşluk): private key'ler
// x3dhAccept ihtiyaç duyduğu sürece saklanmalı, retention politikası ayrı
// bir adımda ele alınacak.
const REGISTRY_KEY = "obscura_opk_registry";

export interface OPKRegistryEntry {
  id: number;
  publicKey: string; // base64, X25519
}

function privKeyStoreKey(id: number): string {
  return `obscura_opk_private_${id}`;
}

export function generateOneTimePreKeyPair(): { privateKey: Uint8Array; publicKey: Uint8Array } {
  const { secretKey: privateKey, publicKey } = x25519.keygen();
  return { privateKey, publicKey };
}

async function loadRegistry(store: KeyValueStore): Promise<OPKRegistryEntry[]> {
  const raw = await store.getItem(REGISTRY_KEY);
  if (!raw) return [];
  return JSON.parse(raw) as OPKRegistryEntry[];
}

async function saveRegistry(store: KeyValueStore, registry: OPKRegistryEntry[]): Promise<void> {
  await store.setItem(REGISTRY_KEY, JSON.stringify(registry));
}

// `count` yeni OPK üretir, private'ları store'a yazar, registry'e ekler.
// id'ler mevcut registry'nin devamı (monoton artan) — sunucudaki opk_id ile
// birebir eşleşecek şekilde upload'a bu şekliyle geçilmeli.
export async function generateBatch(
  count: number,
  store: KeyValueStore = secureStore
): Promise<OPKRegistryEntry[]> {
  const registry = await loadRegistry(store);
  let nextId = registry.reduce((max, e) => Math.max(max, e.id), -1) + 1;
  const fresh: OPKRegistryEntry[] = [];

  for (let i = 0; i < count; i++) {
    const { privateKey, publicKey } = generateOneTimePreKeyPair();
    const entry: OPKRegistryEntry = { id: nextId, publicKey: u8ToBase64(publicKey) };
    await store.setItem(privKeyStoreKey(nextId), u8ToHex(privateKey));
    registry.push(entry);
    fresh.push(entry);
    nextId++;
  }

  await saveRegistry(store, registry);
  return fresh;
}

export async function getOneTimePreKeyPrivate(
  id: number,
  store: KeyValueStore = secureStore
): Promise<Uint8Array | null> {
  const hex = await store.getItem(privKeyStoreKey(id));
  return hex ? hexToU8(hex) : null;
}

// Başarılı bir x3dhAccept SONRASI tüketilen OPK private'ını siler — "one-time"
// garantisi: aynı bootstrap zarfının replay'i ile shared_key bir daha
// türetilemez. (Genel retention/pruning politikası hâlâ ayrı bir adımın işi;
// bu sadece KULLANILMIŞ anahtarın imhası.) Registry'deki public kayıt kalır —
// id sayacının monotonluğu bozulmaz.
export async function deleteOneTimePreKeyPrivate(
  id: number,
  store: KeyValueStore = secureStore
): Promise<void> {
  await store.deleteItem(privKeyStoreKey(id));
}
