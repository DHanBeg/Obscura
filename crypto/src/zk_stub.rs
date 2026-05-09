//! ZK Proof stubs — Circom/Groth16 entegrasyonuna kadar yer tutucu
//!
//! Gerçek devre:
//!   - identity_proof.circom  → Groth16 (kimlik doğrulama)
//!   - credit_threshold.circom → Groth16 (kredi eşiği kanıtı)
//!   - token_balance.circom   → Groth16 (bakiye gizlilik kanıtı)
//!   - vote_validity.circom   → Groth16 (oy geçerlilik kanıtı)
//!
//! Şu an: HMAC-SHA256 tabanlı commitment + "kanıt" = stub
//!        Gerçek ZK'ya geçiş için sadece bu modülü değiştirmek yeterli.

use hmac::{Hmac, Mac};
use sha2::{Sha256, Digest};
use serde::{Serialize, Deserialize};
use zeroize::Zeroize;

type HmacSha256 = Hmac<Sha256>;

// ─────────────────────────────────────────────────────────────────────────────
// Proof Types
// ─────────────────────────────────────────────────────────────────────────────

/// ZK kanıt türleri
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum ProofType {
    /// Kimlik doğrulama — DID sahibini kanıtla, private key açıklamadan
    Identity,
    /// Kredi eşiği — belirli bir eşiğin üzerinde olduğunu kanıtla
    CreditThreshold,
    /// Token bakiye — belirli bir bakiyeye sahip olduğunu kanıtla
    TokenBalance,
    /// Oy geçerliliği — geçerli bir oy kullandığını kanıtla
    VoteValidity,
}

/// ZK kanıt (stub: Groth16 proof bytes yerine commitment hash)
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ZkProof {
    /// Kanıt türü
    pub proof_type: ProofType,
    /// Kanıt bytes (stub: HMAC-SHA256 commitment, 32 byte)
    /// Gerçek: Groth16 serialized proof (~192 byte)
    pub proof_bytes: Vec<u8>,
    /// Public inputs (devre bağımlı)
    pub public_inputs: Vec<String>,
    /// Commitment (stub'da hash, gerçekte Pedersen commitment)
    pub commitment: Vec<u8>,
    /// Zaman damgası (Unix, saniye)
    pub timestamp: u64,
    /// Kanıtı üreten DID
    pub prover_did: String,
}

/// Identity kanıt witness (gizli)
pub struct IdentityWitness {
    /// Private key bytes (32 byte)
    pub private_key: Vec<u8>,
    /// DID string
    pub did: String,
    /// Nonce (replay saldırısı önleme)
    pub nonce: [u8; 32],
}

impl Drop for IdentityWitness {
    fn drop(&mut self) { self.private_key.zeroize(); }
}

/// Kredi eşiği witness (gizli)
pub struct CreditWitness {
    /// Gerçek kredi puanı (−20..100)
    pub actual_score: i32,
    /// Eşik değer (public input)
    pub threshold: i32,
    /// Gizli tuz
    pub salt: [u8; 32],
}

impl Drop for CreditWitness {
    fn drop(&mut self) { self.salt.zeroize(); }
}

/// Token bakiye witness (gizli)
pub struct TokenWitness {
    /// Gerçek bakiye (mikro-OBS)
    pub balance: u64,
    /// Minimum bakiye (public input)
    pub minimum: u64,
    /// Gizli tuz
    pub salt: [u8; 32],
}

impl Drop for TokenWitness {
    fn drop(&mut self) { self.salt.zeroize(); }
}

// ─────────────────────────────────────────────────────────────────────────────
// Kanıt Üretimi
// ─────────────────────────────────────────────────────────────────────────────

/// Kimlik ZK kanıtı üret
///
/// Public: DID
/// Private: private_key
/// Kanıt: DID, private_key'den türetilebilir → HMAC commitment
pub fn prove_identity(witness: &IdentityWitness, timestamp: u64) -> Result<ZkProof, String> {
    // Commitment: HMAC-SHA256(key=private_key, data=DID||nonce||timestamp)
    let mut mac = HmacSha256::new_from_slice(&witness.private_key)
        .map_err(|_| "HMAC key hatası")?;
    mac.update(witness.did.as_bytes());
    mac.update(&witness.nonce);
    mac.update(&timestamp.to_be_bytes());
    let commitment = mac.finalize().into_bytes().to_vec();

    // Stub kanıt: commitment'ın SHA256'sı (Groth16 proof yerine)
    let mut hasher = Sha256::new();
    hasher.update(b"OBSCURA_IDENTITY_PROOF_V1");
    hasher.update(&commitment);
    hasher.update(witness.did.as_bytes());
    let proof_bytes = hasher.finalize().to_vec();

    Ok(ZkProof {
        proof_type: ProofType::Identity,
        proof_bytes,
        public_inputs: vec![witness.did.clone()],
        commitment,
        timestamp,
        prover_did: witness.did.clone(),
    })
}

/// Kredi eşiği ZK kanıtı üret
///
/// Public: threshold, commitment
/// Private: actual_score, salt
/// Kanıt: actual_score ≥ threshold
pub fn prove_credit_threshold(
    witness: &CreditWitness,
    prover_did: &str,
    timestamp: u64,
) -> Result<ZkProof, String> {
    if witness.actual_score < witness.threshold {
        return Err(format!(
            "Kanıt üretilemez: {} < {} eşiği",
            witness.actual_score, witness.threshold
        ));
    }

    // Commitment: HMAC-SHA256(key=salt, data=score_bytes||threshold_bytes)
    let mut mac = HmacSha256::new_from_slice(&witness.salt)
        .map_err(|_| "HMAC key hatası")?;
    mac.update(&witness.actual_score.to_be_bytes());
    mac.update(&witness.threshold.to_be_bytes());
    let commitment = mac.finalize().into_bytes().to_vec();

    // Stub kanıt
    let mut hasher = Sha256::new();
    hasher.update(b"OBSCURA_CREDIT_PROOF_V1");
    hasher.update(&commitment);
    hasher.update(witness.threshold.to_string().as_bytes());
    let proof_bytes = hasher.finalize().to_vec();

    Ok(ZkProof {
        proof_type: ProofType::CreditThreshold,
        proof_bytes,
        public_inputs: vec![
            witness.threshold.to_string(),
            format!("score_gte_{}", witness.threshold),
        ],
        commitment,
        timestamp,
        prover_did: prover_did.to_string(),
    })
}

/// Token bakiye ZK kanıtı üret
///
/// Public: minimum, commitment
/// Private: balance, salt
pub fn prove_token_balance(
    witness: &TokenWitness,
    prover_did: &str,
    timestamp: u64,
) -> Result<ZkProof, String> {
    if witness.balance < witness.minimum {
        return Err(format!(
            "Kanıt üretilemez: {} < {} minimum",
            witness.balance, witness.minimum
        ));
    }

    let mut mac = HmacSha256::new_from_slice(&witness.salt)
        .map_err(|_| "HMAC key hatası")?;
    mac.update(&witness.balance.to_be_bytes());
    mac.update(&witness.minimum.to_be_bytes());
    let commitment = mac.finalize().into_bytes().to_vec();

    let mut hasher = Sha256::new();
    hasher.update(b"OBSCURA_TOKEN_PROOF_V1");
    hasher.update(&commitment);
    hasher.update(witness.minimum.to_string().as_bytes());
    let proof_bytes = hasher.finalize().to_vec();

    Ok(ZkProof {
        proof_type: ProofType::TokenBalance,
        proof_bytes,
        public_inputs: vec![witness.minimum.to_string()],
        commitment,
        timestamp,
        prover_did: prover_did.to_string(),
    })
}

// ─────────────────────────────────────────────────────────────────────────────
// Kanıt Doğrulama
// ─────────────────────────────────────────────────────────────────────────────

/// ZK kanıtını doğrula
///
/// Stub: kanıtın commitment ile tutarlı olduğunu kontrol eder.
/// Gerçek: bellman/ark-groth16 verify_proof() çağırır.
pub fn verify_proof(proof: &ZkProof) -> Result<bool, String> {
    // Zamanlama kontrolü: 5 dakikadan eski kanıtları reddet
    // (gerçek implementasyonda on-chain replay prevention var)
    // Stub: sadece format kontrolü yap

    if proof.proof_bytes.len() != 32 {
        return Ok(false);
    }
    if proof.commitment.len() != 32 {
        return Ok(false);
    }
    if proof.prover_did.is_empty() {
        return Ok(false);
    }
    if proof.public_inputs.is_empty() {
        return Ok(false);
    }

    // Stub doğrulama: proof_bytes, commitment'ı içermeli
    // Gerçek: Groth16 pairing check
    let mut hasher = Sha256::new();
    let type_tag = match proof.proof_type {
        ProofType::Identity       => b"OBSCURA_IDENTITY_PROOF_V1".as_slice(),
        ProofType::CreditThreshold => b"OBSCURA_CREDIT_PROOF_V1".as_slice(),
        ProofType::TokenBalance   => b"OBSCURA_TOKEN_PROOF_V1".as_slice(),
        ProofType::VoteValidity   => b"OBSCURA_VOTE_PROOF_V1".as_slice(),
    };
    hasher.update(type_tag);
    hasher.update(&proof.commitment);
    if let Some(first_input) = proof.public_inputs.first() {
        hasher.update(first_input.as_bytes());
    }
    let expected = hasher.finalize().to_vec();

    Ok(expected == proof.proof_bytes)
}

/// Kimlik kanıtını doğrula ve DID'i çıkar
pub fn verify_identity_proof(proof: &ZkProof) -> Result<String, String> {
    if proof.proof_type != ProofType::Identity {
        return Err("Yanlış kanıt türü".into());
    }
    if !verify_proof(proof)? {
        return Err("Kimlik kanıtı geçersiz".into());
    }
    Ok(proof.prover_did.clone())
}

/// Kredi eşiği kanıtını doğrula
///
/// Returns: threshold değeri (public) — puanın bu eşiğin üzerinde olduğu garanti
pub fn verify_credit_proof(proof: &ZkProof) -> Result<i32, String> {
    if proof.proof_type != ProofType::CreditThreshold {
        return Err("Yanlış kanıt türü".into());
    }
    if !verify_proof(proof)? {
        return Err("Kredi kanıtı geçersiz".into());
    }
    let threshold: i32 = proof.public_inputs
        .first()
        .ok_or("Eşik değeri eksik")?
        .parse()
        .map_err(|_| "Eşik değeri parse hatası")?;
    Ok(threshold)
}

// ─────────────────────────────────────────────────────────────────────────────
// JSON serializasyonu
// ─────────────────────────────────────────────────────────────────────────────

impl ZkProof {
    pub fn to_json(&self) -> String {
        serde_json::to_string(self).unwrap_or_default()
    }

    pub fn from_json(s: &str) -> Result<Self, String> {
        serde_json::from_str(s).map_err(|e| e.to_string())
    }
}

// ─────────────────────────────────────────────────────────────────────────────
// Testler
// ─────────────────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;

    fn nonce() -> [u8; 32] { [0x11u8; 32] }
    fn salt() -> [u8; 32] { [0x22u8; 32] }

    #[test]
    fn identity_proof_roundtrip() {
        let witness = IdentityWitness {
            private_key: vec![0xAAu8; 32],
            did: "did:obs:abc123".to_string(),
            nonce: nonce(),
        };
        let proof = prove_identity(&witness, 1700000000).unwrap();
        let valid = verify_proof(&proof).unwrap();
        assert!(valid, "Kimlik kanıtı geçersiz");
    }

    #[test]
    fn credit_proof_valid() {
        let witness = CreditWitness {
            actual_score: 75,
            threshold: 50,
            salt: salt(),
        };
        let proof = prove_credit_threshold(&witness, "did:obs:xyz", 1700000000).unwrap();
        let valid = verify_proof(&proof).unwrap();
        assert!(valid, "Kredi kanıtı geçersiz");

        let t = verify_credit_proof(&proof).unwrap();
        assert_eq!(t, 50);
    }

    #[test]
    fn credit_proof_fail_below_threshold() {
        let witness = CreditWitness {
            actual_score: 30,
            threshold: 50,
            salt: salt(),
        };
        let result = prove_credit_threshold(&witness, "did:obs:xyz", 1700000000);
        assert!(result.is_err(), "Eşik altı kanıt üretilmemeli");
    }

    #[test]
    fn token_proof_roundtrip() {
        let witness = TokenWitness {
            balance: 1000,
            minimum: 500,
            salt: salt(),
        };
        let proof = prove_token_balance(&witness, "did:obs:abc", 1700000000).unwrap();
        assert!(verify_proof(&proof).unwrap());
    }

    #[test]
    fn proof_json_roundtrip() {
        let witness = IdentityWitness {
            private_key: vec![0xBBu8; 32],
            did: "did:obs:test123".to_string(),
            nonce: nonce(),
        };
        let proof = prove_identity(&witness, 1700000000).unwrap();
        let json = proof.to_json();
        let proof2 = ZkProof::from_json(&json).unwrap();
        assert_eq!(proof2.prover_did, "did:obs:test123");
        assert!(verify_proof(&proof2).unwrap());
    }
}
