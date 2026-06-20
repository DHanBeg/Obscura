//! SRTP key derivation — WebRTC medya şifrelemesi için (spec Bölüm 5.3)
//!
//! HKDF-SHA256 ile shared_secret → SRTP master key + salt türetimi.
//! Caller (Go backend) bu key'leri coturn'a SRTP context olarak verir.

use hkdf::Hkdf;
use sha2::Sha256;
use serde::{Serialize, Deserialize};

/// RFC 3711 SRTP master key + salt.
#[derive(Debug, Clone, Serialize, Deserialize, zeroize::Zeroize, zeroize::ZeroizeOnDrop)]
pub struct SRTPKeys {
    /// AES-128 master key (16 bytes, base64 encoded for JSON transport)
    pub master_key: Vec<u8>,
    /// SRTP master salt (14 bytes)
    pub master_salt: Vec<u8>,
}

/// Türetilen SRTP key'lerini JSON-serileştirilebilir struct olarak döndür.
pub fn derive_srtp_keys(shared_secret: &[u8; 32], call_id: &str) -> SRTPKeys {
    let info = format!("obscura-srtp-v1:{call_id}");
    let hk = Hkdf::<Sha256>::new(None, shared_secret);

    // 30 byte OKM: ilk 16 = master key, son 14 = master salt
    let mut okm = [0u8; 30];
    hk.expand(info.as_bytes(), &mut okm).expect("HKDF expand (SRTP)");

    SRTPKeys {
        master_key: okm[..16].to_vec(),
        master_salt: okm[16..].to_vec(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn srtp_keys_correct_lengths() {
        let secret = [0x42u8; 32];
        let keys = derive_srtp_keys(&secret, "test-call-id-abc");
        assert_eq!(keys.master_key.len(), 16);
        assert_eq!(keys.master_salt.len(), 14);
    }

    #[test]
    fn srtp_keys_deterministic() {
        let secret = [0xDEu8; 32];
        let k1 = derive_srtp_keys(&secret, "call-1");
        let k2 = derive_srtp_keys(&secret, "call-1");
        assert_eq!(k1.master_key, k2.master_key);
        assert_eq!(k1.master_salt, k2.master_salt);
    }

    #[test]
    fn srtp_keys_differ_by_call_id() {
        let secret = [0xABu8; 32];
        let k1 = derive_srtp_keys(&secret, "call-a");
        let k2 = derive_srtp_keys(&secret, "call-b");
        assert_ne!(k1.master_key, k2.master_key);
    }
}
