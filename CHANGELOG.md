# Changelog

All notable changes to Obscura will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- 30 sub-agent definitions in `.claude/agents/` (specialty + generalist agents)
- 13 Obscura-specific skill files in `.claude/skills/`
- 18 cloned external skill repos in `.claude/skills/external/` (~379 SKILL.md, ~2200 markdown)
- ADR system: `docs/adr/` with 5 initial records
- Comprehensive `CLAUDE.md` with 4-phase spec reference, agent registry, workflow rules
- Doc structure: `docs/{spec,adr,api,architecture,circuits,design,economics,postmortems,protocols,runbooks,sessions}/`
- Full spec copy at `docs/spec/obscura_spec_v3.txt`
- GitHub Actions CI: backend, frontend, mobile, desktop, circuits, security, openapi, docker
- Release workflow with SBOM generation
- Pre-commit hook (gitleaks, gofmt, go vet, tsc, cargo fmt)
- Dependabot config for go/npm/cargo/docker/actions
- PR template
- SECURITY.md, CONTRIBUTING.md

### Changed
- `.claude/settings.json` — explicit allow list for common dev commands

### Fixed
- (n/a in this release — tooling only)

## [1.0.0-alpha] - 2026-05-09 (initial commit)

### Added (FAZ 1 partial)
- Backend Go node: JWT auth, SMS OTP, X3DH prekey, WebSocket hub, messaging, credit, gossip, push, MinIO media, Prometheus
- Next.js 14 web client (login, chats, settings, calls, Service Worker)
- React Native/Expo mobile (login, chats, settings, calls)
- Tauri 2.x desktop (system tray, window mgmt, native commands)
- ZK circuits: identity_proof, credit_threshold, message_integrity (sources only, no .zkey yet)
- Docker Compose 5-node + nginx + MinIO + coturn + Prometheus
- Initial migration system, .env.example, Makefile

### Known issues / spec gaps
- libp2p not implemented (using HTTP gossip — see ADR-0003)
- MLS group encryption not implemented
- Reed-Solomon shard storage not implemented
- Rust `obscura-crypto` crate not implemented (using Go — see ADR-0004)
- Flutter client not implemented (using RN+Next.js+Tauri — see ADR-0002)
- ZK trusted setup ceremony not run; .zkey artifacts missing
- 9+ ZK circuits from spec missing (token_balance, vote_proof, storage_proof, age, activity, node, msg_count, endorsement, streak)
- 12-word BIP39 mnemonic not implemented
- Cross-signing (multi-device) not implemented
