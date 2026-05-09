//! Kimlik anahtarları — Ed25519 imzalama + X25519 anahtar değişimi
use ed25519_dalek::{SigningKey, VerifyingKey, Signature, Signer, Verifier};
use x25519_dalek::{StaticSecret, PublicKey as X25519Public};
use rand::rngs::OsRng;
use serde::{Serialize, Deserialize};
use base64::{Engine, engine::general_purpose::STANDARD as B64};
use sha2::{Sha256, Digest};
use zeroize::Zeroize;

/// Kimlik anahtar çifti (her kullanıcı için bir kez üretilir, cihazda tutulur)
#[derive(Serialize, Deserialize)]
pub struct IdentityKeyPair {
    /// Ed25519 imzalama anahtarı (gizli, ASLA sunucuya gönderilmez)
    signing_private: Vec<u8>,
    /// Ed25519 doğrulama anahtarı (herkese açık)
    pub signing_public: Vec<u8>,
    /// X25519 DH anahtarı (gizli)
    dh_private: Vec<u8>,
    /// X25519 DH açık anahtarı
    pub dh_public: Vec<u8>,
}

impl Drop for IdentityKeyPair {
    fn drop(&mut self) {
        self.signing_private.zeroize();
        self.dh_private.zeroize();
    }
}

impl IdentityKeyPair {
    /// Yeni kimlik anahtarı üret — OsRng kullanır (sistem entropi kaynağı)
    pub fn generate() -> Self {
        let signing_key = SigningKey::generate(&mut OsRng);
        let dh_secret = StaticSecret::random_from_rng(OsRng);
        let dh_public = X25519Public::from(&dh_secret);

        Self {
            signing_public: signing_key.verifying_key().as_bytes().to_vec(),
            signing_private: signing_key.to_bytes().to_vec(),
            dh_public: dh_public.as_bytes().to_vec(),
            dh_private: dh_secret.to_bytes().to_vec(),
        }
    }

    /// DID üret: did:obs:<SHA256(dh_public)[:16]>
    pub fn did(&self) -> String {
        let mut hasher = Sha256::new();
        hasher.update(&self.dh_public);
        let hash = hasher.finalize();
        format!("did:obs:{}", hex::encode(&hash[..16]))
    }

    /// Mesajı imzala
    pub fn sign(&self, message: &[u8]) -> Vec<u8> {
        let key_bytes: [u8; 32] = self.signing_private[..32].try_into().unwrap();
        let signing_key = SigningKey::from_bytes(&key_bytes);
        signing_key.sign(message).to_bytes().to_vec()
    }

    /// Açık Ed25519 anahtarı ile imzayı doğrula
    pub fn verify_signature(public_key: &[u8], message: &[u8], signature: &[u8]) -> bool {
        let pk_bytes: [u8; 32] = match public_key.try_into() {
            Ok(b) => b,
            Err(_) => return false,
        };
        let sig_bytes: [u8; 64] = match signature.try_into() {
            Ok(b) => b,
            Err(_) => return false,
        };
        let vk = match VerifyingKey::from_bytes(&pk_bytes) {
            Ok(k) => k,
            Err(_) => return false,
        };
        let sig = Signature::from_bytes(&sig_bytes);
        vk.verify(message, &sig).is_ok()
    }

    /// DH private key → StaticSecret
    pub fn dh_secret(&self) -> StaticSecret {
        let bytes: [u8; 32] = self.dh_private[..32].try_into().unwrap();
        StaticSecret::from(bytes)
    }

    /// DH private key bytes (32 byte) — X3DH için
    pub fn dh_private_bytes(&self) -> Vec<u8> {
        self.dh_private.clone()
    }

    /// Base64 encode for transport
    pub fn public_key_b64(&self) -> String {
        B64.encode(&self.dh_public)
    }

    pub fn signing_public_b64(&self) -> String {
        B64.encode(&self.signing_public)
    }

    /// JSON serialize (sadece açık anahtarlar için güvenli)
    pub fn to_json(&self) -> String {
        serde_json::json!({
            "dh_public": B64.encode(&self.dh_public),
            "signing_public": B64.encode(&self.signing_public),
            "did": self.did(),
        }).to_string()
    }

    /// Tam serialize (cihaz depolama — SADECE şifreli depola)
    pub fn to_secure_json(&self) -> String {
        serde_json::to_string(self).unwrap_or_default()
    }

    pub fn from_secure_json(json: &str) -> Option<Self> {
        serde_json::from_str(json).ok()
    }
}
