// Prekey yükleme — TEK KAYNAK.
//
// Önceki hal (login/page.tsx:310-317, inline): her login generatePreKeyStore()
// çağırıyordu, bu da HER SEFERİNDE taze rastgele SPK + 100 OPK (id 0-99)
// üretip POST /v1/keys/upload'a gönderiyordu. Backend upsert olduğu için SPK
// koşulsuz üstüne yazılıyordu (mevcut in-flight X3DH handshake'leri kırar);
// OPK satırları ise DB'de PK olarak uuid kullandığından (opk_id değil —
// backend/internal/db/database.go one_time_prekeys.id) ON CONFLICT DO
// NOTHING hiç tetiklenmiyor, her login'de +100 satır sınırsız birikiyordu.
// AppShell.tsx'te bu akışın ikinci bir kopyası YOK (grep ile doğrulandı) —
// tek kaynak buradaydı, ama idempotent değildi.
//
// Mobile lib/keys-sync.ts ensureKeyBundleUploaded ile aynı desen: SPK yerelde
// üretilip kalıcı tutulur (yoksa üret, varsa AYNI SPK'yı yeniden gönder —
// upsert no-op), OPK sadece sunucu watermark'ın altındaysa tamamlanır.
//
// Faz-1c-3 (VDS merge): bu modül artık decrypt tarafının ihtiyacı olan
// PreKeyStore'u da döndürür. OPK private anahtarları yerelde (e2ee.ts
// savePreKeyStore, parola-bazlı + şifreli) saklanır — eskiden yalnız public
// yüklenip private atılıyordu, x3dhAccept OPK'yi bulamayıp sessizce 3-DH'ye
// düşüyor, Alice 4-DH yaptığı için ilk mesaj çözülemiyordu. SPK registry'si de
// e2ee.ts'in identityStorageKey(passphrase) anahtarlamasıyla hesap-bazlıdır.
import { api } from "./api";
import {
  generateX25519,
  ed25519Sign,
  loadPreKeyStore,
  savePreKeyStore,
  removeUsedOneTimePreKey,
  X25519_KEY_LEN,
  type IdentityKeys,
  type OPK,
  type PreKeyStore,
  type X25519KeyPair,
} from "./e2ee";

function toB64(bytes: Uint8Array): string {
  return btoa(String.fromCharCode(...Array.from(bytes)));
}
function fromB64(b64: string): Uint8Array {
  return Uint8Array.from(atob(b64), (c) => c.charCodeAt(0));
}

// Format 2 = X25519 ham byte (2026-09-17 P-256→X25519 geçişi). Format 1
// (marker YOK) = eski WebCrypto P-256 kaydı: priv pkcs8-DER, pub 65 byte
// uncompressed. e2ee.ts'teki IDENTITY_FORMAT_VERSION ile aynı gerekçe/tarih —
// bkz. orada.
const SPK_FORMAT_VERSION = 2;

// mobile/lib/prekeys.ts SPKRegistryEntry[] ile AYNI desen (çoğul, tek slot
// DEĞİL) — grep'le doğrulandı: mobile eski SPK'yı retiredAt alıp RETENTION_MS
// (30 gün) private key'iyle birlikte saklıyor. Web'de periyodik rotasyon
// (mobile'daki 7 günlük ROTATION_INTERVAL_MS) tetikleyicisi YOK — bu görevin
// kapsamı dışı, eklenmedi. Ama tek canlı regen yolu (bozuk/okunamaz kayıt)
// eskiden eskiyi SESSİZCE SİLİYORDU; artık mobile ile aynı şekilde retiredAt
// alıp registry'de kalıyor, 30 gün sonra budanıyor. Bu, storage'ın TAMAMEN
// silinmesi senaryosunu (registry'nin kendisi de gider — mobile dahil hiçbir
// client-side registry bunu çözemez, kanıtlı) çözmez; çözdüğü, geçerli bir
// SPK'nın gelecekte herhangi bir nedenle (rotasyon eklenirse, ya da bugünkü
// "kayıt bozuk" fallback'i genişlerse) sessizce kaybolmasını önlemek.
export const SIGNED_PREKEY_STORAGE_KEY = "obscura_signed_prekey_registry_v1";
// Hesap-bazlı: e2ee.ts identityStorageKey(passphrase) ile aynı desen — aksi
// halde aynı tarayıcıdaki ikinci hesap birincinin SPK'sını (birincinin imza
// anahtarıyla imzalı) içe aktarıp kendi adına yüklerdi.
export function signedPreKeyStorageKey(passphrase: string): string {
  return `${SIGNED_PREKEY_STORAGE_KEY}:${passphrase}`;
}
const RETENTION_MS = 30 * 24 * 60 * 60 * 1000; // mobile prekeys.ts:15 ile aynı
const OPK_LOW_WATERMARK = 20;
const OPK_BATCH_SIZE = 100;

interface SignedPreKeyRegistryEntry {
  format?: number; // yoksa eski (P-256) kayıt — bkz. SPK_FORMAT_VERSION
  pub: string; // base64, raw X25519 public key (32 byte)
  priv: string; // base64, raw X25519 private key (32 byte)
  sig: string; // base64, Ed25519 signature over pub
  createdAt: number; // epoch ms
  retiredAt: number | null;
  // Bu SPK'nın sunucuya tam bundle olarak yazıldığı an. GET /v1/keys/opk/count
  // bundle'ı hiç olmayan kullanıcıda da 200 + count:0 döner (hata vermez), yani
  // "sunucuda bundle var mı" o endpoint'ten ayırt edilemez — yerel işaret şart.
  uploadedAt?: number | null;
}

function loadRegistry(passphrase: string): SignedPreKeyRegistryEntry[] {
  if (typeof localStorage === "undefined") return [];
  const raw = localStorage.getItem(signedPreKeyStorageKey(passphrase));
  if (!raw) return [];
  try {
    return JSON.parse(raw) as SignedPreKeyRegistryEntry[];
  } catch {
    return [];
  }
}

function saveRegistry(passphrase: string, registry: SignedPreKeyRegistryEntry[]): void {
  localStorage.setItem(signedPreKeyStorageKey(passphrase), JSON.stringify(registry));
}

function pruneExpired(registry: SignedPreKeyRegistryEntry[], now: number): SignedPreKeyRegistryEntry[] {
  return registry.filter((e) => e.retiredAt === null || now - e.retiredAt < RETENTION_MS);
}

async function importSignedPreKey(
  entry: SignedPreKeyRegistryEntry
): Promise<{ keyPair: X25519KeyPair; signature: Uint8Array }> {
  if (entry.format !== SPK_FORMAT_VERSION) {
    console.warn(
      `SPK kaydı eski formatta (format=${entry.format ?? "yok"}, beklenen ${SPK_FORMAT_VERSION}) ` +
      `— P-256'dan X25519'a geçiş (2026-09-17). Eski kayıt GEÇERSİZ sayılıyor, retire edilecek.`
    );
    throw new Error("eski formatlı SPK kaydı (P-256) — X25519 olarak yorumlanmadı");
  }

  const privateKey = fromB64(entry.priv);
  const publicKeyBytes = fromB64(entry.pub);
  if (privateKey.length !== X25519_KEY_LEN || publicKeyBytes.length !== X25519_KEY_LEN) {
    throw new Error("SPK kaydı beklenmeyen byte uzunluğunda (format doğru ama boyut yanlış)");
  }

  return {
    keyPair: { privateKey, publicKeyBytes },
    signature: fromB64(entry.sig),
  };
}

// Aktif SPK'yı döndürür (upload'a HER ZAMAN aynı değer gider — upsert
// no-op). Aktif kayıt yoksa ya da okunamıyorsa (bozuk JSON/import hatası)
// yenisi üretilir — eskisi varsa SİLİNMEZ, retiredAt alıp registry'de kalır
// (mobile rotateIfNeeded ile aynı taşıma deseni).
async function getOrCreateSignedPreKey(
  identity: IdentityKeys,
  passphrase: string
): Promise<{ keyPair: X25519KeyPair; signature: Uint8Array; uploaded: boolean }> {
  const now = Date.now();
  const registry = loadRegistry(passphrase);
  const activeIdx = registry.findIndex((e) => e.retiredAt === null);

  if (activeIdx !== -1) {
    try {
      const result = await importSignedPreKey(registry[activeIdx]);
      saveRegistry(passphrase, pruneExpired(registry, now));
      return { ...result, uploaded: registry[activeIdx].uploadedAt != null };
    } catch {
      // Bozuk/okunamaz aktif kayıt — silinmeden retire edilir, aşağıda
      // yenisi üretilir.
      registry[activeIdx] = { ...registry[activeIdx], retiredAt: now };
    }
  }

  const keyPair = await generateX25519();
  const signature = await ed25519Sign(identity.signingKeyPair.privateKey, keyPair.publicKeyBytes);
  registry.push({
    format: SPK_FORMAT_VERSION,
    pub: toB64(keyPair.publicKeyBytes),
    priv: toB64(keyPair.privateKey),
    sig: toB64(signature),
    createdAt: now,
    retiredAt: null,
    uploadedAt: null,
  });
  saveRegistry(passphrase, pruneExpired(registry, now));
  return { keyPair, signature, uploaded: false };
}

function markSpkUploaded(publicKeyB64: string, passphrase: string): void {
  const registry = loadRegistry(passphrase);
  const entry = registry.find((e) => e.pub === publicKeyB64 && e.retiredAt === null);
  if (!entry) return;
  entry.uploadedAt = Date.now();
  saveRegistry(passphrase, registry);
}

async function generateOPKBatch(count: number, takenIds: Set<number>): Promise<OPK[]> {
  const batch: OPK[] = [];
  while (batch.length < count) {
    // Sunucuda (did, opk_id) benzersiz + ON CONFLICT DO NOTHING (keys.go): aynı
    // id'yle yeniden yükleme eski public'i korur, yerelde yeni private kalırdı.
    // Bu yüzden sabit 0-99 yerine rastgele id, yerelde çakışma da elenir.
    const id = Math.floor(Math.random() * 2 ** 31);
    if (takenIds.has(id)) continue;
    takenIds.add(id);
    batch.push({ id, keyPair: await generateX25519() });
  }
  return batch;
}

export interface EnsurePreKeysResult {
  uploaded: boolean;
  reason: "initial" | "replenished" | "sufficient";
  opkCount: number;
}

export interface SyncPreKeysResult extends EnsurePreKeysResult {
  store: PreKeyStore;
}

// AppShell bootstrap'ından çağrılan senkronizasyon çekirdeği. Sunucudaki
// mevcut OPK sayısını okur: yeterliyse hiçbir şey üretmez/yüklemez; watermark
// altındaysa sadece eksik OPK'yı tamamlar (SPK'ya dokunmadan); bundle hiç yoksa
// (getOPKCount hata verir) tam bundle'ı (identity + SPK + ilk OPK batch'i) tek
// seferde yükler. Her durumda decrypt için PreKeyStore döner.
//
// Sıra önemli: OPK private anahtarları sunucuya public gitmeden ÖNCE yerelde
// kalıcı yazılır — tersi (yükle, sonra yaz) yazma başarısız olursa sunucunun
// dağıttığı OPK'nin private'ı hiç bulunamaz.
//
// In-flight kilit (parola başına): effect'in üst üste (remount/StrictMode)
// tetiklenmesinde iki eşzamanlı senkronizasyon FARKLI OPK setleri üretip
// ikisini de yükleyebiliyordu.
const inFlight = new Map<string, Promise<SyncPreKeysResult>>();

// DID → parola: consumeOneTimePreKey'in (decrypt yolu, parolayı bilmez) doğru
// hesabın deposuna yazabilmesi için. syncPreKeys her çağrıda günceller.
const passphraseByDid = new Map<string, string>();

export async function syncPreKeys(
  identity: IdentityKeys,
  passphrase: string
): Promise<SyncPreKeysResult> {
  const existingCall = inFlight.get(passphrase);
  if (existingCall) return existingCall;

  const call = doSyncPreKeys(identity, passphrase);
  inFlight.set(passphrase, call);
  try {
    return await call;
  } finally {
    inFlight.delete(passphrase);
  }
}

async function doSyncPreKeys(identity: IdentityKeys, passphrase: string): Promise<SyncPreKeysResult> {
  passphraseByDid.set(identity.did, passphrase);

  const {
    keyPair: signedPreKey,
    signature: signedPreKeySig,
    uploaded: spkUploaded,
  } = await getOrCreateSignedPreKey(identity, passphrase);

  // SPK'nın tek doğruluk kaynağı registry; kalıcı PreKeyStore'dan yalnız OPK'ler alınır.
  const localOPKs = (await loadPreKeyStore(identity, passphrase))?.oneTimePreKeys ?? [];
  const assemble = (opks: OPK[]): PreKeyStore => ({
    identity,
    signedPreKey,
    signedPreKeySig,
    oneTimePreKeys: opks,
  });

  let opkCount = 0;
  let bundleExists = true;
  try {
    const res = await api.getOPKCount();
    opkCount = res.count;
  } catch {
    bundleExists = false;
  }

  // Tam bundle (identity + SPK + OPK) gerekir: count endpoint'i hata verdiyse
  // YA DA bu SPK henüz hiç yüklenmediyse (count:0 "bundle yok" ile "bundle var
  // ama OPK bitmiş" arasında ayırt edemez — bkz. SignedPreKeyRegistryEntry.uploadedAt).
  const needsBundle = !bundleExists || !spkUploaded;

  if (!needsBundle && opkCount >= OPK_LOW_WATERMARK) {
    return { uploaded: false, reason: "sufficient", opkCount, store: assemble(localOPKs) };
  }

  // Tam bundle yeniden yüklenirken (ilk kurulum, sunucu sıfırlanması ya da önceki
  // yükleme yarıda kalmışsa) private'ı zaten elimizde olan yerel OPK'ler yeniden
  // kullanılır; aksi halde sunucu yokken her açılış +OPK_BATCH_SIZE private
  // biriktirirdi. Replenish'te ise her zaman taze batch gider.
  const reusable = needsBundle ? localOPKs.slice(0, OPK_BATCH_SIZE) : [];
  const generated = await generateOPKBatch(
    OPK_BATCH_SIZE - reusable.length,
    new Set(localOPKs.map((o) => o.id))
  );
  const freshOPKs = [...reusable, ...generated];
  const store = assemble([...localOPKs, ...generated]);
  await savePreKeyStore(store, passphrase);

  if (needsBundle) {
    await api.uploadPrekeys({
      identity_key: toB64(identity.dhKeyPair.publicKeyBytes),
      signing_key: toB64(identity.signingKeyPair.publicKeyBytes),
      signed_prekey: toB64(signedPreKey.publicKeyBytes),
      signed_prekey_sig: toB64(signedPreKeySig),
      signed_prekey_id: 0,
      one_time_prekeys: freshOPKs.map((opk) => ({
        id: opk.id,
        public_key: toB64(opk.keyPair.publicKeyBytes),
      })),
    });
    markSpkUploaded(toB64(signedPreKey.publicKeyBytes), passphrase);
    return { uploaded: true, reason: "initial", opkCount: freshOPKs.length, store };
  }

  await api.replenishOPK({
    one_time_prekeys: freshOPKs.map((opk) => ({
      id: opk.id,
      public_key: toB64(opk.keyPair.publicKeyBytes),
    })),
  });
  return { uploaded: true, reason: "replenished", opkCount: opkCount + freshOPKs.length, store };
}

// Yerel (ağsız) PreKeyStore: aktif SPK registry'den + kalıcı OPK'ler. Hiçbir
// şey üretmez/yüklemez. Kayıt yoksa/okunamıyorsa null.
async function loadLocalPreKeyStore(
  identity: IdentityKeys,
  passphrase: string
): Promise<PreKeyStore | null> {
  const active = loadRegistry(passphrase).find((e) => e.retiredAt === null);
  if (!active) return null;
  try {
    const { keyPair, signature } = await importSignedPreKey(active);
    const opks = (await loadPreKeyStore(identity, passphrase))?.oneTimePreKeys ?? [];
    return { identity, signedPreKey: keyPair, signedPreKeySig: signature, oneTimePreKeys: opks };
  } catch {
    return null;
  }
}

// AppShell bootstrap'ının TEK giriş noktası. Decrypt yolu ağa bağımlı
// olmamalı: senkronizasyon (sunucu count/upload) başarısız olursa ve yerelde
// kullanılabilir bir depo varsa o döner; yoksa hata yukarı fırlar.
export async function ensurePreKeysUploaded(
  identity: IdentityKeys,
  passphrase: string
): Promise<PreKeyStore> {
  try {
    return (await syncPreKeys(identity, passphrase)).store;
  } catch (e) {
    const local = await loadLocalPreKeyStore(identity, passphrase);
    if (!local) throw e;
    console.warn("ensurePreKeysUploaded: sunucu senkronu başarısız, yerel PreKeyStore kullanılıyor:", e);
    return local;
  }
}

// x3dhAccept bir OPK tükettikten sonra (e2ee-session decryptIncoming) çağrılır:
// kullanılan OPK'nin private'ını kalıcı depodan siler (tek kullanımlık). Bellek-
// içi store bayat olabileceğinden (önceden silinmiş OPK'yi geri yazmamak için)
// kalıcı depo yeniden okunur.
export async function consumeOneTimePreKey(store: PreKeyStore, opkId: number): Promise<void> {
  const passphrase = passphraseByDid.get(store.identity.did);
  if (!passphrase) {
    console.warn("consumeOneTimePreKey: bu DID için parola bilinmiyor (syncPreKeys çağrılmamış) — OPK silinmedi");
    return;
  }
  const current = (await loadPreKeyStore(store.identity, passphrase)) ?? store;
  await removeUsedOneTimePreKey(current, opkId, passphrase);
}
