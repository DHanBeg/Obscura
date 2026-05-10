//! Obscura MLS CLI — JSON-RPC subprocess bridge for Go backend.
//!
//! Reads one JSON command per line on stdin, writes one JSON response per line on stdout.
//! Designed for CGO_ENABLED=0 (subprocess instead of FFI link).
//!
//! Persistent process: Go opens this once per session and pipes commands.
//! State (groups, identities, key packages) lives in this process for the session.
//!
//! Wire protocol:
//!   request:  {"id": "uuid", "op": "create_group", "params": {...}}
//!   response: {"id": "uuid", "ok": true, "result": {...}}
//!         OR  {"id": "uuid", "ok": false, "error": "msg"}
//!
//! Operations:
//!   - new_identity {did} -> {identity_id}
//!   - generate_key_package {identity_id} -> {key_package_b64}
//!   - create_group {identity_id} -> {group_id}
//!   - add_member {group_id, identity_id, key_package_b64} -> {commit_b64, welcome_b64}
//!   - process_welcome {identity_id, welcome_b64} -> {group_id}
//!   - encrypt {group_id, identity_id, plaintext_b64} -> {ciphertext_b64}
//!   - process_message {group_id, message_b64} -> {plaintext_b64 | null}
//!   - export_group {group_id} -> {state_b64}        (for persistence by Go)
//!   - import_group {state_b64} -> {group_id}        (restore from Go)
//!   - drop_group {group_id} -> {}
//!   - drop_identity {identity_id} -> {}
//!   - ping -> {pong: true}

use std::collections::HashMap;
use std::io::{self, BufRead, Write};
use std::sync::Mutex;

use base64::engine::general_purpose::STANDARD as B64;
use base64::Engine;
use openmls::prelude::tls_codec::{Deserialize, Serialize as TlsSerialize};
use openmls::prelude::{KeyPackageIn, MlsGroup, MlsMessageBodyIn, MlsMessageIn, ProtocolVersion};
use openmls_rust_crypto::OpenMlsRustCrypto;
use openmls_traits::OpenMlsProvider;
use serde::{Deserialize as SerdeDeserialize, Serialize};
use serde_json::{json, Value};

use obscura_crypto::mls::{
    add_member, create_group, encrypt_message, generate_key_package, new_provider,
    process_message, process_welcome, Identity,
};
use obscura_crypto::mnemonic;

// ─── State ───────────────────────────────────────────────────────────────────

struct State {
    /// Each identity has its own provider (in-memory keystore)
    identities: HashMap<String, (Identity, OpenMlsRustCrypto)>,
    /// Active groups, keyed by openmls group_id (b64)
    /// Each group remembers which identity it belongs to for signing operations.
    groups: HashMap<String, (MlsGroup, String /* identity_id */)>,
}

impl State {
    fn new() -> Self {
        Self {
            identities: HashMap::new(),
            groups: HashMap::new(),
        }
    }
}

// ─── Wire types ──────────────────────────────────────────────────────────────

#[derive(SerdeDeserialize)]
struct Request {
    #[serde(default)]
    id: String,
    op: String,
    #[serde(default)]
    params: Value,
}

#[derive(Serialize)]
struct Response<T: Serialize> {
    id: String,
    ok: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    result: Option<T>,
    #[serde(skip_serializing_if = "Option::is_none")]
    error: Option<String>,
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

fn b64(bytes: &[u8]) -> String { B64.encode(bytes) }
fn unb64(s: &str) -> Result<Vec<u8>, String> {
    B64.decode(s).map_err(|e| format!("invalid base64: {}", e))
}

fn group_id_str(group: &MlsGroup) -> String {
    b64(group.group_id().as_slice())
}

// ─── Op handlers ─────────────────────────────────────────────────────────────

fn handle(state: &Mutex<State>, op: &str, params: &Value) -> Result<Value, String> {
    match op {
        "ping" => Ok(json!({"pong": true, "version": env!("CARGO_PKG_VERSION")})),

        "new_identity" => {
            let did = params["did"].as_str().ok_or("did required")?.to_string();
            let provider = new_provider();
            let identity = Identity::generate(&did, &provider).map_err(|e| e.to_string())?;
            let id = format!("id_{}", did);
            state.lock().unwrap().identities.insert(id.clone(), (identity, provider));
            Ok(json!({"identity_id": id, "did": did}))
        }

        "generate_key_package" => {
            let id = params["identity_id"].as_str().ok_or("identity_id required")?;
            let mut s = state.lock().unwrap();
            let (identity, provider) = s.identities.get(id).ok_or("identity not found")?;
            let kp = generate_key_package(identity, provider).map_err(|e| e.to_string())?;
            let bytes = kp.tls_serialize_detached().map_err(|e| format!("serialize kp: {}", e))?;
            Ok(json!({"key_package_b64": b64(&bytes)}))
        }

        "create_group" => {
            let id = params["identity_id"].as_str().ok_or("identity_id required")?;
            let mut s = state.lock().unwrap();
            let (identity, provider) = s.identities.get(id).ok_or("identity not found")?;
            let group = create_group(identity, provider).map_err(|e| e.to_string())?;
            let gid = group_id_str(&group);
            s.groups.insert(gid.clone(), (group, id.to_string()));
            Ok(json!({"group_id": gid}))
        }

        "add_member" => {
            let gid = params["group_id"].as_str().ok_or("group_id required")?;
            let id = params["identity_id"].as_str().ok_or("identity_id required")?;
            let kp_b64 = params["key_package_b64"].as_str().ok_or("key_package_b64 required")?;
            let kp_bytes = unb64(kp_b64)?;

            let mut s = state.lock().unwrap();
            // KeyPackageIn is the unvalidated wire form; validate to get KeyPackage.
            let kp_in = KeyPackageIn::tls_deserialize(&mut kp_bytes.as_slice())
                .map_err(|e| format!("deserialize kp: {}", e))?;

            // Need identity provider too — get it first, then group
            let (identity, provider) = s.identities.get(id)
                .ok_or("identity not found")?;
            let kp = kp_in
                .validate(provider.crypto(), ProtocolVersion::default())
                .map_err(|e| format!("validate kp: {:?}", e))?;
            // Clone what we need before mutating s.groups
            let identity = identity as *const Identity;
            let provider = provider as *const OpenMlsRustCrypto;

            // SAFETY: identity and provider are owned by `s` which we hold mutably.
            // We only borrow them immutably during this call.
            let identity = unsafe { &*identity };
            let provider = unsafe { &*provider };

            let (group, _owner) = s.groups.get_mut(gid).ok_or("group not found")?;
            let (commit, welcome) = add_member(group, identity, kp, provider)
                .map_err(|e| e.to_string())?;

            let commit_bytes = commit.tls_serialize_detached().map_err(|e| format!("ser commit: {}", e))?;
            let welcome_bytes = welcome.tls_serialize_detached().map_err(|e| format!("ser welcome: {}", e))?;
            Ok(json!({
                "commit_b64": b64(&commit_bytes),
                "welcome_b64": b64(&welcome_bytes),
                "epoch": group.epoch().as_u64(),
            }))
        }

        "process_welcome" => {
            let id = params["identity_id"].as_str().ok_or("identity_id required")?;
            let welcome_b64 = params["welcome_b64"].as_str().ok_or("welcome_b64 required")?;
            let bytes = unb64(welcome_b64)?;

            let mut s = state.lock().unwrap();
            let (_identity, provider) = s.identities.get(id).ok_or("identity not found")?;

            // Parse incoming MLS message → expect Welcome
            let in_msg = MlsMessageIn::tls_deserialize(&mut bytes.as_slice())
                .map_err(|e| format!("deserialize welcome: {}", e))?;
            let welcome = match in_msg.extract() {
                MlsMessageBodyIn::Welcome(w) => w,
                other => return Err(format!("expected Welcome, got {:?}", other)),
            };

            let provider_ref = provider as *const OpenMlsRustCrypto;
            let provider_ref = unsafe { &*provider_ref };
            let group = process_welcome(welcome, provider_ref).map_err(|e| e.to_string())?;
            let gid = group_id_str(&group);
            let epoch = group.epoch().as_u64();
            s.groups.insert(gid.clone(), (group, id.to_string()));
            Ok(json!({"group_id": gid, "epoch": epoch}))
        }

        "encrypt" => {
            let gid = params["group_id"].as_str().ok_or("group_id required")?;
            let id = params["identity_id"].as_str().ok_or("identity_id required")?;
            let pt_b64 = params["plaintext_b64"].as_str().ok_or("plaintext_b64 required")?;
            let plaintext = unb64(pt_b64)?;

            let mut s = state.lock().unwrap();
            let (identity, provider) = s.identities.get(id).ok_or("identity not found")?;
            let identity = identity as *const Identity;
            let provider = provider as *const OpenMlsRustCrypto;
            let identity = unsafe { &*identity };
            let provider = unsafe { &*provider };

            let (group, _) = s.groups.get_mut(gid).ok_or("group not found")?;
            let ct = encrypt_message(group, identity, &plaintext, provider)
                .map_err(|e| e.to_string())?;
            let bytes = ct.tls_serialize_detached().map_err(|e| format!("ser ct: {}", e))?;
            Ok(json!({"ciphertext_b64": b64(&bytes)}))
        }

        "process_message" => {
            let gid = params["group_id"].as_str().ok_or("group_id required")?;
            let id_opt = params["identity_id"].as_str();
            let msg_b64 = params["message_b64"].as_str().ok_or("message_b64 required")?;
            let bytes = unb64(msg_b64)?;

            let mut s = state.lock().unwrap();
            // Determine which identity owns this group
            let owner = s.groups.get(gid)
                .map(|(_, o)| o.clone())
                .ok_or("group not found")?;
            let owner = id_opt.map(|s| s.to_string()).unwrap_or(owner);
            let (_identity, provider) = s.identities.get(&owner).ok_or("identity not found")?;
            let provider = provider as *const OpenMlsRustCrypto;
            let provider = unsafe { &*provider };

            let in_msg = MlsMessageIn::tls_deserialize(&mut bytes.as_slice())
                .map_err(|e| format!("deserialize msg: {}", e))?;

            let (group, _) = s.groups.get_mut(gid).ok_or("group not found")?;
            let plaintext = process_message(group, in_msg, provider)
                .map_err(|e| e.to_string())?;

            Ok(json!({
                "plaintext_b64": plaintext.as_ref().map(|p| b64(p)),
                "epoch": group.epoch().as_u64(),
            }))
        }

        "drop_group" => {
            let gid = params["group_id"].as_str().ok_or("group_id required")?;
            state.lock().unwrap().groups.remove(gid);
            Ok(json!({"removed": true}))
        }

        "drop_identity" => {
            let id = params["identity_id"].as_str().ok_or("identity_id required")?;
            state.lock().unwrap().identities.remove(id);
            Ok(json!({"removed": true}))
        }

        "list" => {
            let s = state.lock().unwrap();
            Ok(json!({
                "identities": s.identities.keys().collect::<Vec<_>>(),
                "groups": s.groups.keys().collect::<Vec<_>>(),
            }))
        }

        // ─── BIP39 Mnemonic ──────────────────────────────────────────────────
        "mnemonic_generate" => {
            let m = mnemonic::generate();
            Ok(json!({"mnemonic": m, "word_count": m.split_whitespace().count()}))
        }

        "mnemonic_validate" => {
            let phrase = params["phrase"].as_str().ok_or("phrase required")?;
            mnemonic::validate(phrase).map_err(|e| e.to_string())?;
            Ok(json!({"valid": true}))
        }

        "mnemonic_derive_did" => {
            let phrase = params["phrase"].as_str().ok_or("phrase required")?;
            let pass = params["passphrase"].as_str();
            let did = mnemonic::derive_did(phrase, pass).map_err(|e| e.to_string())?;
            let secret = mnemonic::derive_identity_secret(phrase, pass)
                .map_err(|e| e.to_string())?;
            let (priv_b, pub_b) = mnemonic::derive_ed25519(phrase, pass)
                .map_err(|e| e.to_string())?;
            Ok(json!({
                "did": did,
                "identity_secret_b64": b64(&secret),
                "ed25519_public_b64": b64(&pub_b),
                "ed25519_private_b64": b64(&priv_b),
            }))
        }

        other => Err(format!("unknown op: {}", other)),
    }
}

fn main() {
    let state = Mutex::new(State::new());
    let stdin = io::stdin();
    let mut stdout = io::stdout().lock();

    // Greeting line so Go knows we're ready
    let _ = writeln!(stdout, r#"{{"ready":true,"version":"{}"}}"#, env!("CARGO_PKG_VERSION"));
    let _ = stdout.flush();

    for line_res in stdin.lock().lines() {
        let line = match line_res {
            Ok(l) if l.trim().is_empty() => continue,
            Ok(l) => l,
            Err(_) => break,
        };

        let req: Request = match serde_json::from_str(&line) {
            Ok(r) => r,
            Err(e) => {
                let resp = Response::<()> {
                    id: String::new(),
                    ok: false,
                    result: None,
                    error: Some(format!("bad request: {}", e)),
                };
                let _ = writeln!(stdout, "{}", serde_json::to_string(&resp).unwrap());
                let _ = stdout.flush();
                continue;
            }
        };

        let resp = match handle(&state, &req.op, &req.params) {
            Ok(value) => Response {
                id: req.id,
                ok: true,
                result: Some(value),
                error: None,
            },
            Err(e) => Response {
                id: req.id,
                ok: false,
                result: None,
                error: Some(e),
            },
        };

        let _ = writeln!(stdout, "{}", serde_json::to_string(&resp).unwrap());
        let _ = stdout.flush();
    }
}
