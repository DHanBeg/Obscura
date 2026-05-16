# PARÇA 3 — Mini App, Fiziksel, Fazlar (Bölüm 10-12)

**Tam metin:** [[full/PARCA-3-miniapp-fiziksel-fazlar|PARCA-3 raw]]

## Bölüm 10 — Mini App Motoru (satır 1027-1149)

**Runtime:** Deno (TypeScript) — sandbox, seccomp-bpf
**Limits:**
- Bellek 128MB / app
- CPU %10 tek çekirdek
- Ağ whitelist
- Storage 10MB IndexedDB
- ZK proof 5/sn (spam)

**Bölüm 10.2 — API Bridge:**
- `identity.{getUserId, getUsername, getZkIdentity}`
- `messaging.{sendMessage, sendGroupMessage, createMLSGroup, onMessage}`
- `wallet.{getBalance, getShieldedBalance, requestPayment, requestShieldedPayment}`
- `zk.{generateProof, verifyProof, getCreditScore}`
- `ui.{showToast, openModal, close, requestZkPermission}`

**Bölüm 10.3 — Manifest:** name, permissions, allowedDomains, **zkPermissions** (ayrı onay), maxMemory, maxCpu

**Bölüm 10.4 — Tier kısıt:**
| Tier | Çalıştır | ZK API | Max user |
|---|---|---|---|
| Bronz | Hayır | Hayır | 0 |
| Gümüş | Evet | Hayır | 100/gün |
| Altın | Evet | Okuma | 500/gün |
| Platin | Evet (oluştur) | Tam | Limitsiz |
| Elmas | Evet (oluştur) | Tam | Limitsiz |

**Status:** ✅ Skeleton (manifest + Deno sandbox subprocess + registry). FAZ 2 GA: object storage entegrasyonu, ZK API bridge.

## Bölüm 11 — Fiziksel (satır 1156-1210)

**11.1 Etkinlik:** Title/desc/location/date/capacity/fee/tier şartı — ❌ implement yok
**11.2 Konum keşfi:** 1km grid, ZK location proof — ❌ FAZ 4
**11.3 QR köprü:** `obscura://{action}/{payload}` — ✅ cross-signing kullanıyor
**11.4 NFC:** Check-in, pairing, ödeme — ❌

## Bölüm 12 — FAZ Yol Haritası (satır 1213-1316)

**FAZ 1 (MVP — 0-90 gün):** ✅ CODE-COMPLETE (ADR-0008+0009)
- Hedef: 10k kullanıcı, 7 gün uptime, P99 <2s, 100 ZK/sn
- Deliverable: 5 node, Signal, MLS, Flutter, OTP, kredi, ZK-ID, P2P call, auto node, Circom
- Ölçüm: ZK 827/sn (parallel), MLS encrypt 0.13ms @ 1000, 0.315ms @ 5000

**FAZ 2 (Çekirdek — 90-180 gün):** ✅ IMPLEMENTATION
- Hedef: 100 validator, 50 mini app, 10K günlük, 1000 ZK/sn, 1000+ MLS
- Deliverable: zk-Rollup, OBS wallet, mini app, ZK-ML, airdrop, governance, MLS 5000+, staking
- Status: 8/8 implementation ✅, GA için audit + frontend wiring + Aztec contracts deployment kalan

**FAZ 3 (Federasyon — 180-365 gün):** 0%
- Hedef: 50+ harici node, 3 kıta, <100ms, kendi kendini sürdüren ekonomi, 10K ZK/sn
- Deliverable: permissionless node, inter-node optimization, BFT, **recursive proof**, post-quantum prep, cross-chain bridge

**FAZ 4 (Otonomi — 365+ gün):** 0%
- Hedef: <%10 merkezi karar, tam DAO, kuantum dayanıklı, 100K ZK/sn
- Deliverable: Tam DAO, CRYSTALS-Kyber/Dilithium, AI optimization, WASM ZK client, sequencer decentralization, GPS+ZK
