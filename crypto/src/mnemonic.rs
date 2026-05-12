//! BIP39 mnemonic generation + Obscura identity secret derivation.
//!
//! Spec Bölüm 4.2 + 5.2 Step 7:
//!   - 12-word mnemonic (BIP39, English wordlist)
//!   - Social recovery support (mnemonic split via Shamir later)
//!   - identity_secret = HKDF-SHA256("obscura-id-v1", seed_from_mnemonic)
//!
//! WARNING: The mnemonic is the master backup. Loss = account loss.
//! Never log, never send to server, store only in OS secure enclave (Keychain/Keystore/SecureStore).

use bip39::{Language, Mnemonic};
use hkdf::Hkdf;
use rand::rngs::OsRng;
use sha2::Sha256;
use zeroize::Zeroize;

const HKDF_INFO: &[u8] = b"obscura-id-v1";
const HKDF_OUT_LEN: usize = 32;

#[derive(Debug, thiserror::Error)]
pub enum MnemonicError {
    #[error("invalid mnemonic phrase: {0}")]
    Invalid(String),
    #[error("hkdf expand: {0}")]
    Hkdf(String),
}

/// Generate a fresh 12-word English BIP39 mnemonic.
pub fn generate() -> String {
    let mut rng = OsRng;
    let m = Mnemonic::generate_in_with(&mut rng, Language::English, 12).expect("12 words");
    m.to_string()
}

/// Validate a mnemonic phrase.
pub fn validate(phrase: &str) -> Result<(), MnemonicError> {
    Mnemonic::parse_in(Language::English, phrase.trim())
        .map_err(|e| MnemonicError::Invalid(e.to_string()))?;
    Ok(())
}

/// Derive Obscura's identity_secret from a mnemonic + optional passphrase.
/// The secret is the input to identity_proof.circom (Bölüm 17.1) and DID derivation.
///
/// SECURITY: Callers receiving the returned `[u8; 32]` MUST `.zeroize()` it
/// after use — it is sensitive key material.
pub fn derive_identity_secret(phrase: &str, passphrase: Option<&str>) -> Result<[u8; 32], MnemonicError> {
    let m = Mnemonic::parse_in(Language::English, phrase.trim())
        .map_err(|e| MnemonicError::Invalid(e.to_string()))?;
    let mut seed = m.to_seed(passphrase.unwrap_or(""));

    // SECURITY: ensure `seed` is zeroized on every exit path, including the
    // error branch from `hk.expand`. Wrapping the fallible work in a closure
    // lets us run zeroize unconditionally before returning.
    let result = (|| -> Result<[u8; 32], MnemonicError> {
        let hk = Hkdf::<Sha256>::new(Some(b"obscura-mnemonic-v1"), &seed);
        let mut out = [0u8; HKDF_OUT_LEN];
        hk.expand(HKDF_INFO, &mut out)
            .map_err(|e| MnemonicError::Hkdf(e.to_string()))?;
        Ok(out)
    })();

    seed.zeroize();
    result
}

/// Derive a deterministic ed25519 keypair from mnemonic.
/// Returns (private_key_bytes, public_key_bytes).
///
/// SECURITY: The returned private key bytes are sensitive — callers MUST
/// `.zeroize()` them after use.
pub fn derive_ed25519(phrase: &str, passphrase: Option<&str>) -> Result<([u8; 32], [u8; 32]), MnemonicError> {
    use ed25519_dalek::{SigningKey, VerifyingKey};

    let mut secret = derive_identity_secret(phrase, passphrase)?;
    let signing = SigningKey::from_bytes(&secret);
    let verifying: VerifyingKey = signing.verifying_key();
    let priv_bytes = signing.to_bytes();
    let pub_bytes = verifying.to_bytes();
    // SECURITY: zeroize the intermediate secret; the SigningKey copy goes out
    // of scope here. Caller still owns `priv_bytes` and must zeroize it.
    secret.zeroize();
    Ok((priv_bytes, pub_bytes))
}

/// Compute Obscura DID from mnemonic.
/// did:obs:hex(sha256("obscura-did-v1" || ed25519_pub))
pub fn derive_did(phrase: &str, passphrase: Option<&str>) -> Result<String, MnemonicError> {
    use sha2::{Digest, Sha256};

    let (_, pub_bytes) = derive_ed25519(phrase, passphrase)?;
    let mut h = Sha256::new();
    h.update(b"obscura-did-v1");
    h.update(pub_bytes);
    let digest = h.finalize();
    Ok(format!("did:obs:{}", hex::encode(digest)))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn generate_validates() {
        let m = generate();
        let words: Vec<&str> = m.split_whitespace().collect();
        assert_eq!(words.len(), 12);
        validate(&m).unwrap();
    }

    #[test]
    fn invalid_mnemonic_rejected() {
        assert!(validate("not a real mnemonic phrase").is_err());
        assert!(validate("abandon abandon abandon").is_err()); // wrong length
    }

    #[test]
    fn deterministic_derivation() {
        let m = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about";
        let s1 = derive_identity_secret(m, None).unwrap();
        let s2 = derive_identity_secret(m, None).unwrap();
        assert_eq!(s1, s2);

        let s3 = derive_identity_secret(m, Some("alice")).unwrap();
        assert_ne!(s1, s3); // passphrase changes output
    }

    #[test]
    fn deterministic_did() {
        let m = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about";
        let did1 = derive_did(m, None).unwrap();
        let did2 = derive_did(m, None).unwrap();
        assert_eq!(did1, did2);
        assert!(did1.starts_with("did:obs:"));
        assert_eq!(did1.len(), 8 + 64); // "did:obs:" + 64 hex chars
    }

    #[test]
    fn derive_ed25519_consistency() {
        let m = generate();
        let (priv1, pub1) = derive_ed25519(&m, None).unwrap();
        let (priv2, pub2) = derive_ed25519(&m, None).unwrap();
        assert_eq!(priv1, priv2);
        assert_eq!(pub1, pub2);
    }
}
