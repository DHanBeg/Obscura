//! C FFI exports — Go backend'in Rust crypto'ya erişimi için
//!
//! Build: `cargo build --release` → `libobscura_crypto.so` / `.dll` / `.dylib`
//!
//! Go tarafı CGO ile `obscura_crypto.h` üzerinden çağırır.
//! Tüm karmaşık veri yapıları JSON string olarak geçirilir.
//! Geri dönen string pointer'ları `obscura_free_string()` ile serbest bırakılmalı.

use std::ffi::{CStr, CString};
use std::os::raw::{c_char, c_int};
use std::ptr;

use crate::identity::IdentityKeyPair;
use crate::prekeys::PreKeyStore;
use crate::x3dh::{x3dh_initiate, x3dh_accept, PreKeyBundle};
use crate::symmetric::{SymKey, encrypt as sym_encrypt, decrypt as sym_decrypt};
use crate::zk_stub::{
    ZkProof, IdentityWitness, CreditWitness,
    prove_identity, prove_credit_threshold, verify_proof,
};

// ─────────────────────────────────────────────────────────────────────────────
// Yardımcı: Error state
// ─────────────────────────────────────────────────────────────────────────────

thread_local! {
    static LAST_ERROR: std::cell::RefCell<Option<String>> = std::cell::RefCell::new(None);
}

fn set_error(msg: &str) {
    LAST_ERROR.with(|e| *e.borrow_mut() = Some(msg.to_string()));
}

fn clear_error() {
    LAST_ERROR.with(|e| *e.borrow_mut() = None);
}

/// Son hatayı al (JSON string döner, caller serbest bırakmalı)
#[no_mangle]
pub extern "C" fn obscura_last_error() -> *const c_char {
    LAST_ERROR.with(|e| {
        match e.borrow().as_ref() {
            Some(msg) => {
                match CString::new(msg.as_str()) {
                    Ok(cs) => cs.into_raw(),
                    Err(_) => ptr::null(),
                }
            }
            None => ptr::null(),
        }
    })
}

// ─────────────────────────────────────────────────────────────────────────────
// Bellek yönetimi
// ─────────────────────────────────────────────────────────────────────────────

/// Rust'tan dönen C string'i serbest bırak
#[no_mangle]
pub extern "C" fn obscura_free_string(s: *mut c_char) {
    if s.is_null() { return; }
    unsafe { drop(CString::from_raw(s)); }
}

/// Rust'tan dönen byte buffer'ı serbest bırak
#[no_mangle]
pub extern "C" fn obscura_free_bytes(ptr: *mut u8, len: usize) {
    if ptr.is_null() { return; }
    unsafe { drop(Vec::from_raw_parts(ptr, len, len)); }
}

// ─────────────────────────────────────────────────────────────────────────────
// Kimlik (Identity)
// ─────────────────────────────────────────────────────────────────────────────

/// Yeni kimlik anahtar çifti üret
///
/// Returns: JSON string {"dh_public":"hex","signing_public":"hex","did":"did:obs:..."}
/// Private key'ler dahil edilmez — sadece güvenli depolama için `obscura_identity_export_secure`
#[no_mangle]
pub extern "C" fn obscura_identity_generate() -> *mut c_char {
    clear_error();
    let kp = IdentityKeyPair::generate();
    let json = kp.to_json();
    match CString::new(json) {
        Ok(cs) => cs.into_raw(),
        Err(e) => { set_error(&e.to_string()); ptr::null_mut() }
    }
}

/// Kimlik anahtar çiftini güvenli JSON formatında dışa aktar (private key dahil)
///
/// `json_in` — `obscura_identity_generate()` çıktısı DEĞİL, tam secure_json
/// Returns: secure JSON string (şifreli depolanmalı)
#[no_mangle]
pub extern "C" fn obscura_identity_new_secure() -> *mut c_char {
    clear_error();
    let kp = IdentityKeyPair::generate();
    let json = kp.to_secure_json();
    match CString::new(json) {
        Ok(cs) => cs.into_raw(),
        Err(e) => { set_error(&e.to_string()); ptr::null_mut() }
    }
}

/// Güvenli JSON'dan kimlik yükle ve DID döndür
#[no_mangle]
pub extern "C" fn obscura_identity_did_from_secure(secure_json: *const c_char) -> *mut c_char {
    clear_error();
    let json_str = unsafe {
        match CStr::from_ptr(secure_json).to_str() {
            Ok(s) => s,
            Err(_) => { set_error("Geçersiz UTF-8"); return ptr::null_mut(); }
        }
    };
    match IdentityKeyPair::from_secure_json(json_str) {
        Some(kp) => {
            match CString::new(kp.did()) {
                Ok(cs) => cs.into_raw(),
                Err(e) => { set_error(&e.to_string()); ptr::null_mut() }
            }
        }
        None => { set_error("Kimlik JSON parse hatası"); ptr::null_mut() }
    }
}

/// İmza oluştur
///
/// `secure_json` — IdentityKeyPair secure JSON
/// `msg_hex` — mesaj hex
/// Returns: imza hex (64 byte = 128 hex char)
#[no_mangle]
pub extern "C" fn obscura_identity_sign(
    secure_json: *const c_char,
    msg_hex: *const c_char,
) -> *mut c_char {
    clear_error();
    let json_str = unsafe { CStr::from_ptr(secure_json).to_str().unwrap_or("") };
    let msg_hex_str = unsafe { CStr::from_ptr(msg_hex).to_str().unwrap_or("") };

    let kp = match IdentityKeyPair::from_secure_json(json_str) {
        Some(k) => k,
        None => { set_error("Kimlik yüklenemedi"); return ptr::null_mut(); }
    };
    let msg = match hex::decode(msg_hex_str) {
        Ok(b) => b,
        Err(_) => { set_error("msg_hex geçersiz"); return ptr::null_mut(); }
    };

    let sig = kp.sign(&msg);
    match CString::new(hex::encode(&sig)) {
        Ok(cs) => cs.into_raw(),
        Err(e) => { set_error(&e.to_string()); ptr::null_mut() }
    }
}

/// İmza doğrula
///
/// Returns: 1 = geçerli, 0 = geçersiz
#[no_mangle]
pub extern "C" fn obscura_identity_verify(
    pub_key_hex: *const c_char,
    msg_hex: *const c_char,
    sig_hex: *const c_char,
) -> c_int {
    clear_error();
    let pk_hex = unsafe { CStr::from_ptr(pub_key_hex).to_str().unwrap_or("") };
    let msg_hex_str = unsafe { CStr::from_ptr(msg_hex).to_str().unwrap_or("") };
    let sig_hex_str = unsafe { CStr::from_ptr(sig_hex).to_str().unwrap_or("") };

    let pk = hex::decode(pk_hex).unwrap_or_default();
    let msg = hex::decode(msg_hex_str).unwrap_or_default();
    let sig = hex::decode(sig_hex_str).unwrap_or_default();

    if IdentityKeyPair::verify_signature(&pk, &msg, &sig) { 1 } else { 0 }
}

// ─────────────────────────────────────────────────────────────────────────────
// PreKey Yönetimi
// ─────────────────────────────────────────────────────────────────────────────

/// PreKey deposu oluştur ve güvenli JSON döndür
#[no_mangle]
pub extern "C" fn obscura_prekeys_generate(secure_identity_json: *const c_char) -> *mut c_char {
    clear_error();
    let json_str = unsafe { CStr::from_ptr(secure_identity_json).to_str().unwrap_or("") };
    let kp = match IdentityKeyPair::from_secure_json(json_str) {
        Some(k) => k,
        None => { set_error("Kimlik yüklenemedi"); return ptr::null_mut(); }
    };
    let store = PreKeyStore::generate(&kp);
    match store.to_secure_json() {
        Ok(json) => match CString::new(json) {
            Ok(cs) => cs.into_raw(),
            Err(e) => { set_error(&e.to_string()); ptr::null_mut() }
        },
        Err(e) => { set_error(&e); ptr::null_mut() }
    }
}

/// PreKey bundle JSON üret (sunucuya yüklemek için)
#[no_mangle]
pub extern "C" fn obscura_prekeys_bundle(
    secure_identity_json: *const c_char,
    secure_store_json: *const c_char,
) -> *mut c_char {
    clear_error();
    let id_json = unsafe { CStr::from_ptr(secure_identity_json).to_str().unwrap_or("") };
    let store_json = unsafe { CStr::from_ptr(secure_store_json).to_str().unwrap_or("") };

    let kp = match IdentityKeyPair::from_secure_json(id_json) {
        Some(k) => k,
        None => { set_error("Kimlik yüklenemedi"); return ptr::null_mut(); }
    };
    let store = match PreKeyStore::from_secure_json(store_json) {
        Ok(s) => s,
        Err(e) => { set_error(&e); return ptr::null_mut(); }
    };

    let bundle = store.bundle(&kp);
    match serde_json::to_string(&bundle) {
        Ok(json) => match CString::new(json) {
            Ok(cs) => cs.into_raw(),
            Err(e) => { set_error(&e.to_string()); ptr::null_mut() }
        },
        Err(e) => { set_error(&e.to_string()); ptr::null_mut() }
    }
}

// ─────────────────────────────────────────────────────────────────────────────
// X3DH
// ─────────────────────────────────────────────────────────────────────────────

/// X3DH başlat (Alice tarafı)
///
/// `secure_identity_json` — Alice'in IdentityKeyPair secure JSON
/// `bundle_json` — Bob'un PreKeyBundle JSON
/// Returns: JSON {"shared_key_hex":"...","ephemeral_public_hex":"...","opk_id":null}
#[no_mangle]
pub extern "C" fn obscura_x3dh_initiate(
    secure_identity_json: *const c_char,
    bundle_json: *const c_char,
) -> *mut c_char {
    clear_error();
    let id_json = unsafe { CStr::from_ptr(secure_identity_json).to_str().unwrap_or("") };
    let b_json = unsafe { CStr::from_ptr(bundle_json).to_str().unwrap_or("") };

    let kp = match IdentityKeyPair::from_secure_json(id_json) {
        Some(k) => k,
        None => { set_error("Kimlik yüklenemedi"); return ptr::null_mut(); }
    };
    let bundle: PreKeyBundle = match serde_json::from_str(b_json) {
        Ok(b) => b,
        Err(e) => { set_error(&e.to_string()); return ptr::null_mut(); }
    };

    match x3dh_initiate(&kp.dh_private_bytes(), &bundle) {
        Ok(result) => {
            let json = serde_json::json!({
                "shared_key_hex": hex::encode(&result.shared_key.0),
                "ephemeral_public_hex": hex::encode(&result.ephemeral_public),
                "opk_id": result.one_time_prekey_id,
            }).to_string();
            match CString::new(json) {
                Ok(cs) => cs.into_raw(),
                Err(e) => { set_error(&e.to_string()); ptr::null_mut() }
            }
        }
        Err(e) => { set_error(&e); ptr::null_mut() }
    }
}

/// X3DH kabul et (Bob tarafı)
///
/// Input JSON: {
///   "bob_identity_priv_hex": "...",
///   "bob_spk_priv_hex": "...",
///   "bob_opk_priv_hex": "..." | null,
///   "alice_identity_pub_hex": "...",
///   "alice_ephemeral_pub_hex": "..."
/// }
/// Returns: JSON {"shared_key_hex":"..."}
#[no_mangle]
pub extern "C" fn obscura_x3dh_accept(params_json: *const c_char) -> *mut c_char {
    clear_error();
    let json_str = unsafe { CStr::from_ptr(params_json).to_str().unwrap_or("") };

    let v: serde_json::Value = match serde_json::from_str(json_str) {
        Ok(v) => v,
        Err(e) => { set_error(&e.to_string()); return ptr::null_mut(); }
    };

    macro_rules! hex_field {
        ($key:expr) => {{
            let s = v[$key].as_str().unwrap_or("");
            match hex::decode(s) {
                Ok(b) => b,
                Err(_) => { set_error(&format!("{} hex parse hatası", $key)); return ptr::null_mut(); }
            }
        }};
    }

    let bob_id_priv   = hex_field!("bob_identity_priv_hex");
    let bob_spk_priv  = hex_field!("bob_spk_priv_hex");
    let alice_id_pub  = hex_field!("alice_identity_pub_hex");
    let alice_ek_pub  = hex_field!("alice_ephemeral_pub_hex");

    let opk_priv: Option<Vec<u8>> = v["bob_opk_priv_hex"].as_str()
        .filter(|s| !s.is_empty())
        .map(|s| hex::decode(s).unwrap_or_default());

    match x3dh_accept(
        &bob_id_priv,
        &bob_spk_priv,
        opk_priv.as_deref(),
        &alice_id_pub,
        &alice_ek_pub,
    ) {
        Ok(result) => {
            let json = serde_json::json!({
                "shared_key_hex": hex::encode(&result.shared_key.0),
            }).to_string();
            match CString::new(json) {
                Ok(cs) => cs.into_raw(),
                Err(e) => { set_error(&e.to_string()); ptr::null_mut() }
            }
        }
        Err(e) => { set_error(&e); ptr::null_mut() }
    }
}

// ─────────────────────────────────────────────────────────────────────────────
// Simetrik Şifreleme
// ─────────────────────────────────────────────────────────────────────────────

/// AES-256-GCM şifrele
///
/// `key_hex` — 32 byte key, hex (64 char)
/// `plaintext_hex` — şifrelenecek veri, hex
/// Returns: hex encoded ciphertext (12 nonce + data + 16 tag)
#[no_mangle]
pub extern "C" fn obscura_encrypt(
    key_hex: *const c_char,
    plaintext_hex: *const c_char,
) -> *mut c_char {
    clear_error();
    let k_hex = unsafe { CStr::from_ptr(key_hex).to_str().unwrap_or("") };
    let pt_hex = unsafe { CStr::from_ptr(plaintext_hex).to_str().unwrap_or("") };

    let key_bytes = match hex::decode(k_hex) {
        Ok(b) if b.len() == 32 => b,
        _ => { set_error("Anahtar 32 byte hex olmalı"); return ptr::null_mut(); }
    };
    let pt = match hex::decode(pt_hex) {
        Ok(b) => b,
        Err(_) => { set_error("plaintext hex geçersiz"); return ptr::null_mut(); }
    };

    let sym_key = SymKey::from_bytes(&key_bytes).unwrap();
    match sym_encrypt(&sym_key, &pt) {
        Ok(ct) => match CString::new(hex::encode(&ct)) {
            Ok(cs) => cs.into_raw(),
            Err(e) => { set_error(&e.to_string()); ptr::null_mut() }
        },
        Err(e) => { set_error(&e); ptr::null_mut() }
    }
}

/// AES-256-GCM şifre çöz
///
/// `key_hex` — 32 byte key, hex
/// `ciphertext_hex` — `obscura_encrypt` çıktısı, hex
/// Returns: düz metin, hex
#[no_mangle]
pub extern "C" fn obscura_decrypt(
    key_hex: *const c_char,
    ciphertext_hex: *const c_char,
) -> *mut c_char {
    clear_error();
    let k_hex = unsafe { CStr::from_ptr(key_hex).to_str().unwrap_or("") };
    let ct_hex = unsafe { CStr::from_ptr(ciphertext_hex).to_str().unwrap_or("") };

    let key_bytes = match hex::decode(k_hex) {
        Ok(b) if b.len() == 32 => b,
        _ => { set_error("Anahtar 32 byte hex olmalı"); return ptr::null_mut(); }
    };
    let ct = match hex::decode(ct_hex) {
        Ok(b) => b,
        Err(_) => { set_error("ciphertext hex geçersiz"); return ptr::null_mut(); }
    };

    let sym_key = SymKey::from_bytes(&key_bytes).unwrap();
    match sym_decrypt(&sym_key, &ct) {
        Ok(pt) => match CString::new(hex::encode(&pt)) {
            Ok(cs) => cs.into_raw(),
            Err(e) => { set_error(&e.to_string()); ptr::null_mut() }
        },
        Err(e) => { set_error(&e); ptr::null_mut() }
    }
}

// ─────────────────────────────────────────────────────────────────────────────
// ZK Kanıtları
// ─────────────────────────────────────────────────────────────────────────────

/// Kimlik ZK kanıtı üret
///
/// `private_key_hex` — 32 byte private key, hex
/// `did` — DID string
/// `timestamp` — Unix timestamp (saniye)
/// Returns: ZkProof JSON
#[no_mangle]
pub extern "C" fn obscura_zk_prove_identity(
    private_key_hex: *const c_char,
    did: *const c_char,
    timestamp: u64,
) -> *mut c_char {
    clear_error();
    let pk_hex = unsafe { CStr::from_ptr(private_key_hex).to_str().unwrap_or("") };
    let did_str = unsafe { CStr::from_ptr(did).to_str().unwrap_or("") };

    let pk = match hex::decode(pk_hex) {
        Ok(b) if b.len() == 32 => b,
        _ => { set_error("Private key 32 byte hex olmalı"); return ptr::null_mut(); }
    };

    let nonce = {
        use rand::RngCore;
        let mut n = [0u8; 32];
        rand::rngs::OsRng.fill_bytes(&mut n);
        n
    };

    let witness = IdentityWitness {
        private_key: pk,
        did: did_str.to_string(),
        nonce,
    };

    match prove_identity(&witness, timestamp) {
        Ok(proof) => match CString::new(proof.to_json()) {
            Ok(cs) => cs.into_raw(),
            Err(e) => { set_error(&e.to_string()); ptr::null_mut() }
        },
        Err(e) => { set_error(&e); ptr::null_mut() }
    }
}

/// Kredi eşiği ZK kanıtı üret
///
/// Returns: ZkProof JSON veya NULL (eşik altında)
#[no_mangle]
pub extern "C" fn obscura_zk_prove_credit(
    actual_score: c_int,
    threshold: c_int,
    did: *const c_char,
    timestamp: u64,
) -> *mut c_char {
    clear_error();
    let did_str = unsafe { CStr::from_ptr(did).to_str().unwrap_or("") };

    use rand::RngCore;
    let mut salt = [0u8; 32];
    rand::rngs::OsRng.fill_bytes(&mut salt);

    let witness = CreditWitness {
        actual_score,
        threshold,
        salt,
    };

    match prove_credit_threshold(&witness, did_str, timestamp) {
        Ok(proof) => match CString::new(proof.to_json()) {
            Ok(cs) => cs.into_raw(),
            Err(e) => { set_error(&e.to_string()); ptr::null_mut() }
        },
        Err(e) => { set_error(&e); ptr::null_mut() }
    }
}

/// ZK kanıtını doğrula
///
/// `proof_json` — ZkProof JSON
/// Returns: 1 = geçerli, 0 = geçersiz
#[no_mangle]
pub extern "C" fn obscura_zk_verify(proof_json: *const c_char) -> c_int {
    clear_error();
    let json_str = unsafe { CStr::from_ptr(proof_json).to_str().unwrap_or("") };

    let proof = match ZkProof::from_json(json_str) {
        Ok(p) => p,
        Err(e) => { set_error(&e); return 0; }
    };

    match verify_proof(&proof) {
        Ok(true) => 1,
        Ok(false) => { set_error("Kanıt geçersiz"); 0 }
        Err(e) => { set_error(&e); 0 }
    }
}

// ─────────────────────────────────────────────────────────────────────────────
// Kütüphane Bilgisi
// ─────────────────────────────────────────────────────────────────────────────

/// Kütüphane versiyonu döndür
#[no_mangle]
pub extern "C" fn obscura_version() -> *const c_char {
    static VERSION: &[u8] = b"obscura-crypto/0.1.0\0";
    VERSION.as_ptr() as *const c_char
}
