# FAZ Status — Live Dashboard

Son güncelleme: 2026-08-08 (mesajlaşma katman-katman teşhis: #13 1:1 kapanış, #43 grup çift-katman kırığı) — önceki: 2026-08-01 (bridge + DID şema + deploy doğrulama oturumu)

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
| 2 | libp2p host + GossipSub + DHT | ✅ `p2p/` paketi — HTTP gossip'in yerini alıyor |
| 3 | Byzantine fault tolerance (BFT) | ⚠️ **İZOLE İSKELET, ENTEGRE DEĞİL** — `consensus/` paketi Tendermint-style PBFT kodu var ama `ProposeBlock()` main.go dışında hiçbir yerden çağrılmıyor, sıfır test dosyası, mesajlaşma/moderation/staking/sequencer'dan tam izole (bilinçli olarak "madde 8'e ertelendi", bkz. commit b5521c3) |
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
