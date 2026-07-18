//! Sealed Sender — gönderen kimliğini sunucudan gizleyen zarf formatı
//!
//! Signal'in "sealed sender" (UnidentifiedSenderMessage) tasarımından uyarlanmıştır:
//! https://signal.org/blog/sealed-sender/
//!
//! İki katmanlı yapı:
//!   1. Efemeral katman: rastgele efemeral X25519 anahtarı × alıcı kimlik anahtarı
//!      → gönderen sertifikasını şifreler. Sunucu yalnızca efemeral açık anahtarı
//!      ve alıcı adresini görür; gönderenin KİM olduğunu göremez.
//!   2. Statik katman: gönderen kimlik anahtarı × alıcı kimlik anahtarı (+ efemeral DH)
//!      → asıl yükü (payload) şifreler. Bu katman gönderen kimliğini doğrular:
//!      yalnızca sertifikadaki kimlik anahtarının sahibi bu anahtarı türetebilir.
//!
//! Zarf wire formatı:
//!   [1 byte versiyon][32 byte efemeral pub][4 byte BE cert_ct uzunluğu][cert_ct][msg_ct]
//!
//! Mevcut primitifler yeniden kullanılır: X25519 (x25519-dalek), HKDF-SHA256,
//! AES-256-GCM (symmetric.rs), Ed25519 sertifika imzası (identity.rs).

use hkdf::Hkdf;
use rand::rngs::OsRng;
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use x25519_dalek::{PublicKey as X25519Public, StaticSecret};
use zeroize::Zeroize;

use crate::identity::IdentityKeyPair;
use crate::symmetric::{decrypt as sym_decrypt, encrypt as sym_encrypt, SymKey, NONCE_SIZE, TAG_SIZE};

/// Zarf format versiyonu
pub const SEALED_SENDER_VERSION: u8 = 1;

/// Zarf minimum boyutu: versiyon + efemeral pub + cert uzunluk alanı
const ENVELOPE_HEADER_LEN: usize = 1 + 32 + 4;

/// AES-GCM çıktısının minimum boyutu (nonce + tag)
const MIN_CT_LEN: usize = NONCE_SIZE + TAG_SIZE;

/// Tek bir sertifika/mesaj şifreli bloğunun kabul edilebilir üst sınırı (DoS koruması)
const MAX_CERT_CT_LEN: usize = 64 * 1024;

/// `pub` — cross-implementation test vektörü (cli.rs `cmd_sealed_sender_vector`)
/// bu etiketle `derive_key()`'i bağımsız olarak yeniden çağırıp k_cert türetir.
pub const INFO_CERT: &[u8] = b"obscura-sealed-sender-v1:cert";
/// `pub` — yukarıdaki gibi, k_msg türetimi için.
pub const INFO_MSG: &[u8] = b"obscura-sealed-sender-v1:msg";
const CERT_SIGNING_PREFIX: &[u8] = b"obscura-sender-cert-v1";

/// Gönderen sertifikası — zarf içinde ŞİFRELİ olarak taşınır.
///
/// NOT: Bu sürümde sertifika gönderenin kendi Ed25519 anahtarıyla imzalanır
/// (self-signed). Sunucu-imzalı sertifika (Signal'deki gibi bir CA) ileride
/// `signature` alanının imzalayanı değiştirilerek eklenebilir.
#[derive(Serialize, Deserialize, Clone, Debug)]
pub struct SenderCertificate {
    /// Gönderenin DID'i (did:obs:...)
    pub did: String,
    /// Gönderenin X25519 kimlik açık anahtarı (32 byte)
    pub identity_dh_pub: Vec<u8>,
    /// Gönderenin Ed25519 imzalama açık anahtarı (32 byte)
    pub signing_pub: Vec<u8>,
    /// Unix saniye cinsinden geçerlilik sonu (0 = süresiz)
    pub expires_at: u64,
    /// Ed25519 imza — signing_bytes() üzerinde
    pub signature: Vec<u8>,
}

impl SenderCertificate {
    /// `pub` — cross-implementation test vektörü bu tam byte diziliminin
    /// (prefix‖did‖identity_dh_pub‖signing_pub‖expires_at_be) TS portunda
    /// birebir aynı şekilde inşa edildiğini doğrular; imza uyuşmazlığında
    /// hangi adımın kaydığını göstermek için ayrı bir vektör alanına girer.
    pub fn signing_bytes(did: &str, identity_dh_pub: &[u8], signing_pub: &[u8], expires_at: u64) -> Vec<u8> {
        let mut m = Vec::with_capacity(
            CERT_SIGNING_PREFIX.len() + did.len() + identity_dh_pub.len() + signing_pub.len() + 8,
        );
        m.extend_from_slice(CERT_SIGNING_PREFIX);
        m.extend_from_slice(did.as_bytes());
        m.extend_from_slice(identity_dh_pub);
        m.extend_from_slice(signing_pub);
        m.extend_from_slice(&expires_at.to_be_bytes());
        m
    }

    /// Gönderenin kimlik anahtarıyla sertifika üret ve imzala
    pub fn issue(sender: &IdentityKeyPair, expires_at: u64) -> Self {
        let did = sender.did();
        let msg = Self::signing_bytes(&did, &sender.dh_public, &sender.signing_public, expires_at);
        let signature = sender.sign(&msg);
        Self {
            did,
            identity_dh_pub: sender.dh_public.clone(),
            signing_pub: sender.signing_public.clone(),
            expires_at,
            signature,
        }
    }

    /// Sertifika bütünlüğünü doğrula:
    ///   1. Ed25519 imzası signing_pub ile geçerli mi?
    ///   2. DID gerçekten identity_dh_pub'dan türetilmiş mi?
    ///   3. Süresi geçmiş mi? (`now` unix saniye; expires_at == 0 → süresiz)
    pub fn verify(&self, now: u64) -> Result<(), String> {
        if self.identity_dh_pub.len() != 32 {
            return Err("Sertifika: identity_dh_pub 32 byte olmalı".into());
        }
        if self.signing_pub.len() != 32 {
            return Err("Sertifika: signing_pub 32 byte olmalı".into());
        }
        let msg = Self::signing_bytes(&self.did, &self.identity_dh_pub, &self.signing_pub, self.expires_at);
        if !IdentityKeyPair::verify_signature(&self.signing_pub, &msg, &self.signature) {
            return Err("Sertifika imzası geçersiz".into());
        }
        // DID ↔ kimlik anahtarı bağlaması (identity.rs::did() ile aynı türetim)
        let mut hasher = Sha256::new();
        hasher.update(&self.identity_dh_pub);
        let hash = hasher.finalize();
        let expected_did = format!("did:obs:{}", hex::encode(&hash[..16]));
        if self.did != expected_did {
            return Err("Sertifika DID'i kimlik anahtarıyla eşleşmiyor".into());
        }
        if self.expires_at != 0 && now > self.expires_at {
            return Err("Sertifika süresi dolmuş".into());
        }
        Ok(())
    }
}

/// Zarf açma sonucu — alıcının öğrendiği gönderen kimliği + düz metin
pub struct UnsealedMessage {
    /// Gönderenin DID'i
    pub sender_did: String,
    /// Gönderenin X25519 kimlik açık anahtarı (32 byte)
    pub sender_identity_dh_pub: Vec<u8>,
    /// Gönderenin Ed25519 imzalama açık anahtarı (32 byte)
    pub sender_signing_pub: Vec<u8>,
    /// Sertifikanın ham Ed25519 imzası (64 byte) — Adım 8: moderasyonun
    /// imza-tabanlı kanıt doğrulaması (backend/internal/moderation/evidence.go)
    /// bunu decrypt ETMEDEN, `SenderCertificate::verify()`'ın burada zaten
    /// yaptığı imza kontrolünü sunucu tarafında BAĞIMSIZCA tekrarlamak için
    /// kullanır — plaintext from_did'e ihtiyaç duymaz.
    pub sender_cert_signature: Vec<u8>,
    /// Sertifikanın geçerlilik sonu (unix saniye, 0 = süresiz) — imzalanan
    /// `signing_bytes`'ın bir parçası, doğrulama bunsuz tekrarlanamaz.
    pub sender_cert_expires_at: u64,
    /// Çözülen yük (ör. Double Ratchet wire mesajı)
    pub payload: Vec<u8>,
}

/// HKDF-SHA256 anahtar türetimi — salt bağlam anahtarlarını bağlar
///
/// `pub` — cross-implementation test vektörü bunu k_cert/k_msg'yi bağımsız
/// yeniden hesaplayıp deterministik parçaları TS portuna karşı doğrulamak için
/// çağırır (bkz. crypto/src/bin/cli.rs `cmd_sealed_sender_vector`).
pub fn derive_key(ikm: &[u8], ephemeral_pub: &[u8; 32], recipient_pub: &[u8; 32], info: &[u8]) -> SymKey {
    let mut salt = Vec::with_capacity(64);
    salt.extend_from_slice(ephemeral_pub);
    salt.extend_from_slice(recipient_pub);

    let hk = Hkdf::<Sha256>::new(Some(&salt), ikm);
    let mut okm = [0u8; 32];
    hk.expand(info, &mut okm).expect("HKDF expand hatası");
    let key = SymKey::from_bytes(&okm).expect("32 byte OKM");
    okm.zeroize();
    key
}

fn to_arr32(bytes: &[u8], label: &str) -> Result<[u8; 32], String> {
    bytes.try_into().map_err(|_| format!("{label} 32 byte olmalı"))
}

/// Zarf oluştur (gönderen tarafı) — rastgele efemeral anahtar (OsRng).
///
/// `sender` — gönderenin tam kimlik anahtar çifti
/// `recipient_identity_pub` — alıcının X25519 kimlik açık anahtarı (32 byte)
/// `payload` — şifrelenecek yük (tipik olarak ratchet-şifreli mesaj bytes)
/// `expires_at` — sertifika geçerlilik sonu (unix saniye, 0 = süresiz)
///
/// Dönen zarf gönderen hakkında AÇIK hiçbir bilgi içermez.
pub fn seal(
    sender: &IdentityKeyPair,
    recipient_identity_pub: &[u8],
    payload: &[u8],
    expires_at: u64,
) -> Result<Vec<u8>, String> {
    let eph_secret_bytes = StaticSecret::random_from_rng(OsRng).to_bytes();
    seal_with_ephemeral(sender, recipient_identity_pub, payload, expires_at, eph_secret_bytes)
}

/// `seal()`'in efemeral anahtarı ÇAĞIRANDAN aldığı hâli.
///
/// Üretim akışında KULLANILMAZ (`seal()` OsRng ile çağırır) — cross-
/// implementation test vektörleri için var: sabit `eph_secret_bytes` verilince
/// zarfın açık (efemeral pub) kısmı deterministik olur, TS portu buna karşı
/// doğrulanabilir (bkz. crypto/test-vectors/sealed_sender_vectors.json).
/// Sertifika bloğu/mesaj bloğu (AES-GCM) hâlâ rastgele nonce kullanır — bu
/// yüzden zarfın TAMAMI değil, deterministik kısımları (efemeral pub, HKDF
/// anahtarları, sertifika imzası) vektöre girer; round-trip gerçek `unseal()`
/// ile ayrıca kanıtlanır.
pub fn seal_with_ephemeral(
    sender: &IdentityKeyPair,
    recipient_identity_pub: &[u8],
    payload: &[u8],
    expires_at: u64,
    eph_secret_bytes: [u8; 32],
) -> Result<Vec<u8>, String> {
    let recip_arr = to_arr32(recipient_identity_pub, "Alıcı kimlik anahtarı")?;
    let recip_pub = X25519Public::from(recip_arr);

    // ── Efemeral katman: sertifikayı şifrele ───────────────────────────────
    let eph_secret = StaticSecret::from(eph_secret_bytes);
    let eph_pub = X25519Public::from(&eph_secret);
    let eph_pub_bytes: [u8; 32] = *eph_pub.as_bytes();

    let dh_eph = eph_secret.diffie_hellman(&recip_pub);
    let k_cert = derive_key(dh_eph.as_bytes(), &eph_pub_bytes, &recip_arr, INFO_CERT);

    let cert = SenderCertificate::issue(sender, expires_at);
    let cert_json = serde_json::to_vec(&cert).map_err(|e| e.to_string())?;
    let cert_ct = sym_encrypt(&k_cert, &cert_json)?;

    // ── Statik katman: yükü şifrele (gönderen doğrulaması sağlar) ──────────
    let dh_static = sender.dh_secret().diffie_hellman(&recip_pub);
    let mut ikm = Vec::with_capacity(64);
    ikm.extend_from_slice(dh_eph.as_bytes());
    ikm.extend_from_slice(dh_static.as_bytes());
    let k_msg = derive_key(&ikm, &eph_pub_bytes, &recip_arr, INFO_MSG);
    ikm.zeroize();

    let msg_ct = sym_encrypt(&k_msg, payload)?;

    // ── Zarfı birleştir ─────────────────────────────────────────────────────
    if cert_ct.len() > MAX_CERT_CT_LEN {
        return Err("Sertifika bloğu çok büyük".into());
    }
    let mut envelope = Vec::with_capacity(ENVELOPE_HEADER_LEN + cert_ct.len() + msg_ct.len());
    envelope.push(SEALED_SENDER_VERSION);
    envelope.extend_from_slice(&eph_pub_bytes);
    envelope.extend_from_slice(&(cert_ct.len() as u32).to_be_bytes());
    envelope.extend_from_slice(&cert_ct);
    envelope.extend_from_slice(&msg_ct);
    Ok(envelope)
}

/// Zarfı aç (alıcı tarafı)
///
/// `recipient` — alıcının tam kimlik anahtar çifti
/// `envelope` — `seal()` çıktısı
/// `now` — unix saniye (sertifika süre kontrolü; 0 verilirse yine de
///         expires_at == 0 olmayan süresi geçmiş sertifikalar kabul EDİLİR —
///         çağıran gerçek zamanı vermelidir)
pub fn unseal(recipient: &IdentityKeyPair, envelope: &[u8], now: u64) -> Result<UnsealedMessage, String> {
    // ── Zarf parse (tüm uzunluklar doğrulanır) ─────────────────────────────
    if envelope.len() < ENVELOPE_HEADER_LEN + MIN_CT_LEN + MIN_CT_LEN {
        return Err("Zarf çok kısa".into());
    }
    if envelope[0] != SEALED_SENDER_VERSION {
        return Err(format!("Desteklenmeyen zarf versiyonu: {}", envelope[0]));
    }
    let eph_pub_bytes: [u8; 32] = envelope[1..33].try_into().unwrap();
    let cert_len = u32::from_be_bytes(envelope[33..37].try_into().unwrap()) as usize;
    if cert_len < MIN_CT_LEN || cert_len > MAX_CERT_CT_LEN {
        return Err("Zarf: geçersiz sertifika bloğu uzunluğu".into());
    }
    let cert_end = ENVELOPE_HEADER_LEN
        .checked_add(cert_len)
        .ok_or("Zarf: uzunluk taşması")?;
    if envelope.len() < cert_end + MIN_CT_LEN {
        return Err("Zarf: sertifika bloğu eksik veya mesaj bloğu yok".into());
    }
    let cert_ct = &envelope[ENVELOPE_HEADER_LEN..cert_end];
    let msg_ct = &envelope[cert_end..];

    let recip_arr = to_arr32(&recipient.dh_public, "Alıcı kimlik açık anahtarı")?;
    let eph_pub = X25519Public::from(eph_pub_bytes);

    // ── Efemeral katman: sertifikayı çöz ───────────────────────────────────
    let dh_eph = recipient.dh_secret().diffie_hellman(&eph_pub);
    let k_cert = derive_key(dh_eph.as_bytes(), &eph_pub_bytes, &recip_arr, INFO_CERT);
    let cert_json = sym_decrypt(&k_cert, cert_ct)
        .map_err(|_| "Zarf açılamadı — yanlış alıcı anahtarı veya bozuk veri".to_string())?;
    let cert: SenderCertificate =
        serde_json::from_slice(&cert_json).map_err(|e| format!("Sertifika parse hatası: {e}"))?;
    cert.verify(now)?;

    // ── Statik katman: yükü çöz (gönderen kimliğini doğrular) ──────────────
    let sender_arr = to_arr32(&cert.identity_dh_pub, "Gönderen kimlik anahtarı")?;
    let sender_pub = X25519Public::from(sender_arr);
    let dh_static = recipient.dh_secret().diffie_hellman(&sender_pub);

    let mut ikm = Vec::with_capacity(64);
    ikm.extend_from_slice(dh_eph.as_bytes());
    ikm.extend_from_slice(dh_static.as_bytes());
    let k_msg = derive_key(&ikm, &eph_pub_bytes, &recip_arr, INFO_MSG);
    ikm.zeroize();

    let payload = sym_decrypt(&k_msg, msg_ct)
        .map_err(|_| "Zarf yükü çözülemedi — gönderen kimliği doğrulanamadı".to_string())?;

    Ok(UnsealedMessage {
        sender_did: cert.did,
        sender_identity_dh_pub: cert.identity_dh_pub,
        sender_signing_pub: cert.signing_pub,
        sender_cert_signature: cert.signature,
        sender_cert_expires_at: cert.expires_at,
        payload,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    /// `haystack` içinde `needle` alt dizisi geçiyor mu?
    fn contains_subslice(haystack: &[u8], needle: &[u8]) -> bool {
        if needle.is_empty() || haystack.len() < needle.len() {
            return false;
        }
        haystack.windows(needle.len()).any(|w| w == needle)
    }

    #[test]
    fn seal_with_ephemeral_matches_seal_behavior_for_fixed_key() {
        // seal()'in refactor edilmiş hâli davranışı bozmamalı: sabit efemeral
        // anahtar verilince (a) efemeral pub bu anahtardan deterministik türer,
        // (b) unseal ile gönderen kimliği + yük yine kurtarılır.
        let sender = IdentityKeyPair::generate();
        let recipient = IdentityKeyPair::generate();
        let payload = b"deterministic ephemeral test";
        let eph_secret_bytes = [7u8; 32];
        let expected_eph_pub = X25519Public::from(&StaticSecret::from(eph_secret_bytes));

        let envelope =
            seal_with_ephemeral(&sender, &recipient.dh_public, payload, 0, eph_secret_bytes).unwrap();

        assert_eq!(
            &envelope[1..33],
            expected_eph_pub.as_bytes(),
            "zarftaki efemeral pub, verilen eph_secret_bytes'tan türeyeni birebir yansıtmalı"
        );

        let opened = unseal(&recipient, &envelope, 1_800_000_000).unwrap();
        assert_eq!(opened.sender_did, sender.did());
        assert_eq!(opened.payload, payload);
    }

    #[test]
    fn seal_unseal_roundtrip_recovers_sender_identity() {
        // Arrange
        let sender = IdentityKeyPair::generate();
        let recipient = IdentityKeyPair::generate();
        let payload = b"ratchet-encrypted message bytes";

        // Act
        let envelope = seal(&sender, &recipient.dh_public, payload, 0).unwrap();
        let opened = unseal(&recipient, &envelope, 1_800_000_000).unwrap();

        // Assert — alıcı gönderen kimliğini ve yükü kurtarır
        assert_eq!(opened.sender_did, sender.did());
        assert_eq!(opened.sender_identity_dh_pub, sender.dh_public);
        assert_eq!(opened.sender_signing_pub, sender.signing_public);
        assert_eq!(opened.payload, payload);

        // Adım 8: dışa verilen ham imza + expires_at, moderasyonun bağımsızca
        // yeniden hesaplayacağı signing_bytes üzerinde GERÇEKTEN doğrulanabilir
        // olmalı (yalnızca "boş değil" değil — tam olarak cert.verify()'ın
        // kabul ettiği imza).
        assert!(!opened.sender_cert_signature.is_empty());
        let signing_bytes = SenderCertificate::signing_bytes(
            &opened.sender_did,
            &opened.sender_identity_dh_pub,
            &opened.sender_signing_pub,
            opened.sender_cert_expires_at,
        );
        assert!(IdentityKeyPair::verify_signature(
            &opened.sender_signing_pub,
            &signing_bytes,
            &opened.sender_cert_signature,
        ));

        // Sahte imza reddedilmeli (Go tarafının da yapacağı kontrolün Rust
        // referansı).
        let mut forged = opened.sender_cert_signature.clone();
        forged[0] ^= 0xFF;
        assert!(!IdentityKeyPair::verify_signature(
            &opened.sender_signing_pub,
            &signing_bytes,
            &forged,
        ));
    }

    #[test]
    fn envelope_never_contains_sender_identity_in_clear() {
        let sender = IdentityKeyPair::generate();
        let recipient = IdentityKeyPair::generate();
        let payload = b"gizli mesaj";

        let envelope = seal(&sender, &recipient.dh_public, payload, 0).unwrap();

        // Sunucunun gördüğü tek şey zarf bytes'ları — gönderen kimliğine dair
        // hiçbir açık iz içermemeli.
        assert!(
            !contains_subslice(&envelope, &sender.dh_public),
            "Zarf gönderenin X25519 kimlik anahtarını açık içeriyor"
        );
        assert!(
            !contains_subslice(&envelope, &sender.signing_public),
            "Zarf gönderenin Ed25519 anahtarını açık içeriyor"
        );
        assert!(
            !contains_subslice(&envelope, sender.did().as_bytes()),
            "Zarf gönderenin DID'ini açık içeriyor"
        );
        assert!(
            !contains_subslice(&envelope, payload),
            "Zarf yükü açık içeriyor"
        );
    }

    #[test]
    fn two_seals_to_same_recipient_are_unlinkable_headers() {
        // Aynı gönderen + alıcı için iki zarfın açık kısmı (efemeral pub)
        // her seferinde farklı olmalı — sunucu zarfları ilişkilendirememeli.
        let sender = IdentityKeyPair::generate();
        let recipient = IdentityKeyPair::generate();

        let e1 = seal(&sender, &recipient.dh_public, b"a", 0).unwrap();
        let e2 = seal(&sender, &recipient.dh_public, b"a", 0).unwrap();
        assert_ne!(&e1[1..33], &e2[1..33], "Efemeral anahtar tekrar kullanılmış");
    }

    #[test]
    fn wrong_recipient_key_fails_to_unseal() {
        let sender = IdentityKeyPair::generate();
        let recipient = IdentityKeyPair::generate();
        let eve = IdentityKeyPair::generate();

        let envelope = seal(&sender, &recipient.dh_public, b"payload", 0).unwrap();

        // Eve (yanlış alıcı) zarfı açamaz ve gönderen kimliğini ÖĞRENEMEZ
        let result = unseal(&eve, &envelope, 0);
        assert!(result.is_err(), "Yanlış alıcı zarfı açabilmemeli");
    }

    #[test]
    fn tampered_envelope_fails() {
        let sender = IdentityKeyPair::generate();
        let recipient = IdentityKeyPair::generate();

        let envelope = seal(&sender, &recipient.dh_public, b"payload", 0).unwrap();

        // Sertifika bloğunda bir byte boz
        let mut t1 = envelope.clone();
        t1[ENVELOPE_HEADER_LEN + 5] ^= 0xFF;
        assert!(unseal(&recipient, &t1, 0).is_err());

        // Mesaj bloğunda (son byte) boz
        let mut t2 = envelope.clone();
        let last = t2.len() - 1;
        t2[last] ^= 0xFF;
        assert!(unseal(&recipient, &t2, 0).is_err());

        // Versiyon boz
        let mut t3 = envelope;
        t3[0] = 99;
        assert!(unseal(&recipient, &t3, 0).is_err());
    }

    #[test]
    fn truncated_and_malformed_envelopes_rejected() {
        let sender = IdentityKeyPair::generate();
        let recipient = IdentityKeyPair::generate();
        let envelope = seal(&sender, &recipient.dh_public, b"payload", 0).unwrap();

        // Boş / çok kısa
        assert!(unseal(&recipient, &[], 0).is_err());
        assert!(unseal(&recipient, &envelope[..ENVELOPE_HEADER_LEN], 0).is_err());

        // cert_len alanı sahte büyük değere ayarlanmış (out-of-bounds okumaya zorlar)
        let mut evil = envelope.clone();
        evil[33..37].copy_from_slice(&u32::MAX.to_be_bytes());
        assert!(unseal(&recipient, &evil, 0).is_err());

        // cert_len zarfın gerçek boyutunu aşıyor ama MAX sınırı altında
        let mut evil2 = envelope;
        evil2[33..37].copy_from_slice(&((MAX_CERT_CT_LEN as u32) - 1).to_be_bytes());
        assert!(unseal(&recipient, &evil2, 0).is_err());
    }

    #[test]
    fn expired_certificate_rejected() {
        let sender = IdentityKeyPair::generate();
        let recipient = IdentityKeyPair::generate();

        let expires_at = 1_000_000u64;
        let envelope = seal(&sender, &recipient.dh_public, b"payload", expires_at).unwrap();

        // Süre dolmadan geçerli
        assert!(unseal(&recipient, &envelope, expires_at - 1).is_ok());
        // Süre dolduktan sonra reddedilir
        assert!(unseal(&recipient, &envelope, expires_at + 1).is_err());
    }

    #[test]
    fn forged_certificate_did_mismatch_rejected() {
        // İmzası geçerli ama DID'i başka bir anahtara ait sertifika reddedilmeli
        let sender = IdentityKeyPair::generate();
        let mut cert = SenderCertificate::issue(&sender, 0);
        cert.did = "did:obs:00000000000000000000000000000000".into();
        // İmza artık uyuşmaz → verify başarısız
        assert!(cert.verify(0).is_err());
    }
}
