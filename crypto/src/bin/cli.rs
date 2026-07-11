//! Obscura Crypto CLI — one-shot subprocess bridge for the Go backend.
//!
//! CGO_ENABLED=0 means Go cannot link the Rust crypto crate directly. This
//! binary is the workaround: Go spawns it with arguments, the CLI does the
//! crypto, and writes a single JSON object to stdout. On error it prints
//! `{"error": "..."}` and exits with code 1.
//!
//! All key material crosses the boundary as lowercase hex. Ratchet state
//! crosses as a JSON string (the serde representation of `RatchetState`); the
//! caller is responsible for storing it encrypted — the CLI never persists it.
//!
//! Commands (see `Command` below for the authoritative list):
//!   keygen
//!   prekey-bundle        --privkey <hex>
//!   x3dh-initiate        --privkey <hex> --remote-bundle <json>
//!   x3dh-respond         --privkey <hex> --ephemeral-pubkey <hex> --sender-identity-pubkey <hex>
//!   ratchet-init-sender  --shared-secret <hex> --remote-pubkey <hex>
//!   ratchet-init-receiver --shared-secret <hex> --our-privkey <hex>
//!   ratchet-encrypt      --state <json> --plaintext <hex>
//!   ratchet-decrypt      --state <json> --ciphertext <hex> --header <hex>
//!   seal                 --sender-identity-json <json> --recipient-identity-pub-hex <hex>
//!                         --payload-hex <hex> [--expires-at <u64>]
//!   unseal                --recipient-identity-json <json> --envelope-hex <hex> [--now <u64>]
//!
//! KEY FORMATS
//!   `keygen` emits a 64-byte privkey/pubkey: the first 32 bytes are the
//!   Ed25519 signing seed / verifying key, the last 32 bytes are the X25519
//!   DH private / public key. Commands that take `--privkey` accept either the
//!   full 64-byte blob or a bare 32-byte X25519 key and use the DH portion.
//!
//!   The signed prekey (SPK) is derived deterministically from the identity DH
//!   private key via HKDF, so the responder can reconstruct its SPK private key
//!   from `--privkey` alone. OPKs are not used by the CLI handshake (X3DH
//!   degrades gracefully to a 3-DH handshake per the Signal spec).

use clap::{Parser, Subcommand};
use ed25519_dalek::SigningKey;
use hkdf::Hkdf;
use rand::rngs::OsRng;
use sha2::Sha256;
use x25519_dalek::{PublicKey as X25519Public, StaticSecret};

use obscura_crypto::identity::IdentityKeyPair;
use obscura_crypto::ratchet::{kdf_ck, kdf_rk, EncryptedMessage, MessageHeader, RatchetState};
use obscura_crypto::sealed_sender::{seal as sealed_seal, unseal as sealed_unseal};
use obscura_crypto::x3dh::{x3dh_accept, x3dh_initiate, x3dh_initiate_with_ephemeral, PreKeyBundle};

#[derive(Parser)]
#[command(name = "obscura-crypto-cli", version, about = "Obscura Signal-protocol CLI bridge")]
struct Cli {
    #[command(subcommand)]
    command: Command,
}

#[derive(Subcommand)]
enum Command {
    /// Generate a new Ed25519 + X25519 identity keypair.
    Keygen,

    /// Generate a new identity keypair in IdentityKeyPair "secure JSON" form
    /// (mirrors `obscura_identity_new_secure` FFI) — the format `seal`/`unseal`
    /// require for `--sender-identity-json` / `--recipient-identity-json`.
    /// Distinct from `keygen`'s raw hex blob, which x3dh/ratchet commands use.
    IdentityNewSecure,

    /// Extract the hex-encoded public keys + DID from a secure-JSON identity
    /// (the format `seal`/`unseal` need `recipient-identity-pub-hex` in).
    IdentityPubHex {
        #[arg(long)]
        identity_json: String,
    },

    /// Produce a public PreKeyBundle from an identity private key.
    PrekeyBundle {
        #[arg(long)]
        privkey: String,
    },

    /// Alice side: run X3DH against a remote PreKeyBundle.
    X3dhInitiate {
        #[arg(long)]
        privkey: String,
        #[arg(long)]
        remote_bundle: String,
    },

    /// Bob side: complete X3DH from Alice's ephemeral + identity public keys.
    X3dhRespond {
        #[arg(long)]
        privkey: String,
        #[arg(long)]
        ephemeral_pubkey: String,
        #[arg(long)]
        sender_identity_pubkey: String,
    },

    /// Initialize a Double Ratchet session as the sender (Alice).
    RatchetInitSender {
        #[arg(long)]
        shared_secret: String,
        #[arg(long)]
        remote_pubkey: String,
    },

    /// Initialize a Double Ratchet session as the receiver (Bob).
    RatchetInitReceiver {
        #[arg(long)]
        shared_secret: String,
        #[arg(long)]
        our_privkey: String,
    },

    /// Encrypt a message, advancing the ratchet state.
    RatchetEncrypt {
        #[arg(long)]
        state: String,
        #[arg(long)]
        plaintext: String,
    },

    /// Decrypt a message, advancing the ratchet state.
    RatchetDecrypt {
        #[arg(long)]
        state: String,
        #[arg(long)]
        ciphertext: String,
        #[arg(long)]
        header: String,
    },

    /// Deterministic X3DH test vector: fixed ephemeral in, shared key out.
    /// For cross-implementation (Rust ↔ JS) verification — NOT for production.
    X3dhVector {
        #[arg(long)]
        alice_identity_priv: String,
        #[arg(long)]
        alice_ephemeral_priv: String,
        #[arg(long)]
        bob_identity_pub: String,
        #[arg(long)]
        bob_spk_pub: String,
        #[arg(long)]
        bob_opk_pub: Option<String>,
    },

    /// Deterministic KDF_RK test vector: (root_key, dh_output) → (new_rk, ck).
    RatchetKdfRkVector {
        #[arg(long)]
        root_key: String,
        #[arg(long)]
        dh_output: String,
    },

    /// Deterministic KDF_CK test vector: chain_key → (new_ck, message_key).
    RatchetKdfCkVector {
        #[arg(long)]
        chain_key: String,
    },

    /// Deterministic Double Ratchet session vector: fixed keys in, first-message
    /// root/chain/message keys + header out, with an internal roundtrip check.
    RatchetSessionVector {
        #[arg(long)]
        shared_key: String,
        #[arg(long)]
        bob_ratchet_pub: String,
        #[arg(long)]
        bob_ratchet_priv: String,
        #[arg(long)]
        alice_dhs_priv: String,
    },

    /// Sealed sender: build an envelope that hides the sender's identity from
    /// the server. Only the holder of `recipient-identity-pub-hex`'s matching
    /// private key can open it (see `unseal`).
    Seal {
        #[arg(long)]
        sender_identity_json: String,
        #[arg(long)]
        recipient_identity_pub_hex: String,
        #[arg(long)]
        payload_hex: String,
        #[arg(long, default_value_t = 0)]
        expires_at: u64,
    },

    /// Sealed sender: open an envelope produced by `seal`, recovering the
    /// sender's identity and the original payload. Requires the recipient's
    /// full identity keypair (private key material) — never call this with a
    /// server-held identity; sealed sender's entire purpose is that the
    /// server itself must NOT be able to unseal.
    Unseal {
        #[arg(long)]
        recipient_identity_json: String,
        #[arg(long)]
        envelope_hex: String,
        #[arg(long, default_value_t = 0)]
        now: u64,
    },
}

// ─── HKDF info labels ─────────────────────────────────────────────────────────
const SPK_DERIVE_INFO: &[u8] = b"ObscuraCLI-SignedPreKey-v1";

// ─── Helpers ───────────────────────────────────────────────────────────────────

fn decode_hex(field: &str, s: &str) -> Result<Vec<u8>, String> {
    hex::decode(s).map_err(|e| format!("{} geçersiz hex: {}", field, e))
}

fn as_32(field: &str, bytes: &[u8]) -> Result<[u8; 32], String> {
    bytes
        .try_into()
        .map_err(|_| format!("{} 32 byte olmalı (alınan: {})", field, bytes.len()))
}

/// Extract the 32-byte X25519 DH private key from a `--privkey` argument that is
/// either a bare 32-byte X25519 key or the 64-byte combined identity blob
/// (Ed25519 seed || X25519 priv).
fn dh_priv_from_arg(field: &str, hex_str: &str) -> Result<[u8; 32], String> {
    let bytes = decode_hex(field, hex_str)?;
    match bytes.len() {
        32 => as_32(field, &bytes),
        64 => as_32(field, &bytes[32..]),
        n => Err(format!("{} 32 veya 64 byte olmalı (alınan: {})", field, n)),
    }
}

/// Derive the deterministic signed-prekey X25519 private key from an identity
/// DH private key. Both the initiator's bundle view and the responder use this,
/// so the handshake stays symmetric without persisting the SPK separately.
fn derive_spk_priv(identity_dh_priv: &[u8; 32]) -> [u8; 32] {
    let hk = Hkdf::<Sha256>::new(None, identity_dh_priv);
    let mut okm = [0u8; 32];
    hk.expand(SPK_DERIVE_INFO, &mut okm)
        .expect("HKDF expand SPK türetme hatası");
    // X25519 clamps internally on use; StaticSecret::from stores raw bytes.
    okm
}

// ─── Command implementations ────────────────────────────────────────────────────

fn cmd_keygen() -> Result<serde_json::Value, String> {
    let signing = SigningKey::generate(&mut OsRng);
    let dh_secret = StaticSecret::random_from_rng(OsRng);
    let dh_public = X25519Public::from(&dh_secret);

    // privkey = Ed25519 seed (32) || X25519 priv (32)
    let mut privkey = Vec::with_capacity(64);
    privkey.extend_from_slice(&signing.to_bytes());
    privkey.extend_from_slice(&dh_secret.to_bytes());

    // pubkey = Ed25519 verifying (32) || X25519 pub (32)
    let mut pubkey = Vec::with_capacity(64);
    pubkey.extend_from_slice(signing.verifying_key().as_bytes());
    pubkey.extend_from_slice(dh_public.as_bytes());

    Ok(serde_json::json!({
        "privkey": hex::encode(&privkey),
        "pubkey": hex::encode(&pubkey),
    }))
}

fn cmd_identity_new_secure() -> Result<serde_json::Value, String> {
    let kp = IdentityKeyPair::generate();
    Ok(serde_json::json!({
        "identity_json": kp.to_secure_json(),
        "did": kp.did(),
    }))
}

fn cmd_identity_pub_hex(identity_json: &str) -> Result<serde_json::Value, String> {
    let kp = IdentityKeyPair::from_secure_json(identity_json).ok_or("identity-json yüklenemedi")?;
    Ok(serde_json::json!({
        "did": kp.did(),
        "dh_public_hex": hex::encode(&kp.dh_public),
        "signing_public_hex": hex::encode(&kp.signing_public),
    }))
}

fn cmd_prekey_bundle(privkey: &str) -> Result<serde_json::Value, String> {
    let bytes = decode_hex("privkey", privkey)?;
    if bytes.len() != 64 {
        return Err("privkey 64 byte olmalı (keygen çıktısı: ed25519_seed||x25519_priv)".into());
    }
    let ed_seed = as_32("ed25519_seed", &bytes[..32])?;
    let dh_priv = as_32("x25519_priv", &bytes[32..])?;

    let signing = SigningKey::from_bytes(&ed_seed);
    let dh_secret = StaticSecret::from(dh_priv);
    let identity_pub = X25519Public::from(&dh_secret);

    // Deterministic signed prekey from the identity DH private key.
    let spk_priv = derive_spk_priv(&dh_priv);
    let spk_secret = StaticSecret::from(spk_priv);
    let spk_pub = X25519Public::from(&spk_secret);

    // Sign the SPK public with the Ed25519 identity key.
    use ed25519_dalek::Signer;
    let spk_sig = signing.sign(spk_pub.as_bytes()).to_bytes();

    Ok(serde_json::json!({
        "identity_key": hex::encode(identity_pub.as_bytes()),
        "signed_prekey": hex::encode(spk_pub.as_bytes()),
        "signed_prekey_sig": hex::encode(spk_sig),
        // CLI handshake uses the OPK-less (3-DH) variant; field kept for shape.
        "one_time_prekeys": Vec::<String>::new(),
    }))
}

fn cmd_x3dh_initiate(privkey: &str, remote_bundle: &str) -> Result<serde_json::Value, String> {
    let alice_dh_priv = dh_priv_from_arg("privkey", privkey)?;

    let v: serde_json::Value =
        serde_json::from_str(remote_bundle).map_err(|e| format!("remote-bundle JSON hatası: {}", e))?;

    let identity_key = decode_hex(
        "remote-bundle.identity_key",
        v["identity_key"].as_str().ok_or("remote-bundle.identity_key eksik")?,
    )?;
    let signed_prekey = decode_hex(
        "remote-bundle.signed_prekey",
        v["signed_prekey"].as_str().ok_or("remote-bundle.signed_prekey eksik")?,
    )?;
    let signed_prekey_sig = decode_hex(
        "remote-bundle.signed_prekey_sig",
        v["signed_prekey_sig"].as_str().ok_or("remote-bundle.signed_prekey_sig eksik")?,
    )?;

    let bundle = PreKeyBundle {
        identity_key,
        signed_prekey,
        signed_prekey_sig,
        one_time_prekey: None,
        one_time_prekey_id: None,
        did: v["did"].as_str().unwrap_or_default().to_string(),
    };

    let result = x3dh_initiate(&alice_dh_priv, &bundle)?;

    Ok(serde_json::json!({
        "shared_secret": hex::encode(result.shared_key.0),
        "ephemeral_pubkey": hex::encode(&result.ephemeral_public),
    }))
}

fn cmd_x3dh_respond(
    privkey: &str,
    ephemeral_pubkey: &str,
    sender_identity_pubkey: &str,
) -> Result<serde_json::Value, String> {
    let bob_dh_priv = dh_priv_from_arg("privkey", privkey)?;
    let ephemeral = decode_hex("ephemeral-pubkey", ephemeral_pubkey)?;
    let sender_identity = decode_hex("sender-identity-pubkey", sender_identity_pubkey)?;

    // Re-derive the deterministic SPK private key from the identity DH key so
    // the DH set matches what the initiator computed against the bundle.
    let spk_priv = derive_spk_priv(&bob_dh_priv);

    let result = x3dh_accept(
        &bob_dh_priv,
        &spk_priv,
        None, // OPK-less variant (matches prekey-bundle output)
        &sender_identity,
        &ephemeral,
    )?;

    Ok(serde_json::json!({
        "shared_secret": hex::encode(result.shared_key.0),
    }))
}

fn cmd_ratchet_init_sender(shared_secret: &str, remote_pubkey: &str) -> Result<serde_json::Value, String> {
    let sk = as_32("shared-secret", &decode_hex("shared-secret", shared_secret)?)?;
    let remote = as_32("remote-pubkey", &decode_hex("remote-pubkey", remote_pubkey)?)?;
    let state = RatchetState::init_sender(&sk, &remote);
    let state_json = serde_json::to_string(&state).map_err(|e| format!("state serialize: {}", e))?;
    Ok(serde_json::json!({ "state": state_json }))
}

fn cmd_ratchet_init_receiver(shared_secret: &str, our_privkey: &str) -> Result<serde_json::Value, String> {
    let sk = as_32("shared-secret", &decode_hex("shared-secret", shared_secret)?)?;
    let our_priv = as_32("our-privkey", &dh_priv_from_arg("our-privkey", our_privkey)?)?;
    let state = RatchetState::init_receiver(&sk, &our_priv);
    let state_json = serde_json::to_string(&state).map_err(|e| format!("state serialize: {}", e))?;
    Ok(serde_json::json!({ "state": state_json }))
}

fn cmd_ratchet_encrypt(state: &str, plaintext: &str) -> Result<serde_json::Value, String> {
    let mut rs: RatchetState =
        serde_json::from_str(state).map_err(|e| format!("state deserialize: {}", e))?;
    let pt = decode_hex("plaintext", plaintext)?;

    // AD is empty at the CLI boundary; the ratchet header is always part of the
    // AEAD additional data internally, so messages remain header-authenticated.
    let msg = rs.encrypt(&pt, b"")?;

    let new_state = serde_json::to_string(&rs).map_err(|e| format!("state serialize: {}", e))?;
    Ok(serde_json::json!({
        "state": new_state,
        "ciphertext": hex::encode(&msg.ciphertext),
        "header": hex::encode(msg.header.to_bytes()),
    }))
}

fn cmd_ratchet_decrypt(state: &str, ciphertext: &str, header: &str) -> Result<serde_json::Value, String> {
    let mut rs: RatchetState =
        serde_json::from_str(state).map_err(|e| format!("state deserialize: {}", e))?;
    let ct = decode_hex("ciphertext", ciphertext)?;
    let hdr_bytes = decode_hex("header", header)?;

    let header = MessageHeader::from_bytes(&hdr_bytes).map_err(|e| e.to_string())?;
    let msg = EncryptedMessage { header, ciphertext: ct };

    let pt = rs.decrypt(&msg, b"")?;

    let new_state = serde_json::to_string(&rs).map_err(|e| format!("state serialize: {}", e))?;
    Ok(serde_json::json!({
        "state": new_state,
        "plaintext": hex::encode(&pt),
    }))
}

// ─── Deterministic test-vector commands (Rust ↔ JS cross-verification) ────────

/// Session vector sabitleri — JS tarafı aynı değerleri kullanmalı.
const VECTOR_PLAINTEXT: &[u8] = b"test";
const VECTOR_AD: &[u8] = b"vector-ad";

/// Serde ile serileştirilmiş `RatchetState` JSON'ından 32-byte'lık bir alanı
/// hex olarak çıkar (alanlar lib'de private; vektör üretimi için serde
/// temsilinden okunur). `[u8; 32]` → JSON sayı dizisi, `Option<[u8; 32]>`
/// Some(...) → yine sayı dizisi olarak serileşir.
fn state_field_hex(state: &serde_json::Value, field: &str) -> Result<String, String> {
    let arr = state[field]
        .as_array()
        .ok_or_else(|| format!("ratchet state '{}' alanı dizi değil", field))?;
    if arr.len() != 32 {
        return Err(format!("ratchet state '{}' alanı 32 byte olmalı", field));
    }
    let bytes = arr
        .iter()
        .map(|n| {
            n.as_u64()
                .filter(|v| *v <= 255)
                .map(|v| v as u8)
                .ok_or_else(|| format!("ratchet state '{}' alanında geçersiz byte", field))
        })
        .collect::<Result<Vec<u8>, String>>()?;
    Ok(hex::encode(bytes))
}

fn cmd_x3dh_vector(
    alice_identity_priv: &str,
    alice_ephemeral_priv: &str,
    bob_identity_pub: &str,
    bob_spk_pub: &str,
    bob_opk_pub: Option<&str>,
) -> Result<serde_json::Value, String> {
    let alice_id = as_32(
        "alice-identity-priv",
        &decode_hex("alice-identity-priv", alice_identity_priv)?,
    )?;
    let alice_eph = as_32(
        "alice-ephemeral-priv",
        &decode_hex("alice-ephemeral-priv", alice_ephemeral_priv)?,
    )?;
    let bob_ik = decode_hex("bob-identity-pub", bob_identity_pub)?;
    let bob_spk = decode_hex("bob-spk-pub", bob_spk_pub)?;
    let bob_opk = bob_opk_pub
        .map(|s| decode_hex("bob-opk-pub", s))
        .transpose()?;

    // Vektör için imza/did alanları anlamsız — shared key türetimine girmezler.
    let opk_id = bob_opk.as_ref().map(|_| 1u32);
    let bundle = PreKeyBundle {
        identity_key: bob_ik,
        signed_prekey: bob_spk,
        signed_prekey_sig: Vec::new(),
        one_time_prekey: bob_opk,
        one_time_prekey_id: opk_id,
        did: String::new(),
    };

    let result = x3dh_initiate_with_ephemeral(&alice_id, &alice_eph, &bundle)?;

    Ok(serde_json::json!({
        "ephemeral_public_hex": hex::encode(&result.ephemeral_public),
        "shared_key_hex": hex::encode(result.shared_key.0),
    }))
}

fn cmd_ratchet_kdf_rk_vector(root_key: &str, dh_output: &str) -> Result<serde_json::Value, String> {
    let rk = as_32("root-key", &decode_hex("root-key", root_key)?)?;
    let dh = as_32("dh-output", &decode_hex("dh-output", dh_output)?)?;

    let (new_rk, ck) = kdf_rk(&rk, &dh);

    Ok(serde_json::json!({
        "new_root_key_hex": hex::encode(new_rk),
        "chain_key_hex": hex::encode(ck),
    }))
}

fn cmd_ratchet_kdf_ck_vector(chain_key: &str) -> Result<serde_json::Value, String> {
    let ck = as_32("chain-key", &decode_hex("chain-key", chain_key)?)?;

    let (new_ck, mk) = kdf_ck(&ck);

    Ok(serde_json::json!({
        "new_chain_key_hex": hex::encode(new_ck),
        "message_key_hex": hex::encode(mk),
    }))
}

fn cmd_ratchet_session_vector(
    shared_key: &str,
    bob_ratchet_pub: &str,
    bob_ratchet_priv: &str,
    alice_dhs_priv: &str,
) -> Result<serde_json::Value, String> {
    let sk = as_32("shared-key", &decode_hex("shared-key", shared_key)?)?;
    let bob_pub = as_32("bob-ratchet-pub", &decode_hex("bob-ratchet-pub", bob_ratchet_pub)?)?;
    let bob_priv = as_32("bob-ratchet-priv", &decode_hex("bob-ratchet-priv", bob_ratchet_priv)?)?;
    let alice_dhs = as_32("alice-dhs-priv", &decode_hex("alice-dhs-priv", alice_dhs_priv)?)?;

    // Tutarlılık: verilen public, verilen private'tan türemeli.
    let derived_pub = X25519Public::from(&StaticSecret::from(bob_priv));
    if derived_pub.as_bytes() != &bob_pub {
        return Err("bob-ratchet-pub, bob-ratchet-priv'den türetilen public ile eşleşmiyor".into());
    }

    // Alice tarafı — deterministik init.
    let mut alice = RatchetState::init_sender_with_dhs(&sk, &bob_pub, &alice_dhs);

    // rk/cks lib'de private → serde temsilinden oku (deterministik değerler).
    let alice_state: serde_json::Value = serde_json::to_value(&alice)
        .map_err(|e| format!("state serialize: {}", e))?;
    let alice_rk_hex = state_field_hex(&alice_state, "rk")?;
    let alice_cks_hex = state_field_hex(&alice_state, "cks")?;

    // İlk mesajın message key'i: MK = KDF_CK(CKs_init).1 — kdf_ck saf olduğu
    // için encrypt'in içeride kullandığı anahtarla birebir aynıdır.
    let cks_bytes = as_32("cks", &decode_hex("cks", &alice_cks_hex)?)?;
    let (_next_ck, message_key) = kdf_ck(&cks_bytes);

    // Uçtan uca roundtrip: Alice şifreler, Bob (mevcut deterministik
    // init_receiver ile) çözer. Ciphertext AEAD nonce'u rastgele olduğundan
    // vektöre girmez; header (dhs_pub‖pn‖n) deterministiktir.
    let msg = alice.encrypt(VECTOR_PLAINTEXT, VECTOR_AD)?;
    let header_hex = hex::encode(msg.header.to_bytes());

    let mut bob = RatchetState::init_receiver(&sk, &bob_priv);
    let decrypted = bob.decrypt(&msg, VECTOR_AD)?;
    let roundtrip_ok = decrypted == VECTOR_PLAINTEXT;
    let decrypted_plaintext = String::from_utf8(decrypted)
        .map_err(|_| "çözülen metin UTF-8 değil")?;

    Ok(serde_json::json!({
        "alice_root_key_after_init_hex": alice_rk_hex,
        "alice_chain_key_after_init_hex": alice_cks_hex,
        "message_key_hex": hex::encode(message_key),
        "header_hex": header_hex,
        "decrypted_plaintext": decrypted_plaintext,
        "roundtrip_ok": roundtrip_ok,
    }))
}

fn cmd_seal(
    sender_identity_json: &str,
    recipient_identity_pub_hex: &str,
    payload_hex: &str,
    expires_at: u64,
) -> Result<serde_json::Value, String> {
    let sender = IdentityKeyPair::from_secure_json(sender_identity_json)
        .ok_or("sender-identity-json yüklenemedi")?;
    let recipient_pub = decode_hex("recipient-identity-pub-hex", recipient_identity_pub_hex)?;
    if recipient_pub.len() != 32 {
        return Err(format!(
            "recipient-identity-pub-hex 32 byte olmalı (alınan: {})",
            recipient_pub.len()
        ));
    }
    let payload = decode_hex("payload-hex", payload_hex)?;

    let envelope = sealed_seal(&sender, &recipient_pub, &payload, expires_at)?;

    Ok(serde_json::json!({
        "envelope_hex": hex::encode(&envelope),
    }))
}

fn cmd_unseal(recipient_identity_json: &str, envelope_hex: &str, now: u64) -> Result<serde_json::Value, String> {
    let recipient = IdentityKeyPair::from_secure_json(recipient_identity_json)
        .ok_or("recipient-identity-json yüklenemedi")?;
    let envelope = decode_hex("envelope-hex", envelope_hex)?;

    let msg = sealed_unseal(&recipient, &envelope, now)?;

    // NOTE: the underlying UnsealedMessage carries the sender's DH identity
    // key and Ed25519 signing key as two DISTINCT fields — they are different
    // keys, not one. Do not collapse them into a single hex field.
    Ok(serde_json::json!({
        "sender_did": msg.sender_did,
        "sender_identity_pub_hex": hex::encode(&msg.sender_identity_dh_pub),
        "sender_signing_pub_hex": hex::encode(&msg.sender_signing_pub),
        "payload_hex": hex::encode(&msg.payload),
    }))
}

fn run(cli: Cli) -> Result<serde_json::Value, String> {
    match cli.command {
        Command::Keygen => cmd_keygen(),
        Command::IdentityNewSecure => cmd_identity_new_secure(),
        Command::IdentityPubHex { identity_json } => cmd_identity_pub_hex(&identity_json),
        Command::PrekeyBundle { privkey } => cmd_prekey_bundle(&privkey),
        Command::X3dhInitiate { privkey, remote_bundle } => cmd_x3dh_initiate(&privkey, &remote_bundle),
        Command::X3dhRespond {
            privkey,
            ephemeral_pubkey,
            sender_identity_pubkey,
        } => cmd_x3dh_respond(&privkey, &ephemeral_pubkey, &sender_identity_pubkey),
        Command::RatchetInitSender { shared_secret, remote_pubkey } => {
            cmd_ratchet_init_sender(&shared_secret, &remote_pubkey)
        }
        Command::RatchetInitReceiver { shared_secret, our_privkey } => {
            cmd_ratchet_init_receiver(&shared_secret, &our_privkey)
        }
        Command::RatchetEncrypt { state, plaintext } => cmd_ratchet_encrypt(&state, &plaintext),
        Command::X3dhVector {
            alice_identity_priv,
            alice_ephemeral_priv,
            bob_identity_pub,
            bob_spk_pub,
            bob_opk_pub,
        } => cmd_x3dh_vector(
            &alice_identity_priv,
            &alice_ephemeral_priv,
            &bob_identity_pub,
            &bob_spk_pub,
            bob_opk_pub.as_deref(),
        ),
        Command::RatchetKdfRkVector { root_key, dh_output } => {
            cmd_ratchet_kdf_rk_vector(&root_key, &dh_output)
        }
        Command::RatchetKdfCkVector { chain_key } => cmd_ratchet_kdf_ck_vector(&chain_key),
        Command::RatchetSessionVector {
            shared_key,
            bob_ratchet_pub,
            bob_ratchet_priv,
            alice_dhs_priv,
        } => cmd_ratchet_session_vector(
            &shared_key,
            &bob_ratchet_pub,
            &bob_ratchet_priv,
            &alice_dhs_priv,
        ),
        Command::RatchetDecrypt { state, ciphertext, header } => {
            cmd_ratchet_decrypt(&state, &ciphertext, &header)
        }
        Command::Seal {
            sender_identity_json,
            recipient_identity_pub_hex,
            payload_hex,
            expires_at,
        } => cmd_seal(&sender_identity_json, &recipient_identity_pub_hex, &payload_hex, expires_at),
        Command::Unseal { recipient_identity_json, envelope_hex, now } => {
            cmd_unseal(&recipient_identity_json, &envelope_hex, now)
        }
    }
}

#[cfg(test)]
mod vector_tests {
    use super::*;

    // ── Sabit test-vektör girdileri ─────────────────────────────────────────
    // `openssl rand -hex 32` ile BİR KEZ üretilip sabitlendi. Değiştirmeyin:
    // test-vectors/x3dh_ratchet_vectors.json ve (Adım 4-5'te) JS testleri
    // bu değerlere bağlı.
    const ALICE_IDENTITY_PRIV: &str =
        "714df4cad99347b73b419bc3f02edb6c3069618c88324d542a7e74c763b0e90f";
    const ALICE_EPHEMERAL_PRIV: &str =
        "c3ebc8584471bb34db338abce2ade43c68c71a7668402ee0582155ebec52c1e7";
    const BOB_IDENTITY_PRIV: &str =
        "8fa24ecc43f9bcc61ce58fe8dc3dd0d84190aed95fad0fa01f9f598936be5271";
    const BOB_SPK_PRIV: &str =
        "b2f602e5a3c1b2ef53858129482667c47086d22b0ba2c627e98d10988f7f8763";
    const BOB_OPK_PRIV: &str =
        "40c14d6909cd517fedacd04d766e9fa51c3a519b7cc2df47932be88f74d4bb19";
    const KDF_RK_ROOT_KEY: &str =
        "48823b83fed9274c7c8d2329f663d93098fb105b14e3c1e938d472309ee491af";
    const KDF_RK_DH_OUTPUT: &str =
        "9d0f543e8e567d3d95405ca088395adfa285cb3f4087f6404d57e6a50ab76adf";
    const KDF_CK_CHAIN_KEY: &str =
        "6aaa656fb4ae7ad09c1fc1e9a5fbb79ed157d1784de45245bc09ada3fa16dc11";
    const SESSION_SHARED_KEY: &str =
        "07fbc0cf398b66f54f50f9a3e99404d633e9a06877f0726fc8f8dd557d447b93";
    const SESSION_BOB_RATCHET_PRIV: &str =
        "b2a062fff643222b0c04df55853a5a1e96d91682ad0abb730b4457ee50e693dc";
    const SESSION_ALICE_DHS_PRIV: &str =
        "575395668c9f062af40fea02a5aec55269f579e699017e566c2fffe00c2d8056";

    const FIXTURE_PATH: &str = concat!(
        env!("CARGO_MANIFEST_DIR"),
        "/test-vectors/x3dh_ratchet_vectors.json"
    );

    fn pub_hex_from_priv_hex(priv_hex: &str) -> String {
        let priv_bytes: [u8; 32] = hex::decode(priv_hex).unwrap().try_into().unwrap();
        let public = X25519Public::from(&StaticSecret::from(priv_bytes));
        hex::encode(public.as_bytes())
    }

    fn run_x3dh_vector() -> serde_json::Value {
        cmd_x3dh_vector(
            ALICE_IDENTITY_PRIV,
            ALICE_EPHEMERAL_PRIV,
            &pub_hex_from_priv_hex(BOB_IDENTITY_PRIV),
            &pub_hex_from_priv_hex(BOB_SPK_PRIV),
            Some(&pub_hex_from_priv_hex(BOB_OPK_PRIV)),
        )
        .unwrap()
    }

    fn run_session_vector() -> serde_json::Value {
        cmd_ratchet_session_vector(
            SESSION_SHARED_KEY,
            &pub_hex_from_priv_hex(SESSION_BOB_RATCHET_PRIV),
            SESSION_BOB_RATCHET_PRIV,
            SESSION_ALICE_DHS_PRIV,
        )
        .unwrap()
    }

    #[test]
    fn x3dh_vector_is_deterministic() {
        let first = run_x3dh_vector();
        let second = run_x3dh_vector();
        assert_eq!(first, second, "x3dh-vector: aynı girdi aynı çıktıyı üretmeli");
        assert_eq!(first["ephemeral_public_hex"].as_str().unwrap().len(), 64);
        assert_eq!(first["shared_key_hex"].as_str().unwrap().len(), 64);
    }

    #[test]
    fn x3dh_vector_cross_checks_with_accept() {
        // Alice deterministik ephemeral ile initiate eder; Bob mevcut
        // (bağımsız kod yolu) x3dh_accept ile AYNI shared_key'e ulaşmalı.
        let init = run_x3dh_vector();

        let bob_id_priv = hex::decode(BOB_IDENTITY_PRIV).unwrap();
        let bob_spk_priv = hex::decode(BOB_SPK_PRIV).unwrap();
        let bob_opk_priv = hex::decode(BOB_OPK_PRIV).unwrap();
        let alice_identity_pub =
            hex::decode(pub_hex_from_priv_hex(ALICE_IDENTITY_PRIV)).unwrap();
        let ephemeral_pub =
            hex::decode(init["ephemeral_public_hex"].as_str().unwrap()).unwrap();

        let accept = x3dh_accept(
            &bob_id_priv,
            &bob_spk_priv,
            Some(&bob_opk_priv),
            &alice_identity_pub,
            &ephemeral_pub,
        )
        .unwrap();

        assert_eq!(
            init["shared_key_hex"].as_str().unwrap(),
            hex::encode(accept.shared_key.0),
            "X3DH vektörü: initiate (deterministik) ve accept aynı shared_key'e ulaşmalı"
        );
    }

    #[test]
    fn kdf_rk_vector_is_deterministic() {
        let first = cmd_ratchet_kdf_rk_vector(KDF_RK_ROOT_KEY, KDF_RK_DH_OUTPUT).unwrap();
        let second = cmd_ratchet_kdf_rk_vector(KDF_RK_ROOT_KEY, KDF_RK_DH_OUTPUT).unwrap();
        assert_eq!(first, second, "kdf-rk-vector: aynı girdi aynı çıktıyı üretmeli");
        assert_eq!(first["new_root_key_hex"].as_str().unwrap().len(), 64);
        assert_eq!(first["chain_key_hex"].as_str().unwrap().len(), 64);
    }

    #[test]
    fn kdf_ck_vector_is_deterministic() {
        let first = cmd_ratchet_kdf_ck_vector(KDF_CK_CHAIN_KEY).unwrap();
        let second = cmd_ratchet_kdf_ck_vector(KDF_CK_CHAIN_KEY).unwrap();
        assert_eq!(first, second, "kdf-ck-vector: aynı girdi aynı çıktıyı üretmeli");
        assert_eq!(first["new_chain_key_hex"].as_str().unwrap().len(), 64);
        assert_eq!(first["message_key_hex"].as_str().unwrap().len(), 64);
    }

    #[test]
    fn session_vector_is_deterministic_and_roundtrips() {
        let first = run_session_vector();
        let second = run_session_vector();
        assert_eq!(first, second, "session-vector: aynı girdi aynı çıktıyı üretmeli");

        assert_eq!(first["roundtrip_ok"], true, "roundtrip_ok true olmalı");
        assert_eq!(
            first["decrypted_plaintext"].as_str().unwrap(),
            "test",
            "Bob'un çözdüğü metin 'test' olmalı"
        );
        // Header: 32 byte Alice dhs_pub || pn=0 || n=0 → 40 byte.
        let header_hex = first["header_hex"].as_str().unwrap();
        assert_eq!(header_hex.len(), 80, "header 40 byte olmalı");
        assert_eq!(
            &header_hex[..64],
            pub_hex_from_priv_hex(SESSION_ALICE_DHS_PRIV),
            "header'ın ilk 32 byte'ı Alice'in DHs public'i olmalı"
        );
        assert_eq!(&header_hex[64..], "0000000000000000", "pn=0, n=0 olmalı");
    }

    /// Fixture dosyası (Adım 4-5'te JS testlerinin okuyacağı tek kaynak)
    /// kodun bugün ürettiği çıktıyla birebir eşleşmeli — drift yakalayıcı.
    #[test]
    fn fixture_file_matches_generated_vectors() {
        let raw = std::fs::read_to_string(FIXTURE_PATH)
            .unwrap_or_else(|e| panic!("fixture okunamadı ({}): {}", FIXTURE_PATH, e));
        let fixture: serde_json::Value = serde_json::from_str(&raw).unwrap();

        let expected = build_fixture_json();
        assert_eq!(
            fixture, expected,
            "test-vectors/x3dh_ratchet_vectors.json kod ile drift etmiş — \
             `cargo test --bin obscura-crypto-cli print_fixture_json -- --ignored --nocapture` \
             çıktısıyla yeniden üretin"
        );
    }

    /// Fixture JSON'unu güncel koddan üret (girdiler + gerçek çıktılar).
    fn build_fixture_json() -> serde_json::Value {
        serde_json::json!({
            "description": "Deterministic X3DH + Double Ratchet cross-implementation vectors. Generated by obscura-crypto-cli; consumed by Rust tests (drift guard) and the mobile TypeScript implementation tests.",
            "x3dh_vector": {
                "input": {
                    "alice_identity_priv": ALICE_IDENTITY_PRIV,
                    "alice_ephemeral_priv": ALICE_EPHEMERAL_PRIV,
                    "bob_identity_priv": BOB_IDENTITY_PRIV,
                    "bob_spk_priv": BOB_SPK_PRIV,
                    "bob_opk_priv": BOB_OPK_PRIV,
                    "bob_identity_pub": pub_hex_from_priv_hex(BOB_IDENTITY_PRIV),
                    "bob_spk_pub": pub_hex_from_priv_hex(BOB_SPK_PRIV),
                    "bob_opk_pub": pub_hex_from_priv_hex(BOB_OPK_PRIV),
                },
                "output": run_x3dh_vector(),
            },
            "ratchet_kdf_rk_vector": {
                "input": {
                    "root_key": KDF_RK_ROOT_KEY,
                    "dh_output": KDF_RK_DH_OUTPUT,
                },
                "output": cmd_ratchet_kdf_rk_vector(KDF_RK_ROOT_KEY, KDF_RK_DH_OUTPUT).unwrap(),
            },
            "ratchet_kdf_ck_vector": {
                "input": {
                    "chain_key": KDF_CK_CHAIN_KEY,
                },
                "output": cmd_ratchet_kdf_ck_vector(KDF_CK_CHAIN_KEY).unwrap(),
            },
            "ratchet_session_vector": {
                "input": {
                    "shared_key": SESSION_SHARED_KEY,
                    "bob_ratchet_priv": SESSION_BOB_RATCHET_PRIV,
                    "bob_ratchet_pub": pub_hex_from_priv_hex(SESSION_BOB_RATCHET_PRIV),
                    "alice_dhs_priv": SESSION_ALICE_DHS_PRIV,
                    "plaintext": "test",
                    "ad": "vector-ad",
                },
                "output": run_session_vector(),
            },
        })
    }

    /// Fixture'ı yeniden üretmek için yardımcı (normal suite'te koşmaz):
    /// `cargo test --bin obscura-crypto-cli print_fixture_json -- --ignored --nocapture`
    #[test]
    #[ignore]
    fn print_fixture_json() {
        println!(
            "{}",
            serde_json::to_string_pretty(&build_fixture_json()).unwrap()
        );
    }
}

fn main() {
    let cli = Cli::parse();
    match run(cli) {
        Ok(value) => {
            println!("{}", value);
        }
        Err(e) => {
            let err = serde_json::json!({ "error": e });
            eprintln!("{}", err);
            println!("{}", err);
            std::process::exit(1);
        }
    }
}
