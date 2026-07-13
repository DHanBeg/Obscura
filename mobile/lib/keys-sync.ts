import { api } from "./api";
import { getIdentityPublicKeyBase64 } from "./e2e";
import { getSigningPublicKeyBase64 } from "./identity";
import { getSignedPreKeyBundle } from "./prekeys";
import { generateBatch } from "./opk";
import { secureStore, KeyValueStore } from "./keyValueStore";

// OPK havuzu bu eşiğin altına düşünce tamamlanır (backend'in "low" eşiğiyle
// aynı, bkz. backend/internal/api/keys.go HandleGetOPKCount).
const OPK_LOW_WATERMARK = 20;
const OPK_BATCH_SIZE = 30;

// Login sonrası çağrılır — gerçek Ed25519-imzalı bundle'ı (identity_key +
// signing_key + SPK + OPK batch) POST /v1/keys/upload'a yükler. Backend
// tarafı upsert olduğu için idempotent: her login'de çağırmak güvenli.
export async function ensureKeyBundleUploaded(store: KeyValueStore = secureStore): Promise<void> {
  const identityKey = await getIdentityPublicKeyBase64();
  const signingKey = await getSigningPublicKeyBase64(store);
  const { signedPrekey, signedPrekeyId, signedPrekeySig } = await getSignedPreKeyBundle(store);

  let opkBatch: { id: number; publicKey: string }[] = [];
  try {
    const { count } = await api.getOPKCount();
    if (count < OPK_LOW_WATERMARK) {
      opkBatch = await generateBatch(OPK_BATCH_SIZE, store);
    }
  } catch {
    // İlk yükleme (henüz bundle/OPK yok) — count endpoint'i 0 döner ya da
    // hata verirse yine de taze bir batch üretip yükle.
    opkBatch = await generateBatch(OPK_BATCH_SIZE, store);
  }

  await api.uploadPrekeys({
    identity_key: identityKey,
    signing_key: signingKey,
    signed_prekey: signedPrekey,
    signed_prekey_id: signedPrekeyId,
    signed_prekey_sig: signedPrekeySig,
    one_time_prekeys: opkBatch.map((e) => ({ id: e.id, public_key: e.publicKey })),
  });
}

// Periyodik bakım (Adım 8/9'da X3DH accept sonrası da çağrılabilir) — sadece
// düşükse yeni OPK üretip POST /v1/keys/opk/replenish'e gönderir.
export async function replenishOPKsIfLow(store: KeyValueStore = secureStore): Promise<void> {
  const { count } = await api.getOPKCount();
  if (count >= OPK_LOW_WATERMARK) return;
  const batch = await generateBatch(OPK_BATCH_SIZE, store);
  await api.replenishOPK({
    one_time_prekeys: batch.map((e) => ({ id: e.id, public_key: e.publicKey })),
  });
}
