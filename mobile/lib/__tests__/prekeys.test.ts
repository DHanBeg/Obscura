import { readFileSync } from "fs";
import { join } from "path";
import { x25519 } from "@noble/curves/ed25519.js";
import {
  generateSignedPreKeyPair,
  rotateIfNeeded,
  getSignedPreKeyBundle,
  getSignedPreKeyPrivate,
} from "../prekeys";
import { getOrCreateSigningKeyPair, verifyWithKey } from "../identity";
import { hexToU8, u8ToHex, base64ToU8 } from "../crypto";
import { createMemoryStore } from "../../test-utils/memoryStore";

const VECTORS_PATH = join(__dirname, "..", "..", "..", "crypto", "test-vectors", "x3dh_ratchet_vectors.json");
const vectors = JSON.parse(readFileSync(VECTORS_PATH, "utf8"));

describe("prekeys.ts — X25519 primitifi Rust x25519-dalek ile birebir eşleşiyor", () => {
  // SPK de identity key gibi düz bir X25519 keypair — aynı primitifi kullanır.
  // Rust CLI'nin ürettiği x3dh_vector içindeki Bob'un SPK'sı burada
  // cross-check ediliyor: aynı private key'den noble ve dalek AYNI public
  // key'i türetiyor mu (clamping/curve implementasyonu uyuşmazlığı olursa
  // burada yakalanır).
  test("bob_spk_priv → bob_spk_pub (vector)", () => {
    const priv = hexToU8(vectors.x3dh_vector.input.bob_spk_priv);
    const pub = x25519.getPublicKey(priv);
    expect(u8ToHex(pub)).toBe(vectors.x3dh_vector.input.bob_spk_pub);
  });

  test("generateSignedPreKeyPair 32/32 byte X25519 keypair üretiyor", () => {
    const { privateKey, publicKey } = generateSignedPreKeyPair();
    expect(privateKey.length).toBe(32);
    expect(publicKey.length).toBe(32);
    // self-consistency: aynı private key'den getPublicKey aynı sonucu vermeli
    expect(u8ToHex(x25519.getPublicKey(privateKey))).toBe(u8ToHex(publicKey));
  });
});

describe("prekeys.ts — SPK üretimi ve id şeması", () => {
  test("ilk çağrıda id=0 ile SPK üretir", async () => {
    const store = createMemoryStore();
    const active = await rotateIfNeeded(store, Date.now());
    expect(active.id).toBe(0);
    expect(active.retiredAt).toBeNull();
  });

  test("obscura_spk_private_{id} anahtarıyla private key SecureStore'da (enjekte edilen store'da) saklanıyor", async () => {
    const store = createMemoryStore();
    const active = await rotateIfNeeded(store, Date.now());
    const raw = await store.getItem(`obscura_spk_private_${active.id}`);
    expect(raw).not.toBeNull();
    expect(hexToU8(raw!).length).toBe(32);
  });

  test("7 günden eski değilse rotasyon yapmaz, AYNI id/anahtarı döner", async () => {
    const store = createMemoryStore();
    const t0 = Date.now();
    const first = await rotateIfNeeded(store, t0);
    const sixDaysLater = t0 + 6 * 24 * 60 * 60 * 1000;
    const second = await rotateIfNeeded(store, sixDaysLater);
    expect(second.id).toBe(first.id);
    expect(second.publicKey).toBe(first.publicKey);
  });

  test("7 günden eskiyse yeni SPK üretir, eskisini retired işaretler", async () => {
    const store = createMemoryStore();
    const t0 = Date.now();
    const first = await rotateIfNeeded(store, t0);
    const eightDaysLater = t0 + 8 * 24 * 60 * 60 * 1000;
    const second = await rotateIfNeeded(store, eightDaysLater);

    expect(second.id).toBe(first.id + 1);
    expect(second.publicKey).not.toBe(first.publicKey);

    // eski private key HÂLÂ okunabilir olmalı (30 gün saklama penceresi içinde)
    const oldPriv = await getSignedPreKeyPrivate(first.id, store);
    expect(oldPriv).not.toBeNull();
  });

  test("retired SPK 30 günden eskiyse private key silinir (pruning)", async () => {
    const store = createMemoryStore();
    const t0 = Date.now();
    const first = await rotateIfNeeded(store, t0);
    const eightDaysLater = t0 + 8 * 24 * 60 * 60 * 1000;
    await rotateIfNeeded(store, eightDaysLater); // first'i retire eder (id 0)

    // retire olalı 31 gün geçmiş — sonraki herhangi bir rotateIfNeeded çağrısı prune etmeli
    const wellPastRetention = eightDaysLater + 31 * 24 * 60 * 60 * 1000;
    await rotateIfNeeded(store, wellPastRetention);

    const oldPriv = await getSignedPreKeyPrivate(first.id, store);
    expect(oldPriv).toBeNull();
  });

  test("retired SPK 30 günden yeni ise private key HÂLÂ duruyor", async () => {
    const store = createMemoryStore();
    const t0 = Date.now();
    const first = await rotateIfNeeded(store, t0);
    const eightDaysLater = t0 + 8 * 24 * 60 * 60 * 1000;
    await rotateIfNeeded(store, eightDaysLater);

    const twentyNineDaysAfterRetire = eightDaysLater + 29 * 24 * 60 * 60 * 1000;
    await rotateIfNeeded(store, twentyNineDaysAfterRetire);

    const oldPriv = await getSignedPreKeyPrivate(first.id, store);
    expect(oldPriv).not.toBeNull();
  });
});

describe("prekeys.ts — imza zinciri (identity.ts Ed25519 signing key ile)", () => {
  test("getSignedPreKeyBundle'ın signedPrekeySig'i, AYNI store'daki signing public key ile doğrulanıyor", async () => {
    const store = createMemoryStore();
    const bundle = await getSignedPreKeyBundle(store, Date.now());
    const { publicKey: signingPub } = await getOrCreateSigningKeyPair(store);

    const spkPublicKeyBytes = base64ToU8(bundle.signedPrekey);
    const sigBytes = base64ToU8(bundle.signedPrekeySig);

    expect(verifyWithKey(signingPub, spkPublicKeyBytes, sigBytes)).toBe(true);
  });

  test("bundle şekli backend PreKeyUploadRequest alan adlarıyla uyumlu (signedPrekey/signedPrekeyId/signedPrekeySig)", async () => {
    const store = createMemoryStore();
    const bundle = await getSignedPreKeyBundle(store, Date.now());
    expect(typeof bundle.signedPrekey).toBe("string");
    expect(typeof bundle.signedPrekeyId).toBe("number");
    expect(typeof bundle.signedPrekeySig).toBe("string");
    // backend keys.go: signed_prekey 32 byte, signed_prekey_sig 64 byte (Ed25519)
    expect(base64ToU8(bundle.signedPrekey).length).toBe(32);
    expect(base64ToU8(bundle.signedPrekeySig).length).toBe(64);
  });

  test("başka bir signing key ile imza DOĞRULANMAMALI (yanlış-pozitif koruması)", async () => {
    const store = createMemoryStore();
    const bundle = await getSignedPreKeyBundle(store, Date.now());
    const otherStore = createMemoryStore();
    const { publicKey: otherSigningPub } = await getOrCreateSigningKeyPair(otherStore);

    const spkPublicKeyBytes = base64ToU8(bundle.signedPrekey);
    const sigBytes = base64ToU8(bundle.signedPrekeySig);

    expect(verifyWithKey(otherSigningPub, spkPublicKeyBytes, sigBytes)).toBe(false);
  });
});
