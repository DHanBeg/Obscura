//! WASM bindings — Browser ZK proof generation
//!
//! Build: `wasm-pack build --target web --features wasm --out-dir ../frontend/public/wasm-zk`
//!
//! Sıfır dış servis: tüm proof üretimi tarayıcıda yapılır.
//! Bu modül `zk_stub` üzerine ince bir köprü — gerçek Groth16 entegrasyonu
//! geldiğinde sadece içerideki `prove_*` çağrıları değişir.
//!
//! NOT: Bu modül `wasm` feature flag'i altında. Native build için derlenmez.
//!
//! API:
//!   - `generate_credit_proof_wasm(score, threshold, secret)` — kredi eşiği
//!   - `generate_identity_proof_wasm(secret, did_private)`    — DID sahipliği
//!   - `verify_proof_wasm(proof_json, vkey_json, public)`     — istemci-tarafı doğrulama

#![cfg(feature = "wasm")]

use wasm_bindgen::prelude::*;
use sha2::{Sha256, Digest};

use crate::zk_stub::{
    CreditWitness, IdentityWitness, ZkProof,
    prove_credit_threshold, prove_identity, verify_proof,
};

// ─────────────────────────────────────────────────────────────────────────────
// Yardımcılar
// ─────────────────────────────────────────────────────────────────────────────

/// Panic'leri tarayıcı console'una yansıt (debug için)
#[wasm_bindgen(start)]
pub fn __obscura_wasm_init() {
    // No-op start hook — call sites can grow (console_error_panic_hook vs.)
    // Şu an minimal; bağımlılık eklemeden init noktası tutuluyor.
}

/// Browser Date.now() / 1000 yerine deterministik (ms olmayan) zaman
/// — caller JS'ten `Math.floor(Date.now()/1000)` geçebilir; geçmezse 0.
fn now_secs(ts: Option<u64>) -> u64 {
    ts.unwrap_or(0)
}

/// 32-byte secret'tan deterministik nonce/salt türet (HKDF değil, hızlı)
fn derive_32(secret: &str, tag: &[u8]) -> [u8; 32] {
    let mut h = Sha256::new();
    h.update(tag);
    h.update(secret.as_bytes());
    let out = h.finalize();
    let mut buf = [0u8; 32];
    buf.copy_from_slice(&out);
    buf
}

/// JsValue olarak Ok/Err response döndüren wrapper
fn ok_json<T: serde::Serialize>(v: &T) -> Result<JsValue, JsValue> {
    serde_wasm_serialize(v).map_err(|e| JsValue::from_str(&e))
}

fn serde_wasm_serialize<T: serde::Serialize>(v: &T) -> Result<JsValue, String> {
    // serde-wasm-bindgen yerine JSON string round-trip (ek dep yok)
    let s = serde_json::to_string(v).map_err(|e| e.to_string())?;
    Ok(JsValue::from_str(&s))
}

// ─────────────────────────────────────────────────────────────────────────────
// Public API
// ─────────────────────────────────────────────────────────────────────────────

/// Kredi eşiği ZK kanıtı üret (browser)
///
/// Parametreler:
///   - `score`     — gerçek kredi puanı (private)
///   - `threshold` — aşılması iddia edilen eşik (public)
///   - `secret`    — kullanıcıya özel gizli string (mnemonic'den türetilir)
///
/// Returns: ZkProof JSON (snarkjs uyumlu shape) JsValue olarak.
#[wasm_bindgen]
pub fn generate_credit_proof_wasm(
    score: u32,
    threshold: u32,
    secret: &str,
    timestamp: Option<u64>,
    prover_did: &str,
) -> Result<JsValue, JsValue> {
    if secret.is_empty() {
        return Err(JsValue::from_str("secret boş olamaz"));
    }
    let salt = derive_32(secret, b"OBSCURA_WASM_CREDIT_SALT_V1");
    let witness = CreditWitness {
        actual_score: score as i32,
        threshold: threshold as i32,
        salt,
    };
    let proof = prove_credit_threshold(&witness, prover_did, now_secs(timestamp))
        .map_err(|e| JsValue::from_str(&e))?;
    ok_json(&proof)
}

/// Kimlik (DID sahipliği) ZK kanıtı üret (browser)
///
/// Parametreler:
///   - `secret`      — kullanıcı master secret (nonce türetimi için)
///   - `did_private` — 32-byte private key hex string
///   - `did`         — public DID (`did:obs:...`)
#[wasm_bindgen]
pub fn generate_identity_proof_wasm(
    secret: &str,
    did_private_hex: &str,
    did: &str,
    timestamp: Option<u64>,
) -> Result<JsValue, JsValue> {
    if did_private_hex.len() != 64 {
        return Err(JsValue::from_str("did_private 64-char hex olmalı (32 byte)"));
    }
    let private_key = hex::decode(did_private_hex)
        .map_err(|e| JsValue::from_str(&format!("hex decode: {e}")))?;
    let nonce = derive_32(secret, b"OBSCURA_WASM_IDENTITY_NONCE_V1");
    let witness = IdentityWitness {
        private_key,
        did: did.to_string(),
        nonce,
    };
    let proof = prove_identity(&witness, now_secs(timestamp))
        .map_err(|e| JsValue::from_str(&e))?;
    ok_json(&proof)
}

/// İstemci-tarafı doğrulama — sunucudan dönen kanıtı tarayıcıda double-check
///
/// Parametreler:
///   - `proof_json`     — `ZkProof` JSON string
///   - `vkey_json`      — verification key (Groth16 entegrasyonu sonrası kullanılır,
///                        stub modunda format kontrolü için)
///   - `public_inputs`  — JSON array string, vkey doğrulamasında kullanılır
///
/// Returns: bool — geçerli mi?
#[wasm_bindgen]
pub fn verify_proof_wasm(
    proof_json: &str,
    vkey_json: &str,
    public_inputs: &str,
) -> bool {
    // vkey ve public_inputs şimdilik parse-only sanity check
    // (Groth16 entegrasyonunda gerçek pairing buraya gelecek)
    if vkey_json.is_empty() || public_inputs.is_empty() {
        return false;
    }
    if serde_json::from_str::<serde_json::Value>(vkey_json).is_err() {
        return false;
    }
    if serde_json::from_str::<serde_json::Value>(public_inputs).is_err() {
        return false;
    }
    let proof = match ZkProof::from_json(proof_json) {
        Ok(p) => p,
        Err(_) => return false,
    };
    verify_proof(&proof).unwrap_or(false)
}

/// Build bilgisi — JS tarafından "doğru sürüm yüklendi mi?" kontrolü
#[wasm_bindgen]
pub fn obscura_wasm_version() -> String {
    format!("obscura-crypto-wasm/{}", env!("CARGO_PKG_VERSION"))
}
