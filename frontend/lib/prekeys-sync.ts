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
import { api } from "./api";
import {
  generateX25519,
  ed25519Sign,
  X25519_KEY_LEN,
  type IdentityKeys,
  type OPK,
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

function loadRegistry(): SignedPreKeyRegistryEntry[] {
  if (typeof localStorage === "undefined") return [];
  const raw = localStorage.getItem(SIGNED_PREKEY_STORAGE_KEY);
  if (!raw) return [];
  try {
    return JSON.parse(raw) as SignedPreKeyRegistryEntry[];
  } catch {
    return [];
  }
}

function saveRegistry(registry: SignedPreKeyRegistryEntry[]): void {
  localStorage.setItem(SIGNED_PREKEY_STORAGE_KEY, JSON.stringify(registry));
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
  identity: IdentityKeys
): Promise<{ keyPair: X25519KeyPair; signature: Uint8Array; uploaded: boolean }> {
  const now = Date.now();
  const registry = loadRegistry();
  const activeIdx = registry.findIndex((e) => e.retiredAt === null);

  if (activeIdx !== -1) {
    try {
      const result = await importSignedPreKey(registry[activeIdx]);
      saveRegistry(pruneExpired(registry, now));
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
  saveRegistry(pruneExpired(registry, now));
  return { keyPair, signature, uploaded: false };
}

function markSpkUploaded(publicKeyB64: string): void {
  const registry = loadRegistry();
  const entry = registry.find((e) => e.pub === publicKeyB64 && e.retiredAt === null);
  if (!entry) return;
  entry.uploadedAt = Date.now();
  saveRegistry(registry);
}

async function generateOPKBatch(count: number): Promise<OPK[]> {
  const batch: OPK[] = [];
  for (let i = 0; i < count; i++) {
    // DB tarafında opk_id benzersizliği zorunlu değil (PK gerçek uuid) —
    // yine de çakışmayı azaltmak için 0-99 sabit aralığı yerine rastgele id.
    const id = Math.floor(Math.random() * 2 ** 31);
    batch.push({ id, keyPair: await generateX25519() });
  }
  return batch;
}

export interface EnsurePreKeysResult {
  uploaded: boolean;
  reason: "initial" | "replenished" | "sufficient";
  opkCount: number;
}

// Login sonrası (ve gelecekte AppShell bootstrap'ından da) çağrılacak TEK
// giriş noktası. Sunucudaki mevcut OPK sayısını okur: yeterliyse hiçbir şey
// üretmez/yüklemez; watermark altındaysa sadece eksik OPK'yı tamamlar (SPK'ya
// dokunmadan); bundle hiç yoksa (getOPKCount hata verir) tam bundle'ı
// (identity + SPK + ilk OPK batch'i) tek seferde yükler.
export async function ensurePreKeysUploaded(identity: IdentityKeys): Promise<EnsurePreKeysResult> {
  const {
    keyPair: signedPreKey,
    signature: signedPreKeySig,
    uploaded: spkUploaded,
  } = await getOrCreateSignedPreKey(identity);

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
    return { uploaded: false, reason: "sufficient", opkCount };
  }

  const freshOPKs = await generateOPKBatch(OPK_BATCH_SIZE);

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
    markSpkUploaded(toB64(signedPreKey.publicKeyBytes));
    return { uploaded: true, reason: "initial", opkCount: freshOPKs.length };
  }

  await api.replenishOPK({
    one_time_prekeys: freshOPKs.map((opk) => ({
      id: opk.id,
      public_key: toB64(opk.keyPair.publicKeyBytes),
    })),
  });
  return { uploaded: true, reason: "replenished", opkCount: opkCount + freshOPKs.length };
}
