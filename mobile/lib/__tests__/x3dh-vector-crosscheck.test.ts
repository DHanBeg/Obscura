import { readFileSync } from "fs";
import { join } from "path";
import { x25519 } from "@noble/curves/ed25519.js";
import { hkdf } from "@noble/hashes/hkdf";
import { sha256 } from "@noble/hashes/sha256";
import { hexToU8, u8ToHex } from "../crypto";

// Bu dosya identity.ts/prekeys.ts'in ÜZERİNE kurulu olduğu ham primitifleri
// (X25519 DH + HKDF-SHA256) Adım 2'de Rust CLI'nin ürettiği
// crypto/test-vectors/x3dh_ratchet_vectors.json'daki x3dh_vector'a karşı
// doğrular. Henüz bir mobile/lib/x3dh.ts YOK (o sonraki adım) — burada
// amaç identity/SPK anahtarlarının kullanacağı DH+HKDF'nin Rust
// (x25519-dalek + hkdf crate) ile BİREBİR aynı çıktıyı verdiğini,
// gerçek Ed25519 imza sınırına girmeden önce kanıtlamak.
const VECTORS_PATH = join(__dirname, "..", "..", "..", "crypto", "test-vectors", "x3dh_ratchet_vectors.json");
const vectors = JSON.parse(readFileSync(VECTORS_PATH, "utf8"));
const v = vectors.x3dh_vector;

describe("X3DH primitifi (X25519 DH + HKDF-SHA256) — Rust vektörüyle çapraz doğrulama", () => {
  test("Bob'un identity/SPK/OPK public key'leri private key'lerden aynı şekilde türetiliyor", () => {
    expect(u8ToHex(x25519.getPublicKey(hexToU8(v.input.bob_identity_priv)))).toBe(v.input.bob_identity_pub);
    expect(u8ToHex(x25519.getPublicKey(hexToU8(v.input.bob_spk_priv)))).toBe(v.input.bob_spk_pub);
    expect(u8ToHex(x25519.getPublicKey(hexToU8(v.input.bob_opk_priv)))).toBe(v.input.bob_opk_pub);
  });

  test("Alice'in efemeral public key'i vector'daki ephemeral_public_hex ile eşleşiyor", () => {
    const pub = x25519.getPublicKey(hexToU8(v.input.alice_ephemeral_priv));
    expect(u8ToHex(pub)).toBe(v.output.ephemeral_public_hex);
  });

  test("tam X3DH shared_key türetimi (DH1..DH4 + HKDF info=obscura-x3dh-v1) Rust ile birebir aynı", () => {
    const ikA = hexToU8(v.input.alice_identity_priv);
    const ekA = hexToU8(v.input.alice_ephemeral_priv);
    const ikBpub = hexToU8(v.input.bob_identity_pub);
    const spkBpub = hexToU8(v.input.bob_spk_pub);
    const opkBpub = hexToU8(v.input.bob_opk_pub);

    // Signal X3DH spec sırası: DH1=IK_A×SPK_B, DH2=EK_A×IK_B, DH3=EK_A×SPK_B, DH4=EK_A×OPK_B
    const dh1 = x25519.getSharedSecret(ikA, spkBpub);
    const dh2 = x25519.getSharedSecret(ekA, ikBpub);
    const dh3 = x25519.getSharedSecret(ekA, spkBpub);
    const dh4 = x25519.getSharedSecret(ekA, opkBpub);

    // IKM = 0xFF×32 || DH1 || DH2 || DH3 || DH4 (crypto/src/x3dh.rs derive_shared_key)
    const F = new Uint8Array(32).fill(0xff);
    const ikm = new Uint8Array(F.length + dh1.length + dh2.length + dh3.length + dh4.length);
    let off = 0;
    for (const part of [F, dh1, dh2, dh3, dh4]) {
      ikm.set(part, off);
      off += part.length;
    }

    const okm = hkdf(sha256, ikm, undefined, new TextEncoder().encode("obscura-x3dh-v1"), 32);
    expect(u8ToHex(okm)).toBe(v.output.shared_key_hex);
  });
});
