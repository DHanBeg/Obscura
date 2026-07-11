import { readFileSync } from "fs";
import { join } from "path";
import { x25519 } from "@noble/curves/ed25519.js";
import { x3dhInitiate, x3dhAccept, PreKeyBundle } from "../x3dh";
import { hexToU8, u8ToHex } from "../crypto";

const VECTORS_PATH = join(__dirname, "..", "..", "..", "crypto", "test-vectors", "x3dh_ratchet_vectors.json");
const vectors = JSON.parse(readFileSync(VECTORS_PATH, "utf8"));
const v = vectors.x3dh_vector;

describe("x3dh.ts — Rust vektörüyle birebir eşleşme (Adım 2 x3dh_ratchet_vectors.json)", () => {
  test("Alice (x3dhInitiate) sabit efemeral ile vector'daki shared_key_hex + ephemeral_public_hex'i üretiyor", () => {
    const bundle: PreKeyBundle = {
      identityKey: hexToU8(v.input.bob_identity_pub),
      signedPrekey: hexToU8(v.input.bob_spk_pub),
      oneTimePrekey: hexToU8(v.input.bob_opk_pub),
      oneTimePrekeyId: 7,
    };

    const result = x3dhInitiate(
      hexToU8(v.input.alice_identity_priv),
      bundle,
      hexToU8(v.input.alice_ephemeral_priv) // deterministik — SADECE test için
    );

    expect(u8ToHex(result.sharedKey)).toBe(v.output.shared_key_hex);
    expect(u8ToHex(result.ephemeralPublic)).toBe(v.output.ephemeral_public_hex);
    expect(result.oneTimePrekeyId).toBe(7);
  });

  test("Bob (x3dhAccept) AYNI vector girdileriyle AYNI shared_key_hex'e ulaşıyor", () => {
    const aliceIdentityPub = x25519.getPublicKey(hexToU8(v.input.alice_identity_priv));
    const aliceEphemeralPub = hexToU8(v.output.ephemeral_public_hex);

    const result = x3dhAccept(
      hexToU8(v.input.bob_identity_priv),
      hexToU8(v.input.bob_spk_priv),
      aliceIdentityPub,
      aliceEphemeralPub,
      hexToU8(v.input.bob_opk_priv)
    );

    expect(u8ToHex(result.sharedKey)).toBe(v.output.shared_key_hex);
  });

  test("Alice ve Bob'un vector üzerinden ulaştığı shared_key BİREBİR aynı (iki yön de test edildi)", () => {
    const bundle: PreKeyBundle = {
      identityKey: hexToU8(v.input.bob_identity_pub),
      signedPrekey: hexToU8(v.input.bob_spk_pub),
      oneTimePrekey: hexToU8(v.input.bob_opk_pub),
      oneTimePrekeyId: 0,
    };
    const aliceResult = x3dhInitiate(
      hexToU8(v.input.alice_identity_priv),
      bundle,
      hexToU8(v.input.alice_ephemeral_priv)
    );

    const aliceIdentityPub = x25519.getPublicKey(hexToU8(v.input.alice_identity_priv));
    const bobResult = x3dhAccept(
      hexToU8(v.input.bob_identity_priv),
      hexToU8(v.input.bob_spk_priv),
      aliceIdentityPub,
      aliceResult.ephemeralPublic,
      hexToU8(v.input.bob_opk_priv)
    );

    expect(u8ToHex(bobResult.sharedKey)).toBe(u8ToHex(aliceResult.sharedKey));
  });
});

describe("x3dh.ts — gerçek (rastgele) anahtarlarla çift yönlü uyuşma", () => {
  function randomIdentity() {
    const { secretKey, publicKey } = x25519.keygen();
    return { priv: secretKey, pub: publicKey };
  }

  test("OPK VARKEN Alice ve Bob aynı shared_key'e ulaşıyor", () => {
    const alice = randomIdentity();
    const bob = randomIdentity();
    const bobSpk = randomIdentity();
    const bobOpk = randomIdentity();

    const bundle: PreKeyBundle = {
      identityKey: bob.pub,
      signedPrekey: bobSpk.pub,
      oneTimePrekey: bobOpk.pub,
      oneTimePrekeyId: 3,
    };

    const aliceResult = x3dhInitiate(alice.priv, bundle);
    const bobResult = x3dhAccept(bob.priv, bobSpk.priv, alice.pub, aliceResult.ephemeralPublic, bobOpk.priv);

    expect(u8ToHex(bobResult.sharedKey)).toBe(u8ToHex(aliceResult.sharedKey));
    expect(aliceResult.sharedKey.length).toBe(32);
  });

  test("OPK YOKKEN (tükenmiş havuz) Alice ve Bob YİNE aynı shared_key'e ulaşıyor (3-DH moduna düşüyor)", () => {
    const alice = randomIdentity();
    const bob = randomIdentity();
    const bobSpk = randomIdentity();

    const bundle: PreKeyBundle = {
      identityKey: bob.pub,
      signedPrekey: bobSpk.pub,
    };

    const aliceResult = x3dhInitiate(alice.priv, bundle);
    const bobResult = x3dhAccept(bob.priv, bobSpk.priv, alice.pub, aliceResult.ephemeralPublic);

    expect(u8ToHex(bobResult.sharedKey)).toBe(u8ToHex(aliceResult.sharedKey));
    expect(aliceResult.oneTimePrekeyId).toBeUndefined();
  });

  test("efemeral verilmezse her çağrıda farklı (rastgele) üretiliyor", () => {
    const alice = randomIdentity();
    const bob = randomIdentity();
    const bobSpk = randomIdentity();
    const bundle: PreKeyBundle = { identityKey: bob.pub, signedPrekey: bobSpk.pub };

    const a = x3dhInitiate(alice.priv, bundle);
    const b = x3dhInitiate(alice.priv, bundle);
    expect(u8ToHex(a.ephemeralPublic)).not.toBe(u8ToHex(b.ephemeralPublic));
    expect(u8ToHex(a.sharedKey)).not.toBe(u8ToHex(b.sharedKey));
  });

  test("farklı Bob identity/SPK ile Alice YANLIŞ shared_key üretir (yanlış-pozitif koruması)", () => {
    const alice = randomIdentity();
    const realBob = randomIdentity();
    const realBobSpk = randomIdentity();
    const impostorBob = randomIdentity();
    const impostorSpk = randomIdentity();

    const realBundle: PreKeyBundle = { identityKey: realBob.pub, signedPrekey: realBobSpk.pub };
    const impostorBundle: PreKeyBundle = { identityKey: impostorBob.pub, signedPrekey: impostorSpk.pub };

    const aliceToReal = x3dhInitiate(alice.priv, realBundle);
    const aliceToImpostor = x3dhInitiate(alice.priv, impostorBundle);

    expect(u8ToHex(aliceToReal.sharedKey)).not.toBe(u8ToHex(aliceToImpostor.sharedKey));
  });
});

describe("x3dh.ts — deniability: imza shared_key türetimine GİRMİYOR", () => {
  test("bundle nesnesine imzayla ilgili fazladan alan eklenmesi shared_key'i DEĞİŞTİRMİYOR", () => {
    const bundleWithoutSig: PreKeyBundle = {
      identityKey: hexToU8(v.input.bob_identity_pub),
      signedPrekey: hexToU8(v.input.bob_spk_pub),
    };
    // Gerçek network response'unda (backend PreKeyBundleResponse) bulunacak
    // signed_prekey_sig alanı simüle ediliyor — x3dhInitiate bunu OKUMUYOR,
    // PreKeyBundle tipinde bu alan yok, olsa bile DH matematiğine karışmaz.
    const bundleWithExtraSigField = {
      ...bundleWithoutSig,
      signedPrekeySig: hexToU8("00".repeat(64)),
    } as PreKeyBundle & { signedPrekeySig: Uint8Array };

    const a = x3dhInitiate(hexToU8(v.input.alice_identity_priv), bundleWithoutSig, hexToU8(v.input.alice_ephemeral_priv));
    const b = x3dhInitiate(
      hexToU8(v.input.alice_identity_priv),
      bundleWithExtraSigField,
      hexToU8(v.input.alice_ephemeral_priv)
    );

    expect(u8ToHex(a.sharedKey)).toBe(u8ToHex(b.sharedKey));
  });

  test("PreKeyBundle tipinde imza alanı (signedPrekeySig) yok — sadece DH'ye giren ham anahtarlar var", () => {
    const bundle: PreKeyBundle = {
      identityKey: hexToU8(v.input.bob_identity_pub),
      signedPrekey: hexToU8(v.input.bob_spk_pub),
    };
    expect(Object.keys(bundle).sort()).toEqual(["identityKey", "signedPrekey"]);
  });
});
