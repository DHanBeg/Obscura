# Security Policy

## Supported versions

Currently in pre-release. Once shipped, last 2 minor versions receive security patches.

## Reporting a vulnerability

**Do not file public GitHub issues for security vulnerabilities.**

Email: security@obscura.network (PGP key TBD)

Include:
- Description of the vulnerability
- Steps to reproduce
- Impact assessment
- Suggested fix (optional)

We aim to:
- Acknowledge within 48 hours
- Initial assessment within 7 days
- Patch deployed within 30 days for high severity

## Bug bounty

Once mainnet:
- Critical: up to $50,000 OBS equivalent
- High: up to $10,000
- Medium: up to $2,500
- Low: $250-$1,000

Scope: production node binaries, official clients, ZK circuits, smart contracts.

Out of scope: third-party dependencies (file with upstream), DoS without proven impact, missing security headers on docs sites.

## Security architecture

See:
- `docs/spec/obscura_spec_v3.txt` Bölüm 4 (Cryptography), Bölüm 4.5 (KESIN security rules)
- `.claude/agents/security-auditor.md` (audit checklist)
- `docs/protocols/` (Signal, MLS, ZK flow diagrams)

## Cryptographic primitives

- Ed25519 (signatures)
- X25519 (DH)
- AES-256-GCM (symmetric)
- SHA-256, SHA-3, Poseidon (hash)
- Groth16 over BN254 (ZK proofs)
- Signal Protocol (X3DH + Double Ratchet) for 1-1 E2EE
- MLS (RFC 9420) for group E2EE
- Future (FAZ 4): CRYSTALS-Kyber (KEM), CRYSTALS-Dilithium (signatures)

We do not roll our own primitives.

## Trusted setup

For production circuits, multi-party trusted setup ceremony required:
- Min 5 contributors from 5 organizations
- Each uses true random entropy
- Transcript published with hash
- Toxic waste destroyed
