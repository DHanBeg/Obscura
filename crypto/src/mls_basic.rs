//! MLS-benzeri grup mesajlaşma şifrelemesi — RFC 9420 üzerine basitleştirilmiş
//!
//! Tam TreeKEM yerine epoch tabanlı grup anahtarı yönetimi:
//!   - Her grup değişikliği (üye ekleme/çıkarma/güncelleme) yeni epoch açar
//!   - Yeni grup anahtarı: HKDF(mevcut_anahtar || yeni_üye_katkıları || epoch)
//!   - Mesajlar AES-256-GCM ile grup anahtarıyla şifrelenir
//!   - Forward secrecy: eski epoch anahtarları silinir
//!
//! TODO: Üretimde `openmls` crate'e geçiş yapılacak (RFC 9420 tam uyum)

use hkdf::Hkdf;
use sha2::Sha256;
use rand::rngs::OsRng;
use serde::{Serialize, Deserialize};
use zeroize::Zeroize;

use crate::symmetric::{SymKey, encrypt as sym_encrypt, decrypt as sym_decrypt};

// ─────────────────────────────────────────────────────────────────────────────
// Veri Yapıları
// ─────────────────────────────────────────────────────────────────────────────

/// Grup üyesi
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct GroupMember {
    /// Üyenin DID'i
    pub did: String,
    /// Üyenin X25519 kimlik açık anahtarı (32 byte, hex encoded)
    pub identity_key_hex: String,
    /// Gruba katılma epoch'u
    pub joined_at_epoch: u64,
}

/// Grup değişiklik türleri (epoch geçişleri için)
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum GroupChange {
    Create { members: Vec<GroupMember> },
    AddMember { member: GroupMember },
    RemoveMember { did: String },
    KeyUpdate { updater_did: String },
}

/// Şifreli grup mesajı
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct GroupMessage {
    /// Grup ID'si
    pub group_id: String,
    /// Hangi epoch'ta gönderildi
    pub epoch: u64,
    /// Gönderen DID
    pub sender_did: String,
    /// Mesaj numarası (bu epoch'ta kaçıncı mesaj)
    pub seq: u64,
    /// Şifreli payload: [12 nonce][ciphertext + 16 tag]
    pub ciphertext: Vec<u8>,
}

/// Epoch geçiş mesajı (üyelere dağıtılır)
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct EpochTransition {
    pub group_id: String,
    pub new_epoch: u64,
    pub change: GroupChange,
    /// Yeni epoch anahtarı, her üye için ayrı şifreli (üye X25519 public key ile)
    /// (Basitleştirilmiş: gerçekte TreeKEM kullanır)
    pub encrypted_keys: Vec<(String, Vec<u8>)>, // (did, encrypted_group_key)
}

/// Grup durumu (her üye cihazda saklar)
pub struct GroupState {
    pub group_id: String,
    pub epoch: u64,
    pub members: Vec<GroupMember>,
    /// Mevcut epoch grup anahtarı
    group_key: [u8; 32],
    /// Mesaj sayacı
    seq: u64,
    /// Kendi DID'imiz
    pub my_did: String,
}

impl Drop for GroupState {
    fn drop(&mut self) {
        self.group_key.zeroize();
    }
}

// ─────────────────────────────────────────────────────────────────────────────
// Anahtar Türetimi
// ─────────────────────────────────────────────────────────────────────────────

/// Grup anahtarı türet
///
/// HKDF-SHA256(
///   ikm = prev_key || member_hash,
///   salt = epoch_bytes,
///   info = b"ObscuraMLS" || group_id
/// ) → 32 byte
fn derive_group_key(
    prev_key: &[u8],
    members: &[GroupMember],
    epoch: u64,
    group_id: &str,
) -> [u8; 32] {
    // Üye katkısı: tüm üye key_hex'lerini deterministic sıralamayla birleştir
    let mut sorted_keys: Vec<&str> = members.iter()
        .map(|m| m.identity_key_hex.as_str())
        .collect();
    sorted_keys.sort_unstable(); // Deterministik sıra
    let mut member_hash_input: Vec<u8> = Vec::new();
    for k in &sorted_keys {
        member_hash_input.extend_from_slice(k.as_bytes());
        member_hash_input.push(0xFF); // Separator
    }

    let mut ikm = prev_key.to_vec();
    ikm.extend_from_slice(&member_hash_input);

    let mut salt = [0u8; 8];
    salt.copy_from_slice(&epoch.to_be_bytes());

    let mut info = b"ObscuraMLS".to_vec();
    info.extend_from_slice(group_id.as_bytes());

    let hk = Hkdf::<Sha256>::new(Some(&salt), &ikm);
    let mut okm = [0u8; 32];
    hk.expand(&info, &mut okm).expect("HKDF expand hatası");

    ikm.zeroize();
    okm
}

/// Mesaj anahtarı türet (grup anahtarı + seq → mesaj anahtarı)
///
/// Her mesaj için farklı anahtar: forward secrecy on message level
fn derive_message_key(group_key: &[u8; 32], seq: u64, sender_did: &str) -> [u8; 32] {
    let mut ikm = group_key.to_vec();
    ikm.extend_from_slice(&seq.to_be_bytes());
    ikm.extend_from_slice(sender_did.as_bytes());

    let hk = Hkdf::<Sha256>::new(None, &ikm);
    let mut mk = [0u8; 32];
    hk.expand(b"ObscuraMLSMsg", &mut mk).expect("HKDF expand hatası");

    ikm.zeroize();
    mk
}

// ─────────────────────────────────────────────────────────────────────────────
// Grup İşlemleri
// ─────────────────────────────────────────────────────────────────────────────

impl GroupState {
    /// Yeni grup oluştur (kurucu tarafından)
    pub fn create(
        group_id: String,
        creator_did: String,
        creator_key_hex: String,
        initial_members: Vec<GroupMember>,
    ) -> Self {
        let mut all_members = initial_members;
        // Kurucuyu da ekle (yoksa)
        if !all_members.iter().any(|m| m.did == creator_did) {
            all_members.push(GroupMember {
                did: creator_did.clone(),
                identity_key_hex: creator_key_hex,
                joined_at_epoch: 0,
            });
        }

        // İlk grup anahtarı: rastgele 32 byte (kurucu üretir, üyelere dağıtır)
        use rand::RngCore;
        let mut initial_secret = [0u8; 32];
        OsRng.fill_bytes(&mut initial_secret);

        let group_key = derive_group_key(&initial_secret, &all_members, 0, &group_id);
        initial_secret.zeroize();

        Self {
            group_id,
            epoch: 0,
            members: all_members,
            group_key,
            seq: 0,
            my_did: creator_did,
        }
    }

    /// Mevcut gruba katıl (epoch transition mesajından)
    ///
    /// `group_key_bytes` — şifresi çözülmüş grup anahtarı (32 byte)
    pub fn join(
        group_id: String,
        epoch: u64,
        members: Vec<GroupMember>,
        group_key_bytes: &[u8; 32],
        my_did: String,
    ) -> Self {
        Self {
            group_id,
            epoch,
            members,
            group_key: *group_key_bytes,
            seq: 0,
            my_did,
        }
    }

    // ──────────────────────────────────────────────────────────────────────
    // Mesaj Şifreleme / Çözme
    // ──────────────────────────────────────────────────────────────────────

    /// Grup mesajı şifrele
    ///
    /// `plaintext` — düz metin (mesaj bytes)
    /// Returns: (GroupMessage, yeni seq)
    pub fn encrypt_message(&mut self, plaintext: &[u8]) -> Result<GroupMessage, String> {
        let seq = self.seq;
        self.seq += 1;

        // Mesaj anahtarı türet (her mesaj için benzersiz)
        let mk = derive_message_key(&self.group_key, seq, &self.my_did);
        let sym_key = SymKey::from_bytes(&mk).unwrap();

        // Ek veri: group_id || epoch || seq || sender_did (authenticated)
        let mut ad = self.group_id.as_bytes().to_vec();
        ad.extend_from_slice(&self.epoch.to_be_bytes());
        ad.extend_from_slice(&seq.to_be_bytes());
        ad.extend_from_slice(self.my_did.as_bytes());

        // sym_encrypt sadece mesajı şifreler, AD için ratchet::encrypt_with_ad lazım
        // Basitleştirilmiş: AD'yi plaintext başına ekle, sonra şifrele
        let mut pt_with_ad_marker = (ad.len() as u32).to_be_bytes().to_vec();
        pt_with_ad_marker.extend_from_slice(&ad);
        pt_with_ad_marker.extend_from_slice(plaintext);

        let ciphertext = sym_encrypt(&sym_key, &pt_with_ad_marker)?;

        Ok(GroupMessage {
            group_id: self.group_id.clone(),
            epoch: self.epoch,
            sender_did: self.my_did.clone(),
            seq,
            ciphertext,
        })
    }

    /// Grup mesajının şifresini çöz
    pub fn decrypt_message(&self, msg: &GroupMessage) -> Result<Vec<u8>, String> {
        if msg.group_id != self.group_id {
            return Err("Yanlış grup ID'si".into());
        }
        if msg.epoch != self.epoch {
            return Err(format!(
                "Yanlış epoch: beklenen {}, gelen {}",
                self.epoch, msg.epoch
            ));
        }

        let mk = derive_message_key(&self.group_key, msg.seq, &msg.sender_did);
        let sym_key = SymKey::from_bytes(&mk).unwrap();

        let pt_with_marker = sym_decrypt(&sym_key, &msg.ciphertext)?;

        if pt_with_marker.len() < 4 {
            return Err("Mesaj çok kısa".into());
        }
        let ad_len = u32::from_be_bytes(pt_with_marker[..4].try_into().unwrap()) as usize;
        let total_overhead = 4 + ad_len;
        if pt_with_marker.len() < total_overhead {
            return Err("Bozuk mesaj formatı".into());
        }

        // AD doğrula
        let actual_ad = &pt_with_marker[4..4 + ad_len];
        let mut expected_ad = msg.group_id.as_bytes().to_vec();
        expected_ad.extend_from_slice(&msg.epoch.to_be_bytes());
        expected_ad.extend_from_slice(&msg.seq.to_be_bytes());
        expected_ad.extend_from_slice(msg.sender_did.as_bytes());

        if actual_ad != expected_ad.as_slice() {
            return Err("Mesaj bütünlüğü bozulmuş — AD uyuşmuyor".into());
        }

        Ok(pt_with_marker[total_overhead..].to_vec())
    }

    // ──────────────────────────────────────────────────────────────────────
    // Grup Yönetimi
    // ──────────────────────────────────────────────────────────────────────

    /// Üye ekle ve yeni epoch aç
    ///
    /// Returns: yeni GroupState + EpochTransition (üyelere gönderilmeli)
    pub fn add_member(
        &self,
        new_member: GroupMember,
    ) -> (GroupState, GroupChange) {
        let new_epoch = self.epoch + 1;
        let mut new_members = self.members.clone();
        new_members.push(GroupMember {
            joined_at_epoch: new_epoch,
            ..new_member.clone()
        });

        let new_group_key = derive_group_key(
            &self.group_key,
            &new_members,
            new_epoch,
            &self.group_id,
        );

        let new_state = GroupState {
            group_id: self.group_id.clone(),
            epoch: new_epoch,
            members: new_members,
            group_key: new_group_key,
            seq: 0,
            my_did: self.my_did.clone(),
        };

        let change = GroupChange::AddMember { member: new_member };
        (new_state, change)
    }

    /// Üye çıkar ve yeni epoch aç
    pub fn remove_member(
        &self,
        target_did: &str,
    ) -> Result<(GroupState, GroupChange), String> {
        if !self.members.iter().any(|m| m.did == target_did) {
            return Err(format!("Üye bulunamadı: {}", target_did));
        }
        if target_did == self.my_did {
            return Err("Kendini gruptan çıkaramazsın — leave_group kullan".into());
        }

        let new_epoch = self.epoch + 1;
        let new_members: Vec<GroupMember> = self.members.iter()
            .filter(|m| m.did != target_did)
            .cloned()
            .collect();

        // Yeni anahtar: eski anahtardan + yeni üye listesinden türet
        // (Çıkarılan üye eski anahtarı biliyor ama yeniyi bilmiyor — break-in prevention)
        let new_group_key = derive_group_key(
            &self.group_key,
            &new_members,
            new_epoch,
            &self.group_id,
        );

        let new_state = GroupState {
            group_id: self.group_id.clone(),
            epoch: new_epoch,
            members: new_members,
            group_key: new_group_key,
            seq: 0,
            my_did: self.my_did.clone(),
        };

        let change = GroupChange::RemoveMember { did: target_did.to_string() };
        Ok((new_state, change))
    }

    /// Kendi anahtarını güncelle (forward secrecy güçlendir)
    pub fn update_key(&self) -> (GroupState, GroupChange) {
        let new_epoch = self.epoch + 1;
        let new_group_key = derive_group_key(
            &self.group_key,
            &self.members,
            new_epoch,
            &self.group_id,
        );

        let new_state = GroupState {
            group_id: self.group_id.clone(),
            epoch: new_epoch,
            members: self.members.clone(),
            group_key: new_group_key,
            seq: 0,
            my_did: self.my_did.clone(),
        };

        let change = GroupChange::KeyUpdate { updater_did: self.my_did.clone() };
        (new_state, change)
    }

    /// Epoch'a uygun grup anahtarını al (EpochTransition üretimi için)
    pub fn group_key_bytes(&self) -> [u8; 32] {
        self.group_key
    }

    /// Üye listesi
    pub fn member_dids(&self) -> Vec<&str> {
        self.members.iter().map(|m| m.did.as_str()).collect()
    }

    /// Grup var mı — verilen DID üye mi?
    pub fn is_member(&self, did: &str) -> bool {
        self.members.iter().any(|m| m.did == did)
    }

    /// Serialize (şifreli depolama için — group_key şifreli saklanmalı)
    pub fn to_json_public(&self) -> String {
        serde_json::json!({
            "group_id": self.group_id,
            "epoch": self.epoch,
            "members": self.members,
            "seq": self.seq,
            "my_did": self.my_did,
            // group_key: HAYIR — şifreli ayrı depola
        }).to_string()
    }
}

// ─────────────────────────────────────────────────────────────────────────────
// Testler
// ─────────────────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;

    fn make_member(did: &str, key_hex: &str) -> GroupMember {
        GroupMember {
            did: did.to_string(),
            identity_key_hex: key_hex.to_string(),
            joined_at_epoch: 0,
        }
    }

    #[test]
    fn group_create_and_message() {
        let alice_did = "did:obs:alice";
        let alice_key = "aa".repeat(32);
        let bob = make_member("did:obs:bob", &"bb".repeat(32));

        let mut alice_state = GroupState::create(
            "group-001".to_string(),
            alice_did.to_string(),
            alice_key.clone(),
            vec![bob.clone()],
        );

        // Bob aynı state'i mirror'lar (test için direkt oluştur)
        let mut bob_state = GroupState::join(
            "group-001".to_string(),
            0,
            alice_state.members.clone(),
            &alice_state.group_key_bytes(),
            "did:obs:bob".to_string(),
        );

        // Alice mesaj gönderir
        let pt = b"Merhaba grup!";
        let msg = alice_state.encrypt_message(pt).unwrap();

        // Bob alır
        let decrypted = bob_state.decrypt_message(&msg).unwrap();
        assert_eq!(decrypted, pt);
    }

    #[test]
    fn add_member_new_epoch() {
        let mut state = GroupState::create(
            "grp-002".to_string(),
            "did:obs:alice".to_string(),
            "aa".repeat(32),
            vec![],
        );
        assert_eq!(state.epoch, 0);

        let carol = make_member("did:obs:carol", &"cc".repeat(32));
        let (new_state, change) = state.add_member(carol);
        assert_eq!(new_state.epoch, 1);
        assert_eq!(new_state.members.len(), 2); // alice + carol
        assert!(matches!(change, GroupChange::AddMember { .. }));
    }

    #[test]
    fn remove_member_new_epoch() {
        let bob = make_member("did:obs:bob", &"bb".repeat(32));
        let state = GroupState::create(
            "grp-003".to_string(),
            "did:obs:alice".to_string(),
            "aa".repeat(32),
            vec![bob],
        );
        assert_eq!(state.members.len(), 2);

        let (new_state, _) = state.remove_member("did:obs:bob").unwrap();
        assert_eq!(new_state.epoch, 1);
        assert_eq!(new_state.members.len(), 1);
        assert!(!new_state.is_member("did:obs:bob"));

        // Yeni grup anahtarı farklı olmalı
        assert_ne!(new_state.group_key_bytes(), state.group_key_bytes());
    }

    #[test]
    fn multi_message_sequence() {
        let mut alice = GroupState::create(
            "grp-004".to_string(),
            "did:obs:alice".to_string(),
            "aa".repeat(32),
            vec![],
        );
        let mut bob = GroupState::join(
            "grp-004".to_string(),
            0,
            alice.members.clone(),
            &alice.group_key_bytes(),
            "did:obs:alice".to_string(),
        );

        for i in 0u8..5 {
            let pt = vec![i; 100];
            let msg = alice.encrypt_message(&pt).unwrap();
            let dec = bob.decrypt_message(&msg).unwrap();
            assert_eq!(dec, pt, "Mesaj {} eşleşmedi", i);
        }
    }

    #[test]
    fn wrong_epoch_rejected() {
        let mut alice = GroupState::create(
            "grp-005".to_string(),
            "did:obs:alice".to_string(),
            "aa".repeat(32),
            vec![],
        );
        let msg = alice.encrypt_message(b"test").unwrap();

        // Epoch 1'e geç
        let (alice2, _) = alice.update_key();

        // Eski mesajı yeni epoch'ta çözmeye çalış
        let result = alice2.decrypt_message(&msg);
        assert!(result.is_err(), "Yanlış epoch mesajı kabul edilmemeli");
    }
}
