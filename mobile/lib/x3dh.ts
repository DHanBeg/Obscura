import "react-native-get-random-values";
import { x25519 } from "@noble/curves/ed25519.js";
import { hkdf } from "@noble/hashes/hkdf";
import { sha256 } from "@noble/hashes/sha256";

// X3DH — Extended Triple Diffie-Hellman (Signal Protocol RFC).
// https://signal.org/docs/specifications/x3dh/
// 1:1 port of crypto/src/x3dh.rs — DH1-4 sırası, IKM şekli (0xFF×32 || DH'ler)
// ve HKDF info string'i ("obscura-x3dh-v1") Rust ile birebir aynı olmalı;
// Adım 2/3'te doğrulanan primitifler burada gerçek X3DH akışına bağlanıyor.
//
// Alice = başlatan (initiate), Bob = kabul eden (accept).
// DH1 = DH(IK_A, SPK_B)   DH2 = DH(EK_A, IK_B)
// DH3 = DH(EK_A, SPK_B)   DH4 = DH(EK_A, OPK_B) — opsiyonel
//
// DENİABİLİTY: PreKeyBundle'daki SPK imzası (signedPrekeySig) SADECE bundle
// bütünlüğünü doğrulamak için var (server'da veya fetch sırasında ayrı
// kontrol edilir) — bu modülün DH/HKDF türetimine hiçbir imza baytı GİRMEZ.
// İmza shared_key'in bir girdisi olsaydı, oturum transkripti "bu imzayı
// sadece Bob üretebilirdi" diye kriptografik olarak bağlayıcı hale gelir ve
// off-the-record (inkâr edilebilirlik) özelliği kırılırdı.

const HKDF_INFO = new TextEncoder().encode("obscura-x3dh-v1");

// Bob'un PreKey bundle'ı — X3DH'nin DH matematiği için gereken HAM baytlar.
// signedPrekeySig kasıtlı olarak burada YOK: bu tip sadece derive edilecek
// alanları taşır, imza ayrı bir doğrulama adımına ait (bkz. yukarıdaki not).
export interface PreKeyBundle {
  identityKey: Uint8Array; // Bob IK_B public (X25519, 32 byte)
  signedPrekey: Uint8Array; // Bob SPK_B public (X25519, 32 byte)
  oneTimePrekey?: Uint8Array; // Bob OPK_B public (X25519, 32 byte), opsiyonel
  oneTimePrekeyId?: number;
}

export interface X3DHInitResult {
  sharedKey: Uint8Array; // 32 byte — türetilmiş paylaşılan simetrik anahtar
  ephemeralPublic: Uint8Array; // Alice'in efemeral public key'i — Bob'a gönderilmeli
  oneTimePrekeyId?: number; // Kullanılan OPK'nın id'si (varsa)
}

export interface X3DHAcceptResult {
  sharedKey: Uint8Array;
}

// Signal spec: F = 32×0xFF, ardından tüm DH çıktıları sırayla eklenir.
function deriveSharedKey(dhOutputs: Uint8Array[]): Uint8Array {
  const F = new Uint8Array(32).fill(0xff);
  const total = F.length + dhOutputs.reduce((sum, dh) => sum + dh.length, 0);
  const ikm = new Uint8Array(total);
  let offset = 0;
  ikm.set(F, offset);
  offset += F.length;
  for (const dh of dhOutputs) {
    ikm.set(dh, offset);
    offset += dh.length;
  }
  return hkdf(sha256, ikm, undefined, HKDF_INFO, 32);
}

// Alice — X3DH başlat.
//
// `ephemeralPriv` verilmezse rastgele üretilir (üretim akışında HER OTURUMDA
// taze olmalı). Deterministik çağrı (sabit ephemeralPriv) SADECE test
// vektörleriyle cross-validation için — üretim kodunda bu parametreyi
// ASLA sabit geçmeyin.
export function x3dhInitiate(
  aliceIdentityPriv: Uint8Array,
  bundle: PreKeyBundle,
  ephemeralPriv?: Uint8Array
): X3DHInitResult {
  const ekAPriv = ephemeralPriv ?? x25519.keygen().secretKey;
  const ekAPub = x25519.getPublicKey(ekAPriv);

  const dh1 = x25519.getSharedSecret(aliceIdentityPriv, bundle.signedPrekey); // IK_A × SPK_B
  const dh2 = x25519.getSharedSecret(ekAPriv, bundle.identityKey); // EK_A × IK_B
  const dh3 = x25519.getSharedSecret(ekAPriv, bundle.signedPrekey); // EK_A × SPK_B

  const dhInputs = [dh1, dh2, dh3];
  let oneTimePrekeyId: number | undefined;
  if (bundle.oneTimePrekey) {
    const dh4 = x25519.getSharedSecret(ekAPriv, bundle.oneTimePrekey); // EK_A × OPK_B
    dhInputs.push(dh4);
    oneTimePrekeyId = bundle.oneTimePrekeyId;
  }

  return {
    sharedKey: deriveSharedKey(dhInputs),
    ephemeralPublic: ekAPub,
    oneTimePrekeyId,
  };
}

// Bob — X3DH kabul et (Alice'in ephemeral public key'iyle aynı shared_key'e ulaşır).
export function x3dhAccept(
  bobIdentityPriv: Uint8Array,
  bobSignedPrekeyPriv: Uint8Array,
  aliceIdentityPub: Uint8Array,
  aliceEphemeralPub: Uint8Array,
  bobOneTimePrekeyPriv?: Uint8Array
): X3DHAcceptResult {
  const dh1 = x25519.getSharedSecret(bobSignedPrekeyPriv, aliceIdentityPub); // SPK_B × IK_A
  const dh2 = x25519.getSharedSecret(bobIdentityPriv, aliceEphemeralPub); // IK_B × EK_A
  const dh3 = x25519.getSharedSecret(bobSignedPrekeyPriv, aliceEphemeralPub); // SPK_B × EK_A

  const dhInputs = [dh1, dh2, dh3];
  if (bobOneTimePrekeyPriv) {
    const dh4 = x25519.getSharedSecret(bobOneTimePrekeyPriv, aliceEphemeralPub); // OPK_B × EK_A
    dhInputs.push(dh4);
  }

  return { sharedKey: deriveSharedKey(dhInputs) };
}
