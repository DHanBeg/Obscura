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
use obscura_crypto::ratchet::{EncryptedMessage, MessageHeader, RatchetState};
use obscura_crypto::sealed_sender::{seal as sealed_seal, unseal as sealed_unseal};
use obscura_crypto::x3dh::{x3dh_accept, x3dh_initiate, PreKeyBundle};

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
