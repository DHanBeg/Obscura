import "react-native-get-random-values";
import { x25519 } from "@noble/curves/ed25519.js";
import { hkdf } from "@noble/hashes/hkdf.js";
import { hmac } from "@noble/hashes/hmac.js";
import { sha256 } from "@noble/hashes/sha256.js";
import { gcm } from "@noble/ciphers/aes.js";
import { u8ToHex, hexToU8 } from "./crypto";

// Double Ratchet — Signal Protocol. 1:1 port of crypto/src/ratchet.rs.
// İki katman: DH Ratchet (her round-trip'te yeni X25519 keypair, forward
// secrecy) + Symmetric Ratchet (HMAC-SHA256 ile chain key → message key).
// Mesaj: AES-256-GCM, header authenticated additional data olarak.
//
// Wire format (Rust ile birebir):
//   header  = [32 byte dh_pub][4 byte pn big-endian][4 byte n big-endian] = 40 byte
//   message = [40 byte header][12 byte nonce][ciphertext][16 byte tag]

const MAX_SKIP = 1000; // out-of-order buffer sınırı — sınırsız saklama = DoS riski
const KDF_RK_INFO = new TextEncoder().encode("obscura-dr-ratchet-v1");

export interface MessageHeader {
  dhPub: Uint8Array; // gönderenin mevcut DH ratchet public key'i (32 byte)
  pn: number; // önceki gönderme zincirindeki mesaj sayısı (DH adımı öncesi)
  n: number; // bu mesajın mevcut zincirdeki numarası
}

export interface EncryptedMessage {
  header: MessageHeader;
  ciphertext: Uint8Array;
}

// RatchetState'in opak, taşınabilir gösterimi — Rust tarafının
// #[derive(Serialize, Deserialize)] karşılığı (bkz. crypto/src/ratchet.rs
// RatchetState doc yorumu: "FFI sınırında ... durumu opak bir JSON blob
// olarak saklamasını sağlar"). SADECE session-store.ts gibi bir katman
// tarafından ŞİFRELİ saklanmalı — tüm zincir anahtarları ve DH private
// burada açık haldedir.
export interface SerializedRatchetState {
  dhsPriv: string; // hex
  dhsPub: string; // hex
  dhr: string | null; // hex
  rk: string; // hex
  cks: string | null; // hex
  ckr: string | null; // hex
  ns: number;
  nr: number;
  pn: number;
  mkSkipped: Array<{ key: string; mk: string }>; // mk: hex
}

function zero(bytes: Uint8Array): void {
  bytes.fill(0);
}

// ─── Header / wire format ──────────────────────────────────────────────────

export function headerToBytes(h: MessageHeader): Uint8Array {
  const out = new Uint8Array(40);
  out.set(h.dhPub, 0);
  out[32] = (h.pn >>> 24) & 0xff;
  out[33] = (h.pn >>> 16) & 0xff;
  out[34] = (h.pn >>> 8) & 0xff;
  out[35] = h.pn & 0xff;
  out[36] = (h.n >>> 24) & 0xff;
  out[37] = (h.n >>> 16) & 0xff;
  out[38] = (h.n >>> 8) & 0xff;
  out[39] = h.n & 0xff;
  return out;
}

export function headerFromBytes(data: Uint8Array): MessageHeader {
  if (data.length < 40) throw new Error("Başlık çok kısa");
  const dhPub = data.slice(0, 32);
  const pn = ((data[32] << 24) | (data[33] << 16) | (data[34] << 8) | data[35]) >>> 0;
  const n = ((data[36] << 24) | (data[37] << 16) | (data[38] << 8) | data[39]) >>> 0;
  return { dhPub, pn, n };
}

export function encryptedMessageToBytes(msg: EncryptedMessage): Uint8Array {
  const headerBytes = headerToBytes(msg.header);
  const out = new Uint8Array(headerBytes.length + msg.ciphertext.length);
  out.set(headerBytes, 0);
  out.set(msg.ciphertext, headerBytes.length);
  return out;
}

export function encryptedMessageFromBytes(data: Uint8Array): EncryptedMessage {
  if (data.length < 40 + 28) throw new Error("Mesaj çok kısa"); // 40 header + 12 nonce + 16 tag
  const header = headerFromBytes(data.slice(0, 40));
  const ciphertext = data.slice(40);
  return { header, ciphertext };
}

function buildAad(ad: Uint8Array, header: MessageHeader): Uint8Array {
  const out = new Uint8Array(ad.length + 40);
  out.set(ad, 0);
  out.set(headerToBytes(header), ad.length);
  return out;
}

// ─── KDF fonksiyonları (saf, deterministik — cross-implementation vektörleri için export) ──

// KDF_RK: Root Key + DH output → (yeni RK, yeni CK)
// HKDF-SHA256(salt=rk, ikm=dh_out, info="obscura-dr-ratchet-v1") → 64 byte
export function kdfRk(rk: Uint8Array, dhOut: Uint8Array): { rootKey: Uint8Array; chainKey: Uint8Array } {
  const okm = hkdf(sha256, dhOut, rk, KDF_RK_INFO, 64);
  const rootKey = okm.slice(0, 32);
  const chainKey = okm.slice(32, 64);
  zero(okm);
  return { rootKey, chainKey };
}

// KDF_CK: Chain Key → (yeni CK, mesaj anahtarı)
// HMAC-SHA256(key=ck, data=0x01) → mesaj anahtarı
// HMAC-SHA256(key=ck, data=0x02) → yeni CK
export function kdfCk(ck: Uint8Array): { chainKey: Uint8Array; messageKey: Uint8Array } {
  const messageKey = hmac(sha256, ck, new Uint8Array([0x01]));
  const chainKey = hmac(sha256, ck, new Uint8Array([0x02]));
  return { chainKey, messageKey };
}

// ─── AES-256-GCM + additional data ─────────────────────────────────────────

function encryptWithAd(key: Uint8Array, plaintext: Uint8Array, aad: Uint8Array): Uint8Array {
  const nonce = new Uint8Array(12);
  crypto.getRandomValues(nonce);
  const ct = gcm(key, nonce, aad).encrypt(plaintext);
  const out = new Uint8Array(12 + ct.length);
  out.set(nonce, 0);
  out.set(ct, 12);
  return out;
}

function decryptWithAd(key: Uint8Array, data: Uint8Array, aad: Uint8Array): Uint8Array {
  if (data.length < 28) throw new Error("Şifreli veri çok kısa");
  const nonce = data.slice(0, 12);
  const ct = data.slice(12);
  try {
    return gcm(key, nonce, aad).decrypt(ct);
  } catch {
    throw new Error("Şifre çözme hatası — bütünlük bozulmuş veya yanlış anahtar");
  }
}

// ─── Double Ratchet durumu ──────────────────────────────────────────────────
//
// Her iki yönlü konuşma için bir instance. Tüm zincir anahtarları ve DH
// private key burada — bellekte sadece gerektiği kadar tutulur, kullanılan
// mesaj anahtarları ve rotasyonla terk edilen root/chain key'ler zero()
// ile sıfırlanır (forward secrecy: geriye türetilemez).
export class RatchetState {
  private dhsPriv: Uint8Array;
  dhsPub: Uint8Array;
  private dhr: Uint8Array | null;
  private rk: Uint8Array;
  private cks: Uint8Array | null;
  private ckr: Uint8Array | null;
  private ns = 0;
  private nr = 0;
  private pn = 0;
  private mkSkipped = new Map<string, Uint8Array>();
  private destroyed = false;

  private constructor(
    dhsPriv: Uint8Array,
    dhsPub: Uint8Array,
    dhr: Uint8Array | null,
    rk: Uint8Array,
    cks: Uint8Array | null,
    ckr: Uint8Array | null
  ) {
    this.dhsPriv = dhsPriv;
    this.dhsPub = dhsPub;
    this.dhr = dhr;
    this.rk = rk;
    this.cks = cks;
    this.ckr = ckr;
  }

  // Gönderen (Alice) başlatması — X3DH shared_key ile.
  // `dhsPriv` verilmezse rastgele üretilir; sabit değer SADECE test
  // vektörleriyle cross-validation için, üretim akışında ASLA sabit geçmeyin.
  static initSender(sharedKey: Uint8Array, bobRatchetPub: Uint8Array, dhsPriv?: Uint8Array): RatchetState {
    const priv = dhsPriv ?? x25519.keygen().secretKey;
    const pub = x25519.getPublicKey(priv);
    const dhOut = x25519.getSharedSecret(priv, bobRatchetPub);
    const { rootKey, chainKey } = kdfRk(sharedKey, dhOut);
    return new RatchetState(priv, pub, bobRatchetPub.slice(), rootKey, chainKey, null);
  }

  // Alan (Bob) başlatması — X3DH shared_key ile.
  static initReceiver(sharedKey: Uint8Array, bobRatchetPriv: Uint8Array): RatchetState {
    const pub = x25519.getPublicKey(bobRatchetPriv);
    return new RatchetState(bobRatchetPriv, pub, null, sharedKey.slice(), null, null);
  }

  encrypt(plaintext: Uint8Array, ad: Uint8Array): EncryptedMessage {
    if (this.destroyed) throw new Error("RatchetState destroy() ile sıfırlandı, kullanılamaz");
    if (!this.cks) throw new Error("Gönderme zincir anahtarı yok — initSender kullan");

    const { chainKey, messageKey } = kdfCk(this.cks);
    zero(this.cks);
    this.cks = chainKey;

    const header: MessageHeader = { dhPub: this.dhsPub.slice(), pn: this.pn, n: this.ns };
    this.ns += 1;

    const aad = buildAad(ad, header);
    const ciphertext = encryptWithAd(messageKey, plaintext, aad);
    zero(messageKey);

    return { header, ciphertext };
  }

  decrypt(msg: EncryptedMessage, ad: Uint8Array): Uint8Array {
    if (this.destroyed) throw new Error("RatchetState destroy() ile sıfırlandı, kullanılamaz");
    const skipped = this.trySkippedMessage(msg.header, msg.ciphertext, ad);
    if (skipped) return skipped;

    const isNewDhr = this.dhr === null || u8ToHex(this.dhr) !== u8ToHex(msg.header.dhPub);
    if (isNewDhr) {
      this.skipMessageKeys(msg.header.pn);
      this.dhRatchet(msg.header);
    }
    this.skipMessageKeys(msg.header.n);

    if (!this.ckr) throw new Error("Alma zincir anahtarı yok");
    const { chainKey, messageKey } = kdfCk(this.ckr);
    zero(this.ckr);
    this.ckr = chainKey;
    this.nr += 1;

    const aad = buildAad(ad, msg.header);
    const plaintext = decryptWithAd(messageKey, msg.ciphertext, aad);
    zero(messageKey);
    return plaintext;
  }

  private dhRatchet(header: MessageHeader): void {
    this.pn = this.ns;
    this.ns = 0;
    this.nr = 0;
    this.dhr = header.dhPub.slice();

    // Alma zinciri: KDF_RK(RK, DH(dhs, dhr))
    const dhOut1 = x25519.getSharedSecret(this.dhsPriv, header.dhPub);
    const step1 = kdfRk(this.rk, dhOut1);
    zero(this.rk);
    this.rk = step1.rootKey;
    this.ckr = step1.chainKey;

    // Yeni DH keypair üret (gönderme için)
    const newDhsPriv = x25519.keygen().secretKey;
    const newDhsPub = x25519.getPublicKey(newDhsPriv);

    // Gönderme zinciri: KDF_RK(RK, DH(new_dhs, dhr))
    const dhOut2 = x25519.getSharedSecret(newDhsPriv, header.dhPub);
    const step2 = kdfRk(this.rk, dhOut2);
    zero(this.rk);
    this.rk = step2.rootKey;
    this.cks = step2.chainKey;

    zero(this.dhsPriv);
    this.dhsPriv = newDhsPriv;
    this.dhsPub = newDhsPub;
  }

  // Atlanmış mesaj anahtarlarını `until`'e kadar sakla (out-of-order desteği).
  // MAX_SKIP sınırı: sınırsız saklama = bellek tükenmesi DoS riski.
  private skipMessageKeys(until: number): void {
    if (this.nr + MAX_SKIP < until) {
      throw new Error(`Çok fazla mesaj atlandı: nr=${this.nr} until=${until} max=${MAX_SKIP}`);
    }
    if (!this.ckr) return;

    let currentCkr = this.ckr;
    const dhrHex = this.dhr ? u8ToHex(this.dhr) : "";
    while (this.nr < until) {
      const { chainKey, messageKey } = kdfCk(currentCkr);
      zero(currentCkr);
      currentCkr = chainKey;
      this.mkSkipped.set(`${dhrHex}:${this.nr}`, messageKey);
      this.nr += 1;
    }
    this.ckr = currentCkr;
  }

  // Skipped mesaj anahtarlarında ara. Bulunursa anahtar HARİTADAN SİLİNİR
  // ve sıfırlanır — aynı mesaj iki kez çözülemez, forward secrecy korunur.
  private trySkippedMessage(header: MessageHeader, ciphertext: Uint8Array, ad: Uint8Array): Uint8Array | null {
    const key = `${u8ToHex(header.dhPub)}:${header.n}`;
    const mk = this.mkSkipped.get(key);
    if (!mk) return null;
    this.mkSkipped.delete(key);

    const aad = buildAad(ad, header);
    const plaintext = decryptWithAd(mk, ciphertext, aad);
    zero(mk);
    return plaintext;
  }

  // Kaç mesaj anahtarı hâlâ out-of-order buffer'da bekliyor (gözlemlenebilir
  // test/debug amaçlı — özel durumu sızdırmaz, sadece sayaç).
  get skippedCount(): number {
    return this.mkSkipped.size;
  }

  // Opak, taşınabilir gösterime çevir (bkz. SerializedRatchetState).
  // Şifrelemeden asla diske/AsyncStorage'a yazılmamalı — bkz. session-store.ts.
  serialize(): SerializedRatchetState {
    return {
      dhsPriv: u8ToHex(this.dhsPriv),
      dhsPub: u8ToHex(this.dhsPub),
      dhr: this.dhr ? u8ToHex(this.dhr) : null,
      rk: u8ToHex(this.rk),
      cks: this.cks ? u8ToHex(this.cks) : null,
      ckr: this.ckr ? u8ToHex(this.ckr) : null,
      ns: this.ns,
      nr: this.nr,
      pn: this.pn,
      mkSkipped: Array.from(this.mkSkipped.entries()).map(([key, mk]) => ({ key, mk: u8ToHex(mk) })),
    };
  }

  static deserialize(s: SerializedRatchetState): RatchetState {
    const state = new RatchetState(
      hexToU8(s.dhsPriv),
      hexToU8(s.dhsPub),
      s.dhr ? hexToU8(s.dhr) : null,
      hexToU8(s.rk),
      s.cks ? hexToU8(s.cks) : null,
      s.ckr ? hexToU8(s.ckr) : null
    );
    state.ns = s.ns;
    state.nr = s.nr;
    state.pn = s.pn;
    for (const { key, mk } of s.mkSkipped) {
      state.mkSkipped.set(key, hexToU8(mk));
    }
    return state;
  }

  // Oturum artık kullanılmayacaksa tüm kalan gizli durumu sıfırla. Rust
  // tarafının Drop+zeroize karşılığı — JS'de GC deterministik olmadığı için
  // çağıran taraf açıkça çağırmalı.
  destroy(): void {
    zero(this.dhsPriv);
    zero(this.rk);
    if (this.cks) zero(this.cks);
    if (this.ckr) zero(this.ckr);
    for (const mk of this.mkSkipped.values()) zero(mk);
    this.mkSkipped.clear();
    this.destroyed = true;
  }
}
