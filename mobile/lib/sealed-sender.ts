import "react-native-get-random-values";
import { x25519, ed25519 } from "@noble/curves/ed25519.js";
import { gcm } from "@noble/ciphers/aes.js";
import { hkdf } from "@noble/hashes/hkdf.js";
import { sha256 } from "@noble/hashes/sha256.js";
import { u8ToHex } from "./crypto";

// Sealed Sender — gönderen kimliğini sunucudan gizleyen zarf formatı.
// 1:1 port of crypto/src/sealed_sender.rs (bkz. o dosyanın başlık yorumu için
// Signal "sealed sender" tasarım referansı: https://signal.org/blog/sealed-sender/).
//
// İki katmanlı yapı (Rust ile birebir):
//   1. Efemeral katman: efemeral X25519 × alıcı kimlik anahtarı → gönderen
//      sertifikasını (SenderCertificate) şifreler. ZARFIN İÇERİĞİ SADECE
//      efemeral public key'i ve alıcı adresini taşır — zarf başlı başına
//      gönderenin KİM olduğunu ifşa etmez.
//      DİKKAT (bkz. ADR-0016): bu, backend'e giden HTTP isteğinin kendisi
//      hakkında bir iddia DEĞİLDİR. Obscura'da gönderim JWT ile kimlik
//      doğrulanmış bir istek olduğundan sunucu gönderen kimliğini isteği
//      işlerken zaten biliyor; sealed-sender bunu sunucudan gizlemez, yalnızca
//      DB'ye kalıcı yazılmasını ve alıcıya/3. taraflara iletilmesini engeller.
//   2. Statik katman: gönderen kimlik anahtarı × alıcı kimlik anahtarı (+
//      efemeral DH) → asıl yükü şifreler. Bu katman gönderen kimliğini
//      DOĞRULAR: yalnızca sertifikadaki kimlik anahtarının sahibi bu anahtarı
//      türetebilir (MITM bir sertifikayı sahte bir kimlikle değiştiremez).
//
// Wire format (Rust ile birebir):
//   [1 byte versiyon][32 byte efemeral pub][4 byte BE cert_ct uzunluğu][cert_ct][msg_ct]
//   cert_ct/msg_ct = [12 byte nonce][ciphertext + 16 byte tag] (AES-256-GCM, AAD YOK)
//
// Adım 2'de doğrulanan primitifler (X25519 DH, HKDF-SHA256, Ed25519 sertifika
// imzası) burada gerçek seal/unseal akışına bağlanıyor — bkz.
// crypto/test-vectors/sealed_sender_vectors.json ve
// __tests__/sealed-sender-vector-crosscheck.test.ts.

export const SEALED_SENDER_VERSION = 1;

const ENVELOPE_HEADER_LEN = 1 + 32 + 4; // versiyon + efemeral pub + cert uzunluk alanı
const NONCE_SIZE = 12;
const TAG_SIZE = 16;
const MIN_CT_LEN = NONCE_SIZE + TAG_SIZE; // AES-GCM çıktısının minimum boyutu
const MAX_CERT_CT_LEN = 64 * 1024; // DoS koruması

const INFO_CERT = new TextEncoder().encode("obscura-sealed-sender-v1:cert");
const INFO_MSG = new TextEncoder().encode("obscura-sealed-sender-v1:msg");
const CERT_SIGNING_PREFIX = new TextEncoder().encode("obscura-sender-cert-v1");

// Gönderen/alıcının tam kimlik anahtar çifti — Rust `IdentityKeyPair`'in TS
// karşılığı. Mobile'da X25519 (e2e.ts) ve Ed25519 (identity.ts) anahtarları
// AYRI saklanıyor; bu tip sadece seal/unseal'ın ihtiyaç duyduğu 4 ham baytı
// bir arada taşır — depolama/birleştirme sorumluluğu çağırana ait (Adım 4).
export interface SealedSenderIdentity {
  dhPriv: Uint8Array; // X25519 DH private (32 byte)
  dhPub: Uint8Array; // X25519 DH public (32 byte)
  signingPriv: Uint8Array; // Ed25519 imzalama private/seed (32 byte)
  signingPub: Uint8Array; // Ed25519 imzalama public (32 byte)
}

// Gönderen sertifikası — zarf içinde ŞİFRELİ olarak taşınır. Bu sürümde
// gönderenin kendi Ed25519 anahtarıyla imzalanır (self-signed) — Rust
// sealed_sender.rs ile aynı NOT geçerli: sunucu-imzalı sertifika (CA) ileride
// eklenebilir, bu `signature`'ın kim tarafından üretildiğini değiştirir.
export interface SenderCertificate {
  did: string;
  identityDhPub: Uint8Array;
  signingPub: Uint8Array;
  expiresAt: number; // unix saniye, 0 = süresiz
  signature: Uint8Array;
}

export interface UnsealedMessage {
  senderDid: string;
  senderIdentityDhPub: Uint8Array;
  senderSigningPub: Uint8Array;
  // Adım 8: sertifikanın ham Ed25519 imzası + expiresAt — Rust
  // sealed_sender.rs UnsealedMessage ile 1:1 (bkz. o dosyadaki yorum).
  // Moderasyonun imza-tabanlı kanıt doğrulaması (backend/internal/moderation/
  // evidence.go) bunları decrypt ETMEDEN, cert.verify()'ın burada zaten
  // yaptığı imza kontrolünü sunucu tarafında bağımsızca tekrarlamak için
  // kullanır.
  senderCertSignature: Uint8Array;
  senderCertExpiresAt: number;
  payload: Uint8Array;
}

function concatBytes(...parts: Uint8Array[]): Uint8Array {
  const total = parts.reduce((n, p) => n + p.length, 0);
  const out = new Uint8Array(total);
  let off = 0;
  for (const p of parts) {
    out.set(p, off);
    off += p.length;
  }
  return out;
}

function writeU32BE(n: number): Uint8Array {
  return new Uint8Array([(n >>> 24) & 0xff, (n >>> 16) & 0xff, (n >>> 8) & 0xff, n & 0xff]);
}

function readU32BE(data: Uint8Array, offset: number): number {
  return ((data[offset] << 24) | (data[offset + 1] << 16) | (data[offset + 2] << 8) | data[offset + 3]) >>> 0;
}

// Rust `expires_at.to_be_bytes()` (u64 big-endian, 8 byte) ile birebir —
// DataView gerekli çünkü 8 byte'lık bir sayı 32-bit bit-shift'e sığmaz.
function u64BE(n: number): Uint8Array {
  const buf = new Uint8Array(8);
  new DataView(buf.buffer).setBigUint64(0, BigInt(n), false);
  return buf;
}

// Rust identity.rs::did(): "did:obs:" + SHA256(dh_public)[:16] hex.
export function didFromDhPublic(dhPub: Uint8Array): string {
  const hash = sha256(dhPub);
  return `did:obs:${u8ToHex(hash.slice(0, 16))}`;
}

// Sertifikanın imzalanan/doğrulanan tam byte dizilimi — Rust
// `SenderCertificate::signing_bytes` ile birebir (prefix‖did‖identity_dh_pub‖
// signing_pub‖expires_at_be).
export function certSigningBytes(
  did: string,
  identityDhPub: Uint8Array,
  signingPub: Uint8Array,
  expiresAt: number
): Uint8Array {
  return concatBytes(CERT_SIGNING_PREFIX, new TextEncoder().encode(did), identityDhPub, signingPub, u64BE(expiresAt));
}

// Gönderenin kimlik anahtarıyla sertifika üret ve imzala.
export function issueCertificate(sender: SealedSenderIdentity, expiresAt: number): SenderCertificate {
  const did = didFromDhPublic(sender.dhPub);
  const msg = certSigningBytes(did, sender.dhPub, sender.signingPub, expiresAt);
  const signature = ed25519.sign(msg, sender.signingPriv);
  return { did, identityDhPub: sender.dhPub, signingPub: sender.signingPub, expiresAt, signature };
}

// Sertifika bütünlüğünü doğrula — geçersizse (imza, DID bağlaması veya süre)
// FIRLATIR. `unseal()` bunu MITM koruması olarak zorunlu çağırır: bir zarfı
// hangi kimliğin gönderdiği iddiası, sadece bu kontrol geçerse kabul edilir.
export function verifyCertificate(cert: SenderCertificate, now: number): void {
  if (cert.identityDhPub.length !== 32) throw new Error("Sertifika: identity_dh_pub 32 byte olmalı");
  if (cert.signingPub.length !== 32) throw new Error("Sertifika: signing_pub 32 byte olmalı");

  const msg = certSigningBytes(cert.did, cert.identityDhPub, cert.signingPub, cert.expiresAt);
  if (!ed25519.verify(cert.signature, msg, cert.signingPub)) {
    throw new Error("Sertifika imzası geçersiz");
  }

  const expectedDid = didFromDhPublic(cert.identityDhPub);
  if (cert.did !== expectedDid) {
    throw new Error("Sertifika DID'i kimlik anahtarıyla eşleşmiyor");
  }

  if (cert.expiresAt !== 0 && now > cert.expiresAt) {
    throw new Error("Sertifika süresi dolmuş");
  }
}

// HKDF-SHA256 anahtar türetimi — salt bağlam anahtarlarını bağlar. `pub`
// (export) — cross-implementation test vektörü bunu bağımsız çağırıp
// deterministik k_cert/k_msg'yi Rust'a karşı doğrular (bkz.
// crypto/src/sealed_sender.rs `derive_key` — aynı sebeple orada da pub).
export function deriveKey(
  ikm: Uint8Array,
  ephemeralPub: Uint8Array,
  recipientPub: Uint8Array,
  info: Uint8Array
): Uint8Array {
  const salt = concatBytes(ephemeralPub, recipientPub);
  return hkdf(sha256, ikm, salt, info, 32);
}

function certToJsonBytes(cert: SenderCertificate): Uint8Array {
  // Rust serde varsayılanı: Vec<u8> alanları JSON sayı dizisi olarak
  // serileşir — TS ↔ Rust arasında zarfın gerçekten taşınabilir olması için
  // bu şekli BİREBİR taklit ediyoruz (base64/hex DEĞİL).
  const json = JSON.stringify({
    did: cert.did,
    identity_dh_pub: Array.from(cert.identityDhPub),
    signing_pub: Array.from(cert.signingPub),
    expires_at: cert.expiresAt,
    signature: Array.from(cert.signature),
  });
  return new TextEncoder().encode(json);
}

function certFromJsonBytes(bytes: Uint8Array): SenderCertificate {
  let obj: any;
  try {
    obj = JSON.parse(new TextDecoder().decode(bytes));
  } catch {
    throw new Error("Sertifika parse hatası");
  }
  return {
    did: obj.did,
    identityDhPub: new Uint8Array(obj.identity_dh_pub),
    signingPub: new Uint8Array(obj.signing_pub),
    expiresAt: obj.expires_at,
    signature: new Uint8Array(obj.signature),
  };
}

// AES-256-GCM, AAD YOK (Rust symmetric.rs ile birebir — ratchet.ts'teki
// AAD'li encryptWithAd/decryptWithAd'DEN FARKLI, karıştırmayın).
function encryptNoAad(key: Uint8Array, plaintext: Uint8Array): Uint8Array {
  const nonce = new Uint8Array(NONCE_SIZE);
  crypto.getRandomValues(nonce);
  const ct = gcm(key, nonce).encrypt(plaintext);
  return concatBytes(nonce, ct);
}

function decryptNoAad(key: Uint8Array, data: Uint8Array): Uint8Array {
  if (data.length < MIN_CT_LEN) throw new Error("Veri çok kısa");
  const nonce = data.slice(0, NONCE_SIZE);
  const ct = data.slice(NONCE_SIZE);
  try {
    return gcm(key, nonce).decrypt(ct);
  } catch {
    throw new Error("Şifre çözme hatası — veri bütünlüğü bozulmuş");
  }
}

// Zarf oluştur (gönderen tarafı).
//
// `ephemeralPriv` verilmezse rastgele üretilir (üretim akışında HER ZARFTA
// taze olmalı — unlinkability bu tazeliğe dayanır). Deterministik çağrı
// (sabit ephemeralPriv) SADECE test vektörleriyle cross-validation için —
// üretim kodunda bu parametreyi ASLA sabit geçmeyin (bkz. x3dh.ts'teki aynı
// uyarı, `ephemeralPriv?` parametresi orada da bu yüzden var).
//
// Dönen zarf gönderen hakkında AÇIK hiçbir bilgi içermez.
export function seal(
  sender: SealedSenderIdentity,
  recipientIdentityPub: Uint8Array,
  payload: Uint8Array,
  expiresAt: number,
  ephemeralPriv?: Uint8Array
): Uint8Array {
  if (recipientIdentityPub.length !== 32) {
    throw new Error("Alıcı kimlik anahtarı 32 byte olmalı");
  }

  const ephPriv = ephemeralPriv ?? x25519.keygen().secretKey;
  const ephPub = x25519.getPublicKey(ephPriv);

  // ── Efemeral katman: sertifikayı şifrele ─────────────────────────────────
  const dhEph = x25519.getSharedSecret(ephPriv, recipientIdentityPub);
  const kCert = deriveKey(dhEph, ephPub, recipientIdentityPub, INFO_CERT);

  const cert = issueCertificate(sender, expiresAt);
  const certCt = encryptNoAad(kCert, certToJsonBytes(cert));
  if (certCt.length > MAX_CERT_CT_LEN) {
    throw new Error("Sertifika bloğu çok büyük");
  }

  // ── Statik katman: yükü şifrele (gönderen doğrulaması sağlar) ────────────
  const dhStatic = x25519.getSharedSecret(sender.dhPriv, recipientIdentityPub);
  const ikm = concatBytes(dhEph, dhStatic);
  const kMsg = deriveKey(ikm, ephPub, recipientIdentityPub, INFO_MSG);
  const msgCt = encryptNoAad(kMsg, payload);

  // ── Zarfı birleştir ───────────────────────────────────────────────────────
  const envelope = new Uint8Array(ENVELOPE_HEADER_LEN + certCt.length + msgCt.length);
  envelope[0] = SEALED_SENDER_VERSION;
  envelope.set(ephPub, 1);
  envelope.set(writeU32BE(certCt.length), 33);
  envelope.set(certCt, ENVELOPE_HEADER_LEN);
  envelope.set(msgCt, ENVELOPE_HEADER_LEN + certCt.length);
  return envelope;
}

// Zarfı aç (alıcı tarafı).
//
// `recipient` — alıcının tam kimlik anahtar çifti. Sertifika DOĞRULANMAZSA
// (imza geçersiz, DID uyuşmuyor veya süresi dolmuş) FIRLATIR — bu MITM
// korumasıdır: sunucu ya da bir saldırgan zarfın içindeki sertifikayı sahte
// bir kimlikle değiştiremez, çünkü statik katman (yük şifrelemesi) o sahte
// kimliğin private key'ine ihtiyaç duyar; ama biz burada AYRICA sertifika
// imzasını da doğrulayıp iddiayı erken (yük çözülmeden) reddediyoruz.
export function unseal(recipient: SealedSenderIdentity, envelope: Uint8Array, now: number): UnsealedMessage {
  if (envelope.length < ENVELOPE_HEADER_LEN + MIN_CT_LEN + MIN_CT_LEN) {
    throw new Error("Zarf çok kısa");
  }
  if (envelope[0] !== SEALED_SENDER_VERSION) {
    throw new Error(`Desteklenmeyen zarf versiyonu: ${envelope[0]}`);
  }

  const ephPub = envelope.slice(1, 33);
  const certLen = readU32BE(envelope, 33);
  if (certLen < MIN_CT_LEN || certLen > MAX_CERT_CT_LEN) {
    throw new Error("Zarf: geçersiz sertifika bloğu uzunluğu");
  }
  const certEnd = ENVELOPE_HEADER_LEN + certLen;
  if (envelope.length < certEnd + MIN_CT_LEN) {
    throw new Error("Zarf: sertifika bloğu eksik veya mesaj bloğu yok");
  }
  const certCt = envelope.slice(ENVELOPE_HEADER_LEN, certEnd);
  const msgCt = envelope.slice(certEnd);

  // ── Efemeral katman: sertifikayı çöz ─────────────────────────────────────
  const dhEph = x25519.getSharedSecret(recipient.dhPriv, ephPub);
  const kCert = deriveKey(dhEph, ephPub, recipient.dhPub, INFO_CERT);
  let certJson: Uint8Array;
  try {
    certJson = decryptNoAad(kCert, certCt);
  } catch {
    throw new Error("Zarf açılamadı — yanlış alıcı anahtarı veya bozuk veri");
  }
  const cert = certFromJsonBytes(certJson);
  verifyCertificate(cert, now); // REDDEDER: imza/DID/süre geçersizse burada durur

  // ── Statik katman: yükü çöz (gönderen kimliğini doğrular) ────────────────
  const dhStatic = x25519.getSharedSecret(recipient.dhPriv, cert.identityDhPub);
  const ikm = concatBytes(dhEph, dhStatic);
  const kMsg = deriveKey(ikm, ephPub, recipient.dhPub, INFO_MSG);
  let payload: Uint8Array;
  try {
    payload = decryptNoAad(kMsg, msgCt);
  } catch {
    throw new Error("Zarf yükü çözülemedi — gönderen kimliği doğrulanamadı");
  }

  return {
    senderDid: cert.did,
    senderIdentityDhPub: cert.identityDhPub,
    senderSigningPub: cert.signingPub,
    senderCertSignature: cert.signature,
    senderCertExpiresAt: cert.expiresAt,
    payload,
  };
}
