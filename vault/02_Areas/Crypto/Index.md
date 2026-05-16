# Crypto Domain

**Kapsam:** Signal Protocol, MLS (openmls/RFC 9420), Circom ZK circuits, BIP39, ed25519/X25519.

## Kod konumu

- `E:\obscura\crypto\` — Rust crate (`obscura_crypto`)
  - `src/mls/` — openmls 0.6 wrapper (RFC 9420, ciphersuite MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519)
  - `src/mnemonic.rs` — BIP39 + identity derivation
  - `src/x3dh.rs`, `src/ratchet.rs` — Signal Protocol
  - `src/bin/mls-cli.rs` — Go backend için JSON-RPC subprocess
- `E:\obscura\circuits\` — Circom 2.1.6 devreler
  - `credit_threshold.circom` — 270 constraints (with v2 user_hash binding fix)
  - `identity_proof.circom` — 487 constraints
  - `message_integrity.circom` — 487 constraints
  - `storage_proof.circom` — 310 constraints (with proof_commitment binding)
  - `token_balance.circom` — 944 constraints (shielded transfer)
  - `vote_proof.circom` — 733 constraints (governance)
- `E:\obscura\backend\internal\zk\` — Go Groth16 verifier (go-rapidsnark, BN254)

## Sub-agent

- [[../../../.claude/agents/crypto-engineer|crypto-engineer]]
- [[../../../.claude/agents/zk-circuit-engineer|zk-circuit-engineer]]
- [[../../../.claude/agents/mls-engineer|mls-engineer]]

## Skill

- [[../../../.claude/skills/circom-zk-circuits/SKILL|circom-zk-circuits]]

## Sık başvurulan ADR'lar

- [[../../../docs/adr/0004-go-crypto-faz1|ADR-0004 Crypto Go (geçici)]]
- [[../../../docs/adr/0006-dev-trusted-setup|ADR-0006 Trusted setup]]
- [[../../../docs/adr/0007-openmls-for-groups|ADR-0007 openmls]]

## Kritik kurallar

- Custom crypto YASAK — Signal/MLS/Circom/snarkjs sadece
- Private key cihazdan çıkmaz, loglanmaz
- ZK circuit'te her public input MUTLAKA kısıtlanır (C3 bug öğretti)
- Trusted setup: dev tek-katılımcı, prod multi-party (ADR-0006)
