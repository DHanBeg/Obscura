// L2 Tuğla 5a — MLS grup state'inin cihazda kalıcılığı (#10 mimari notu:
// KeyPackage private-key + ClientState LOCAL saklanır, backend'den değil).
//
// Şifreleme YENİDEN YAZILMADI — session-store.ts'in AES-256-GCM primitifini
// (getOrCreateMasterKey/encryptBlob/decryptBlob) doğrudan çağırıyoruz, sadece
// ayrı bir SecureStore anahtarıyla (MLS_MASTER_KEY_STORE_KEY) — ratchet
// oturumlarının master key'inden BAĞIMSIZ, biri sızarsa diğerini etkilemez.
//
// AsyncStorage setItem her iki platformda da atomik (Android: SQLite
// transaction + CONFLICT_REPLACE; iOS: writeToFile atomically:YES) — bu
// yüzden write-then-swap (geçici key → doğrula → swap → eski sil) YOK, tek
// setItem çağrısı zaten kesintide ya eski ya yeni bütün değeri bırakır.
//
// TRİM YASAK: session-store.ts'in skip-buffer kırpma deseni (trimSkippedForStorage)
// buraya KOPYALANMADI. Ratchet'te kayıp bir skipped-key sadece tek bir geçmiş
// mesajın kaybı demek — kabul edilebilir. MLS ClientState'te öyle değil: state'in
// bir kısmı eksik kalırsa sonraki commit hiç işlenemez, üye gruptan tamamen
// kopar. Bu modül state'i HER ZAMAN tam yazar/okur, kısmi/kırpılmış yazım yok.
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
import { secureStore, asyncStore, KeyValueStore } from "../keyValueStore";
import { getOrCreateMasterKey, encryptBlob, decryptBlob } from "../session-store";
import { hexToU8, u8ToHex } from "../crypto";
import type { RawPrivateKeyPackage } from "./group";

const MLS_MASTER_KEY_STORE_KEY = "obscura_mls_master_key";
const GROUP_STORE_KEY_PREFIX = "obscura_mls_group_";
const KEYPKG_STORE_KEY_PREFIX = "obscura_mls_keypkg_";

// PCS kalibrasyonu (Tuğla 5a bölüm B) — compromise anında son N epoch'un
// mesajları hâlâ çözülebilir demek (ts-mls varsayılanı 4). 2'ye çekmek bu
// pencereyi daraltır; geç-mesaj toleransını da aynı oranda azaltır. Tek
// yerden değiştirilebilsin diye sabit — clientConfig'e gömülü değil.
export const MLS_RETAIN_EPOCHS = 2;

/** Grup state'i oluşturulurken/katılırken KULLANILMASI gereken clientConfig —
 * authService gibi fonksiyon içeren alanlar taşıdığı için DİSKE YAZILMAZ
 * (bkz. saveGroupState); her load'da bu fonksiyonla taze üretilip state'e
 * yeniden eklenir. createGroup/joinGroup çağıran HER yer (group.ts) aynı
 * config'i kullanmalı — biri varsayılan (4) diğeri 2 kullanırsa üyeler
 * arası pencere tutarsızlığı doğar. */
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

/** ClientState'i şifreli olarak sakla. Sadece ts-mls'in kendi TLS-codec
 * encode'u ile üretilen GroupState byte'ları yazılır (clientConfig HARİÇ —
 * fonksiyon içerdiği için serialize edilemez, load'da mlsClientConfig()
 * ile yeniden eklenir). Tek key, tek blob — bkz. modül başı not. */
export async function saveGroupState(groupId: string, state: ClientState, stores: MlsStores = {}): Promise<void> {
  const secStore = stores.secure ?? secureStore;
  const asyStore = stores.async ?? asyncStore;

  const masterKey = await getOrCreateMasterKey(secStore, MLS_MASTER_KEY_STORE_KEY);
  const plaintext = encodeGroupState(state);
  const blob = encryptBlob(masterKey, plaintext);

  await asyStore.setItem(groupStoreKey(groupId), u8ToHex(blob));
}

/** Şifreli grup state'ini yükle. Kayıt yoksa null döner. clientConfig
 * mlsClientConfig() ile taze eklenir — diskten okunmaz. */
export async function loadGroupState(groupId: string, stores: MlsStores = {}): Promise<ClientState | null> {
  const secStore = stores.secure ?? secureStore;
  const asyStore = stores.async ?? asyncStore;

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
  const asyStore = stores.async ?? asyncStore;
  await asyStore.deleteItem(groupStoreKey(groupId));
}

// ─── Kendi KeyPackage private-state'i (#10: local-only, backend'e gitmez) ──

export interface StoredOwnKeyPackage {
  keyPackageWireB64: string;
  privateKeyPackage: RawPrivateKeyPackage;
}

export async function saveOwnKeyPackage(did: string, kp: StoredOwnKeyPackage, stores: MlsStores = {}): Promise<void> {
  const secStore = stores.secure ?? secureStore;
  const asyStore = stores.async ?? asyncStore;

  const masterKey = await getOrCreateMasterKey(secStore, MLS_MASTER_KEY_STORE_KEY);
  const plaintext = new TextEncoder().encode(JSON.stringify(kp));
  const blob = encryptBlob(masterKey, plaintext);

  await asyStore.setItem(keyPackageStoreKey(did), u8ToHex(blob));
}

export async function loadOwnKeyPackage(did: string, stores: MlsStores = {}): Promise<StoredOwnKeyPackage | null> {
  const secStore = stores.secure ?? secureStore;
  const asyStore = stores.async ?? asyncStore;

  const hex = await asyStore.getItem(keyPackageStoreKey(did));
  if (!hex) return null;

  const masterKey = await getOrCreateMasterKey(secStore, MLS_MASTER_KEY_STORE_KEY);
  const plaintext = decryptBlob(masterKey, hexToU8(hex));
  return JSON.parse(new TextDecoder().decode(plaintext)) as StoredOwnKeyPackage;
}

export async function deleteOwnKeyPackage(did: string, stores: MlsStores = {}): Promise<void> {
  const asyStore = stores.async ?? asyncStore;
  await asyStore.deleteItem(keyPackageStoreKey(did));
}

// ─── "Backend'e yüklendi mi" bayrağı (Tuğla 5c bölüm 2 — davet-edilebilirlik) ─
//
// Şifreli DEĞİL — gizli bir şey taşımıyor, sadece "bu did için mevcut own
// KeyPackage'ı backend'e yükledim mi" boole'u. saveOwnKeyPackage/
// loadOwnKeyPackage'dan AYRI: bir own-keypackage lokal olarak VAR OLABİLİR
// ama hiç upload edilmemiş olabilir (createGroupFlow.ts'in creator-tarafı
// tam olarak bunu yapıyor — kendi kullanımı için üretip sadece lokale
// kaydediyor, backend'e hiç göndermiyor).

const KEYPKG_UPLOADED_FLAG_PREFIX = "obscura_mls_keypkg_uploaded_";

function keyPackageUploadedFlagKey(did: string): string {
  return `${KEYPKG_UPLOADED_FLAG_PREFIX}${did}`;
}

export async function markKeyPackageUploaded(did: string, stores: MlsStores = {}): Promise<void> {
  const asyStore = stores.async ?? asyncStore;
  await asyStore.setItem(keyPackageUploadedFlagKey(did), "1");
}

export async function hasUploadedKeyPackage(did: string, stores: MlsStores = {}): Promise<boolean> {
  const asyStore = stores.async ?? asyncStore;
  return (await asyStore.getItem(keyPackageUploadedFlagKey(did))) === "1";
}
