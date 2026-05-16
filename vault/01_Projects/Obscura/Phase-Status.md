# FAZ Status — Live Dashboard

Son güncelleme: 2026-05-16

## FAZ 1 (MVP) — ✅ CODE-COMPLETE + AUDIT-CLEAN

ADR: [[../../03_Resources/ADRs/Index#0008]], [[../../03_Resources/ADRs/Index#0009]]

| # | Deliverable | Durum |
|---|---|---|
| 1 | 5 node kurulumu | ✅ |
| 2 | E2EE Signal | ✅ |
| 3 | MLS basic | ✅ |
| 4 | Flutter | ⚪ Kabul sapma (ADR-0002) |
| 5 | Telefon OTP | ✅ |
| 6 | Kredi + ZK tier upgrade | ✅ |
| 7 | ZK-ID + BIP39 + cross-signing | ✅ |
| 8 | P2P sesli arama | ✅ |
| 9 | Otomatik node seçimi (nginx LB) | ✅ |
| 10 | ZK proof altyapısı (4 circuit) | ✅ |

**Performans hedefleri (Bölüm 15.2) — hepsi aşıldı:**
- ZK verify: ~5ms (target 500ms) — 100x
- ZK throughput parallel: 827/s (target 100/s) — 8x
- MLS encrypt 1000 üye: 0.13ms (target 100ms) — 770x
- MLS encrypt 5000 üye: 0.315ms (target 100ms) — 317x

**FAZ 1 GA (production) için kalan:**
- 7-gün uptime + 10k user smoke (deploy yapılmadı)
- Multi-party trusted setup ceremony
- Real SMS/FCM provider
- SSL sertifikaları (Let's Encrypt)
- Pen test

## FAZ 2 (Çekirdek) — IMPLEMENTATION ✅, AUDIT PENDING

ADR'lar: [[../../03_Resources/ADRs/Index#0010-0013]]

| # | Deliverable | Durum |
|---|---|---|
| 1 | zk-Rollup (Aztec) | ✅ stub bridge + Noir contract (Phase 0/4) |
| 2 | OBS wallet (transparent + shielded) | ✅ (FAZ 3 = depth-32 Merkle) |
| 3 | Mini app motoru (Deno sandbox) | ✅ skeleton + manifest |
| 4 | ZK-ML moderation | ✅ heuristic now, ezkl FAZ 3 |
| 5 | Airdrop (Sybil-resistant) | ✅ |
| 6 | Yönetişim (ZK vote + timelock) | ✅ |
| 7 | MLS 5000+ üye | ✅ bulk add + bench |
| 8 | Staking + slashing | ✅ |

**Yeni ZK circuits:** token_balance (944 constraints), vote_proof (733 constraints) — trusted setup tam, backend verifier kayıtlı.

**Test sonuçları:** 9 paket × hepsi PASS (zk, token, staking, governance, airdrop, miniapp, moderation, blockchain, mls).

**Kalan:**
- FAZ 2 audit pass (code-reviewer + security-auditor + spec-checker) — rate limit nedeniyle bekliyor
- Frontend wiring (MLS UI, wallet UI, governance UI, mini app store, staking dashboard)
- Mobile bridge (RN MLS, wallet)
- FAZ 2 GA: prod deploy, audit, multi-party ceremony

## FAZ 3 (Federasyon) — 0%

Spec deliverables: permissionless nodes, BFT, recursive ZK, post-quantum prep, cross-chain bridges.

## FAZ 4 (Otonomi) — 0%

Spec deliverables: full DAO, quantum crypto, AI optimization, sequencer decentralization, GPS+ZK.

## ADR Aktivite

- ADR-0001..0007: tek tek altyapı kararları (SQLite, Flutter, gossip, crypto, ceremony, openmls)
- ADR-0008: FAZ 1 code-complete claim
- ADR-0009: FAZ 1 post-audit hardening (6 critical fix)
- ADR-0010: OBS token economics
- ADR-0011: Staking + slashing parametreleri
- ADR-0012: Governance (ZK vote, eligibility, quorum, veto)
- ADR-0013: ZK-ML moderation hybrid yaklaşım
- ADR-0005: Aztec rollup seçimi (artık Accepted)

## Notlar

- Geliştirme makinesi: Windows + Lexar USB (E: drive, NTFS, ~50GB obscura)
- Test ortamı yok, prod yok
- ZK trusted setup dev-grade (single-contributor) — FAZ 1 GA'da multi-party ceremony şart
