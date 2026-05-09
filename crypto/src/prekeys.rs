//! PreKey yönetimi — Signal Protocol PreKey bundle üretimi ve yönetimi
//!
//! Her kullanıcı:
//!   - 1 adet imzalı PreKey (SPK) — haftalık rotasyon önerilir
//!   - 100 adet tek kullanımlık PreKey (OPK) — tükenince yenisi üretilir

use std::collections::HashMap;
use x25519_dalek::{StaticSecret, PublicKey as X25519Public};
use rand::rngs::OsRng;
use serde::{Serialize, Deserialize};
use zeroize::Zeroize;

use crate::identity::IdentityKeyPair;
use crate::x3dh::PreKeyBundle;

pub const OPK_COUNT: u32 = 100;

/// Tek kullanımlık PreKey (OPK)
#[derive(Serialize, Deserialize, Clone)]
pub struct OneTimePreKey {
    pub id: u32,
    pub public_key: Vec<u8>,  // X25519 public, 32 byte
    #[serde(skip)]
    pub private_key: [u8; 32], // Saklanır, gizli
}

impl Drop for OneTimePreKey {
    fn drop(&mut self) { self.private_key.zeroize(); }
}

impl OneTimePreKey {
    fn generate(id: u32) -> Self {
        let secret = StaticSecret::random_from_rng(OsRng);
        let public = X25519Public::from(&secret);
        Self {
            id,
            public_key: public.as_bytes().to_vec(),
            private_key: secret.to_bytes(),
        }
    }
}

/// PreKey deposu — cihazda şifreli saklanır
pub struct PreKeyStore {
    /// İmzalı PreKey private bytes
    pub signed_prekey_priv: Vec<u8>,
    /// İmzalı PreKey public bytes
    pub signed_prekey_pub: Vec<u8>,
    /// İmzalı PreKey ID (rotasyon için)
    pub signed_prekey_id: u32,
    /// OPK havuzu: id → key
    opk_map: HashMap<u32, OneTimePreKey>,
    /// Sonraki OPK ID sayacı
    next_opk_id: u32,
}

impl Drop for PreKeyStore {
    fn drop(&mut self) {
        self.signed_prekey_priv.zeroize();
        for opk in self.opk_map.values_mut() {
            opk.private_key.zeroize();
        }
    }
}

impl PreKeyStore {
    /// Yeni depo oluştur — identity key ile imzalı PreKey + 100 OPK üret
    pub fn generate(_identity: &IdentityKeyPair) -> Self {
        // İmzalı PreKey üret
        let spk_secret = StaticSecret::random_from_rng(OsRng);
        let spk_public = X25519Public::from(&spk_secret);
        let spk_priv_bytes = spk_secret.to_bytes().to_vec();
        let spk_pub_bytes = spk_public.as_bytes().to_vec();

        // OPK'ları üret
        let mut opk_map = HashMap::new();
        for id in 0..OPK_COUNT {
            opk_map.insert(id, OneTimePreKey::generate(id));
        }

        Self {
            signed_prekey_priv: spk_priv_bytes,
            signed_prekey_pub: spk_pub_bytes,
            signed_prekey_id: 0,
            opk_map,
            next_opk_id: OPK_COUNT,
        }
    }

    /// Sunucuya yüklenecek PreKeyBundle üret
    ///
    /// OPK havuzundaki ilk mevcut anahtarı içerir (varsa).
    pub fn bundle(&self, identity: &IdentityKeyPair) -> PreKeyBundle {
        // SPK'yı Ed25519 ile imzala
        let sig = identity.sign(&self.signed_prekey_pub);

        // İlk mevcut OPK'yı seç (sunucuya gönder, sonra consume et)
        let (opk_pub, opk_id) = self.opk_map
            .values()
            .next()
            .map(|o| (Some(o.public_key.clone()), Some(o.id)))
            .unwrap_or((None, None));

        PreKeyBundle {
            identity_key: identity.dh_public.clone(),
            signed_prekey: self.signed_prekey_pub.clone(),
            signed_prekey_sig: sig,
            one_time_prekey: opk_pub,
            one_time_prekey_id: opk_id,
            did: identity.did(),
        }
    }

    /// Kullanılan OPK'yı depoda sil ve private key döndür
    ///
    /// X3DH'de Bob, Alice'in kullandığı OPK ID'sini öğrenir ve bunu consume eder.
    pub fn consume_opk(&mut self, id: u32) -> Option<Vec<u8>> {
        self.opk_map.remove(&id).map(|mut opk| {
            let priv_bytes = opk.private_key.to_vec();
            opk.private_key.zeroize();
            priv_bytes
        })
    }

    /// OPK sayısını döndür (sunucu eksikse yeni yüklemek için kontrol)
    pub fn opk_count(&self) -> usize {
        self.opk_map.len()
    }

    /// Yeni OPK'lar üret (havuz dolduğunda çağrılır)
    ///
    /// `count` adet yeni OPK üretir ve hepsini döndürür (sunucuya yükleme için).
    pub fn replenish_opks(&mut self, count: u32) -> Vec<(u32, Vec<u8>)> {
        let mut new_keys = Vec::new();
        for _ in 0..count {
            let id = self.next_opk_id;
            let opk = OneTimePreKey::generate(id);
            let pub_bytes = opk.public_key.clone();
            self.opk_map.insert(id, opk);
            new_keys.push((id, pub_bytes));
            self.next_opk_id += 1;
        }
        new_keys
    }

    /// SPK rotasyonu — yeni imzalı PreKey üret
    pub fn rotate_signed_prekey(&mut self, identity: &IdentityKeyPair) -> (Vec<u8>, Vec<u8>) {
        let spk_secret = StaticSecret::random_from_rng(OsRng);
        let spk_public = X25519Public::from(&spk_secret);

        self.signed_prekey_priv.zeroize();
        self.signed_prekey_priv = spk_secret.to_bytes().to_vec();
        self.signed_prekey_pub = spk_public.as_bytes().to_vec();
        self.signed_prekey_id += 1;

        let sig = identity.sign(&self.signed_prekey_pub);
        (self.signed_prekey_pub.clone(), sig)
    }

    /// JSON serialize — şifreli depolama için (private key'ler dahil)
    pub fn to_secure_json(&self) -> Result<String, String> {
        // OPK'ları serialize et
        let opks: Vec<serde_json::Value> = self.opk_map.values().map(|o| {
            serde_json::json!({
                "id": o.id,
                "public_key": hex::encode(&o.public_key),
                "private_key": hex::encode(&o.private_key),
            })
        }).collect();

        let json = serde_json::json!({
            "signed_prekey_priv": hex::encode(&self.signed_prekey_priv),
            "signed_prekey_pub": hex::encode(&self.signed_prekey_pub),
            "signed_prekey_id": self.signed_prekey_id,
            "next_opk_id": self.next_opk_id,
            "opks": opks,
        });

        serde_json::to_string(&json).map_err(|e| e.to_string())
    }

    /// JSON'dan yükle (şifreli depolama)
    pub fn from_secure_json(json_str: &str) -> Result<Self, String> {
        let v: serde_json::Value = serde_json::from_str(json_str)
            .map_err(|e| e.to_string())?;

        let signed_prekey_priv = hex::decode(
            v["signed_prekey_priv"].as_str().ok_or("signed_prekey_priv eksik")?
        ).map_err(|e| e.to_string())?;

        let signed_prekey_pub = hex::decode(
            v["signed_prekey_pub"].as_str().ok_or("signed_prekey_pub eksik")?
        ).map_err(|e| e.to_string())?;

        let signed_prekey_id = v["signed_prekey_id"].as_u64().unwrap_or(0) as u32;
        let next_opk_id = v["next_opk_id"].as_u64().unwrap_or(OPK_COUNT as u64) as u32;

        let mut opk_map = HashMap::new();
        if let Some(opks) = v["opks"].as_array() {
            for o in opks {
                let id = o["id"].as_u64().ok_or("OPK id eksik")? as u32;
                let pub_hex = o["public_key"].as_str().ok_or("OPK public_key eksik")?;
                let priv_hex = o["private_key"].as_str().ok_or("OPK private_key eksik")?;

                let pub_bytes = hex::decode(pub_hex).map_err(|e| e.to_string())?;
                let priv_bytes = hex::decode(priv_hex).map_err(|e| e.to_string())?;
                let priv_arr: [u8; 32] = priv_bytes.as_slice().try_into()
                    .map_err(|_| "OPK private_key 32 byte değil")?;

                opk_map.insert(id, OneTimePreKey {
                    id,
                    public_key: pub_bytes,
                    private_key: priv_arr,
                });
            }
        }

        Ok(Self {
            signed_prekey_priv,
            signed_prekey_pub,
            signed_prekey_id,
            opk_map,
            next_opk_id,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::identity::IdentityKeyPair;

    #[test]
    fn prekey_generation() {
        let id = IdentityKeyPair::generate();
        let store = PreKeyStore::generate(&id);
        assert_eq!(store.opk_count(), OPK_COUNT as usize);
    }

    #[test]
    fn bundle_valid_signature() {
        let id = IdentityKeyPair::generate();
        let store = PreKeyStore::generate(&id);
        let bundle = store.bundle(&id);

        // SPK imzasını doğrula
        let valid = IdentityKeyPair::verify_signature(
            &id.signing_public,
            &bundle.signed_prekey,
            &bundle.signed_prekey_sig,
        );
        assert!(valid, "SPK imzası geçersiz");
    }

    #[test]
    fn opk_consume() {
        let id = IdentityKeyPair::generate();
        let mut store = PreKeyStore::generate(&id);
        let bundle = store.bundle(&id);

        if let Some(opk_id) = bundle.one_time_prekey_id {
            let priv_key = store.consume_opk(opk_id);
            assert!(priv_key.is_some(), "OPK consume başarısız");
            assert_eq!(store.opk_count(), (OPK_COUNT - 1) as usize);

            // İkinci consume None dönmeli
            assert!(store.consume_opk(opk_id).is_none());
        }
    }

    #[test]
    fn secure_json_roundtrip() {
        let id = IdentityKeyPair::generate();
        let store = PreKeyStore::generate(&id);
        let json = store.to_secure_json().unwrap();
        let store2 = PreKeyStore::from_secure_json(&json).unwrap();
        assert_eq!(store2.opk_count(), OPK_COUNT as usize);
        assert_eq!(store2.signed_prekey_pub, store.signed_prekey_pub);
    }
}
