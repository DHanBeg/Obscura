//! Double Ratchet algoritması — Signal Protocol
//! https://signal.org/docs/specifications/doubleratchet/
//!
//! İki ratchet katmanı:
//!   1. DH Ratchet: Her round-trip'te yeni X25519 key pair, forward secrecy
//!   2. Symmetric Ratchet: HMAC-SHA256 ile chain key → message key türetimi
//!
//! Mesaj şifreleme: AES-256-GCM (header authenticated additional data olarak)

use std::collections::HashMap;
use x25519_dalek::{StaticSecret, PublicKey as X25519Public};
use hmac::{Hmac, Mac};
use sha2::Sha256;
use hkdf::Hkdf;
use rand::rngs::OsRng;
use zeroize::Zeroize;
use serde::{Serialize, Deserialize};

use crate::symmetric::SymKey;

type HmacSha256 = Hmac<Sha256>;

/// Atlanabilecek maksimum mesaj sayısı (out-of-order buffer)
const MAX_SKIP: u32 = 1000;

/// Mesaj başlığı (wire format üzerinde gönderilir, authenticated)
///
/// Wire: [32 byte DH_pub][4 byte PN big-endian][4 byte N big-endian] = 40 byte
#[derive(Clone, Debug)]
pub struct MessageHeader {
    /// Gönderenin mevcut DH ratchet açık anahtarı
    pub dh_pub: [u8; 32],
    /// Önceki gönderme zincirindeki mesaj sayısı (DH adımı öncesi)
    pub pn: u32,
    /// Bu mesajın numarası (mevcut zincirde)
    pub n: u32,
}

impl MessageHeader {
    /// Başlığı 40 byte binary formata dönüştür
    pub fn to_bytes(&self) -> [u8; 40] {
        let mut out = [0u8; 40];
        out[..32].copy_from_slice(&self.dh_pub);
        out[32..36].copy_from_slice(&self.pn.to_be_bytes());
        out[36..40].copy_from_slice(&self.n.to_be_bytes());
        out
    }

    /// 40 byte'tan başlık parse et
    pub fn from_bytes(data: &[u8]) -> Result<Self, &'static str> {
        if data.len() < 40 { return Err("Başlık çok kısa"); }
        let mut dh_pub = [0u8; 32];
        dh_pub.copy_from_slice(&data[..32]);
        let pn = u32::from_be_bytes(data[32..36].try_into().unwrap());
        let n  = u32::from_be_bytes(data[36..40].try_into().unwrap());
        Ok(Self { dh_pub, pn, n })
    }
}

/// Şifreli mesaj (header + ciphertext birlikte iletilir)
///
/// Wire: [40 byte header][rest = AES-GCM output (12+len+16)]
pub struct EncryptedMessage {
    pub header: MessageHeader,
    pub ciphertext: Vec<u8>,
}

impl EncryptedMessage {
    pub fn to_bytes(&self) -> Vec<u8> {
        let mut out = Vec::with_capacity(40 + self.ciphertext.len());
        out.extend_from_slice(&self.header.to_bytes());
        out.extend_from_slice(&self.ciphertext);
        out
    }

    pub fn from_bytes(data: &[u8]) -> Result<Self, String> {
        if data.len() < 40 + 28 { // 40 header + 12 nonce + 16 tag minimum
            return Err("Mesaj çok kısa".into());
        }
        let header = MessageHeader::from_bytes(&data[..40])
            .map_err(|e| e.to_string())?;
        let ciphertext = data[40..].to_vec();
        Ok(Self { header, ciphertext })
    }
}

/// Double Ratchet durumu — her iki yönlü konuşma için bir instance
///
/// `Serialize`/`Deserialize` — FFI sınırında Go tarafının durumu opak bir
/// JSON blob olarak saklamasını sağlar (her mesajdan sonra geri yazılır).
/// SADECE şifreli depolanmalı: tüm zincir anahtarları ve DH private burada.
#[derive(Serialize, Deserialize)]
pub struct RatchetState {
    // ── DH Ratchet ─────────────────────────────────────────────────────────
    /// Bizim gönderme DH private key (statik, gerektiğinde döndürülür)
    dhs_priv: [u8; 32],
    /// Bizim gönderme DH public key
    pub dhs_pub: [u8; 32],
    /// Karşı tarafın DH ratchet public key (None = henüz almadık)
    dhr: Option<[u8; 32]>,

    // ── Symmetric Ratchet ───────────────────────────────────────────────────
    /// Root key (her DH adımında güncellenir)
    rk: [u8; 32],
    /// Gönderme zincir anahtarı (None = DH adımı henüz yapılmadı)
    cks: Option<[u8; 32]>,
    /// Alma zincir anahtarı
    ckr: Option<[u8; 32]>,

    // ── Sayaçlar ────────────────────────────────────────────────────────────
    /// Gönderilen mesaj sayacı (mevcut zincirde)
    ns: u32,
    /// Alınan mesaj sayacı (mevcut zincirde)
    nr: u32,
    /// Önceki gönderme zincirinin uzunluğu
    pn: u32,

    // ── Out-of-order buffer ─────────────────────────────────────────────────
    /// Atlanmış mesaj anahtarları: "hex(dh_pub):msg_num" → message_key
    mk_skipped: HashMap<String, [u8; 32]>,
}

impl Drop for RatchetState {
    fn drop(&mut self) {
        self.dhs_priv.zeroize();
        self.rk.zeroize();
        if let Some(ref mut k) = self.cks { k.zeroize(); }
        if let Some(ref mut k) = self.ckr { k.zeroize(); }
        for v in self.mk_skipped.values_mut() { v.zeroize(); }
    }
}

impl RatchetState {
    // ──────────────────────────────────────────────────────────────────────
    // Başlatma
    // ──────────────────────────────────────────────────────────────────────

    /// Gönderen (Alice) başlatması — X3DH shared_key ile
    ///
    /// `shared_key` — X3DH çıktısı (32 byte)
    /// `bob_ratchet_pub` — Bob'un ilk DH ratchet açık anahtarı (32 byte)
    pub fn init_sender(shared_key: &[u8; 32], bob_ratchet_pub: &[u8; 32]) -> Self {
        // Alice yeni DH ratchet key pair üretir
        let dhs = StaticSecret::random_from_rng(OsRng);
        let dhs_pub = X25519Public::from(&dhs);

        // KDF_RK(SK, DH(dhs, bob_ratchet_pub))
        let dh_out = dhs.diffie_hellman(&X25519Public::from(*bob_ratchet_pub));
        let (new_rk, cks) = kdf_rk(shared_key, dh_out.as_bytes());

        Self {
            dhs_priv: dhs.to_bytes(),
            dhs_pub: *dhs_pub.as_bytes(),
            dhr: Some(*bob_ratchet_pub),
            rk: new_rk,
            cks: Some(cks),
            ckr: None,
            ns: 0,
            nr: 0,
            pn: 0,
            mk_skipped: HashMap::new(),
        }
    }

    /// Alan (Bob) başlatması — X3DH shared_key ile
    ///
    /// `shared_key` — X3DH çıktısı (32 byte)
    /// `bob_ratchet_key` — Bob'un DH ratchet private key bytes (32 byte)
    pub fn init_receiver(shared_key: &[u8; 32], bob_ratchet_priv: &[u8; 32]) -> Self {
        let dhs = StaticSecret::from(*bob_ratchet_priv);
        let dhs_pub = X25519Public::from(&dhs);

        Self {
            dhs_priv: *bob_ratchet_priv,
            dhs_pub: *dhs_pub.as_bytes(),
            dhr: None,
            rk: *shared_key,
            cks: None,
            ckr: None,
            ns: 0,
            nr: 0,
            pn: 0,
            mk_skipped: HashMap::new(),
        }
    }

    // ──────────────────────────────────────────────────────────────────────
    // Şifreleme / Şifre çözme
    // ──────────────────────────────────────────────────────────────────────

    /// Mesaj şifrele
    ///
    /// `plaintext` — düz metin
    /// `ad` — additional data (ör. konuşma ID'si, her iki tarafça bilinir)
    pub fn encrypt(&mut self, plaintext: &[u8], ad: &[u8]) -> Result<EncryptedMessage, String> {
        // CKs yoksa DH adımı yapılmamış — ilk mesaj öncesi DH ratchet al
        // (init_sender zaten CKs üretir; burası sadece güvenlik için)
        let cks = self.cks.ok_or("Gönderme zincir anahtarı yok — init_sender kullan")?;

        // Symmetric ratchet: CKs ilerlet, mesaj anahtarı al
        let (new_cks, mk) = kdf_ck(&cks);
        self.cks = Some(new_cks);

        let header = MessageHeader {
            dh_pub: self.dhs_pub,
            pn: self.pn,
            n: self.ns,
        };
        self.ns += 1;

        // Authenticated additional data: AD || header_bytes
        let mut aad = ad.to_vec();
        aad.extend_from_slice(&header.to_bytes());

        let sym_key = SymKey::from_bytes(&mk).unwrap();
        let ciphertext = encrypt_with_ad(&sym_key, plaintext, &aad)?;

        Ok(EncryptedMessage { header, ciphertext })
    }

    /// Mesaj şifresini çöz
    ///
    /// `msg` — alınan şifreli mesaj
    /// `ad` — additional data (aynı)
    pub fn decrypt(&mut self, msg: &EncryptedMessage, ad: &[u8]) -> Result<Vec<u8>, String> {
        // Önce skipped mesaj anahtarlarında ara
        if let Some(plaintext) = self.try_skipped_message(&msg.header, &msg.ciphertext, ad)? {
            return Ok(plaintext);
        }

        // Yeni DH ratchet key geldi mi?
        let is_new_dhr = self.dhr.as_ref()
            .map(|dhr| dhr != &msg.header.dh_pub)
            .unwrap_or(true);

        if is_new_dhr {
            // Mevcut zincirdeki atlanmış mesajları sakla
            self.skip_message_keys(msg.header.pn)?;
            // DH ratchet adımı
            self.dh_ratchet(&msg.header)?;
        }

        // Alma zincirindeki atlanmış mesajları sakla
        self.skip_message_keys(msg.header.n)?;

        // Alma zincirini ilerlet ve mesaj anahtarını al
        let ckr = self.ckr.ok_or("Alma zincir anahtarı yok")?;
        let (new_ckr, mk) = kdf_ck(&ckr);
        self.ckr = Some(new_ckr);
        self.nr += 1;

        // Şifre çöz
        let mut aad = ad.to_vec();
        aad.extend_from_slice(&msg.header.to_bytes());
        let sym_key = SymKey::from_bytes(&mk).unwrap();
        decrypt_with_ad(&sym_key, &msg.ciphertext, &aad)
    }

    // ──────────────────────────────────────────────────────────────────────
    // Yardımcı: DH Ratchet adımı
    // ──────────────────────────────────────────────────────────────────────

    fn dh_ratchet(&mut self, header: &MessageHeader) -> Result<(), String> {
        self.pn = self.ns;
        self.ns = 0;
        self.nr = 0;
        self.dhr = Some(header.dh_pub);

        // Alma zinciri: KDF_RK(RK, DH(dhs, dhr))
        let dhs = StaticSecret::from(self.dhs_priv);
        let dhr_pub = X25519Public::from(header.dh_pub);
        let dh_out = dhs.diffie_hellman(&dhr_pub);
        let (new_rk, new_ckr) = kdf_rk(&self.rk, dh_out.as_bytes());
        self.rk = new_rk;
        self.ckr = Some(new_ckr);

        // Yeni DH key pair üret (gönderme için)
        let new_dhs = StaticSecret::random_from_rng(OsRng);
        let new_dhs_pub = X25519Public::from(&new_dhs);

        // Gönderme zinciri: KDF_RK(RK, DH(new_dhs, dhr))
        let dh_out2 = new_dhs.diffie_hellman(&dhr_pub);
        let (new_rk2, new_cks) = kdf_rk(&self.rk, dh_out2.as_bytes());
        self.rk = new_rk2;
        self.cks = Some(new_cks);

        // Yeni gizli anahtarı kaydet (eski otomatik zeroize)
        self.dhs_priv.zeroize();
        self.dhs_priv = new_dhs.to_bytes();
        self.dhs_pub = *new_dhs_pub.as_bytes();

        Ok(())
    }

    /// Atlanmış mesaj anahtarlarını sakla (N'e kadar)
    fn skip_message_keys(&mut self, until: u32) -> Result<(), String> {
        if self.nr + MAX_SKIP < until {
            return Err(format!(
                "Çok fazla mesaj atlandı: nr={} until={} max={}",
                self.nr, until, MAX_SKIP
            ));
        }

        if let Some(ckr) = self.ckr {
            let mut current_ckr = ckr;
            let dhr_hex = self.dhr
                .as_ref()
                .map(|d| hex::encode(d))
                .unwrap_or_default();

            while self.nr < until {
                let (new_ckr, mk) = kdf_ck(&current_ckr);
                current_ckr = new_ckr;
                let key = format!("{}:{}", dhr_hex, self.nr);
                self.mk_skipped.insert(key, mk);
                self.nr += 1;
            }
            self.ckr = Some(current_ckr);
        }

        Ok(())
    }

    /// Skipped mesaj anahtarlarında ara
    fn try_skipped_message(
        &mut self,
        header: &MessageHeader,
        ciphertext: &[u8],
        ad: &[u8],
    ) -> Result<Option<Vec<u8>>, String> {
        let key = format!("{}:{}", hex::encode(&header.dh_pub), header.n);
        if let Some(mk) = self.mk_skipped.remove(&key) {
            let mut aad = ad.to_vec();
            aad.extend_from_slice(&header.to_bytes());
            let sym_key = SymKey::from_bytes(&mk).unwrap();
            let pt = decrypt_with_ad(&sym_key, ciphertext, &aad)?;
            return Ok(Some(pt));
        }
        Ok(None)
    }
}

// ────────────────────────────────────────────────────────────────────────────
// KDF fonksiyonları
// ────────────────────────────────────────────────────────────────────────────

/// KDF_RK: Root Key + DH output → (yeni RK, yeni CK)
///
/// HKDF-SHA256(salt=rk, ikm=dh_out, info="ObscuraRatchetRK") → 64 byte
fn kdf_rk(rk: &[u8; 32], dh_out: &[u8]) -> ([u8; 32], [u8; 32]) {
    let hk = Hkdf::<Sha256>::new(Some(rk), dh_out);
    let mut okm = [0u8; 64];
    hk.expand(b"obscura-dr-ratchet-v1", &mut okm).expect("HKDF expand hatası");

    let mut new_rk = [0u8; 32];
    let mut new_ck = [0u8; 32];
    new_rk.copy_from_slice(&okm[..32]);
    new_ck.copy_from_slice(&okm[32..]);
    okm.zeroize();
    (new_rk, new_ck)
}

/// KDF_CK: Chain Key → (yeni CK, mesaj anahtarı)
///
/// HMAC-SHA256(key=ck, data=0x02) → yeni CK
/// HMAC-SHA256(key=ck, data=0x01) → mesaj anahtarı
fn kdf_ck(ck: &[u8; 32]) -> ([u8; 32], [u8; 32]) {
    let mut mac_mk = HmacSha256::new_from_slice(ck).expect("HMAC key hatası");
    mac_mk.update(&[0x01]);
    let mk_bytes = mac_mk.finalize().into_bytes();

    let mut mac_ck = HmacSha256::new_from_slice(ck).expect("HMAC key hatası");
    mac_ck.update(&[0x02]);
    let ck_bytes = mac_ck.finalize().into_bytes();

    let mut mk = [0u8; 32];
    let mut new_ck = [0u8; 32];
    mk.copy_from_slice(&mk_bytes);
    new_ck.copy_from_slice(&ck_bytes);
    (new_ck, mk)
}

// ────────────────────────────────────────────────────────────────────────────
// AES-256-GCM + additional data
// ────────────────────────────────────────────────────────────────────────────

fn encrypt_with_ad(key: &SymKey, plaintext: &[u8], aad: &[u8]) -> Result<Vec<u8>, String> {
    use aes_gcm::{Aes256Gcm, Key, Nonce, aead::{Aead, KeyInit, Payload}};
    use rand::{rngs::OsRng, RngCore};

    let cipher = Aes256Gcm::new(Key::<Aes256Gcm>::from_slice(&key.0));
    let mut nonce_bytes = [0u8; 12];
    OsRng.fill_bytes(&mut nonce_bytes);
    let nonce = Nonce::from_slice(&nonce_bytes);

    let payload = Payload { msg: plaintext, aad };
    let ct = cipher.encrypt(nonce, payload)
        .map_err(|e| format!("Şifreleme hatası: {:?}", e))?;

    let mut out = nonce_bytes.to_vec();
    out.extend_from_slice(&ct);
    Ok(out)
}

fn decrypt_with_ad(key: &SymKey, data: &[u8], aad: &[u8]) -> Result<Vec<u8>, String> {
    use aes_gcm::{Aes256Gcm, Key, Nonce, aead::{Aead, KeyInit, Payload}};

    if data.len() < 28 { // 12 nonce + 16 tag
        return Err("Şifreli veri çok kısa".into());
    }

    let cipher = Aes256Gcm::new(Key::<Aes256Gcm>::from_slice(&key.0));
    let nonce = Nonce::from_slice(&data[..12]);
    let payload = Payload { msg: &data[12..], aad };

    cipher.decrypt(nonce, payload)
        .map_err(|_| "Şifre çözme hatası — bütünlük bozulmuş veya yanlış anahtar".into())
}

// ────────────────────────────────────────────────────────────────────────────
// Testler
// ────────────────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;

    fn make_shared_key() -> [u8; 32] {
        [0x42u8; 32]
    }

    fn make_bob_ratchet() -> ([u8; 32], [u8; 32]) {
        let secret = StaticSecret::random_from_rng(OsRng);
        let public = X25519Public::from(&secret);
        (secret.to_bytes(), *public.as_bytes())
    }

    #[test]
    fn basic_send_receive() {
        let sk = make_shared_key();
        let (bob_ratchet_priv, bob_ratchet_pub) = make_bob_ratchet();

        let mut alice = RatchetState::init_sender(&sk, &bob_ratchet_pub);
        let mut bob   = RatchetState::init_receiver(&sk, &bob_ratchet_priv);

        let ad = b"conv-id-001";
        let pt = b"Merhaba Bob!";

        let enc = alice.encrypt(pt, ad).unwrap();
        let dec = bob.decrypt(&enc, ad).unwrap();
        assert_eq!(dec, pt);
    }

    #[test]
    fn multi_message_alice_to_bob() {
        let sk = make_shared_key();
        let (bob_priv, bob_pub) = make_bob_ratchet();
        let mut alice = RatchetState::init_sender(&sk, &bob_pub);
        let mut bob   = RatchetState::init_receiver(&sk, &bob_priv);
        let ad = b"conv-001";

        for i in 0u8..10 {
            let pt = vec![i; 64];
            let enc = alice.encrypt(&pt, ad).unwrap();
            let dec = bob.decrypt(&enc, ad).unwrap();
            assert_eq!(dec, pt, "Mesaj {} eşleşmedi", i);
        }
    }

    #[test]
    fn bidirectional_ratchet() {
        let sk = make_shared_key();
        let (bob_priv, bob_pub) = make_bob_ratchet();
        let mut alice = RatchetState::init_sender(&sk, &bob_pub);
        let mut bob   = RatchetState::init_receiver(&sk, &bob_priv);
        let ad = b"conv-bidir";

        // Alice -> Bob
        let m1 = alice.encrypt(b"A-to-B message 1", ad).unwrap();
        let d1 = bob.decrypt(&m1, ad).unwrap();
        assert_eq!(d1, b"A-to-B message 1");

        // Bob -> Alice (DH ratchet devreye girer)
        let m2 = bob.encrypt(b"B-to-A reply", ad).unwrap();
        let d2 = alice.decrypt(&m2, ad).unwrap();
        assert_eq!(d2, b"B-to-A reply");

        // Alice -> Bob (tekrar)
        let m3 = alice.encrypt(b"A-to-B message 2", ad).unwrap();
        let d3 = bob.decrypt(&m3, ad).unwrap();
        assert_eq!(d3, b"A-to-B message 2");
    }

    #[test]
    fn header_serialization() {
        let hdr = MessageHeader {
            dh_pub: [0xABu8; 32],
            pn: 42,
            n: 7,
        };
        let bytes = hdr.to_bytes();
        let hdr2 = MessageHeader::from_bytes(&bytes).unwrap();
        assert_eq!(hdr2.dh_pub, hdr.dh_pub);
        assert_eq!(hdr2.pn, 42);
        assert_eq!(hdr2.n, 7);
    }

    #[test]
    fn encrypted_message_wire_format() {
        let sk = make_shared_key();
        let (bob_priv, bob_pub) = make_bob_ratchet();
        let mut alice = RatchetState::init_sender(&sk, &bob_pub);
        let mut bob   = RatchetState::init_receiver(&sk, &bob_priv);
        let ad = b"wire-test";
        let pt = b"Wire format test mesaji";

        let enc = alice.encrypt(pt, ad).unwrap();
        let wire = enc.to_bytes();

        // Parse from wire
        let parsed = EncryptedMessage::from_bytes(&wire).unwrap();
        let dec = bob.decrypt(&parsed, ad).unwrap();
        assert_eq!(dec, pt);
    }
}
