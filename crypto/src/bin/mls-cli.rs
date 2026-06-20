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
//!   - add_members_bulk {group_id, identity_id, key_packages_b64: [...]} -> {commit_b64, welcome_b64, epoch}
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
use std::sync::{Arc, Mutex};

use base64::engine::general_purpose::STANDARD as B64;
use base64::Engine;
use openmls::prelude::tls_codec::{Deserialize, Serialize as TlsSerialize};
use openmls::prelude::{KeyPackageIn, MlsGroup, MlsMessageBodyIn, MlsMessageIn, ProtocolVersion};
use openmls_rust_crypto::OpenMlsRustCrypto;
use openmls_traits::OpenMlsProvider;
use serde::{Deserialize as SerdeDeserialize, Serialize};
use serde_json::{json, Value};

use obscura_crypto::mls::{
    add_member, add_members, create_group, encrypt_message, generate_key_package, new_provider,
    process_message, process_welcome, remove_member, self_update, Identity,
};
use obscura_crypto::mnemonic;

// ─── State ───────────────────────────────────────────────────────────────────

/// An identity bundle owns its private keys + the in-memory provider/keystore.
/// Wrapped in Arc so handlers can clone a cheap reference and release the
/// outer State lock before doing any crypto work.
struct IdentityBundle {
    identity: Identity,
    provider: OpenMlsRustCrypto,
}

/// A group bundle owns the MlsGroup state and remembers which identity created
/// it (for default signing on operations like process_message). Wrapped in
/// Arc<Mutex<…>> so concurrent ops on different groups never block each other
/// and we never have to hold the outer State lock across a crypto call.
struct GroupBundle {
    group: MlsGroup,
    owner_id: String,
}

struct State {
    /// Each identity has its own provider (in-memory keystore).
    identities: HashMap<String, Arc<IdentityBundle>>,
    /// Active groups, keyed by openmls group_id (b64).
    groups: HashMap<String, Arc<Mutex<GroupBundle>>>,
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

/// Look up an identity bundle, cloning the Arc and releasing the State lock.
fn get_identity(state: &Mutex<State>, id: &str) -> Result<Arc<IdentityBundle>, String> {
    let s = state.lock().unwrap();
    s.identities
        .get(id)
        .cloned()
        .ok_or_else(|| "identity not found".to_string())
}

/// Look up a group bundle Arc, cloning it and releasing the State lock.
fn get_group(state: &Mutex<State>, gid: &str) -> Result<Arc<Mutex<GroupBundle>>, String> {
    let s = state.lock().unwrap();
    s.groups
        .get(gid)
        .cloned()
        .ok_or_else(|| "group not found".to_string())
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
            let bundle = Arc::new(IdentityBundle { identity, provider });
            state.lock().unwrap().identities.insert(id.clone(), bundle);
            Ok(json!({"identity_id": id, "did": did}))
        }

        "generate_key_package" => {
            let id = params["identity_id"].as_str().ok_or("identity_id required")?;
            let bundle = get_identity(state, id)?;
            let kp = generate_key_package(&bundle.identity, &bundle.provider)
                .map_err(|e| e.to_string())?;
            let bytes = kp
                .tls_serialize_detached()
                .map_err(|e| format!("serialize kp: {}", e))?;
            Ok(json!({"key_package_b64": b64(&bytes)}))
        }

        "create_group" => {
            let id = params["identity_id"].as_str().ok_or("identity_id required")?;
            let bundle = get_identity(state, id)?;
            let group = create_group(&bundle.identity, &bundle.provider)
                .map_err(|e| e.to_string())?;
            let gid = group_id_str(&group);
            let group_bundle = Arc::new(Mutex::new(GroupBundle {
                group,
                owner_id: id.to_string(),
            }));
            state.lock().unwrap().groups.insert(gid.clone(), group_bundle);
            Ok(json!({"group_id": gid}))
        }

        "add_member" => {
            let gid = params["group_id"].as_str().ok_or("group_id required")?;
            let id = params["identity_id"].as_str().ok_or("identity_id required")?;
            let kp_b64 = params["key_package_b64"].as_str().ok_or("key_package_b64 required")?;
            let kp_bytes = unb64(kp_b64)?;

            let bundle = get_identity(state, id)?;
            let group_arc = get_group(state, gid)?;

            // KeyPackageIn is the unvalidated wire form; validate to get KeyPackage.
            let kp_in = KeyPackageIn::tls_deserialize(&mut kp_bytes.as_slice())
                .map_err(|e| format!("deserialize kp: {}", e))?;
            let kp = kp_in
                .validate(bundle.provider.crypto(), ProtocolVersion::default())
                .map_err(|e| format!("validate kp: {:?}", e))?;

            let mut g = group_arc.lock().unwrap();
            let (commit, welcome) =
                add_member(&mut g.group, &bundle.identity, kp, &bundle.provider)
                    .map_err(|e| e.to_string())?;

            let commit_bytes = commit
                .tls_serialize_detached()
                .map_err(|e| format!("ser commit: {}", e))?;
            let welcome_bytes = welcome
                .tls_serialize_detached()
                .map_err(|e| format!("ser welcome: {}", e))?;
            Ok(json!({
                "commit_b64": b64(&commit_bytes),
                "welcome_b64": b64(&welcome_bytes),
                "epoch": g.group.epoch().as_u64(),
            }))
        }

        "process_welcome" => {
            let id = params["identity_id"].as_str().ok_or("identity_id required")?;
            let welcome_b64 = params["welcome_b64"].as_str().ok_or("welcome_b64 required")?;
            let bytes = unb64(welcome_b64)?;

            let bundle = get_identity(state, id)?;

            // Parse incoming MLS message → expect Welcome
            let in_msg = MlsMessageIn::tls_deserialize(&mut bytes.as_slice())
                .map_err(|e| format!("deserialize welcome: {}", e))?;
            let welcome = match in_msg.extract() {
                MlsMessageBodyIn::Welcome(w) => w,
                other => return Err(format!("expected Welcome, got {:?}", other)),
            };

            let group = process_welcome(welcome, &bundle.provider).map_err(|e| e.to_string())?;
            let gid = group_id_str(&group);
            let epoch = group.epoch().as_u64();
            let group_bundle = Arc::new(Mutex::new(GroupBundle {
                group,
                owner_id: id.to_string(),
            }));
            state.lock().unwrap().groups.insert(gid.clone(), group_bundle);
            Ok(json!({"group_id": gid, "epoch": epoch}))
        }

        "encrypt" => {
            let gid = params["group_id"].as_str().ok_or("group_id required")?;
            let id = params["identity_id"].as_str().ok_or("identity_id required")?;
            let pt_b64 = params["plaintext_b64"].as_str().ok_or("plaintext_b64 required")?;
            let plaintext = unb64(pt_b64)?;

            let bundle = get_identity(state, id)?;
            let group_arc = get_group(state, gid)?;

            let mut g = group_arc.lock().unwrap();
            let ct = encrypt_message(&mut g.group, &bundle.identity, &plaintext, &bundle.provider)
                .map_err(|e| e.to_string())?;
            let bytes = ct
                .tls_serialize_detached()
                .map_err(|e| format!("ser ct: {}", e))?;
            Ok(json!({"ciphertext_b64": b64(&bytes)}))
        }

        "process_message" => {
            let gid = params["group_id"].as_str().ok_or("group_id required")?;
            let id_opt = params["identity_id"].as_str();
            let msg_b64 = params["message_b64"].as_str().ok_or("message_b64 required")?;
            let bytes = unb64(msg_b64)?;

            let group_arc = get_group(state, gid)?;
            // Determine which identity owns this group (or use override)
            let owner = match id_opt {
                Some(s) => s.to_string(),
                None => group_arc.lock().unwrap().owner_id.clone(),
            };
            let bundle = get_identity(state, &owner)?;

            let in_msg = MlsMessageIn::tls_deserialize(&mut bytes.as_slice())
                .map_err(|e| format!("deserialize msg: {}", e))?;

            let mut g = group_arc.lock().unwrap();
            let plaintext = process_message(&mut g.group, in_msg, &bundle.provider)
                .map_err(|e| e.to_string())?;

            Ok(json!({
                "plaintext_b64": plaintext.as_ref().map(|p| b64(p)),
                "epoch": g.group.epoch().as_u64(),
            }))
        }

        "add_members_bulk" => {
            let gid = params["group_id"].as_str().ok_or("group_id required")?;
            let id = params["identity_id"].as_str().ok_or("identity_id required")?;
            let kps_arr = params["key_packages_b64"].as_array().ok_or("key_packages_b64 required")?;

            let bundle = get_identity(state, id)?;
            let group_arc = get_group(state, gid)?;

            let mut kps = Vec::with_capacity(kps_arr.len());
            for kp_val in kps_arr {
                let kp_b64 = kp_val.as_str().ok_or("key_package must be string")?;
                let kp_bytes = unb64(kp_b64)?;
                let kp_in = KeyPackageIn::tls_deserialize(&mut kp_bytes.as_slice())
                    .map_err(|e| format!("deserialize kp: {}", e))?;
                let kp = kp_in
                    .validate(bundle.provider.crypto(), ProtocolVersion::default())
                    .map_err(|e| format!("validate kp: {:?}", e))?;
                kps.push(kp);
            }

            let mut g = group_arc.lock().unwrap();
            let (commit, welcome) = add_members(&mut g.group, &bundle.identity, &kps, &bundle.provider)
                .map_err(|e| e.to_string())?;
            let commit_bytes = commit.tls_serialize_detached().map_err(|e| format!("ser commit: {}", e))?;
            let welcome_bytes = welcome.tls_serialize_detached().map_err(|e| format!("ser welcome: {}", e))?;
            Ok(json!({
                "commit_b64": b64(&commit_bytes),
                "welcome_b64": b64(&welcome_bytes),
                "epoch": g.group.epoch().as_u64(),
                "added": kps_arr.len(),
            }))
        }

        "remove_member" => {
            let gid = params["group_id"].as_str().ok_or("group_id required")?;
            let id = params["identity_id"].as_str().ok_or("identity_id required")?;
            let leaf_index = params["leaf_index"].as_u64().ok_or("leaf_index required")? as u32;

            let bundle = get_identity(state, id)?;
            let group_arc = get_group(state, gid)?;

            let mut g = group_arc.lock().unwrap();
            let commit = remove_member(&mut g.group, &bundle.identity, leaf_index, &bundle.provider)
                .map_err(|e| e.to_string())?;
            let commit_bytes = commit.tls_serialize_detached().map_err(|e| format!("ser commit: {}", e))?;
            Ok(json!({
                "commit_b64": b64(&commit_bytes),
                "epoch": g.group.epoch().as_u64(),
            }))
        }

        "update_key" => {
            let gid = params["group_id"].as_str().ok_or("group_id required")?;
            let id = params["identity_id"].as_str().ok_or("identity_id required")?;

            let bundle = get_identity(state, id)?;
            let group_arc = get_group(state, gid)?;

            let mut g = group_arc.lock().unwrap();
            let commit = self_update(&mut g.group, &bundle.identity, &bundle.provider)
                .map_err(|e| e.to_string())?;
            let commit_bytes = commit.tls_serialize_detached().map_err(|e| format!("ser commit: {}", e))?;
            Ok(json!({
                "commit_b64": b64(&commit_bytes),
                "epoch": g.group.epoch().as_u64(),
            }))
        }

        "export_group" => {
            let gid = params["group_id"].as_str().ok_or("group_id required")?;
            let group_arc = get_group(state, gid)?;
            let g = group_arc.lock().unwrap();
            let owner_id = g.owner_id.clone();
            let epoch = g.group.epoch().as_u64();
            // Drop lock before getting identity
            drop(g);
            let bundle = get_identity(state, &owner_id)?;
            let g = group_arc.lock().unwrap();
            let msg = g.group
                .export_group_info(&bundle.provider, &bundle.identity.signature_keys, true)
                .map_err(|e| format!("export: {:?}", e))?;
            let bytes = msg
                .tls_serialize_detached()
                .map_err(|e| format!("ser export: {}", e))?;
            Ok(json!({
                "state_b64": b64(&bytes),
                "group_id": gid,
                "epoch": epoch,
                "owner_id": owner_id,
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
