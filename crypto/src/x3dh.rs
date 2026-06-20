//! X3DH — Extended Triple Diffie-Hellman (Signal Protocol RFC)
//! https://signal.org/docs/specifications/x3dh/
//!
//! Alice = Mesaj gönderen, Bob = Mesaj alan
//!
//! DH1 = DH(IK_A, SPK_B)   — Alice kimlik × Bob imzalı PreKey
//! DH2 = DH(EK_A, IK_B)   — Alice efemeral × Bob kimlik
//! DH3 = DH(EK_A, SPK_B)  — Alice efemeral × Bob imzalı PreKey
//! DH4 = DH(EK_A, OPK_B)  — Alice efemeral × Bob tek kullanımlık PreKey (opsiyonel)
//!
//! ÖNEMLI: EK_A tüm DH'lerde AYNI ephemeral key — StaticSecret kullanılır (reusable).

use x25519_dalek::{StaticSecret, PublicKey as X25519Public};
use hkdf::Hkdf;
use sha2::Sha256;
use rand::rngs::OsRng;
use serde::{Serialize, Deserialize};
use zeroize::Zeroize;

use crate::symmetric::SymKey;

/// Bob'un PreKey bundle — sunucuda saklanır, Alice indirir
#[derive(Serialize, Deserialize, Clone, Debug)]
pub struct PreKeyBundle {
    /// Bob'un kimlik açık anahtarı (X25519) — 32 byte
    pub identity_key: Vec<u8>,
    /// İmzalı PreKey açık anahtarı (X25519) — 32 byte
    pub signed_prekey: Vec<u8>,
    /// İmzalı PreKey imzası (Ed25519, identity signing key ile) — 64 byte
    pub signed_prekey_sig: Vec<u8>,
    /// Tek kullanımlık PreKey açık anahtarı (opsiyonel, tükenirse None)
    pub one_time_prekey: Option<Vec<u8>>,
    /// Kullanılan OPK'nın ID'si
    pub one_time_prekey_id: Option<u32>,
    /// Kullanıcının DID'i
    pub did: String,
}

/// X3DH başlatma sonucu — gönderen (Alice) tarafından üretilir
pub struct X3DHInitResult {
    /// Türetilmiş paylaşılan simetrik anahtar (ek bilgi gönderme şifrelemesi için)
    pub shared_key: SymKey,
    /// Alice'in efemeral açık anahtarı — Bob'a gönderilmeli
    pub ephemeral_public: Vec<u8>,
    /// Kullanılan OPK ID (varsa)
    pub one_time_prekey_id: Option<u32>,
}

/// X3DH kabul sonucu — alan (Bob) tarafından üretilir
pub struct X3DHAcceptResult {
    pub shared_key: SymKey,
}

/// HKDF-SHA256 ile paylaşılan anahtarı türet
///
/// Signal standardına göre:
///   IKM = 0xFF×32 || DH1 || DH2 || DH3 [|| DH4]
///   OKM = HKDF(salt=None, ikm, info="obscura-x3dh-v1")[..32]
fn derive_shared_key(dh_outputs: &[&[u8]]) -> SymKey {
    // Signal spec: F = 32 adet 0xFF, tüm DH çıktılarından önce eklenir
    let mut ikm: Vec<u8> = Vec::with_capacity(32 + dh_outputs.len() * 32);
    ikm.extend_from_slice(&[0xFFu8; 32]);
    for dh in dh_outputs {
        ikm.extend_from_slice(dh);
    }

    let hk = Hkdf::<Sha256>::new(None, &ikm);
    let mut okm = [0u8; 32];
    hk.expand(b"obscura-x3dh-v1", &mut okm).expect("HKDF expand hatası");

    ikm.zeroize();

    let key = SymKey::from_bytes(&okm).unwrap();
    okm.zeroize();
    key
}

/// Alice — X3DH başlat (Bob'un PreKey bundle'ı ile)
///
/// `alice_identity_priv` — Alice'in X25519 private key bytes (32 byte)
pub fn x3dh_initiate(
    alice_identity_priv: &[u8],
    bundle: &PreKeyBundle,
) -> Result<X3DHInitResult, String> {
    // ── Alice kimlik secret ─────────────────────────────────────────────────
    let ik_a_bytes: [u8; 32] = alice_identity_priv.try_into()
        .map_err(|_| "Alice kimlik anahtarı 32 byte olmalı")?;
    let ik_a = StaticSecret::from(ik_a_bytes);

    // ── Bob açık anahtarları ────────────────────────────────────────────────
    let ik_b_bytes: [u8; 32] = bundle.identity_key.as_slice().try_into()
        .map_err(|_| "Bob kimlik anahtarı 32 byte olmalı")?;
    let ik_b = X25519Public::from(ik_b_bytes);

    let spk_b_bytes: [u8; 32] = bundle.signed_prekey.as_slice().try_into()
        .map_err(|_| "Bob SPK 32 byte olmalı")?;
    let spk_b = X25519Public::from(spk_b_bytes);

    // ── Alice TEK efemeral anahtar (tüm DH'lerde reuse edilir) ─────────────
    // StaticSecret::diffie_hellman(&self, ...) → tüketimsiz, birden fazla kullanılabilir
    let ek_a = StaticSecret::random_from_rng(OsRng);
    let ek_a_pub = X25519Public::from(&ek_a);

    // ── DH hesaplamaları ────────────────────────────────────────────────────
    let dh1 = ik_a.diffie_hellman(&spk_b);  // IK_A × SPK_B
    let dh2 = ek_a.diffie_hellman(&ik_b);   // EK_A × IK_B
    let dh3 = ek_a.diffie_hellman(&spk_b);  // EK_A × SPK_B

    let (dh_inputs, one_time_prekey_id) = if let Some(opk_bytes) = &bundle.one_time_prekey {
        let opk_b_bytes: [u8; 32] = opk_bytes.as_slice().try_into()
            .map_err(|_| "Bob OPK 32 byte olmalı")?;
        let opk_b = X25519Public::from(opk_b_bytes);
        let dh4 = ek_a.diffie_hellman(&opk_b); // EK_A × OPK_B

        let inputs: Vec<Vec<u8>> = vec![
            dh1.as_bytes().to_vec(),
            dh2.as_bytes().to_vec(),
            dh3.as_bytes().to_vec(),
            dh4.as_bytes().to_vec(),
        ];
        (inputs, bundle.one_time_prekey_id)
    } else {
        let inputs: Vec<Vec<u8>> = vec![
            dh1.as_bytes().to_vec(),
            dh2.as_bytes().to_vec(),
            dh3.as_bytes().to_vec(),
        ];
        (inputs, None)
    };

    let refs: Vec<&[u8]> = dh_inputs.iter().map(|v| v.as_slice()).collect();
    let shared_key = derive_shared_key(&refs);

    Ok(X3DHInitResult {
        shared_key,
        ephemeral_public: ek_a_pub.as_bytes().to_vec(),
        one_time_prekey_id,
    })
}

/// Bob — X3DH kabul et (Alice'den gelen efemeral key ile)
///
/// Bob, Alice'in `ephemeral_public`'ını alır ve aynı shared_key'i türetir.
pub fn x3dh_accept(
    bob_identity_priv: &[u8],          // Bob'un X25519 identity private key (32 byte)
    bob_signed_prekey_priv: &[u8],     // Bob'un imzalı PreKey private key (32 byte)
    bob_one_time_prekey_priv: Option<&[u8]>, // Kullanılan OPK private key (32 byte, varsa)
    alice_identity_pub: &[u8],         // Alice'in X25519 identity public key (32 byte)
    alice_ephemeral_pub: &[u8],        // Alice'in efemeral public key (32 byte)
) -> Result<X3DHAcceptResult, String> {
    // ── Bob private keys ────────────────────────────────────────────────────
    let ik_b_bytes: [u8; 32] = bob_identity_priv.try_into()
        .map_err(|_| "Bob kimlik anahtarı 32 byte olmalı")?;
    let ik_b = StaticSecret::from(ik_b_bytes);

    let spk_b_bytes: [u8; 32] = bob_signed_prekey_priv.try_into()
        .map_err(|_| "Bob SPK private 32 byte olmalı")?;
    let spk_b = StaticSecret::from(spk_b_bytes);

    // ── Alice public keys ───────────────────────────────────────────────────
    let ik_a_bytes: [u8; 32] = alice_identity_pub.try_into()
        .map_err(|_| "Alice kimlik pub 32 byte olmalı")?;
    let ik_a_pub = X25519Public::from(ik_a_bytes);

    let ek_a_bytes: [u8; 32] = alice_ephemeral_pub.try_into()
        .map_err(|_| "Alice efemeral pub 32 byte olmalı")?;
    let ek_a_pub = X25519Public::from(ek_a_bytes);

    // ── DH hesaplamaları (Alice'in DH'larının tersi) ────────────────────────
    // DH1 = DH(SPK_B, IK_A)  ←→ DH(IK_A, SPK_B)
    let dh1 = spk_b.diffie_hellman(&ik_a_pub);
    // DH2 = DH(IK_B, EK_A)   ←→ DH(EK_A, IK_B)
    let dh2 = ik_b.diffie_hellman(&ek_a_pub);
    // DH3 = DH(SPK_B, EK_A)  ←→ DH(EK_A, SPK_B)
    // StaticSecret clone desteklenmez → bytes'tan yeniden oluştur
    let spk_b2 = StaticSecret::from(spk_b_bytes);
    let dh3 = spk_b2.diffie_hellman(&ek_a_pub);

    let dh_inputs: Vec<Vec<u8>> = if let Some(opk_priv) = bob_one_time_prekey_priv {
        let opk_bytes: [u8; 32] = opk_priv.try_into()
            .map_err(|_| "Bob OPK private 32 byte olmalı")?;
        let opk = StaticSecret::from(opk_bytes);
        let dh4 = opk.diffie_hellman(&ek_a_pub);

        vec![
            dh1.as_bytes().to_vec(),
            dh2.as_bytes().to_vec(),
            dh3.as_bytes().to_vec(),
            dh4.as_bytes().to_vec(),
        ]
    } else {
        vec![
            dh1.as_bytes().to_vec(),
            dh2.as_bytes().to_vec(),
            dh3.as_bytes().to_vec(),
        ]
    };

    let refs: Vec<&[u8]> = dh_inputs.iter().map(|v| v.as_slice()).collect();
    let shared_key = derive_shared_key(&refs);

    Ok(X3DHAcceptResult { shared_key })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::identity::IdentityKeyPair;
    use crate::prekeys::PreKeyStore;

    #[test]
    fn x3dh_roundtrip() {
        // Bob hazırlık
        let bob_id = IdentityKeyPair::generate();
        let mut bob_store = PreKeyStore::generate(&bob_id);
        let bundle = bob_store.bundle(&bob_id);

        // Alice X3DH başlatır
        let alice_id = IdentityKeyPair::generate();
        let init = x3dh_initiate(&alice_id.dh_private_bytes(), &bundle).unwrap();

        // Bob kabul eder
        let opk_priv = bundle.one_time_prekey_id
            .and_then(|id| bob_store.consume_opk(id));

        let accept = x3dh_accept(
            &bob_id.dh_private_bytes(),
            &bob_store.signed_prekey_priv,
            opk_priv.as_deref(),
            &alice_id.dh_public,
            &init.ephemeral_public,
        ).unwrap();

        assert_eq!(init.shared_key.0, accept.shared_key.0,
            "X3DH: Alice ve Bob aynı shared_key'e ulaşmalı");
    }
}
