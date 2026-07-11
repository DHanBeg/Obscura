import {
  generateSigningKeyPair,
  signingPublicKey,
  signWithKey,
  verifyWithKey,
  getOrCreateSigningKeyPair,
  getSigningPublicKeyBase64,
  signBytes,
} from "../identity";
import { hexToU8, u8ToHex, u8ToBase64 } from "../crypto";
import { createMemoryStore } from "../../test-utils/memoryStore";

// RFC 8032 §7.1 TEST 1 — resmi Ed25519 test vektörü (rfc-editor.org/rfc/rfc8032.txt).
// Bu proje-özel bir vektör değil, spec'in kendisi — noble'ın Ed25519
// implementasyonunun RFC'ye bire bir uyduğunu (imza formatı, doğrulama)
// üçüncü taraf bir referansla doğrular. Backend Go crypto/ed25519 de aynı
// RFC'yi implemente ettiği için, buradan üretilen imzalar backend'in
// ed25519.Verify'ıyla uyumlu olur (backend/internal/api/keys_ed25519_test.go
// zaten Go tarafında gerçek Ed25519 imza/doğrulamayı test ediyor).
const RFC8032_TEST1 = {
  secretKey: "9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60",
  publicKey: "d75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a",
  signature:
    "e5564300c360ac729086e2cc806e828a84877f1eb8e5d974d873e065224901555fb8821590a33bacc61e39701cf9b46bd25bf5f0595bbe24655141438e7a100b",
};

describe("identity.ts — Ed25519 primitifleri (RFC 8032 cross-check)", () => {
  test("getPublicKey RFC 8032 TEST 1 ile eşleşiyor", () => {
    const priv = hexToU8(RFC8032_TEST1.secretKey);
    const pub = signingPublicKey(priv);
    expect(u8ToHex(pub)).toBe(RFC8032_TEST1.publicKey);
  });

  test("sign RFC 8032 TEST 1 ile birebir aynı imzayı üretiyor (boş mesaj)", () => {
    const priv = hexToU8(RFC8032_TEST1.secretKey);
    const sig = signWithKey(priv, new Uint8Array(0));
    expect(u8ToHex(sig)).toBe(RFC8032_TEST1.signature);
  });

  test("RFC vektörünün imzası kendi verify'ıyla geçerli", () => {
    const pub = hexToU8(RFC8032_TEST1.publicKey);
    const sig = hexToU8(RFC8032_TEST1.signature);
    expect(verifyWithKey(pub, new Uint8Array(0), sig)).toBe(true);
  });

  test("tek byte değişen mesaj imzayı geçersiz kılıyor", () => {
    const priv = hexToU8(RFC8032_TEST1.secretKey);
    const pub = signingPublicKey(priv);
    const sig = signWithKey(priv, new TextEncoder().encode("obscura"));
    const tampered = new TextEncoder().encode("obscura!");
    expect(verifyWithKey(pub, tampered, sig)).toBe(false);
  });
});

describe("identity.ts — X25519 identity key'den ayrı olma garantisi", () => {
  test("üretilen signing keypair her seferinde farklı (deterministik değil)", () => {
    const a = generateSigningKeyPair();
    const b = generateSigningKeyPair();
    expect(u8ToHex(a.privateKey)).not.toBe(u8ToHex(b.privateKey));
  });

  test("32 byte private, 32 byte public key üretiyor", () => {
    const { privateKey, publicKey } = generateSigningKeyPair();
    expect(privateKey.length).toBe(32);
    expect(publicKey.length).toBe(32);
  });
});

describe("identity.ts — SecureStore persistence (in-memory store enjekte edilmiş)", () => {
  test("ilk çağrıda üretir, ikinci çağrıda AYNI keypair'i döner", async () => {
    const store = createMemoryStore();
    const first = await getOrCreateSigningKeyPair(store);
    const second = await getOrCreateSigningKeyPair(store);
    expect(u8ToHex(second.privateKey)).toBe(u8ToHex(first.privateKey));
    expect(u8ToHex(second.publicKey)).toBe(u8ToHex(first.publicKey));
  });

  test("iki ayrı store iki ayrı keypair üretir (izolasyon doğrulaması)", async () => {
    const storeA = createMemoryStore();
    const storeB = createMemoryStore();
    const a = await getOrCreateSigningKeyPair(storeA);
    const b = await getOrCreateSigningKeyPair(storeB);
    expect(u8ToHex(a.privateKey)).not.toBe(u8ToHex(b.privateKey));
  });

  test("getSigningPublicKeyBase64 tutarlı base64 döner", async () => {
    const store = createMemoryStore();
    const { publicKey } = await getOrCreateSigningKeyPair(store);
    const b64 = await getSigningPublicKeyBase64(store);
    expect(b64).toBe(u8ToBase64(publicKey));
  });

  test("signBytes depolanan private key ile imzalıyor, depolanan public key ile doğrulanabiliyor", async () => {
    const store = createMemoryStore();
    const { publicKey } = await getOrCreateSigningKeyPair(store);
    const message = new TextEncoder().encode("spk-public-key-bytes");
    const sig = await signBytes(message, store);
    expect(verifyWithKey(publicKey, message, sig)).toBe(true);
  });
});
