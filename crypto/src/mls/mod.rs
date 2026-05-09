//! Obscura MLS (RFC 9420) group encryption — openmls wrapper
//!
//! Spec: docs/spec/obscura_spec_v3.txt Bölüm 6.3, 14.1
//! Decision: docs/adr/0007-openmls-for-groups.md
//!
//! Ciphersuite: MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519 (0x0001)
//!
//! Minimal API:
//!   - generate_credential(did) -> Credential + signing key
//!   - generate_key_package(...)  -> KeyPackage
//!   - create_group(...)          -> MlsGroup
//!   - add_member(...)            -> (Welcome, Commit)
//!   - process_welcome(...)       -> MlsGroup
//!   - process_commit(...)        -> ()
//!   - encrypt_message(...)       -> MlsMessageOut
//!   - decrypt_message(...)       -> Vec<u8>

use openmls::prelude::*;
use openmls_basic_credential::SignatureKeyPair;
use openmls_rust_crypto::OpenMlsRustCrypto;
use openmls_traits::types::SignatureScheme;
use openmls_traits::OpenMlsProvider;
use thiserror::Error;

pub use openmls::prelude::{
    BasicCredential, CredentialWithKey, KeyPackage, MlsGroup,
    MlsMessageBodyIn, MlsMessageIn, MlsMessageOut, ProcessedMessageContent,
    Welcome,
};

/// Spec'in zorunlu kıldığı ciphersuite (Bölüm 4.1)
pub const OBSCURA_CIPHERSUITE: Ciphersuite =
    Ciphersuite::MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519;

#[derive(Debug, Error)]
pub enum MlsError {
    #[error("openmls error: {0}")]
    OpenMls(String),
    #[error("serialization error: {0}")]
    Serde(#[from] serde_json::Error),
    #[error("invalid input: {0}")]
    Invalid(String),
}

impl MlsError {
    fn from_debug<E: std::fmt::Debug>(e: E) -> Self {
        MlsError::OpenMls(format!("{:?}", e))
    }
}

pub type Result<T> = std::result::Result<T, MlsError>;

/// Identity for an Obscura user — DID + signature keypair.
/// Persisted client-side; the SignatureKeyPair private part NEVER leaves device.
pub struct Identity {
    pub did: String,
    pub credential_with_key: CredentialWithKey,
    pub signature_keys: SignatureKeyPair,
}

impl Identity {
    /// Generate a new identity for the given DID.
    pub fn generate(did: impl Into<String>, provider: &impl OpenMlsProvider) -> Result<Self> {
        let did = did.into();
        let credential = BasicCredential::new(did.as_bytes().to_vec());
        let signature_keys = SignatureKeyPair::new(SignatureScheme::ED25519)
            .map_err(MlsError::from_debug)?;

        // Store the signature keys in the provider's keystore so
        // openmls can retrieve them during commits/welcomes.
        signature_keys
            .store(provider.storage())
            .map_err(MlsError::from_debug)?;

        let credential_with_key = CredentialWithKey {
            credential: credential.into(),
            signature_key: signature_keys.public().into(),
        };

        Ok(Self { did, credential_with_key, signature_keys })
    }
}

/// Generate a new KeyPackage that other users can use to add this identity to a group.
/// Spec Bölüm 4.2: rotated periodically (90 days).
pub fn generate_key_package(
    identity: &Identity,
    provider: &impl OpenMlsProvider,
) -> Result<KeyPackage> {
    let kp_bundle = KeyPackage::builder()
        .build(
            OBSCURA_CIPHERSUITE,
            provider,
            &identity.signature_keys,
            identity.credential_with_key.clone(),
        )
        .map_err(MlsError::from_debug)?;
    Ok(kp_bundle.key_package().clone())
}

/// Create a new MLS group as the initial member.
pub fn create_group(
    identity: &Identity,
    provider: &impl OpenMlsProvider,
) -> Result<MlsGroup> {
    let group_config = MlsGroupCreateConfig::builder()
        .ciphersuite(OBSCURA_CIPHERSUITE)
        .use_ratchet_tree_extension(true)
        .build();

    let group = MlsGroup::new(
        provider,
        &identity.signature_keys,
        &group_config,
        identity.credential_with_key.clone(),
    )
    .map_err(MlsError::from_debug)?;

    Ok(group)
}

/// Add a member by their KeyPackage.
/// Returns the Commit (broadcast to existing members) and Welcome (sent to new member).
pub fn add_member(
    group: &mut MlsGroup,
    identity: &Identity,
    new_member_kp: KeyPackage,
    provider: &impl OpenMlsProvider,
) -> Result<(MlsMessageOut, MlsMessageOut)> {
    let (commit, welcome, _group_info) = group
        .add_members(provider, &identity.signature_keys, &[new_member_kp])
        .map_err(MlsError::from_debug)?;

    // Apply the commit so our local state advances to the new epoch.
    group
        .merge_pending_commit(provider)
        .map_err(MlsError::from_debug)?;

    Ok((commit, welcome))
}

/// Process an incoming Welcome to join a group.
pub fn process_welcome(
    welcome: Welcome,
    provider: &impl OpenMlsProvider,
) -> Result<MlsGroup> {
    let join_config = MlsGroupJoinConfig::builder()
        .use_ratchet_tree_extension(true)
        .build();

    let staged = StagedWelcome::new_from_welcome(provider, &join_config, welcome, None)
        .map_err(MlsError::from_debug)?;

    let group = staged
        .into_group(provider)
        .map_err(MlsError::from_debug)?;

    Ok(group)
}

/// Encrypt a message for the group.
pub fn encrypt_message(
    group: &mut MlsGroup,
    identity: &Identity,
    plaintext: &[u8],
    provider: &impl OpenMlsProvider,
) -> Result<MlsMessageOut> {
    group
        .create_message(provider, &identity.signature_keys, plaintext)
        .map_err(MlsError::from_debug)
}

/// Decrypt or process a received MLS message.
/// Returns the application plaintext, or None if it was a control message
/// (Commit/Proposal — state is updated as a side effect).
pub fn process_message(
    group: &mut MlsGroup,
    msg_in: MlsMessageIn,
    provider: &impl OpenMlsProvider,
) -> Result<Option<Vec<u8>>> {
    let protocol_message: ProtocolMessage = match msg_in.extract() {
        MlsMessageBodyIn::PrivateMessage(m) => m.into(),
        MlsMessageBodyIn::PublicMessage(m) => m.into(),
        other => return Err(MlsError::Invalid(format!("unexpected msg body: {:?}", other))),
    };

    let processed = group
        .process_message(provider, protocol_message)
        .map_err(MlsError::from_debug)?;

    match processed.into_content() {
        ProcessedMessageContent::ApplicationMessage(app) => {
            Ok(Some(app.into_bytes()))
        }
        ProcessedMessageContent::ProposalMessage(_proposal) => {
            // Buffered until Commit arrives
            Ok(None)
        }
        ProcessedMessageContent::StagedCommitMessage(staged) => {
            group
                .merge_staged_commit(provider, *staged)
                .map_err(MlsError::from_debug)?;
            Ok(None)
        }
        ProcessedMessageContent::ExternalJoinProposalMessage(_) => Ok(None),
    }
}

/// Provider factory — uses the in-memory keystore (RAM-only).
/// For persistent storage, swap with a file/SQLite-backed provider.
pub fn new_provider() -> OpenMlsRustCrypto {
    OpenMlsRustCrypto::default()
}

#[cfg(test)]
mod tests;
