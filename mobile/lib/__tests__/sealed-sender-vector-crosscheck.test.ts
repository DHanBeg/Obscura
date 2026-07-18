import { readFileSync } from "fs";
import { join } from "path";
import { x25519, ed25519 } from "@noble/curves/ed25519.js";
import { hexToU8, u8ToHex } from "../crypto";
import { deriveKey, certSigningBytes, issueCertificate, verifyCertificate, didFromDhPublic, SealedSenderIdentity } from "../sealed-sender";

// Adım 2'de bu dosya ham primitifleri (henüz mobile/lib/sealed-sender.ts YOKTU)
// Rust vektörüne karşı doğruluyordu. Adım 3'te modül yazıldı — artık ham
// primitifleri TEKRAR ETMİYOR, gerçek export edilen fonksiyonları (deriveKey,
// certSigningBytes, issueCertificate, verifyCertificate) çağırıp AYNI vektöre
// karşı sınıyor. Böylece bu dosya modülün kendisine bağlı bir regresyon
// testi haline geliyor — sealed-sender.test.ts'teki davranış testlerinden
// FARKLI olarak, burada amaç HALA "Rust'la bir bit bile kaymadan eşleşiyor mu".
const VECTORS_PATH = join(
  __dirname,
  "..",
  "..",
  "..",
  "crypto",
  "test-vectors",
  "sealed_sender_vectors.json",
);
const vectors = JSON.parse(readFileSync(VECTORS_PATH, "utf8"));
const v = vectors.sealed_sender_vector;

describe("sealed-sender.ts (gerçek modül) — Rust vektörüyle çapraz doğrulama", () => {
  test("efemeral X25519 public key, sabit ephemeral_priv'ten vector'daki ephemeral_public_hex'e türüyor", () => {
    const pub = x25519.getPublicKey(hexToU8(v.input.ephemeral_priv));
    expect(u8ToHex(pub)).toBe(v.output.ephemeral_public_hex);
  });

  test("issueCertificate(): gönderenin kimlik public key'i + DID'i vector'daki değerlerle eşleşiyor", () => {
    const sender: SealedSenderIdentity = {
      dhPriv: hexToU8(v.input.sender_dh_priv),
      dhPub: x25519.getPublicKey(hexToU8(v.input.sender_dh_priv)),
      signingPriv: hexToU8(v.input.sender_signing_seed),
      signingPub: ed25519.getPublicKey(hexToU8(v.input.sender_signing_seed)),
    };

    const cert = issueCertificate(sender, v.input.expires_at);

    expect(u8ToHex(cert.identityDhPub)).toBe(v.output.cert_identity_dh_pub_hex);
    expect(cert.did).toBe(v.output.sender_did);
    expect(cert.did).toBe(v.output.opened_sender_did);
    expect(didFromDhPublic(sender.dhPub)).toBe(v.output.sender_did);
  });

  test("issueCertificate(): gönderenin Ed25519 imzalama public key'i vector'daki cert_signing_pub_hex'e türüyor", () => {
    const sender: SealedSenderIdentity = {
      dhPriv: hexToU8(v.input.sender_dh_priv),
      dhPub: x25519.getPublicKey(hexToU8(v.input.sender_dh_priv)),
      signingPriv: hexToU8(v.input.sender_signing_seed),
      signingPub: ed25519.getPublicKey(hexToU8(v.input.sender_signing_seed)),
    };

    const cert = issueCertificate(sender, v.input.expires_at);

    expect(u8ToHex(cert.signingPub)).toBe(v.output.cert_signing_pub_hex);
  });

  test("deriveKey(): k_cert ve k_msg (HKDF-SHA256, salt=eph_pub‖recipient_pub) Rust ile birebir aynı", () => {
    const ephPriv = hexToU8(v.input.ephemeral_priv);
    const ephPub = x25519.getPublicKey(ephPriv);
    const recipientPub = x25519.getPublicKey(hexToU8(v.input.recipient_dh_priv));
    const senderDhPriv = hexToU8(v.input.sender_dh_priv);

    const dhEph = x25519.getSharedSecret(ephPriv, recipientPub);
    const kCert = deriveKey(dhEph, ephPub, recipientPub, new TextEncoder().encode("obscura-sealed-sender-v1:cert"));
    expect(u8ToHex(kCert)).toBe(v.output.k_cert_hex);

    const dhStatic = x25519.getSharedSecret(senderDhPriv, recipientPub);
    const ikm = new Uint8Array(dhEph.length + dhStatic.length);
    ikm.set(dhEph, 0);
    ikm.set(dhStatic, dhEph.length);
    const kMsg = deriveKey(ikm, ephPub, recipientPub, new TextEncoder().encode("obscura-sealed-sender-v1:msg"));
    expect(u8ToHex(kMsg)).toBe(v.output.k_msg_hex);
  });

  test("certSigningBytes(): sertifika şablonu (prefix‖did‖identity_dh_pub‖signing_pub‖expires_at_be) Rust ile birebir aynı", () => {
    const signingBytes = certSigningBytes(
      v.output.sender_did,
      hexToU8(v.output.cert_identity_dh_pub_hex),
      hexToU8(v.output.cert_signing_pub_hex),
      v.output.cert_expires_at,
    );
    expect(u8ToHex(signingBytes)).toBe(v.output.cert_signing_bytes_hex);
  });

  test("issueCertificate(): Ed25519 imzası Rust'la BİREBİR aynı bytes'ı üretiyor (EdDSA deterministik)", () => {
    const sender: SealedSenderIdentity = {
      dhPriv: hexToU8(v.input.sender_dh_priv),
      dhPub: x25519.getPublicKey(hexToU8(v.input.sender_dh_priv)),
      signingPriv: hexToU8(v.input.sender_signing_seed),
      signingPub: ed25519.getPublicKey(hexToU8(v.input.sender_signing_seed)),
    };

    const cert = issueCertificate(sender, v.input.expires_at);

    expect(u8ToHex(cert.signature)).toBe(v.output.cert_signature_hex);
  });

  test("verifyCertificate(): Rust'ın ürettiği sertifika (vector'dan yeniden inşa edilmiş) geçerli kabul ediliyor", () => {
    const cert = {
      did: v.output.sender_did,
      identityDhPub: hexToU8(v.output.cert_identity_dh_pub_hex),
      signingPub: hexToU8(v.output.cert_signing_pub_hex),
      expiresAt: v.output.cert_expires_at,
      signature: hexToU8(v.output.cert_signature_hex),
    };

    expect(() => verifyCertificate(cert, v.input.now)).not.toThrow();
  });
});
