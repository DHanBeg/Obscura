//! Kyber-768 + MLS hybrid handshake — FAZ 4 / Spec Bölüm 12.4
//!
//! Bu modül, klasik MLS (RFC 9420) handshake'ini CRYSTALS-Kyber-768 KEM ile
//! güçlendirir. Hibrit mod: hem klasik X25519 (MLS içinde) hem de post-quantum
//! Kyber-768 anahtarları birlikte kullanılır. Sonuç epoch_secret = KDF(classical || pq).
//!
//! Tasarım amaçları:
//!   1. "Harvest now, decrypt later" saldırılarına karşı koruma.
//!    Saldırgan bugün şifreli mesajı kaydedip, yarın kuantum bilgisayarla
//!    X25519 anahtarını kırarsa bile, Kyber-768 hâlâ direnç gösterir.
//!   2. Geriye uyumluluk: KyberMLSKeyPackage içindeki `mls_key_package`
//!    standart bir RFC 9420 KeyPackage'tır; PQ ek alanlar görmezden gelinebilir.
//!   3. NIST PQC standardına uyum: Kyber-768, ML-KEM olarak FIPS 203'te
//!    standartlaştı; pqcrypto-kyber crate'i NIST referans submission koduna
//!    bağlıdır (audit'li, custom impl değil — bkz CLAUDE.md "Never roll custom PQ").
//!
//! API:
//!   - `generate_kyber_mls_keypair`  — yeni Kyber-768 anahtar + MLS KeyPackage üret
//!   - `encapsulate_epoch_secret`    — gönderen: ciphertext + shared secret üret
//!   - `decapsulate_epoch_secret`    — alıcı: ciphertext'i çözüp shared secret çıkar
//!   - `derive_hybrid_epoch_secret`  — MLS epoch_secret ile PQ secret'ı birleştir (HKDF)
//!
//! Storage notu (CLAUDE.md "Storage: PQ keys are 1-50KB"):
//!   - Kyber-768 public key  : 1184 bytes
//!   - Kyber-768 secret key  : 2400 bytes
//!   - Kyber-768 ciphertext  : 1088 bytes
//!   - Kyber-768 shared secret: 32 bytes
//! Klasik X25519'un 32 byte'ına kıyasla ~37x daha büyük; KeyPackage rotasyonu
//! 90 günden sık olmasın (spec Bölüm 4.2).

use pqcrypto_kyber::kyber768;
use pqcrypto_traits::kem::{
    Ciphertext as KemCiphertext, PublicKey as KemPublicKey, SecretKey as KemSecretKey,
    SharedSecret as KemSharedSecret,
};

use hkdf::Hkdf;
use sha2::Sha256;

use openmls::prelude::*;
use openmls_traits::OpenMlsProvider;
use tls_codec::Serialize as TlsSerialize;

use super::{generate_key_package, Identity, MlsError, Result};

/// Hybrid (klasik MLS KeyPackage + Kyber-768 PQ public key) anahtar paketi.
///
/// `kyber_ciphertext` sadece encapsulate aşamasından sonra dolar; ham KeyPackage
/// yayınlanırken `None` olur. Alıcı (group admin) bunu doldurup yeni epoch'ta
/// kullanır.
pub struct KyberMLSKeyPackage {
    /// Kyber-768 public key (1184 bytes serialized)
    pub kyber_public: kyber768::PublicKey,
    /// Encapsulate sonucu doluyor; KeyPackage paylaşılırken None
    pub kyber_ciphertext: Option<kyber768::Ciphertext>,
    /// Serialized MLS KeyPackage (RFC 9420 / TLS presentation language)
    pub mls_key_package: Vec<u8>,
}

impl KyberMLSKeyPackage {
    /// Kyber public key'in serialized halini al (1184 bytes).
    pub fn kyber_public_bytes(&self) -> Vec<u8> {
        self.kyber_public.as_bytes().to_vec()
    }

    /// Eğer encapsulate edilmişse, ciphertext'i serialize et (1088 bytes).
    pub fn kyber_ciphertext_bytes(&self) -> Option<Vec<u8>> {
        self.kyber_ciphertext.as_ref().map(|c| c.as_bytes().to_vec())
    }
}

/// Yeni bir Kyber-768 + MLS hibrit anahtar çifti üret.
///
/// Dönen tuple:
///   - `KyberMLSKeyPackage`: paylaşılabilir public bundle
///   - `kyber768::SecretKey`: SADECE sahip cihazında saklanır (asla yayınlanmaz)
///
/// MLS Identity için zaten Ed25519 imzaları openmls içinde üretiliyor; bu fonksiyon
/// bunun üstüne PQ KEM katmanı ekler.
pub fn generate_kyber_mls_keypair(
    identity: &Identity,
    provider: &impl OpenMlsProvider,
) -> Result<(KyberMLSKeyPackage, kyber768::SecretKey)> {
    // 1) MLS KeyPackage üret (klasik tarafı)
    let kp = generate_key_package(identity, provider)?;
    let mls_bytes = kp
        .tls_serialize_detached()
        .map_err(|e| MlsError::Invalid(format!("MLS KeyPackage serialize: {e:?}")))?;

    // 2) Kyber-768 anahtar çifti
    let (kyber_pub, kyber_sk) = kyber768::keypair();

    Ok((
        KyberMLSKeyPackage {
            kyber_public: kyber_pub,
            kyber_ciphertext: None,
            mls_key_package: mls_bytes,
        },
        kyber_sk,
    ))
}

/// Encapsulate: gönderen alıcının public key'iyle ortak gizliyi kapsüller.
///
/// Dönen tuple:
///   - `Vec<u8>`: ciphertext (alıcıya gönderilir, 1088 bytes)
///   - `Vec<u8>`: shared secret (gönderen yerel kullanır, 32 bytes)
///
/// Bu shared secret doğrudan epoch secret değildir — `derive_hybrid_epoch_secret`
/// ile MLS'in kendi epoch secret'ıyla harmanlanmalıdır.
pub fn encapsulate_epoch_secret(public: &KyberMLSKeyPackage) -> Result<(Vec<u8>, Vec<u8>)> {
    let (shared, ct) = kyber768::encapsulate(&public.kyber_public);
    Ok((ct.as_bytes().to_vec(), shared.as_bytes().to_vec()))
}

/// Decapsulate: alıcı kendi secret key'i ile ciphertext'i çözer.
///
/// Dönen `Vec<u8>` = 32 byte shared secret (gönderenin elindekiyle aynıdır).
pub fn decapsulate_epoch_secret(
    ciphertext: &[u8],
    secret: &kyber768::SecretKey,
) -> Result<Vec<u8>> {
    let ct = kyber768::Ciphertext::from_bytes(ciphertext)
        .map_err(|e| MlsError::Invalid(format!("kyber ciphertext parse: {e:?}")))?;
    let shared = kyber768::decapsulate(&ct, secret);
    Ok(shared.as_bytes().to_vec())
}

/// Hybrid epoch secret türetme: MLS'in kendi epoch_secret'ı ile Kyber shared
/// secret'ı HKDF-SHA256 ile birleştirir.
///
/// Resulting secret = HKDF(salt = mls_epoch_secret, ikm = pq_shared || classical_dh)
/// Hibrit security: ya MLS klasik tarafı ya da PQ tarafı KIRILMADIKÇA güvenli.
///
/// `info` parametresi domain separation içindir (örn. b"obscura/mls/epoch/v1").
pub fn derive_hybrid_epoch_secret(
    mls_epoch_secret: &[u8],
    pq_shared_secret: &[u8],
    info: &[u8],
) -> Result<[u8; 32]> {
    let mut ikm = Vec::with_capacity(mls_epoch_secret.len() + pq_shared_secret.len());
    ikm.extend_from_slice(mls_epoch_secret);
    ikm.extend_from_slice(pq_shared_secret);

    // salt: PQ tarafı kompromise olursa MLS hâlâ güvenlik sağlasın diye
    // mls_epoch_secret'i salt olarak da kullanıyoruz (defense in depth).
    let hk = Hkdf::<Sha256>::new(Some(mls_epoch_secret), &ikm);
    let mut okm = [0u8; 32];
    hk.expand(info, &mut okm)
        .map_err(|e| MlsError::Invalid(format!("HKDF expand: {e:?}")))?;
    Ok(okm)
}

/// Yardımcı: KeyPackage'ı yeniden deserialize et (bir node ağdan aldığında).
pub fn parse_mls_key_package(bytes: &[u8]) -> Result<KeyPackage> {
    use tls_codec::Deserialize as TlsDeserialize;
    let mut cursor = bytes;
    let kp_in = KeyPackageIn::tls_deserialize(&mut cursor)
        .map_err(|e| MlsError::Invalid(format!("KeyPackage deserialize: {e:?}")))?;
    // KeyPackageIn → KeyPackage validate sırasında ciphersuite check yapar.
    let kp: KeyPackage = kp_in
        .validate(
            openmls_rust_crypto::OpenMlsRustCrypto::default().crypto(),
            ProtocolVersion::Mls10,
        )
        .map_err(|e| MlsError::Invalid(format!("KeyPackage validate: {e:?}")))?;
    Ok(kp)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::mls::new_provider;

    #[test]
    fn kyber_kem_roundtrip() {
        let provider = new_provider();
        let identity = Identity::generate("did:obs:test-alice", &provider).unwrap();
        let (bundle, sk) = generate_kyber_mls_keypair(&identity, &provider).unwrap();

        // Sender side
        let (ct, sender_ss) = encapsulate_epoch_secret(&bundle).unwrap();
        assert_eq!(sender_ss.len(), 32);
        assert_eq!(ct.len(), 1088, "Kyber-768 ciphertext is 1088 bytes");

        // Receiver side
        let receiver_ss = decapsulate_epoch_secret(&ct, &sk).unwrap();
        assert_eq!(receiver_ss, sender_ss, "KEM shared secrets must match");
    }

    #[test]
    fn hybrid_epoch_secret_is_deterministic() {
        let mls_secret = [1u8; 32];
        let pq_secret = [2u8; 32];
        let info = b"obscura/mls/epoch/v1";

        let s1 = derive_hybrid_epoch_secret(&mls_secret, &pq_secret, info).unwrap();
        let s2 = derive_hybrid_epoch_secret(&mls_secret, &pq_secret, info).unwrap();
        assert_eq!(s1, s2, "HKDF deterministic");

        // Farklı info → farklı secret (domain separation)
        let s3 = derive_hybrid_epoch_secret(&mls_secret, &pq_secret, b"other").unwrap();
        assert_ne!(s1, s3);
    }

    #[test]
    fn kyber_keypackage_sizes_match_spec() {
        let provider = new_provider();
        let identity = Identity::generate("did:obs:test-bob", &provider).unwrap();
        let (bundle, _sk) = generate_kyber_mls_keypair(&identity, &provider).unwrap();

        assert_eq!(bundle.kyber_public_bytes().len(), 1184, "Kyber-768 pubkey = 1184 bytes");
        assert!(bundle.kyber_ciphertext.is_none(), "Fresh bundle has no ciphertext");
        assert!(!bundle.mls_key_package.is_empty(), "MLS KeyPackage serialized");
    }
}
