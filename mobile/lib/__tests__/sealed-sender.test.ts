import { readFileSync } from "fs";
import { join } from "path";
import { x25519, ed25519 } from "@noble/curves/ed25519.js";
import {
  seal,
  unseal,
  issueCertificate,
  verifyCertificate,
  didFromDhPublic,
  SealedSenderIdentity,
} from "../sealed-sender";
import { hexToU8, u8ToHex } from "../crypto";

const VECTORS_PATH = join(__dirname, "..", "..", "..", "crypto", "test-vectors", "sealed_sender_vectors.json");
const vectors = JSON.parse(readFileSync(VECTORS_PATH, "utf8"));
const v = vectors.sealed_sender_vector;

function randomIdentity(): SealedSenderIdentity {
  const dh = x25519.keygen();
  const signing = ed25519.keygen();
  return { dhPriv: dh.secretKey, dhPub: dh.publicKey, signingPriv: signing.secretKey, signingPub: signing.publicKey };
}

describe("sealed-sender.ts — Rust vektörüyle birebir eşleşme (Adım 1 sealed_sender_vectors.json)", () => {
  test("seal() sabit ephemeralPriv ile vector'daki ephemeral_public_hex'i üreten zarfı yazıyor", () => {
    const sender: SealedSenderIdentity = {
      dhPriv: hexToU8(v.input.sender_dh_priv),
      dhPub: x25519.getPublicKey(hexToU8(v.input.sender_dh_priv)),
      signingPriv: hexToU8(v.input.sender_signing_seed),
      signingPub: ed25519.getPublicKey(hexToU8(v.input.sender_signing_seed)),
    };
    const recipientDhPub = x25519.getPublicKey(hexToU8(v.input.recipient_dh_priv));

    const envelope = seal(
      sender,
      recipientDhPub,
      hexToU8(v.input.payload_hex),
      v.input.expires_at,
      hexToU8(v.input.ephemeral_priv)
    );

    expect(u8ToHex(envelope.slice(1, 33))).toBe(v.output.ephemeral_public_hex);
  });

  test("gerçek seal()+unseal() roundtrip'i vector'daki sender_did/opened_payload_hex ile eşleşiyor", () => {
    const sender: SealedSenderIdentity = {
      dhPriv: hexToU8(v.input.sender_dh_priv),
      dhPub: x25519.getPublicKey(hexToU8(v.input.sender_dh_priv)),
      signingPriv: hexToU8(v.input.sender_signing_seed),
      signingPub: ed25519.getPublicKey(hexToU8(v.input.sender_signing_seed)),
    };
    const recipient: SealedSenderIdentity = {
      dhPriv: hexToU8(v.input.recipient_dh_priv),
      dhPub: x25519.getPublicKey(hexToU8(v.input.recipient_dh_priv)),
      signingPriv: hexToU8(v.input.recipient_signing_seed),
      signingPub: ed25519.getPublicKey(hexToU8(v.input.recipient_signing_seed)),
    };

    const envelope = seal(
      sender,
      recipient.dhPub,
      hexToU8(v.input.payload_hex),
      v.input.expires_at,
      hexToU8(v.input.ephemeral_priv)
    );
    const opened = unseal(recipient, envelope, v.input.now);

    expect(opened.senderDid).toBe(v.output.sender_did);
    expect(opened.senderDid).toBe(v.output.opened_sender_did);
    expect(u8ToHex(opened.payload)).toBe(v.output.opened_payload_hex);
  });

  test("issueCertificate() vector'daki cert_signature_hex'i BİREBİR üretiyor (EdDSA deterministik)", () => {
    const sender: SealedSenderIdentity = {
      dhPriv: hexToU8(v.input.sender_dh_priv),
      dhPub: x25519.getPublicKey(hexToU8(v.input.sender_dh_priv)),
      signingPriv: hexToU8(v.input.sender_signing_seed),
      signingPub: ed25519.getPublicKey(hexToU8(v.input.sender_signing_seed)),
    };

    const cert = issueCertificate(sender, v.input.expires_at);

    expect(cert.did).toBe(v.output.sender_did);
    expect(u8ToHex(cert.identityDhPub)).toBe(v.output.cert_identity_dh_pub_hex);
    expect(u8ToHex(cert.signingPub)).toBe(v.output.cert_signing_pub_hex);
    expect(u8ToHex(cert.signature)).toBe(v.output.cert_signature_hex);
  });
});

describe("sealed-sender.ts — gerçek (rastgele) anahtarlarla roundtrip + güvenlik davranışı", () => {
  test("roundtrip: seal→unseal payload'ı birebir kurtarıyor, gönderen kimliği doğru", () => {
    const sender = randomIdentity();
    const recipient = randomIdentity();
    const payload = new TextEncoder().encode("obscura test mesajı 123");

    const envelope = seal(sender, recipient.dhPub, payload, 0);
    const opened = unseal(recipient, envelope, 1_800_000_000);

    expect(u8ToHex(opened.payload)).toBe(u8ToHex(payload));
    expect(opened.senderDid).toBe(didFromDhPublic(sender.dhPub));
    expect(u8ToHex(opened.senderIdentityDhPub)).toBe(u8ToHex(sender.dhPub));
    expect(u8ToHex(opened.senderSigningPub)).toBe(u8ToHex(sender.signingPub));
  });

  test("YANLIŞ alıcı anahtarıyla unseal REDDEDİLİYOR", () => {
    const sender = randomIdentity();
    const recipient = randomIdentity();
    const eve = randomIdentity();
    const payload = new TextEncoder().encode("gizli mesaj");

    const envelope = seal(sender, recipient.dhPub, payload, 0);

    expect(() => unseal(eve, envelope, 0)).toThrow();
  });

  test("zarf gönderen hakkında AÇIK hiçbir bilgi içermiyor (X25519 kimlik, Ed25519 kimlik, DID, payload)", () => {
    const sender = randomIdentity();
    const recipient = randomIdentity();
    const payload = new TextEncoder().encode("gizli mesaj");

    const envelope = seal(sender, recipient.dhPub, payload, 0);
    const envelopeHex = u8ToHex(envelope);

    expect(envelopeHex).not.toContain(u8ToHex(sender.dhPub));
    expect(envelopeHex).not.toContain(u8ToHex(sender.signingPub));
    expect(envelopeHex).not.toContain(u8ToHex(payload));
    expect(envelopeHex).not.toContain(Buffer.from(didFromDhPublic(sender.dhPub)).toString("hex"));
  });

  test("aynı gönderen+alıcıya iki zarf, farklı (bağlantısız) efemeral pub üretiyor", () => {
    const sender = randomIdentity();
    const recipient = randomIdentity();
    const payload = new TextEncoder().encode("a");

    const e1 = seal(sender, recipient.dhPub, payload, 0);
    const e2 = seal(sender, recipient.dhPub, payload, 0);

    expect(u8ToHex(e1.slice(1, 33))).not.toBe(u8ToHex(e2.slice(1, 33)));
  });

  test("bozulmuş zarf (sertifika bloğunda, mesaj bloğunda, versiyonda) REDDEDİLİYOR", () => {
    const sender = randomIdentity();
    const recipient = randomIdentity();
    const envelope = seal(sender, recipient.dhPub, new TextEncoder().encode("payload"), 0);

    const t1 = envelope.slice();
    t1[37 + 5] ^= 0xff; // ENVELOPE_HEADER_LEN(37) sonrası — sertifika bloğu
    expect(() => unseal(recipient, t1, 0)).toThrow();

    const t2 = envelope.slice();
    t2[t2.length - 1] ^= 0xff; // mesaj bloğunun son byte'ı
    expect(() => unseal(recipient, t2, 0)).toThrow();

    const t3 = envelope.slice();
    t3[0] = 99; // versiyon
    expect(() => unseal(recipient, t3, 0)).toThrow();
  });

  test("süresi dolmuş sertifika REDDEDİLİYOR (MITM/replay koruması)", () => {
    const sender = randomIdentity();
    const recipient = randomIdentity();
    const expiresAt = 1_000_000;
    const envelope = seal(sender, recipient.dhPub, new TextEncoder().encode("payload"), expiresAt);

    expect(() => unseal(recipient, envelope, expiresAt - 1)).not.toThrow();
    expect(() => unseal(recipient, envelope, expiresAt + 1)).toThrow();
  });

  test("verifyCertificate: DID'i kimlik anahtarıyla uyuşmayan sertifika REDDEDİLİYOR", () => {
    const sender = randomIdentity();
    const cert = issueCertificate(sender, 0);
    const forged = { ...cert, did: "did:obs:00000000000000000000000000000000" };

    expect(() => verifyCertificate(forged, 0)).toThrow();
  });
});
