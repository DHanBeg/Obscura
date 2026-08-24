# FAZ Status — Live Dashboard

Son güncelleme: 2026-08-11 (#31 marketplace escrow (B) karar mühürlendi — internalMove primitifi, buyer ücreti değişmiyor, 6 adımlı implementasyon planı) — önceki: 2026-08-08 (mesajlaşma katman-katman teşhis: #13 1:1 kapanış, #43 grup çift-katman kırığı; #31 marketplace dispute/iade kapsam teşhisi: orta-büyük) — önceki: 2026-08-01 (bridge + DID şema + deploy doğrulama oturumu)

## FAZ 1 (MVP) — ✅ CODE-COMPLETE + AUDIT-CLEAN

ADR: [[../../03_Resources/ADRs/Index#0008]], [[../../03_Resources/ADRs/Index#0009]]

| # | Deliverable | Durum |
|---|---|---|
| 1 | 5 node kurulumu | ✅ |
| 2 | E2EE Signal (1:1) | ✅ **gerçek+bağlı, #13 kapalı** — bkz. "Grup Mesajlaşma — #43" |
| 3 | MLS basic (grup) | ⚠️ **backend gerçek, mobile SIFIR — #43 launch-blocker** — bkz. "Grup Mesajlaşma — #43" |
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

**Tamamlanan (2026-05-17):**
- Frontend wiring ✅: wallet, shielded wallet, governance (liste + detay + oy), staking, mini app store, airdrop sayfaları
- Mobile bridge ✅: wallet + governance ekranları, FAZ 2 API fonksiyonları (wallet, staking, governance, airdrop, apps)
- `frontend/lib/api.ts` ✅: FAZ 2 tüm API fonksiyonları eklendi, çift nodeStatus düzeltildi, mini app URL düzeltildi
- `AppShell.tsx` ✅: çift import düzeltildi
- `GravityWell.tsx` ✅: wallet, governance, apps nav item'ları eklendi
- Mobile `_layout.tsx` ✅: wallet + governance tab'ları eklendi
- Security audit ✅: SQL injection yok, hardcoded secret yok, auth guard 66/66 handler

**Tamamlanan (2026-05-17 oturum 2):**
- Mobile staking ekranı ✅: kilitle/aç/çek, pozisyon tab (durum badge), kesinti tab
- Mobile apps ekranı ✅: liste, kur/kaldır, tier kilit gösterimi
- `mobile/lib/api.ts` stakingSlashes ✅ eklendi
- `mobile/app/(main)/_layout.tsx` staking + apps tab'ları ✅ eklendi

**Kalan (test/prod hariç):**
- FAZ 2 GA: prod deploy, multi-party trusted setup ceremony, real SMS/FCM
- ZK proof entegrasyonu frontend governance/airdrop (şu an stub — FAZ 3'te full)

## FAZ 3 (Federasyon) — IMPLEMENTATION COMPLETE (~95%)

ADR'lar: [[../../03_Resources/ADRs/Index#0014]] (FAZ 3 — libp2p + BFT + federation)

| # | Deliverable | Durum |
|---|---|---|
| 1 | Açık node kaydı (permissionless) | ✅ `federation/` paketi + DB + API |
| 2 | libp2p host + GossipSub + DHT | ⚠️ **DÜZELTME 2026-08-24: "HTTP gossip'in yerini alıyor" YANLIŞTI.** Kod gerçek (`p2p/host.go:40-114`) ama `docker-compose.yml` 5/5 node'da `P2P_ENABLED: "false"` — prod'da HTTP gossip (`gossip.go`, ADR-0003) hâlâ TEK aktif inter-node sync yolu, o da test'siz. Bkz. [[Ground-Truth-Audit-2026-08-24]]. |
| 3 | Byzantine fault tolerance (BFT) | ⚠️ **KISMİ (~55-60%), GÜNCELLENDİ 2026-08-24** — bu satır BAYATTI (commit `98466a4`, 2026-08-02'den beri yanlış). Gerçek durum: `main.go:209-259` engine+proposer loop gerçekten kuruyor, gerçek libp2p GossipSub transport, `token.SetOpRecorder` ile gerçek besleme (6 test PASS), bütünlük+persistence gerçek (27 test PASS, `TestEndToEnd_TwoNodes_ReachQuorumAndBothCommit` dahil). AMA: imza doğrulama STUB (`bft.go:61` "Ed25519 (stub)", Sig hiç okunmuyor), quorum stake-ağırlıklı DEĞİL (düz peer-sayısı), token yazması konsensüsten GEÇMİYOR (sadece post-hoc audit-log, ADR-0017 kasıtlı tasarımı). Bkz. [[Ground-Truth-Audit-2026-08-24]]. |
| 4 | Tam topluluk yönetimi | ✅ FAZ 2'de tamamlandı |
| 5 | ZK-ML gelişmiş moderasyon | ✅ `moderation/zkml.go` — ezkl proof doğrulama |
| 6 | Post-quantum kripto hazırlığı | ✅ `pqcrypto/` paketi — Kyber-768 (cloudflare/circl) |
| 7 | Cross-chain bridge | ✅ `bridge/` paketi — ETH + DOT RPC stub |

**Ek tamamlananlar (oturum 3):**
- libp2p QUIC transport ✅ `/ip4/0.0.0.0/udp/9001/quic-v1`
- Frontend Node Explorer ✅ `app/nodes/page.tsx`
- Frontend Bridge UI ✅ `app/bridge/page.tsx`
- GravityWell nodes + bridge nav ✅

**Ek tamamlananlar (oturum 3 devam):**
- recursive_proof.circom ✅ (Poseidon commitment, circuit_id, epoch, consistency)
- zkml_moderation.circom ✅ (score_bucket, model commitment, inference consistency constraint)
- Mobile Node Explorer ✅ `mobile/app/(main)/nodes.tsx`

**Kalan (FAZ 3 GA — test/prod hariç):**
- Trusted setup ceremony: recursive_proof + zkml_moderation (multi-party)
- Bridge gerçek relayer + on-chain contract deploy — ✅ **TAMAMLANDI (2026-08-01)**. OBSToken + OBSBridge Sepolia'ya deploy edildi, relayer canlı Sepolia'yı polluyor, gerçek uçtan uca transfer test edildi: 10 OBS lock edildi (Sepolia tx), relayer Locked event'ini yakaladı, DOT extrinsic'i hazırlayıp Paseo'ya gönderdi — **finalized block'a girdi, alıcı bakiyesi +10 PAS arttı** (bkz. `docs/sessions/2026-08-01-bridge-dot.md`). İdempotency doğrulandı (aynı event ikinci kez işlenmedi). Go tarafı (`dot.go`/`extrinsic.go`/`scale.go`/`metadata.go`/`watch.go`, 18 test PASS) commit `0bedc0f`'te; relayer.go'nun DOT-wiring'i + Solidity kontratları henüz ayrı commit'lenmedi.
- P2P prod test + 50+ harici node
- **#40 yeniden sınıflandı (2026-08-07): "ölü kod / sil" DEĞİL → "P2P bootstrap discovery BAĞLI DEĞİL — launch-blocker".** Kanıt: `discovery.go:45-109`'daki `DiscoverBootstrapPeers` (4 kaynak: env/DNS/ENS/peer_cache) hiçbir yerden çağrılmıyor; gerçek yol sadece `BOOTSTRAP_PEERS` env → `host.go:286` boşsa direkt return; `docker-compose.yml`'de 5/5 node `BOOTSTRAP_PEERS=""` ile başlıyor → taze deploy düğümleri birbirini bulamaz; `AddPeerCacheMigration` de 0 çağrı (`peer_cache.go:6` yorumu yanıltıcı — ikincil ölü kod). Etki: prod'da P2P ağı kurulamaz; şu an compose'da `P2P_ENABLED=false` olduğu için maskeli. Bağımlılık: FAZ 3'ün #2 satırındaki ("libp2p host + GossipSub + DHT ✅") ve Federasyon'un gerçek prod çalışması buna bağlı. Statü: launch öncesi çözülmeli.

**Ek tamamlananlar (oturum 4):**
- Mobile Bridge ekranı ✅ `mobile/app/(main)/bridge.tsx`

## FAZ 4 (Otonomi) — IMPLEMENTATION COMPLETE (~90%)

| # | Deliverable | Durum |
|---|---|---|
| 1 | Tam DAO yönetimi (timelock + guardian veto + süper çoğunluk) | ✅ backend + frontend + mobile |
| 2 | Kuantum dayanıklı kripto — Dilithium3 imzalama | ✅ `pqcrypto/dilithium.go` + handler |
| 3 | AI node optimizasyonu (EMA + lineer regresyon) | ✅ `ai/optimizer.go` + handler |
| 4 | Decentralized sequencer (VRF stake-ağırlıklı) | ✅ `sequencer/sequencer.go` + frontend + mobile |
| 5 | GPS + ZK location proof circuit | ✅ `circuits/location_proof.circom` + handler (stub) |

**Tamamlananlar (oturum 3–4):**
- `dao/dao.go` ✅: CreateProposal, Finalize, Execute, GuardianVeto — timelock 48s, veto window 24s, süper çoğunluk protokol/hazine için 67%
- `pqcrypto/dilithium.go` ✅: DilithiumGenerateKeyPair, DilithiumSign, DilithiumVerify (NIST ML-DSA, cloudflare/circl mode3)
- `ai/optimizer.go` ✅: EMA latency, linear regression load, score-based peer selection
- `sequencer/sequencer.go` ✅: VRF SHA256 stake-ağırlıklı rotasyon, 4s epoch, min 1000 OBS stake
- `api/faz4_handlers.go` ✅: HandleDAOCreate/List/Finalize/Execute/Veto, HandleDilithiumKeygen/Verify, HandleAIMetrics/Peers, HandleSequencerRegister/List/Batches/SubmitBatch, HandleLocationVerify
- `frontend/app/dao/page.tsx` ✅: DAO governance UI (öneri oluştur, tally barlar, finalize/execute/veto)
- `frontend/app/sequencer/page.tsx` ✅: aktif sequencer, aday listesi (stake weight bar), batch history
- `frontend/components/GravityWell.tsx` ✅: DAO + Sequencer nav item'ları eklendi
- `mobile/app/(main)/dao.tsx` ✅: DAO ekranı (öneri oluştur, durum badge, tally, aksiyonlar)
- `mobile/app/(main)/sequencer.tsx` ✅: sequencer adaylar + batch history
- `mobile/app/(main)/_layout.tsx` ✅: DAO + Sequencer tab'ları eklendi
- `mobile/lib/api.ts` ✅: daoCreateProposal, daoListProposals, daoFinalize, daoExecute, daoVeto, dilithiumKeygen, aiMetrics, aiPeers, sequencerList, sequencerBatches, sequencerRegister, bridgeLock eklendi

**Oturum 4 devam — gerçek implementasyonlar:**
- `backend/internal/storage/sharding.go` ✅ — Reed-Solomon 4-of-6, 256KB shard, SHA256 bütünlük kontrolü, 30 gün TTL pruner
- `backend/internal/identity/bip39.go` ✅ — BIP39 mnemonic üretimi, HKDF identity_secret, DID türetme, GF(256) Shamir 3-of-5
- `circuits/age_proof.circom` ✅ — hesap yaşı kanıtı (Poseidon commitment, GreaterEqThan(32))
- `circuits/activity_proof.circom` ✅ — aktivite eşiği (active_days + msg_count, 30 gün sınır)
- `circuits/msg_count_proof.circom` ✅ — toplam mesaj sayısı kanıtı
- `circuits/node_proof.circom` ✅ — node çalıştırma kanıtı (uptime + stake)
- `circuits/endorsement_proof.circom` ✅ — onay sayısı kanıtı (max 1000 Sybil koruması)
- `circuits/streak_proof.circom` ✅ — kesintisiz aktivite serisi kanıtı (max 365 gün)
- `backend/internal/zk/verifier.go` ✅ — yeni circuit constantları eklendi, non-core circuit'ler için graceful fallback (trusted setup bekleniyor)
- `backend/internal/api/identity_handlers.go` ✅ — mnemonic generate/validate, derive, Shamir split/combine
- `backend/internal/api/storage_handlers.go` ✅ — shard upload/retrieve/delete/stats, node-to-node local-shard
- `backend/internal/ai/probing.go` ✅ — gerçek federation node'larına 30s periyodik latency probe
- `HandleLocationVerify` ✅ — circuit yüklüyse gerçek ZK doğrulama, yoksa 503 (stub kaldırıldı)
- `mobile/lib/api.ts` ✅ — identity (mnemonic, shamir) API fonksiyonları eklendi
- `frontend/lib/api.ts` ✅ — identity + shard storage API fonksiyonları eklendi
- `crypto/build-mls-cli.sh` ✅ — Rust MLS CLI build script

**Tamamlananlar (oturum 5):**
- MLS CLI binary derlendi ✅ — `crypto/Cargo.toml [[bin]] mls-cli` eklendi, `target/release/mls-cli.exe` (2.2MB) smoke test geçti
- ZK trusted setup ceremony ✅ — 7 yeni circuit (age, activity, msg_count, node, endorsement, streak, location) dağıtıldı → frontend/public/zk/ + mobile/assets/zk/
- WASM ZK client ✅ — `frontend/lib/zk.ts` 7 yeni prover fonksiyonu (proveAge, proveActivity, proveMsgCount, proveNode, proveEndorsement, proveStreak, proveLocation)
- Docker Compose ✅ — `crypto-builder` servisi (rust:1.78-alpine), `mls-bin` volume, `MLS_CLI_PATH` env, 5 node'a depends_on eklendi
- Sequencer on-chain ✅ — `staking.NodeOperatorStakeOBS` + `sequencer.SetStakeLookup` + `StartEpochRotation` (4h epoch) — gerçek DB stake ile doğrulama

**Kalan (production GA için):**
- location_proof.circom trusted setup gerçek GPS entegrasyonu
- Dilithium3'ün mesaj akışına tam entegrasyonu
- Multi-party trusted setup ceremony (dev → production) — **Madde 4 prod-blocker (2026-08-07):** `circuits/build-new-circuits.sh` kendi yorumu "DEV ceremony (tek katkıcı) — production multi-party gerektirir." Satır 150'deki ✅ "dağıtıldı" demek, "production-ready" demek değil — msg_count_proof/streak_proof dahil 7 circuit'in hepsi bu tek-katılımcılı dev setup'la üretildi. Proof üretimi/doğrulaması çalışıyor (test kanıtlı, bkz. Madde 4 Adım E), ama gerçek kullanıcı öncesi multi-party ceremony şart — çözülmedi, sadece işaretlendi.
- Real SMS/FCM provider

## Sealed-Sender Doğrulaması (2026-08-01)

Çalışma kopyasında `messaging/sealed_sender.go`, `signal/sealed_sender_cli.go`, `signal/crypto_cli.go` (+ testleri) ve `pqcrypto/kyber.go` commit edilmeden silinmiş bulundu (git geçmişinde hiç yok — düz dosya sistemi silmesi, ne zaman/neden olduğu git ile izlenemedi). `git checkout --` ile HEAD'den restore edildi, `git diff` HEAD'e karşı sıfır fark verdi. Gerçek `obscura-crypto-cli` subprocess binary'siyle (`CRYPTO_CLI_PATH` set edilerek) yeniden test edildi: `TestCryptoCLI_X3DHRoundtrip`, `TestSealedSenderCLI_Roundtrip`, `TestSealedSenderCLI_WrongRecipientFails`, `TestSealedSenderCLI_EnvelopeHidesSenderPubHex` — hepsi gerçek subprocess ile PASS (skip değil). Mobile wiring doğrulandı: `mobile/lib/e2e.ts`'in gerçek gönderim/alım yolu (`getSealedSenderIdentity`, satır 397-424) sealed-sender'ı fiilen çağırıyor.

## Grup Mesajlaşma — #43 (2026-08-08, launch-blocker, çift katman)

Önceki oturumun #13 sorusu ("mesajlaşma kripto katmanı bağlı mı") 1:1 ve grup için ayrı ayrı teşhis edildi. Sonuç ikisi de birbirinden bağımsız ama çok farklı: **1:1 kapalı (gerçek+bağlı), grup açık ve büyük (#43, yeni).**

**#13 kapanış (1:1):** `mobile/lib/e2e.ts` (X3DH+Double Ratchet+sealed-sender, saf TypeScript, Rust'tan bağımsız) gerçek gönderim yoluna bağlı — `mobile/lib/message-send.ts:37-53` (`sendSealedMessage`), kanıt commit `d2932eb` ("sealed-sender'ı gerçek gönderim yoluna bağla"). Gönderim: `chat/[id].tsx:210` → `sealAndEncryptMessage` → gerçek zarf → `encryption_type:"sealed"` ile POST. Alım: `chat/[id].tsx:161` → `receiveMessage` (`e2e.ts:445-488`) → gerçek `unseal()`+ratchet decrypt. Backend `handlers.go:665` (`HandleSendMessage`) kasıtlı olarak şifrelemiyor — `internal/signal/session.go:1-11`'in tasarım ilkesi ("E2EE-blind: never sees plaintext, never derives keys") — bu bir eksiklik değil. Rust `obscura-crypto-cli` (`crypto/src/bin/cli.rs`, 1143 satır, `cargo build --release` ile derlendi, `keygen` ile fonksiyonel test edildi) canlı yolda DEĞİL — sadece `mobile/lib/__tests__/*-vector-crosscheck.test.ts` üzerinden TS↔Rust çapraz-doğrulama aracı. **#13 kapalı, sadece 1:1 kapsamında.**

**#43 — Grup mesajlaşması mobile'da tamamen kırık (çift katman):**

- **Katman 1 (taşıma, şifrelemeden ÖNCE kırık):** `chat/[id].tsx`'teki HER işlem (mesaj yükleme :139, text gönderme :183, resim/video :249, dosya :296, konum :317, ses :340/357, sesli/görüntülü arama linki :686/692) `conv?.peer_did` şartına bağlı. Backend `handlers.go:533`: `LEFT JOIN conv_members cm2 ON ... AND c.is_group = 0` — grup konuşmalarında `peer_did` **hiçbir zaman** dolmaz. Sonuç: grup ekranından hiçbir mesaj (düz metin dahil) gönderilemez/alınamaz. Ayrı grup-chat ekranı yok — `new-group.tsx:85` grup oluşunca aynı `chat/[id].tsx`'e yönlendiriyor.
- **Katman 2 (kripto):** Backend MLS relay'i GERÇEK ve TAM — `mls_handlers.go:1-16` (RFC 9420, 1306 satır, KeyPackage/Commit/Welcome/encrypted-broadcast, "server holds NO group secrets"), `internal/mls/client.go` Rust `mls-cli`'ye JSON-RPC köprü, `cargo build --release --bin mls-cli` başarılı (49s). AMA mobile'da MLS istemci kütüphanesi **YOK** — `grep mls mobile/` → sadece `settings-advanced.tsx:17,154-161`'deki işlevsiz debug switch'i. Mobile `/v1/mls/*` endpoint'lerinin **hiçbirini** çağırmıyor (sıfır sonuç).
- **Bağımlılık:** `chat/[id].tsx`'in `peer_did` gate'i grup-farkında hale getirilmesi (Katman 1) taşıma için ön-koşul; MLS istemci kütüphanesi (`mls.ts` — KeyPackage üretimi, Commit/Welcome işleme, grup encrypt/decrypt) ondan sonra gelir.
- **Boyut:** orta değil, ayrı planlama. 1:1'den büyük — 1:1'de mobile kripto zaten hazırdı (`e2e.ts`), sadece tel bağlıydı (`d2932eb`); grupta mobile kripto **da** yok, sıfırdan yazılacak.

**#37 yeniden çerçeveleme (kapalı ama not):** #37'de düzeltilen grup okundu-bilgisi/delivery-status backend yolu (`extra_handlers.go` grup üyelik yetkisi + `message_read_status` per-reader tablo, commit `b4e037b`/`00aca50`/`4edcb4b`) GERÇEK ve test edilmiş — ama mobile grup mesajı hiç gönderemediği için (#43 Katman 1) prod'da şu an **erişilemez/tetiklenemez**. Bug fix DEĞİL, doğru — sadece #43'ün altyapısının bir parçası: #43 Katman 1 bağlanınca otomatik aktifleşecek, ayrı iş gerektirmiyor.

**META-BULGU (denetim güvenilirliği):** Önceki denetim backend-only bakıyordu. Mesajlaşmayı ~%15 iskele olarak skorlamıştı çünkü backend crypto_cli çağrısı yok — ama gerçek kripto client'ta (`e2e.ts`), bu YANLIŞ düşük skordu. Client-ağırlıklı her özellik (mesajlaşma, muhtemelen panik-modu/self-destruct, seccomp) aynı sebeple denetimde olduğundan düşük skorlanmış olabilir — ayrı ayrı doğrulanmalı. Grup ise TAM TERSİ yönde yanlış: denetim MLS'i ~%90 gibi değerlendirmişti (backend gerçek+tam olduğu için), ama mobile tarafı sıfır olduğundan gerçek kullanıcı-görünür durum ~%0'a yakın. **Ders: hiçbir özelliğe tek katman (sadece backend YA DA sadece client) bakarak skor verilmemeli — her ikisi ayrı ayrı doğrulanmalı.**

**#43 — ts-mls entegrasyonu BLOKE (2026-08-09/10, npm monorepo hoisting):** Katman 2 (mobile MLS istemcisi) için ts-mls (saf-TS, RN uyumlu, suite 0x0001 backend openmls 0.6 ile birebir) seçildi, interop spike 7/7 yeşil (KeyPackage/Welcome/application-message/UpdateKey-Commit byte-uyumlu). Commit 0 (`f4cf2a2`) sağlam: `HandleMlsUpdateKey` route'a bağlandı, `chat/[id].tsx`'in 6 grup-gönderim yolu fail-closed doğrulandı (peer_did boşken erken return, düz-metin sızıntısı yok). Commit 1'de (ts-mls kurulumu) tıkandı: ts-mls'in `@noble/curves` optional peer'i (`2.0.1`, mobile'ın kendi X3DH kullanımıyla — `2.2.0` — çakışıyor) npm'i curves'ü `mobile/node_modules`'e izole nested kopya olarak yerleştirmeye itiyor; o kopyanın `@noble/hashes` bağımlılığı (2.x serisi TAMAMEN ESM-only, `"type":"module"`) mobile-seviyesinde shadow oluşturup Jest'in CJS `require()`'ını kırıyor (`ratchet.ts`/`crypto.ts`/`sealed-sender.ts`'in doğrudan `@noble/hashes/hkdf.js` importları etkileniyor). 4 npm stratejisi denendi — hepsi aynı ikilemde tıkandı: **doğru hoisting XOR sıfır react-native drift**, ikisi asla birlikte değil:
1. Targeted install (`npm install ts-mls@1.6.2 --save-exact --workspace=mobile`) → RN güvenli, shadow kalıyor.
2. `npm dedupe` → RN güvenli, shadow kalıyor (npm'in incremental resolver'ı var olan yerleşimi koruyor).
3. `npm install --workspace=mobile` (paketsiz, mobile subtree'sinin tam scoped resolve'u) → RN güvenli, shadow YİNE kalıyor.
4. `overrides: {"@noble/curves":"2.2.0"}` + tam sıfırdan silme (`rm -rf node_modules + package-lock.json`, fresh `npm install`) → **tek çalışan hoisting düzeltmesi** (curves root'a tek kopya hoistleniyor, shadow kalkıyor) — AMA aynı fresh-resolve, mobile'ın exact-pinned `react-native: "0.74.0"`'ını sessizce **0.86.2**'ye kaydırıyor (başka workspace'in gevşek range'i tetikliyor, kaynağı tam haritalanmadı). Kabul edilemez — geri sarıldı.

Her denemede geri sarma disiplini uygulandı: `git checkout -- package.json mobile/package.json package-lock.json` + `npm ci` ile baseline'a dönüş, 59/59 kripto testi + backend build/vet her seferinde yeniden doğrulandı. Şu an working tree tamamen temiz, `f4cf2a2` sağlam, hiçbir sapma kod tabanında kalmadı.

**Kalan iki teknik yol** (ikisi de bir sonraki oturumun işi):
1. **Jest+Metro çift-yama** — npm ağacını olduğu gibi bırak (shadow kalır), `@noble/hashes` çözümlemesini `moduleNameMapper` (Jest) + Metro resolver config'inde ayrı ayrı zorla düzelt. Daha az riskli (npm'e dokunmuyor) ama kalıcı config borcu, iki farklı bundler'da paralel bakım.
2. **İzolasyon** — ts-mls'i mobile'ın paylaşılan bağımlılık ağacından yapısal olarak ayır (ayrı alt-paket/workspace ile kendi node_modules'ünde tam izolasyon). npm hoisting sorununu kaynağında çözer ama daha büyük yapısal iş.

**AÇIK STRATEJİK KARAR (teknik yoldan önce):** Grup mesajlaşma gerçekten şart mı (→ izolasyon, büyük iş gerekçesini haklı çıkarır) yoksa 1:1 mesajlaşma (zaten kapalı, #13) ürün için yeterli mi (→ ts-mls bırak, grup rafa, launch-blocker etiketi kalkar)? Karar verilmeden teknik yol seçilmemeli — Jest+Metro yaması da izolasyon da bu sorunun cevabına göre anlamlı/anlamsız.

**GÜNCELLEME (2026-08-24, denetim):** Yukarıdaki karar zımnen "izolasyon" yönünde verilip ilerlenmiş — ADR-0019 revize edildi (workspace-dışı vendor izolasyonu), L2 Tuğla 1-5c commit edildi (2026-08-13/15): MLS kripto çekirdeği (`group.ts`), API client (`mlsApi.ts`), şifreli state persistence (`mls-store.ts`), create-group + join-group akışları — hepsi GERÇEK ve test'li (64/64 mobile jest testi PASS, kendim çalıştırdım). **AMA E1'in ADR-0019'un kendi tanımladığı tek kapanış kriteri hâlâ karşılanmadı**: `chat/[id].tsx`'te `mlsApi`/`encryptGroupMessage`/`decryptApplicationMessageWire`'a **sıfır çağrı** — gerçek chat ekranından şifreli grup mesajı gönderip alma hiç bağlanmadı. Bileşen tamamlanma ~85-90%, kullanıcı-erişilebilir özellik hâlâ %0. Bkz. [[Ground-Truth-Audit-2026-08-24]].

## Create-Group Retry — #44 (2026-08-15, E1-sonrası, launch-öncesi, küçük)

TICKET (E1-sonrası, launch-öncesi): create-group retry. Şu an başarısız denemeden sonra yeni group_id ile baştan başlar → yetim local MLS state (5a persistence'ta silinmez) + potansiyel öksüz backend MLS group (createGroup geçip addMember koparsa). Fail-closed guard sızmayı önler ama kaynak-israfı + tutarsız çift birikir. Gereken: retry aynı group_id'den kaldığı yerden devam (createMlsGroupConversation adım-idempotent olsun — hangi adım bitti bilsin, oradan sürsün). Ref: 5b-2, createGroupFlow.ts.

## KeyPackage Havuzu — #45 (2026-08-15, E1-sonrası, launch-blocker)

TICKET #45 (E1-sonrası, launch-blocker): KeyPackage havuzu. Şu an Bob tek KeyPackage tutuyor (mls-store tek-slot, obscura_mls_keypkg_{did}). KeyPackage tek-kullanımlık (getKeyPackage used=1 tüketir, mls_handlers.go:107). Sonuç: Bob aynı anda SADECE BİR gruba davet edilebilir — ilk davet KeyPackage'ı tüketir, ikinci davet-eden "KeyPackage yok" alır. ensureInvitable bayrağı "bir kez yükledim" der, tüketim sonrası yeniden-yükleme tetiklemez → Bob kalıcı davet-edilemez olabilir. Gereken: (a) KeyPackage havuzu (N slot, her birinin private key'i ayrı saklı + Welcome'ın hangi KP'yi hedeflediğini çözme), (b) "kaç KP kaldı" endpoint'i + proaktif yeniden-doldurma, (c) mls-store çok-KP şeması. Ref: 5c, inviteBootstrap.ts, mls-handlers.go:107.

## Marketplace Dispute/İade — #31 (2026-08-08, orta-büyük, ayrı planlama)

Önceki denetim #31'i "refund kodu hiç yok, sıfırdan" demişti. #36'da aynı denetimin "ölü/yok" dediği admin-action aslında hazır+kablosuz çıkmıştı (bkz. #36 commit `63b4e7a`) — o sürpriz burada TEKRARLAMIYOR, denetim doğru: gerçekten sıfırdan.

**Hüküm: (2) ORTA-BÜYÜK, ayrı planlama, salt backend** (#43'ün aksine yeni kripto/client kütüphanesi gerekmiyor).

**Kanıt:**
- **Escrow YOK:** `marketplace.go:288` (`Purchase()`) — `token.Transfer` anında, senkron, tam bakiye satıcıya geçiyor. `pending_purchase` durumu (satır 276-279) sadece double-sell race kilidi, para tutma değil. `grep escrow|held` marketplace+token'da sıfır sonuç.
- **dispute/refunded kolon/tablo/enum olarak hiç yok** — `marketplace.go:47` yorumunda string olarak geçiyor, gerçek const/kolon/tablo değil. #36'daki "kolon var, yazan yok" durumunun AYNISI değil — burada gerçekten hiçbir şey yok.
- **Durum modeli genişleyecek:** `marketplace_transactions.status` şu an sadece `'completed'` üretiyor (`marketplace.go:48`) — `disputed`/`refunded` + dispute meta-kolonları (reason/evidence/opened_at/resolved_by) eklenmeli.
- **Admin-resolve iskeleti kısmen reuse edilebilir** (review_queue, AdminMiddleware, action-switch deseni — `admin_handlers.go`) ama aksiyonun kendisi YENİ: `removeContentByType` switch'inde (`admin_handlers.go:294-309`) `"transaction"`/`"refund"` case'i yok; mevcut model tek-taraflı-ceza şekilli (dismiss/confirm_remove/confirm_warn), dispute ise iki-taraflı+parasal.

**AÇIK MİMARİ KARAR (kod başlamadan önce verilmeli):** escrow yokluğu iadeyi "geri transfer" yapıyor ama satıcı bakiyesi garanti değil (harcamış olabilir). İki yol:
- **(A) Escrow'suz MVP** — bakiye yeterliyse iade başarılı, değilse manuel reconciliation (airdrop post-claim mint / marketplace.go:314 RECONCILIATION-log felsefesiyle aynı). Hızlı ama iade garantisi yok.
- **(B) Escrow'lu** — `Purchase()` baştan yazılır, para teslime kadar tutulur. İade garantili ama marketplace çekirdeği refactor gerektirir.

(A) yolu seçilirse kapsam: (a) transaction/dispute durum modeli+kolonlar, (b) dispute-açma endpoint'i, (c) admin resolve'a `confirm_refund`→reverse transfer (başarısızsa reconciliation log), (d) escrow-yok kararının belgelenmesi.

### KARAR VERİLDİ (2026-08-11): (B) Escrow'lu — implementasyon planı mühürlendi

**Adım 0 kararı:** `token.internalMove` primitifi — çift-`Transfer()` DEĞİL. Gerekçe: `Transfer()` her çağrıda `TransferFee()` (0.01 OBS) kesiyor (`token.go:243-246`); escrow'u mevcut `Transfer()`'ı iki kez çağırarak (buyer→escrow, escrow→seller) kurmak escrow'un elinde `price` varken release'in `price+fee` kesmeye çalışmasına yol açar → `ErrInsufficientBalance`, para donar. Alternatif (escrow'a `price+fee` yatırmak) buyer'a sessiz ek ücret bindirirdi — reddedildi. **Buyer davranışı/ücreti DEĞİŞMİYOR.**

**Mimari:**
- `token.internalMove(ctx, from, to, amount)` — `Transfer()`'ın atomik tx iskeleti, fee/supply mantığı çıkarılmış, ücretsiz iç-hareket. Escrow hold/release'in TEK para-hareketi aracı.
- Escrow = ayrı well-known DID hesabı (`did:obs:marketplace-escrow`, `FeePoolDID` ile AYNI desen — `token.go:39`).
- State `marketplace_transactions.status`'ta: `held → released` VEYA `held → refunded`, İKİSİ DE TERMİNAL, `held`'e geri dönüş yok.
- **Double-release önleme:** state-flip (`UPDATE ... WHERE status='held'` + `RowsAffected` kontrolü) para hareketinden ÖNCE — `Purchase()`'ın mevcut listing-rezervasyon deseniyle (satır 276-286) birebir aynı mantık. Sadece bir çağrı `affected=1` alır, para hareketi SADECE o çağrıda denenir.
- **Para donması:** state-flip başarılı ama `internalMove` başarısız olursa — hata KESİNSE `held`'e geri al (retry edilebilir); hata BELİRSİZSE (örn. context timeout, commit olup olmadığı bilinmiyor) geri ALMA, mevcut `MARKETPLACE RECONCILIATION NEEDED` log felsefesiyle (satır 314) operatöre bırak.
- Dispute/refund admin akışı review_queue'ya SOKULMUYOR (semantik uymuyor — "içerik kaldır/uyar" ≠ "kime öde") — ayrı `marketplace_disputes` tablosu + ayrı `POST /v1/admin/marketplace-disputes/{id}/resolve`, AYNI `AdminMiddleware`/`OBSCURA_ADMIN_DIDS` zinciri.

**Adım sırası (her biri ayrı commit, token kodunda tek büyük commit YOK):**
1. Şema + migration (`marketplace_transactions` durum genişletme + `marketplace_disputes` tablosu) — para hareketi YOK, sadece şema.
2. `token.internalMove` primitifi — birim test: atomiklik, yetersiz bakiye reddi.
3. `Purchase()` → escrow'a tut (`marketplace.go:288` seller yerine escrow DID).
4. Release yolu (buyer teslim onayı).
5. Dispute/refund yolu (admin).
6. Timeout otomasyonu — **ERTELENDİ, v1 dışı** (ayrı race yüzeyi, ayrı planlama).

**Her para-hareketi adımında (3, 4, 5) ZORUNLU test üçlüsü:** double-release (eşzamanlı 2 çağrı, sadece 1 başarı), race (`go test -race`), donma (internalMove hata döndürünce state doğru geri dönüyor mu / escrow bakiyesi kayboluyor mu).

Sıradaki adım: Adım 1 (şema+migration), ayrı onayla başlar.

**GÜNCELLEME (2026-08-24, denetimde bulundu — bu bölüm hiç güncellenmemişti):** Adım 1-5'in HEPSİ commit edilmiş ve test edilmiş (`7707396`,`0ad9386`,`5b89282`,`6c24c8d`,`8b17ce6`,`c53eeac`, 2026-08-12). `go test ./internal/marketplace/...` → PASS (double-release/race/stuck-money test üçlüsü dahil). Adım 6 (timeout otomasyonu) planlandığı gibi v1-dışı bırakıldı. **AMA: hiçbir client'ta (mobile/web) marketplace UI'ı yok** (`grep -rli marketplace mobile/ frontend/` → 0 sonuç) — backend tam hazır ama kullanıcı erişimi sıfır, bu launch-blocker olarak görünmez durumdaydı. Bkz. [[Ground-Truth-Audit-2026-08-24]].

## Deploy Durumu (2026-08-01)

Backend Railway'de **canlı ve dışarıdan erişilebilir** doğrulandı: `https://railway-status-production-2ea6.up.railway.app/v1/node/status` → HTTP 200, `{"status":"healthy","version":"3.0.0"}`. Test/dev ortamı, gerçek kullanıcı yok. Proje adı henüz "railway status" (Railway dashboard'dan "obscura"ya çevrilecek — kod/config tarafında yapılacak bir şey yok, sadece isimlendirme).

**GÜNCELLEME (2026-08-07):** Yukarıdaki "volume yok" bilgisi ESKİ — doğrulandı, artık YANLIŞ. `railway status` / `railway volume list` ile kontrol edildi: `obscura-node-2` servisine `obscura-node-2-volume` bağlı, mount `/app/data`, 54MB/500MB kullanımda, status Ready. `railway variables` ile `DATA_DIR=/app/data` set olduğu da doğrulandı — mount path ile eşleşiyor. Volume'ün tam ne zaman eklendiği CLI/git'ten çıkarılamadı (Railway volume'leri repo config'inde izlenmiyor, muhtemelen önceki bir oturumda dashboard'dan elle eklenmiş). Kalıcılık riski **yok**, acil aksiyon gerekmiyor.

**Eksik/dikkat (2026-08-01, kısmen eski):**
- ~~Bu deploy'da volume yok~~ → düzeltildi, yukarı bak.
- ~~JWT_SECRET dev fallback~~ → doğrulandı (2026-08-07): `railway variables` tablo çıktısı değerleri kesiyor, gerçek uzunluk `--json` ile doğrulandı: 64 hex karakter (256-bit), kod placeholder'ı (`CHANGE_THIS_JWT_SECRET_IN_PRODUCTION`) değil. Rotasyon gerekmiyor.
- ~~NODE_INTERNAL_SECRET, TURN_SECRET, OBSCURA_SUBSCRIBER_KEY, OBSCURA_MESSAGE_OWNER_PEPPER, OBSCURA_PHONE_PEPPER dev fallback~~ → doğrulandı (2026-08-07), bu satır YANLIŞTI: Task #9'da rotasyon gerçekten yapılmış. Her biri kod'daki placeholder sabitinden (`dev-only-placeholder-not-for-prod`, `obscura-insecure-dev-*-change-me`) farklı; NODE_INTERNAL_SECRET/TURN_SECRET/OBSCURA_MESSAGE_OWNER_PEPPER/OBSCURA_PHONE_PEPPER = 64 hex (256-bit), OBSCURA_SUBSCRIBER_KEY = base64 → decode 32 byte (AES-256 gereksinimini tam karşılıyor). Rotasyon gerekmiyor.

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
- Test/dev ortamı Railway'de canlı (2026-08-01'den itibaren — bkz. "Deploy Durumu"); gerçek prod/gerçek kullanıcı hâlâ yok
- ZK trusted setup dev-grade (single-contributor) — FAZ 1 GA'da multi-party ceremony şart
