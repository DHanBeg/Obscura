//! AES-256-GCM simetrik şifreleme
use aes_gcm::{
    Aes256Gcm, Key, Nonce,
    aead::{Aead, KeyInit},
};
use rand::{rngs::OsRng, RngCore};
use zeroize::Zeroize;

pub const KEY_SIZE: usize = 32;
pub const NONCE_SIZE: usize = 12;
pub const TAG_SIZE: usize = 16;

/// 256-bit AES-GCM anahtarı — Drop'ta sıfırlanır
#[derive(Clone)]
pub struct SymKey(pub [u8; KEY_SIZE]);

impl Drop for SymKey {
    fn drop(&mut self) { self.0.zeroize(); }
}

impl SymKey {
    pub fn random() -> Self {
        let mut key = [0u8; KEY_SIZE];
        OsRng.fill_bytes(&mut key);
        Self(key)
    }

    pub fn from_bytes(bytes: &[u8]) -> Option<Self> {
        if bytes.len() < KEY_SIZE { return None; }
        let mut k = [0u8; KEY_SIZE];
        k.copy_from_slice(&bytes[..KEY_SIZE]);
        Some(Self(k))
    }
}

/// Şifrele — çıktı: [12 byte nonce][ciphertext + 16 byte tag]
pub fn encrypt(key: &SymKey, plaintext: &[u8]) -> Result<Vec<u8>, String> {
    let cipher = Aes256Gcm::new(Key::<Aes256Gcm>::from_slice(&key.0));
    let mut nonce_bytes = [0u8; NONCE_SIZE];
    OsRng.fill_bytes(&mut nonce_bytes);
    let nonce = Nonce::from_slice(&nonce_bytes);

    let ciphertext = cipher.encrypt(nonce, plaintext)
        .map_err(|e| format!("Şifreleme hatası: {:?}", e))?;

    let mut out = nonce_bytes.to_vec();
    out.extend_from_slice(&ciphertext);
    Ok(out)
}

/// Şifre çöz — girdi: [12 byte nonce][ciphertext + 16 byte tag]
pub fn decrypt(key: &SymKey, data: &[u8]) -> Result<Vec<u8>, String> {
    if data.len() < NONCE_SIZE + TAG_SIZE {
        return Err("Veri çok kısa".into());
    }
    let cipher = Aes256Gcm::new(Key::<Aes256Gcm>::from_slice(&key.0));
    let nonce = Nonce::from_slice(&data[..NONCE_SIZE]);

    cipher.decrypt(nonce, &data[NONCE_SIZE..])
        .map_err(|_| "Şifre çözme hatası — veri bütünlüğü bozulmuş".into())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn roundtrip() {
        let key = SymKey::random();
        let plain = b"Obscura test mesaji 123";
        let enc = encrypt(&key, plain).unwrap();
        let dec = decrypt(&key, &enc).unwrap();
        assert_eq!(dec, plain);
    }

    #[test]
    fn wrong_key_fails() {
        let key1 = SymKey::random();
        let key2 = SymKey::random();
        let enc = encrypt(&key1, b"gizli mesaj").unwrap();
        assert!(decrypt(&key2, &enc).is_err());
    }

    #[test]
    fn tamper_detection() {
        let key = SymKey::random();
        let mut enc = encrypt(&key, b"test veri").unwrap();
        // Son byte'ı boz
        let last = enc.len() - 1;
        enc[last] ^= 0xFF;
        assert!(decrypt(&key, &enc).is_err());
    }
}
