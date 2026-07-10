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
use crate::ratchet::{RatchetState, EncryptedMessage};
use crate::sealed_sender::{seal as sealed_seal, unseal as sealed_unseal};
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

/// NULL-güvenli C string → &str dönüşümü (FFI sınırındaki TEK CStr::from_ptr noktası)
///
/// Go tarafı hiçbir zaman NULL geçmemeli, ama upstream bir allocation hatası
/// nil pointer sızdırabilir — NULL burada UB yerine kontrollü hata üretir.
///
/// # Safety (unsafe bloğun gerekçesi)
/// `p` NULL değilse, NUL ile sonlanmış geçerli bir C string'e işaret etmelidir
/// (Go'nun `C.CString`'i bunu garantiler). NUL sonlandırması olmayan bir buffer
/// geçirilirse `CStr::from_ptr` buffer sonunu aşarak okur — bu, C string ABI
/// sözleşmesinin bir gereğidir ve çağıran tarafın sorumluluğundadır.
fn cstr_arg<'a>(p: *const c_char, name: &str) -> Result<&'a str, ()> {
    if p.is_null() {
        set_error(&format!("{name}: NULL pointer"));
        return Err(());
    }
    match unsafe { CStr::from_ptr(p) }.to_str() {
        Ok(s) => Ok(s),
        Err(_) => {
            set_error(&format!("{name}: geçersiz UTF-8"));
            Err(())
        }
    }
}

/// `cstr_arg` + hata durumunda `ptr::null_mut()` döndüren kısayol
macro_rules! cstr_or_null {
    ($p:expr, $name:expr) => {
        match cstr_arg($p, $name) {
            Ok(s) => s,
            Err(_) => return ptr::null_mut(),
        }
    };
}

/// `cstr_arg` + hata durumunda `0` (c_int) döndüren kısayol
macro_rules! cstr_or_zero {
    ($p:expr, $name:expr) => {
        match cstr_arg($p, $name) {
            Ok(s) => s,
            Err(_) => return 0,
        }
    };
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
    let json_str = cstr_or_null!(secure_json, "secure_json");
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
    let json_str = cstr_or_null!(secure_json, "secure_json");
    let msg_hex_str = cstr_or_null!(msg_hex, "msg_hex");

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
    let pk_hex = cstr_or_zero!(pub_key_hex, "pub_key_hex");
    let msg_hex_str = cstr_or_zero!(msg_hex, "msg_hex");
    let sig_hex_str = cstr_or_zero!(sig_hex, "sig_hex");

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
    let json_str = cstr_or_null!(secure_identity_json, "secure_identity_json");
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
    let id_json = cstr_or_null!(secure_identity_json, "secure_identity_json");
    let store_json = cstr_or_null!(secure_store_json, "secure_store_json");

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
    let id_json = cstr_or_null!(secure_identity_json, "secure_identity_json");
    let b_json = cstr_or_null!(bundle_json, "bundle_json");

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
    let json_str = cstr_or_null!(params_json, "params_json");

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
    let k_hex = cstr_or_null!(key_hex, "key_hex");
    let pt_hex = cstr_or_null!(plaintext_hex, "plaintext_hex");

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
    let k_hex = cstr_or_null!(key_hex, "key_hex");
    let ct_hex = cstr_or_null!(ciphertext_hex, "ciphertext_hex");

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
    let pk_hex = cstr_or_null!(private_key_hex, "private_key_hex");
    let did_str = cstr_or_null!(did, "did");

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
    let did_str = cstr_or_null!(did, "did");

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
    let json_str = cstr_or_zero!(proof_json, "proof_json");

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
// Double Ratchet
// ─────────────────────────────────────────────────────────────────────────────
//
// RatchetState durumsaldır: her mesajdan sonra değişir. FFI sınırında Go,
// durumu OPAK bir JSON blob olarak saklar ve her çağrıda geri verir.
// Akış:
//   1. X3DH sonrası: obscura_ratchet_init_sender / _init_receiver  → state JSON
//   2. Gönder: obscura_ratchet_encrypt(state, plaintext, ad) → {state, message}
//   3. Al:     obscura_ratchet_decrypt(state, message, ad)    → {state, plaintext}
// Her çağrı GÜNCELLENMIŞ state döndürür — eski state ATILMALI.

/// 32-byte hex string → [u8; 32]
fn hex32(s: &str) -> Result<[u8; 32], String> {
    let bytes = hex::decode(s).map_err(|_| "hex parse hatası".to_string())?;
    bytes.as_slice().try_into().map_err(|_| "32 byte olmalı".to_string())
}

/// Gönderen (Alice) ratchet başlat — X3DH shared_key ile
///
/// Input JSON: {"shared_key_hex":"...","remote_ratchet_pub_hex":"..."}
///   `remote_ratchet_pub_hex` — Bob'un ilk DH ratchet açık anahtarı (32 byte)
/// Returns: RatchetState secure JSON (Go tarafı şifreli saklar)
#[no_mangle]
pub extern "C" fn obscura_ratchet_init_sender(params_json: *const c_char) -> *mut c_char {
    clear_error();
    let json_str = cstr_or_null!(params_json, "params_json");
    let v: serde_json::Value = match serde_json::from_str(json_str) {
        Ok(v) => v,
        Err(e) => { set_error(&e.to_string()); return ptr::null_mut(); }
    };

    let sk = match hex32(v["shared_key_hex"].as_str().unwrap_or("")) {
        Ok(b) => b,
        Err(e) => { set_error(&format!("shared_key_hex: {e}")); return ptr::null_mut(); }
    };
    let remote_pub = match hex32(v["remote_ratchet_pub_hex"].as_str().unwrap_or("")) {
        Ok(b) => b,
        Err(e) => { set_error(&format!("remote_ratchet_pub_hex: {e}")); return ptr::null_mut(); }
    };

    let state = RatchetState::init_sender(&sk, &remote_pub);
    state_to_cstr(&state)
}

/// Alan (Bob) ratchet başlat — X3DH shared_key ile
///
/// Input JSON: {"shared_key_hex":"...","ratchet_priv_hex":"..."}
///   `ratchet_priv_hex` — Bob'un DH ratchet private anahtarı (32 byte).
///   Bu, bundle'da yayınlanan SPK'nın private'ı olabilir.
/// Returns: RatchetState secure JSON
#[no_mangle]
pub extern "C" fn obscura_ratchet_init_receiver(params_json: *const c_char) -> *mut c_char {
    clear_error();
    let json_str = cstr_or_null!(params_json, "params_json");
    let v: serde_json::Value = match serde_json::from_str(json_str) {
        Ok(v) => v,
        Err(e) => { set_error(&e.to_string()); return ptr::null_mut(); }
    };

    let sk = match hex32(v["shared_key_hex"].as_str().unwrap_or("")) {
        Ok(b) => b,
        Err(e) => { set_error(&format!("shared_key_hex: {e}")); return ptr::null_mut(); }
    };
    let ratchet_priv = match hex32(v["ratchet_priv_hex"].as_str().unwrap_or("")) {
        Ok(b) => b,
        Err(e) => { set_error(&format!("ratchet_priv_hex: {e}")); return ptr::null_mut(); }
    };

    let state = RatchetState::init_receiver(&sk, &ratchet_priv);
    state_to_cstr(&state)
}

/// Mesaj şifrele (ratchet ilerletilir)
///
/// Input JSON: {"state":<RatchetState JSON>,"plaintext_hex":"...","ad_hex":"..."}
/// Returns: {"state":<güncel RatchetState JSON>,"message_hex":"..."}
///   `message_hex` — wire formatı (40 byte header + AES-GCM çıktısı)
/// ÖNEMLI: dönen `state` ile çağrılan eski state DEĞİŞTİRİLMELI.
#[no_mangle]
pub extern "C" fn obscura_ratchet_encrypt(params_json: *const c_char) -> *mut c_char {
    clear_error();
    let json_str = cstr_or_null!(params_json, "params_json");
    let v: serde_json::Value = match serde_json::from_str(json_str) {
        Ok(v) => v,
        Err(e) => { set_error(&e.to_string()); return ptr::null_mut(); }
    };

    let mut state: RatchetState = match serde_json::from_value(v["state"].clone()) {
        Ok(s) => s,
        Err(e) => { set_error(&format!("state parse: {e}")); return ptr::null_mut(); }
    };
    let plaintext = match hex::decode(v["plaintext_hex"].as_str().unwrap_or("")) {
        Ok(b) => b,
        Err(_) => { set_error("plaintext_hex geçersiz"); return ptr::null_mut(); }
    };
    let ad = hex::decode(v["ad_hex"].as_str().unwrap_or("")).unwrap_or_default();

    match state.encrypt(&plaintext, &ad) {
        Ok(enc) => {
            let state_json = match serde_json::to_value(&state) {
                Ok(s) => s,
                Err(e) => { set_error(&e.to_string()); return ptr::null_mut(); }
            };
            let out = serde_json::json!({
                "state": state_json,
                "message_hex": hex::encode(enc.to_bytes()),
            }).to_string();
            match CString::new(out) {
                Ok(cs) => cs.into_raw(),
                Err(e) => { set_error(&e.to_string()); ptr::null_mut() }
            }
        }
        Err(e) => { set_error(&e); ptr::null_mut() }
    }
}

/// Mesaj şifresini çöz (ratchet ilerletilir)
///
/// Input JSON: {"state":<RatchetState JSON>,"message_hex":"...","ad_hex":"..."}
/// Returns: {"state":<güncel RatchetState JSON>,"plaintext_hex":"..."}
/// ÖNEMLI: dönen `state` ile çağrılan eski state DEĞİŞTİRİLMELI.
#[no_mangle]
pub extern "C" fn obscura_ratchet_decrypt(params_json: *const c_char) -> *mut c_char {
    clear_error();
    let json_str = cstr_or_null!(params_json, "params_json");
    let v: serde_json::Value = match serde_json::from_str(json_str) {
        Ok(v) => v,
        Err(e) => { set_error(&e.to_string()); return ptr::null_mut(); }
    };

    let mut state: RatchetState = match serde_json::from_value(v["state"].clone()) {
        Ok(s) => s,
        Err(e) => { set_error(&format!("state parse: {e}")); return ptr::null_mut(); }
    };
    let msg_bytes = match hex::decode(v["message_hex"].as_str().unwrap_or("")) {
        Ok(b) => b,
        Err(_) => { set_error("message_hex geçersiz"); return ptr::null_mut(); }
    };
    let ad = hex::decode(v["ad_hex"].as_str().unwrap_or("")).unwrap_or_default();

    let enc = match EncryptedMessage::from_bytes(&msg_bytes) {
        Ok(m) => m,
        Err(e) => { set_error(&e); return ptr::null_mut(); }
    };

    match state.decrypt(&enc, &ad) {
        Ok(plaintext) => {
            let state_json = match serde_json::to_value(&state) {
                Ok(s) => s,
                Err(e) => { set_error(&e.to_string()); return ptr::null_mut(); }
            };
            let out = serde_json::json!({
                "state": state_json,
                "plaintext_hex": hex::encode(&plaintext),
            }).to_string();
            match CString::new(out) {
                Ok(cs) => cs.into_raw(),
                Err(e) => { set_error(&e.to_string()); ptr::null_mut() }
            }
        }
        Err(e) => { set_error(&e); ptr::null_mut() }
    }
}

/// RatchetState'i C string olarak serialize et (yardımcı)
fn state_to_cstr(state: &RatchetState) -> *mut c_char {
    match serde_json::to_string(state) {
        Ok(json) => match CString::new(json) {
            Ok(cs) => cs.into_raw(),
            Err(e) => { set_error(&e.to_string()); ptr::null_mut() }
        },
        Err(e) => { set_error(&e.to_string()); ptr::null_mut() }
    }
}

// ─────────────────────────────────────────────────────────────────────────────
// Sealed Sender — gönderen kimliğini sunucudan gizleyen zarf
// ─────────────────────────────────────────────────────────────────────────────

/// Sealed sender zarfı oluştur (gönderen tarafı)
///
/// Input JSON: {
///   "sender_identity_json": "<IdentityKeyPair secure JSON>" | <nesne>,
///   "recipient_identity_pub_hex": "<64 hex char — alıcının X25519 kimlik pub>",
///   "payload_hex": "<şifrelenecek yük, tipik olarak ratchet mesajı>",
///   "expires_at": <u64 unix saniye, 0 = süresiz>
/// }
/// Returns: JSON {"envelope_hex":"..."} — zarf gönderen kimliğini AÇIK içermez.
/// Hata: NULL döner, detay `obscura_last_error()`.
/// Bellek: dönen string `obscura_free_string()` ile serbest bırakılmalı.
#[no_mangle]
pub extern "C" fn obscura_sealed_sender_seal(params_json: *const c_char) -> *mut c_char {
    clear_error();
    let json_str = cstr_or_null!(params_json, "params_json");
    let v: serde_json::Value = match serde_json::from_str(json_str) {
        Ok(v) => v,
        Err(e) => { set_error(&e.to_string()); return ptr::null_mut(); }
    };

    // sender_identity_json: string olarak veya iç içe JSON nesnesi olarak kabul et
    let sender_json = match v["sender_identity_json"].as_str() {
        Some(s) => s.to_string(),
        None => v["sender_identity_json"].to_string(),
    };
    let sender = match IdentityKeyPair::from_secure_json(&sender_json) {
        Some(k) => k,
        None => { set_error("Gönderen kimliği yüklenemedi"); return ptr::null_mut(); }
    };

    let recipient_pub = match hex::decode(v["recipient_identity_pub_hex"].as_str().unwrap_or("")) {
        Ok(b) if b.len() == 32 => b,
        _ => { set_error("recipient_identity_pub_hex 32 byte hex olmalı"); return ptr::null_mut(); }
    };
    let payload = match hex::decode(v["payload_hex"].as_str().unwrap_or("")) {
        Ok(b) => b,
        Err(_) => { set_error("payload_hex geçersiz"); return ptr::null_mut(); }
    };
    let expires_at = v["expires_at"].as_u64().unwrap_or(0);

    match sealed_seal(&sender, &recipient_pub, &payload, expires_at) {
        Ok(envelope) => {
            let out = serde_json::json!({
                "envelope_hex": hex::encode(&envelope),
            }).to_string();
            match CString::new(out) {
                Ok(cs) => cs.into_raw(),
                Err(e) => { set_error(&e.to_string()); ptr::null_mut() }
            }
        }
        Err(e) => { set_error(&e); ptr::null_mut() }
    }
}

/// Sealed sender zarfını aç (alıcı tarafı)
///
/// Input JSON: {
///   "recipient_identity_json": "<IdentityKeyPair secure JSON>" | <nesne>,
///   "envelope_hex": "<obscura_sealed_sender_seal çıktısı>",
///   "now": <u64 unix saniye — sertifika süre kontrolü için>
/// }
/// Returns: JSON {
///   "sender_did":"did:obs:...",
///   "sender_identity_pub_hex":"...",
///   "sender_signing_pub_hex":"...",
///   "payload_hex":"..."
/// }
/// Hata (yanlış alıcı anahtarı dahil): NULL döner, detay `obscura_last_error()`.
/// Bellek: dönen string `obscura_free_string()` ile serbest bırakılmalı.
#[no_mangle]
pub extern "C" fn obscura_sealed_sender_unseal(params_json: *const c_char) -> *mut c_char {
    clear_error();
    let json_str = cstr_or_null!(params_json, "params_json");
    let v: serde_json::Value = match serde_json::from_str(json_str) {
        Ok(v) => v,
        Err(e) => { set_error(&e.to_string()); return ptr::null_mut(); }
    };

    let recipient_json = match v["recipient_identity_json"].as_str() {
        Some(s) => s.to_string(),
        None => v["recipient_identity_json"].to_string(),
    };
    let recipient = match IdentityKeyPair::from_secure_json(&recipient_json) {
        Some(k) => k,
        None => { set_error("Alıcı kimliği yüklenemedi"); return ptr::null_mut(); }
    };

    let envelope = match hex::decode(v["envelope_hex"].as_str().unwrap_or("")) {
        Ok(b) => b,
        Err(_) => { set_error("envelope_hex geçersiz"); return ptr::null_mut(); }
    };
    let now = v["now"].as_u64().unwrap_or(0);

    match sealed_unseal(&recipient, &envelope, now) {
        Ok(msg) => {
            let out = serde_json::json!({
                "sender_did": msg.sender_did,
                "sender_identity_pub_hex": hex::encode(&msg.sender_identity_dh_pub),
                "sender_signing_pub_hex": hex::encode(&msg.sender_signing_pub),
                "payload_hex": hex::encode(&msg.payload),
            }).to_string();
            match CString::new(out) {
                Ok(cs) => cs.into_raw(),
                Err(e) => { set_error(&e.to_string()); ptr::null_mut() }
            }
        }
        Err(e) => { set_error(&e); ptr::null_mut() }
    }
}

// ─────────────────────────────────────────────────────────────────────────────
// Kütüphane Bilgisi
// ─────────────────────────────────────────────────────────────────────────────

/// Kütüphane versiyonu döndür
///
/// ÖNEMLI: Statik bellek döner — `obscura_free_string()` ile SERBEST BIRAKILMAMALI.
#[no_mangle]
pub extern "C" fn obscura_version() -> *const c_char {
    static VERSION: &[u8] = b"obscura-crypto/0.1.0\0";
    VERSION.as_ptr() as *const c_char
}

// ─────────────────────────────────────────────────────────────────────────────
// Testler — FFI sınırı üzerinden Double Ratchet roundtrip
// ─────────────────────────────────────────────────────────────────────────────

#[cfg(test)]
mod ffi_tests {
    use super::*;
    use std::ffi::CString;
    use x25519_dalek::{StaticSecret, PublicKey as X25519Public};

    /// extern fn'i Go gibi çağır: input JSON → output JSON string
    fn call(f: extern "C" fn(*const c_char) -> *mut c_char, input: &str) -> String {
        let c_in = CString::new(input).unwrap();
        let raw = f(c_in.as_ptr());
        assert!(!raw.is_null(), "FFI null döndürdü: {input}");
        let out = unsafe { CStr::from_ptr(raw).to_str().unwrap().to_string() };
        obscura_free_string(raw);
        out
    }

    #[test]
    fn ratchet_ffi_roundtrip() {
        // Bob'un ratchet anahtar çifti (X3DH sonrası SPK gibi)
        let bob_secret = StaticSecret::random_from_rng(rand::rngs::OsRng);
        let bob_pub = X25519Public::from(&bob_secret);
        let sk = "42".repeat(32); // 32 byte ortak sır (test)

        // Alice gönderen olarak başlar
        let mut alice_state = call(
            obscura_ratchet_init_sender,
            &serde_json::json!({
                "shared_key_hex": sk,
                "remote_ratchet_pub_hex": hex::encode(bob_pub.as_bytes()),
            }).to_string(),
        );

        // Bob alan olarak başlar
        let mut bob_state = call(
            obscura_ratchet_init_receiver,
            &serde_json::json!({
                "shared_key_hex": sk,
                "ratchet_priv_hex": hex::encode(bob_secret.to_bytes()),
            }).to_string(),
        );

        let ad_hex = hex::encode(b"conv-001");

        // Alice → Bob, birden fazla mesaj (state her seferinde güncellenir)
        for i in 0u8..5 {
            let pt = hex::encode([i; 48]);
            let enc_out = call(
                obscura_ratchet_encrypt,
                &serde_json::json!({
                    "state": serde_json::from_str::<serde_json::Value>(&alice_state).unwrap(),
                    "plaintext_hex": pt,
                    "ad_hex": ad_hex,
                }).to_string(),
            );
            let enc_v: serde_json::Value = serde_json::from_str(&enc_out).unwrap();
            alice_state = enc_v["state"].to_string();
            let msg_hex = enc_v["message_hex"].as_str().unwrap();

            let dec_out = call(
                obscura_ratchet_decrypt,
                &serde_json::json!({
                    "state": serde_json::from_str::<serde_json::Value>(&bob_state).unwrap(),
                    "message_hex": msg_hex,
                    "ad_hex": ad_hex,
                }).to_string(),
            );
            let dec_v: serde_json::Value = serde_json::from_str(&dec_out).unwrap();
            bob_state = dec_v["state"].to_string();
            assert_eq!(dec_v["plaintext_hex"].as_str().unwrap(), pt, "mesaj {i} eşleşmedi");
        }

        // Bob → Alice cevap (DH ratchet adımı tetiklenir, state serde'den geri yüklenmiş haliyle)
        let reply = hex::encode(b"B-to-A reply");
        let enc_out = call(
            obscura_ratchet_encrypt,
            &serde_json::json!({
                "state": serde_json::from_str::<serde_json::Value>(&bob_state).unwrap(),
                "plaintext_hex": reply,
                "ad_hex": ad_hex,
            }).to_string(),
        );
        let enc_v: serde_json::Value = serde_json::from_str(&enc_out).unwrap();
        let msg_hex = enc_v["message_hex"].as_str().unwrap();

        let dec_out = call(
            obscura_ratchet_decrypt,
            &serde_json::json!({
                "state": serde_json::from_str::<serde_json::Value>(&alice_state).unwrap(),
                "message_hex": msg_hex,
                "ad_hex": ad_hex,
            }).to_string(),
        );
        let dec_v: serde_json::Value = serde_json::from_str(&dec_out).unwrap();
        assert_eq!(dec_v["plaintext_hex"].as_str().unwrap(), reply, "DH ratchet cevabı çözülemedi");
    }

    #[test]
    fn sealed_sender_ffi_roundtrip() {
        use crate::identity::IdentityKeyPair;

        let sender = IdentityKeyPair::generate();
        let recipient = IdentityKeyPair::generate();
        let payload_hex = hex::encode(b"ffi sealed payload");

        // Seal
        let seal_out = call(
            obscura_sealed_sender_seal,
            &serde_json::json!({
                "sender_identity_json": sender.to_secure_json(),
                "recipient_identity_pub_hex": hex::encode(&recipient.dh_public),
                "payload_hex": payload_hex,
                "expires_at": 0,
            }).to_string(),
        );
        let seal_v: serde_json::Value = serde_json::from_str(&seal_out).unwrap();
        let envelope_hex = seal_v["envelope_hex"].as_str().unwrap();

        // Zarf gönderen kimliğini açık içermemeli (sunucu perspektifi)
        assert!(!envelope_hex.contains(&hex::encode(&sender.dh_public)),
            "Zarf hex'i gönderen kimlik anahtarını içeriyor");

        // Unseal
        let unseal_out = call(
            obscura_sealed_sender_unseal,
            &serde_json::json!({
                "recipient_identity_json": recipient.to_secure_json(),
                "envelope_hex": envelope_hex,
                "now": 1_800_000_000u64,
            }).to_string(),
        );
        let v: serde_json::Value = serde_json::from_str(&unseal_out).unwrap();
        assert_eq!(v["sender_did"].as_str().unwrap(), sender.did());
        assert_eq!(v["sender_identity_pub_hex"].as_str().unwrap(), hex::encode(&sender.dh_public));
        assert_eq!(v["payload_hex"].as_str().unwrap(), payload_hex);

        // Yanlış alıcı → NULL + hata
        let eve = IdentityKeyPair::generate();
        let c_in = CString::new(serde_json::json!({
            "recipient_identity_json": eve.to_secure_json(),
            "envelope_hex": envelope_hex,
            "now": 0,
        }).to_string()).unwrap();
        let raw = obscura_sealed_sender_unseal(c_in.as_ptr());
        assert!(raw.is_null(), "Yanlış alıcı NULL almalı");
    }

    #[test]
    fn null_pointer_inputs_return_error_not_crash() {
        // NULL girişler UB yerine kontrollü hata üretmeli
        assert!(obscura_identity_did_from_secure(ptr::null()).is_null());
        assert!(obscura_x3dh_accept(ptr::null()).is_null());
        assert!(obscura_ratchet_encrypt(ptr::null()).is_null());
        assert!(obscura_sealed_sender_seal(ptr::null()).is_null());
        assert!(obscura_sealed_sender_unseal(ptr::null()).is_null());
        assert_eq!(obscura_identity_verify(ptr::null(), ptr::null(), ptr::null()), 0);
        assert_eq!(obscura_zk_verify(ptr::null()), 0);

        // Hata mesajı set edilmiş olmalı
        let err = obscura_last_error();
        assert!(!err.is_null());
        obscura_free_string(err as *mut c_char);
    }
}
