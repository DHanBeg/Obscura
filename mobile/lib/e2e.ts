import {
  generateX25519KeyPair,
  x25519PublicKey,
  decryptFromEnvelope,
  parseEnvelope,
  u8ToBase64,
  base64ToU8,
  hexToU8,
  u8ToHex,
} from "./crypto";
import { verifyWithKey } from "./identity";
import { x3dhInitiate, x3dhAccept, PreKeyBundle } from "./x3dh";
import { RatchetState, encryptedMessageToBytes, encryptedMessageFromBytes } from "./ratchet";
import { saveSession, loadSession, SessionStores } from "./session-store";
import { findSignedPreKeyPrivateByPublicKey } from "./prekeys";
import { getOneTimePreKeyPrivate, deleteOneTimePreKeyPrivate } from "./opk";
import { secureStore, asyncStore, KeyValueStore } from "./keyValueStore";
import { getCachedPlaintext, cachePlaintext } from "./plaintext-cache";
import { api } from "./api";

// E2E mesajlaşmanın public API'si — protokol versiyon dispatch katmanı.
//
// v1 (ESKİ, sadece ÇÖZME): tek anahtarlı ECIES (crypto.ts EncryptedEnvelope
//   {v:1, eph, n, ct}). Geriye uyumluluk: eski mesajlar AYNEN çözülmeye devam
//   eder (parse edilemeyen içerik plaintext fallback sayılır — korunuyor).
//   v1 zarf ÜRETİMİ artık yok.
//
// v2 (YENİ): X3DH oturum kurulumu + Double Ratchet.
//   Zarf şeması: { v: 2, x3dh?: { ik, ek, spk, opkId? }, msg: "<base64>" }
//   - msg: ratchet.encryptedMessageToBytes() çıktısı (40B header + nonce+ct+tag)
//   - x3dh: oturum kurulum (bootstrap) bilgisi. Gönderen, karşı taraftan İLK
//     ratchet cevabını ALANA KADAR bunu her mesajda taşır (Signal'ın
//     PreKey-mesaj davranışı: ilk mesaj kaybolsa/geç kalsa bile sonraki her
//     bootstrap'lı mesaj oturumu kurabilir, kaçanlar skipped-key ile çözülür).
//   - x3dh.spk: kullanılan alıcı SPK'sının PUBLIC key'i (base64). Backend
//     GET /v1/keys/{did} yanıtı signed_prekey_id DÖNDÜRMEDİĞİ için id yerine
//     public key taşınır; alıcı kendi SPK kayıt defterinden public key ile
//     private'ı bulur (prekeys.findSignedPreKeyPrivateByPublicKey).
//   - DENİABİLİTY: zarfın hiçbir alanında imza YOK. SPK imza doğrulaması
//     SADECE oturum kurulmadan önce, bundle üzerinde ayrı bir adım olarak
//     yapılır (verifySignedPreKeyOrThrow) — bkz. x3dh.ts başındaki not.
//
// AD (AEAD additional data): iki tarafın X25519 identity public key'lerinin
// hex sıralamasına göre (küçük || büyük) birleşimi — rol bağımsız olduğu için
// iki taraf da aynı 64 baytı türetir. Oturumu kimliklere bağlar (identity
// misbinding koruması); içerik iki PUBLIC key olduğundan gizli veri değildir,
// peer IK düz (ama cihaz-yerel) storage'da saklanabilir.
//
// Eşzamanlılık: chat ekranı gelen mesajları Promise.all ile paralel çözüyor.
// Ratchet oturumu stateful (load → mutate → save) olduğundan aynı peer'a ait
// tüm encrypt/decrypt işlemleri per-DID kuyrukta SIRAYLA çalıştırılır.

const KEY_PRIV = "obscura_x25519_private";
const KEY_PUB = "obscura_x25519_public";
const PEER_IK_PREFIX = "obscura_e2e_peer_ik_";
const PENDING_X3DH_PREFIX = "obscura_e2e_x3dh_pending_";
const UNDECRYPTABLE_FALLBACK = "🔒 Şifresi çözülemeyen mesaj";

export type E2EStores = SessionStores;

// v2 zarfın oturum kurulum alanı — sadece public değerler taşır.
export interface X3DHBootstrapInfo {
  ik: string; // gönderenin X25519 identity public key'i (base64)
  ek: string; // gönderenin X3DH ephemeral public key'i (base64)
  spk: string; // kullanılan alıcı SPK public key'i (base64) — id yerine, bkz. üst not
  opkId?: number; // tüketilen OPK id'si (varsa)
}

export interface EnvelopeV2 {
  v: 2;
  x3dh?: X3DHBootstrapInfo;
  msg: string; // base64(encryptedMessageToBytes)
}

// ─── Identity keypair (X25519 — X3DH DH kimlik anahtarı) ────────────────────

export async function getOrCreateKeyPair(
  store: KeyValueStore = secureStore
): Promise<{ privateKey: Uint8Array; publicKey: Uint8Array }> {
  const privHex = await store.getItem(KEY_PRIV);
  if (privHex) {
    const privateKey = hexToU8(privHex);
    return { privateKey, publicKey: x25519PublicKey(privateKey) };
  }
  const { privateKey, publicKey } = generateX25519KeyPair();
  await store.setItem(KEY_PRIV, u8ToHex(privateKey));
  await store.setItem(KEY_PUB, u8ToHex(publicKey));
  return { privateKey, publicKey };
}

export async function getIdentityPublicKeyBase64(store: KeyValueStore = secureStore): Promise<string> {
  const { publicKey } = await getOrCreateKeyPair(store);
  return u8ToBase64(publicKey);
}

// ─── Per-peer işlem kuyruğu ─────────────────────────────────────────────────

const peerLocks = new Map<string, Promise<void>>();

async function withPeerLock<T>(peerDid: string, fn: () => Promise<T>): Promise<T> {
  const prev = peerLocks.get(peerDid) ?? Promise.resolve();
  const run = prev.then(fn); // prev asla reject olmaz (aşağıda yutuluyor)
  peerLocks.set(
    peerDid,
    run.then(
      () => undefined,
      () => undefined
    )
  );
  return run;
}

// ─── AD ve peer kimlik bağlamı ──────────────────────────────────────────────

function peerIkStoreKey(did: string): string {
  return `${PEER_IK_PREFIX}${did}`;
}

function pendingStoreKey(did: string): string {
  return `${PENDING_X3DH_PREFIX}${did}`;
}

// İki identity public key'in rol bağımsız, deterministik birleşimi.
function computeAd(myIdentityPub: Uint8Array, peerIdentityPub: Uint8Array): Uint8Array {
  const mine = u8ToHex(myIdentityPub);
  const theirs = u8ToHex(peerIdentityPub);
  const [first, second] =
    mine <= theirs ? [myIdentityPub, peerIdentityPub] : [peerIdentityPub, myIdentityPub];
  const out = new Uint8Array(first.length + second.length);
  out.set(first, 0);
  out.set(second, first.length);
  return out;
}

// Kurulu oturumun AD'sini, oturum kurulurken saklanan peer IK'dan türetir.
async function loadPeerAd(peerDid: string, stores: E2EStores): Promise<Uint8Array | null> {
  const asyStore = stores.async ?? asyncStore;
  const peerIkB64 = await asyStore.getItem(peerIkStoreKey(peerDid));
  if (!peerIkB64) return null;
  const { publicKey } = await getOrCreateKeyPair(stores.secure);
  return computeAd(publicKey, base64ToU8(peerIkB64));
}

// ─── Bundle doğrulama (MITM koruması) ───────────────────────────────────────

interface RemotePreKeyBundle {
  identity_key?: string;
  signing_key?: string;
  signed_prekey?: string;
  signed_prekey_sig?: string;
  one_time_prekey?: string | null;
  one_time_prekey_id?: number | null;
}

// SPK imzası signing_key ile doğrulanamazsa OTURUM KURULMAZ (sert gereksinim).
// Bu doğrulama mesaj gönderiminden ÖNCE, ayrı bir adımdır — imza baytları ne
// zarfa ne de anahtar türetimine girer (deniability, bkz. x3dh.ts).
function verifySignedPreKeyOrThrow(bundle: RemotePreKeyBundle): void {
  if (
    !bundle?.identity_key ||
    !bundle.signing_key ||
    !bundle.signed_prekey ||
    !bundle.signed_prekey_sig
  ) {
    throw new Error("Güvenli anahtar doğrulanamadı — anahtar paketi eksik alan içeriyor");
  }
  let valid = false;
  try {
    valid = verifyWithKey(
      base64ToU8(bundle.signing_key),
      base64ToU8(bundle.signed_prekey),
      base64ToU8(bundle.signed_prekey_sig)
    );
  } catch {
    valid = false; // bozuk base64 / geçersiz Ed25519 noktası = geçersiz imza
  }
  if (!valid) {
    throw new Error(
      "Güvenli anahtar doğrulanamadı — SPK imzası geçersiz, oturum kurulmadı (olası MITM)"
    );
  }
}

// ─── Gönderme ───────────────────────────────────────────────────────────────

// Kurulu oturum varsa ratchet ile şifreler; yoksa peer'ın PreKey bundle'ını
// çekip imzasını doğrular, X3DH + initSender ile oturum kurar. Dönen değer
// JSON string (v2 zarf) — api.sendMessage'ın ciphertext alanına aynen gider.
export async function encryptMessage(
  plaintext: string,
  peerDid: string,
  stores: E2EStores = {}
): Promise<string> {
  return withPeerLock(peerDid, async () => {
    const asyStore = stores.async ?? asyncStore;

    let session = await loadSession(peerDid, stores);
    let bootstrap: X3DHBootstrapInfo | undefined;
    let ad: Uint8Array;

    if (session) {
      const loadedAd = await loadPeerAd(peerDid, stores);
      if (!loadedAd) {
        throw new Error("Güvenli oturum bağlamı eksik — sohbeti kapatıp yeniden açmayı deneyin");
      }
      ad = loadedAd;
      // Karşı taraftan henüz ratchet cevabı almadıysak bootstrap bilgisini
      // taşımaya devam et (ilk mesaj kaybolsa bile oturum kurulabilsin).
      const pendingRaw = await asyStore.getItem(pendingStoreKey(peerDid));
      if (pendingRaw) bootstrap = JSON.parse(pendingRaw) as X3DHBootstrapInfo;
    } else {
      const remote = (await api.getPreKeyBundle(peerDid)) as RemotePreKeyBundle;
      verifySignedPreKeyOrThrow(remote);

      const bundle: PreKeyBundle = {
        identityKey: base64ToU8(remote.identity_key!),
        signedPrekey: base64ToU8(remote.signed_prekey!),
        oneTimePrekey: remote.one_time_prekey ? base64ToU8(remote.one_time_prekey) : undefined,
        oneTimePrekeyId: remote.one_time_prekey_id ?? undefined,
      };
      const { privateKey: myPriv, publicKey: myPub } = await getOrCreateKeyPair(stores.secure);
      const init = x3dhInitiate(myPriv, bundle);
      // Bob'un henüz kendi ratchet mesajı yok — ilk "ratchet pub"ı SPK'sı
      // (ratchet.ts initSender/initReceiver sözleşmesi; Bob ilk decrypt'te
      // header'daki taze dhPub'ı görüp otomatik dhRatchet yapar).
      session = RatchetState.initSender(init.sharedKey, bundle.signedPrekey);
      ad = computeAd(myPub, bundle.identityKey);
      bootstrap = {
        ik: u8ToBase64(myPub),
        ek: u8ToBase64(init.ephemeralPublic),
        spk: remote.signed_prekey!,
        ...(init.oneTimePrekeyId !== undefined ? { opkId: init.oneTimePrekeyId } : {}),
      };
      await asyStore.setItem(peerIkStoreKey(peerDid), remote.identity_key!);
      await asyStore.setItem(pendingStoreKey(peerDid), JSON.stringify(bootstrap));
    }

    const encrypted = session.encrypt(new TextEncoder().encode(plaintext), ad);
    await saveSession(peerDid, session, stores);

    const envelope: EnvelopeV2 = {
      v: 2,
      ...(bootstrap ? { x3dh: bootstrap } : {}),
      msg: u8ToBase64(encryptedMessageToBytes(encrypted)),
    };
    return JSON.stringify(envelope);
  });
}

// ─── Alma ───────────────────────────────────────────────────────────────────

// v2 → ratchet/X3DH-bootstrap, v1 → eski ECIES (davranış birebir korunuyor),
// parse edilemeyen → plaintext fallback (legacy uyumluluk).
// `messageId` verilirse başarılı v2 çözümler şifreli önbelleğe yazılır ve
// sonraki çağrılar oradan döner — ratchet anahtarları tek kullanımlık olduğu
// için aynı v2 ciphertext ikinci kez ÇÖZÜLEMEZ (bkz. plaintext-cache.ts).
export async function decryptMessage(
  raw: string,
  senderDid: string,
  stores: E2EStores = {},
  messageId?: string
): Promise<string> {
  if (messageId) {
    const cached = await getCachedPlaintext(messageId, stores);
    if (cached !== null) return cached;
  }

  let parsed: unknown = null;
  try {
    parsed = JSON.parse(raw);
  } catch {
    parsed = null;
  }
  if (parsed && (parsed as { v?: unknown }).v === 2) {
    return decryptV2(parsed as EnvelopeV2, senderDid, stores, messageId);
  }

  // v1 (eski ECIES) — AYNEN eski decryptMessage mantığı, geriye uyumluluk.
  const envelope = parseEnvelope(raw);
  if (!envelope) return raw; // plaintext fallback for legacy messages
  try {
    const { privateKey } = await getOrCreateKeyPair(stores.secure);
    return decryptFromEnvelope(envelope, privateKey);
  } catch {
    return UNDECRYPTABLE_FALLBACK;
  }
}

async function decryptV2(
  env: EnvelopeV2,
  senderDid: string,
  stores: E2EStores,
  messageId?: string
): Promise<string> {
  return withPeerLock(senderDid, async () => {
    const secStore = stores.secure ?? secureStore;
    const asyStore = stores.async ?? asyncStore;
    try {
      const encrypted = encryptedMessageFromBytes(base64ToU8(env.msg));

      // 1) Kurulu oturum varsa SADECE ratchet yolu. (Oturum varken x3dh
      // alanına asla bootstrap yapılmaz — replay edilmiş eski bir bootstrap
      // zarfının ilerlemiş oturumu geri sarmasına izin vermemek için.)
      const session = await loadSession(senderDid, stores);
      if (session) {
        const ad = await loadPeerAd(senderDid, stores);
        if (!ad) return UNDECRYPTABLE_FALLBACK;
        const plaintextBytes = session.decrypt(encrypted, ad);
        // decrypt başarısızsa (throw) oturum diske YAZILMAZ — bellekte
        // yarım kalmış mutasyon atılır, sonraki deneme temiz state yükler.
        await saveSession(senderDid, session, stores);
        // Peer'dan ratchet mesajı aldık → bizim başlattığımız oturumun
        // bootstrap'ını artık taşımaya gerek yok.
        await asyStore.deleteItem(pendingStoreKey(senderDid));
        const plaintext = new TextDecoder().decode(plaintextBytes);
        if (messageId) await cachePlaintext(messageId, plaintext, stores);
        return plaintext;
      }

      // 2) Oturum yok + bootstrap var → X3DH accept (Bob rolü).
      if (!env.x3dh) return UNDECRYPTABLE_FALLBACK;

      const { privateKey: myPriv, publicKey: myPub } = await getOrCreateKeyPair(stores.secure);
      const spkPriv = await findSignedPreKeyPrivateByPublicKey(env.x3dh.spk, secStore);
      if (!spkPriv) return UNDECRYPTABLE_FALLBACK; // SPK retention süresi dolmuş/yabancı

      let opkPriv: Uint8Array | undefined;
      if (env.x3dh.opkId !== undefined && env.x3dh.opkId !== null) {
        const found = await getOneTimePreKeyPrivate(env.x3dh.opkId, secStore);
        if (!found) return UNDECRYPTABLE_FALLBACK; // OPK zaten tüketilmiş (replay?) → DH4 türetilemez
        opkPriv = found;
      }

      const senderIk = base64ToU8(env.x3dh.ik);
      const { sharedKey } = x3dhAccept(
        myPriv,
        spkPriv,
        senderIk,
        base64ToU8(env.x3dh.ek),
        opkPriv
      );
      const state = RatchetState.initReceiver(sharedKey, spkPriv);
      const ad = computeAd(myPub, senderIk);
      const plaintextBytes = state.decrypt(encrypted, ad);

      // Sıra önemli: önce oturum + peer IK kalıcılaşır, OPK imhası EN SON —
      // araya hata girerse mesaj kaybolmaz (OPK hâlâ durur, yeniden denenir).
      await asyStore.setItem(peerIkStoreKey(senderDid), env.x3dh.ik);
      await saveSession(senderDid, state, stores);
      if (env.x3dh.opkId !== undefined && env.x3dh.opkId !== null) {
        await deleteOneTimePreKeyPrivate(env.x3dh.opkId, secStore);
      }

      const plaintext = new TextDecoder().decode(plaintextBytes);
      if (messageId) await cachePlaintext(messageId, plaintext, stores);
      return plaintext;
    } catch {
      // Bozuk zarf, bütünlük hatası, AD uyuşmazlığı, base64 hatası… —
      // kullanıcıya v1'dekiyle aynı fallback gösterilir, hata yayılmaz.
      return UNDECRYPTABLE_FALLBACK;
    }
  });
}
