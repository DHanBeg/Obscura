# Obscura — Claude Çalışma Kılavuzu

**Bu dosyayı her oturumun başında oku. Spec özetlerine güvenme — tam spec `docs/spec/obscura_spec_v3.txt`.**

**🧠 Bilgi tabanı:** `vault/INDEX.md` — Obsidian-uyumlu PARA yapısı. Hızlı durum: `vault/01_Projects/Obscura/Phase-Status.md`. Domain index'leri: `vault/02_Areas/{Crypto,Backend,Security,...}/Index.md`. Obsidian uygulamasında `vault/` klasörünü vault olarak aç.

## Proje Nedir

Obscura, WhatsApp/Telegram/Signal'a rakip, sıfır dış bağımlılıklı, Zero-Knowledge tabanlı federe mesajlaşma platformu.
Protocol-first mimari: backend protokolü sabittir, client sadece protokolü konuşan bir arayüzdür.

---

## Spec'in 4 Resmi Fazı (KENDİ İÇ FAZLARINI UYDURMA)

Spec'te (Bölüm 12) tanımlı 4 faz var. "FAZ 3 bitti" demek = spec'in FAZ 3 deliverable'larının HEPSİ tamamlandı demek. Aksi halde bitti deme.

| Faz | Odak | Spec Deliverables | Mevcut Durum |
|---|---|---|---|
| **FAZ 1** MVP | 5-node, E2EE Signal, MLS basic, Flutter, OTP, kredi, ZK-ID basic, P2P call, ZK Circom basic | Bölüm 12.1 | **CODE-COMPLETE + AUDIT-CLEAN (ADR-0008 + ADR-0009)** — 9/10 ✅, 1/10 ⚪ kabul sapma. 6 critical güvenlik bug'ı post-audit ile düzeltildi. Production GA için 7-gün uptime + 10k user smoke + 11 deferred medium/low kalan. |
| **FAZ 2** Çekirdek | zk-Rollup, OBS wallet, mini app, ZK-ML, governance, MLS 5000+, staking | Bölüm 12.2 | **CODE-COMPLETE** — Tüm bileşenler implement edildi. Rollup settlement stub (FAZ 3+). Audit + prod deploy kalan. |
| **FAZ 3** Federasyon | Permissionless nodes, BFT, recursive ZK, post-quantum prep, cross-chain | Bölüm 12.3 | **KISMEN CODE-COMPLETE** — libp2p+GossipSub+DHT, Kyber-768+Dilithium3, bridge, zkml, tüm circuit'ler derlenmiş çalışıyor. **BFT (`internal/consensus`) İZOLE İSKELET, ENTEGRE DEĞİL** — `ProposeBlock()` main.go dışında hiçbir yerden çağrılmıyor, sıfır test dosyası, mesajlaşma/moderation/staking/sequencer'dan tam izole (bkz. commit b5521c3, bilinçli olarak "madde 8'e ertelendi" diye belgelenmiş — CODE-COMPLETE etiketi buraya yanlış uygulanmıştı, 2026-08-01 audit ile düzeltildi). ENS resolver stub. |
| **FAZ 4** Otonomi | Full DAO, quantum crypto, AI optimization, sequencer decentralization, GPS+ZK | Bölüm 12.4 | **CODE-COMPLETE** — Backend, frontend, mobile, WASM ZK, MLS CLI hepsi tamamlandı. GPU ZK feature flag arkasında. |

**Hatalı geçmiş:** Önceki oturumlarda kendi içimde işi 3 parçaya böldüm (handler→client→tooling) ve "FAZ 3 bitti" dedim. Bu YANLIŞTI. Spec FAZ'ları farklı.

---

## Tech Stack

| Katman | Teknoloji |
|---|---|
| Backend node | Go 1.22, gorilla/mux, gorilla/websocket |
| Veritabanı | SQLite (modernc.org/sqlite — CGO_ENABLED=0, pure Go) |
| Kripto crate | Rust (obscura-crypto) — henüz implement edilmedi |
| ZK devreleri | Circom 2.1.6 + snarkjs, Groth16, Poseidon hash |
| Frontend | Next.js 14 (App Router, "use client") |
| Mobile | React Native + Expo (expo-router, SecureStore) |
| Desktop | Tauri 2.x (TrayIconBuilder, get_webview_window) |
| Medya storage | MinIO S3 (AWS Sig v4, sıfır SDK) |
| Push | FCM HTTP v1 + APNs |
| Monitoring | Prometheus (text/plain format, custom endpoint) |
| Deploy | Docker Compose — 5 node + nginx + MinIO + coturn |

---

## Klasör Yapısı

```
obscura/
├── backend/               Go node (obscura-node)
│   ├── cmd/node/main.go   Tüm route kayıtları burada
│   ├── internal/api/      HTTP handler'lar
│   ├── internal/auth/     JWT + OTP
│   ├── internal/credit/   Kredi puanı sistemi
│   ├── internal/db/       SQLite + migration
│   ├── internal/gossip/   Node-arası HTTP relay
│   ├── internal/media/    MinIO S3 client
│   ├── internal/messaging/ WebSocket hub
│   ├── internal/models/   Veri modelleri
│   ├── internal/push/     FCM/APNs
│   └── internal/sms/      SMS OTP
├── frontend/              Next.js 14 web client
├── mobile/                Expo React Native client
├── desktop/               Tauri 2.x desktop client
├── circuits/              Circom ZK devreleri
├── crypto/                Rust crate (HENÜZ BOŞ — kendi .git'i var)
├── packages/              Shared JS paketleri
│   ├── api/               @obscura/api
│   ├── e2ee/              @obscura/e2ee (SKELETON — implement edilmedi)
│   └── store/             @obscura/store
├── nginx/                 nginx.conf
├── coturn/                turnserver.conf
└── prometheus/            prometheus.yml + alerts.yml
```

---

## Mevcut API Endpoint'leri (backend/cmd/node/main.go)

```
POST   /v1/auth/request-otp
POST   /v1/auth/verify-otp
GET    /v1/users/me
PATCH  /v1/users/me
GET    /v1/users/{did}
GET    /v1/users/search
GET    /v1/conversations
POST   /v1/conversations
GET    /v1/conversations/{id}/messages
POST   /v1/messages
DELETE /v1/messages/{id}
GET    /v1/credit/score
GET    /v1/credit/history
POST   /v1/spam/report
POST   /v1/keys/upload
GET    /v1/keys/{did}
POST   /v1/media/upload
POST   /v1/devices/register
POST   /v1/zk/verify
GET    /v1/rtc/turn-credentials
GET    /v1/node/status
GET    /v1/metrics
POST   /v1/internal/relay
GET    /v1/stream  (WebSocket)
```

---

## ✅ YAPILDI — Spec FAZ 1'in tamamlanan kısımları

### Backend
- [x] JWT auth + SMS OTP (Twilio stub)
- [x] DID tabanlı kimlik (`did:obs:sha256(pubkey)`)
- [x] X3DH prekey upload / fetch
- [x] WebSocket hub (gerçek zamanlı mesajlaşma)
- [x] Mesaj gönder / al / soft-delete
- [x] Kredi puanı + tier sistemi (bronze→diamond)
- [x] Spam raporu endpoint
- [x] WebRTC TURN credential endpoint
- [x] ZK kanıt doğrulama endpoint
- [x] SQLite + idempotent migration sistemi
- [x] HTTP gossip relay (NODE_PEERS env ile)
- [x] FCM + APNs push bildirimi (LogProvider dev stub)
- [x] MinIO S3 medya upload (AWS Signature v4)
- [x] Prometheus metrics endpoint
- [x] Docker (CGO_ENABLED=0, multi-stage, non-root, healthcheck)

### Frontend (Next.js 14)
- [x] Login (SMS OTP flow)
- [x] Sohbet listesi
- [x] Sohbet detayı (E2EE, medya upload, mesaj silme UI)
- [x] Ayarlar (profil düzenleme, avatar upload)
- [x] Aramalar sayfası
- [x] Service Worker + Web Push API
- [x] ZK kanıt üretimi browser'da (snarkjs lazy import)
- [x] **FAZ 2 — OBS Cüzdan** (bakiye, transfer, işlem geçmişi, gizli pool)
- [x] **FAZ 2 — Staking Dashboard** (stake/unstake/withdraw, pozisyonlar, slash)
- [x] **FAZ 2 — Governance Portalı** (öneri listesi, detay, ZK oy)
- [x] **FAZ 2 — Mini App Store** (liste, kur, kaldır, çalıştır)
- [x] **FAZ 2 — Airdrop** (kampanya listesi, ZK-gated talep)
- [x] **FAZ 2 — lib/api.ts** tüm FAZ 2 endpoint'leri (wallet, staking, governance, airdrop, MLS, apps)
- [x] **FAZ 2 — GravityWell nav** (wallet, governance, apps tab'ları)

### Mobile (React Native/Expo)
- [x] Login (SMS OTP)
- [x] Sohbet listesi + detayı
- [x] Ayarlar (profil düzenleme modal)
- [x] Aramalar
- [x] **FAZ 2 — OBS Cüzdan ekranı** (bakiye, transfer modal, geçmiş)
- [x] **FAZ 2 — Governance ekranı** (öneri listesi, Alert ile oy)
- [x] **FAZ 2 — Staking ekranı** (kilitle/aç/çek, pozisyon + kesinti tab'ları)
- [x] **FAZ 2 — Mini Apps ekranı** (liste, kur/kaldır, tier kilit)
- [x] **FAZ 2 — Mobile lib/api.ts** FAZ 2 tüm endpoint'leri (stakingSlashes eklendi)
- [x] **FAZ 2 — _layout.tsx** staking + apps tab'ları eklendi

### Desktop (Tauri 2.x)
- [x] System tray (TrayIconBuilder — Tauri 2.x API)
- [x] Pencere yönetimi (get_webview_window)
- [x] Native komutlar (get_app_version)

### ZK Devreleri (uçtan uca çalışıyor — 2026-05-10)
- [x] credit_threshold.circom — puan eşik kanıtı (Poseidon, 270 non-linear constraints)
- [x] identity_proof.circom — DID sahipliği
- [x] message_integrity.circom — anonim grup mesajı
- [x] Powers of Tau Phase 1 (BN128 power 14, dev ceremony — ADR-0006)
- [x] Phase 2 setup + .zkey her circuit için
- [x] Artifact dağıtım: frontend/public/zk/, mobile/assets/zk/, backend/internal/zk/keys/
- [x] Backend Go verifier (iden3/go-rapidsnark, gerçek BN254 pairing check)
- [x] End-to-end test: snarkjs proof gen → Go verify → PASS, tampering reddediliyor
- [x] Performans: prove 494ms (hedef <3s ✓), verify ~10ms (hedef <500ms ✓)

### Altyapı
- [x] Docker Compose (5 node + nginx + MinIO + coturn + Prometheus)
- [x] Prometheus alert rules
- [x] nginx load balancer config
- [x] coturn TURN config
- [x] .env.example, Makefile, .gitignore

---

## ✅ YAPILDI (2026-06-21 Audit) — Tüm FAZ 1-4 CODE-COMPLETE

> **NOT:** CLAUDE.md 2026-05-17'de yazıldı. 2026-06-21 audit'i tüm "eksik" maddelerin
> tamamlandığını doğruladı. Aşağıdaki eski YAPILMADI listesi artık geçersiz.

### FAZ 1 — Tamamlananlar (Audit doğruladı)
- [x] libp2p host + DHT + QUIC + GossipSub — `backend/internal/p2p/host.go`
- [x] ZK proof ile node yetki doğrulaması — `p2p/peer_auth.go`
- [x] MLS FFI exports — `crypto/src/ffi.rs` (X3DH, DR, identity, symmetric)
- [x] MLS Backend Go entegrasyonu — `internal/mls/client.go` (subprocess JSON-RPC)
- [x] MLS DB schema — `internal/db/migrations/mls_schema.go`
- [x] MLS API endpoints — `api/mls_handlers.go` (POST /v1/mls/*, GET /v1/mls/*)
- [x] MLS WebSocket types — `internal/messaging/mls_types.go`
- [x] MLS KeyPackage rotation (90 gün) — `internal/mls/rotation.go`
- [x] MLS Remove member + key update — mls_handlers.go
- [x] Shard storage 256KB + RS 4-of-6 + TTL — `internal/storage/sharding.go`
- [x] X3DH, Double Ratchet, SRTP — `crypto/src/{x3dh,ratchet,srtp}.rs`
- [x] WASM ZK prover — `crypto/src/wasm.rs`
- [x] BIP39 mnemonic (12 kelime) — `internal/identity/bip39.go`
- [x] Social recovery (3-of-5 Shamir GF256) — `bip39.go:SplitMnemonic/CombineShares`
- [x] Cross-signing QR — `api/cross_signing.go`
- [x] ZK-ID secret türetme — `bip39.go:DeriveZKIDSecret`

### FAZ 2 — Tamamlananlar (Audit doğruladı)
- [x] Tüm 17 ZK circuit derlenmiş — `circuits/build/*/` (.r1cs + _final.zkey + verification_key.json)
- [x] ZK artifact dağıtımı — `frontend/public/zk/` (34 dosya), `mobile/assets/zk/` (30 dosya)
- [x] OBS Token — `internal/token/token.go`
- [x] Shielded wallet (UTXO + Merkle) — `internal/token/shielded.go`
- [x] Staking — `api/staking_handlers.go`
- [x] Revenue sharing — `token.go` (50% burn, 50% pool)
- [x] ZK vote + governance — `internal/governance/governance.go`
- [x] 3/5 multisig + DAO — `internal/dao/dao.go`
- [x] Proposal + 48h timelock — `dao.go`
- [x] Governance portal — `frontend/app/governance/page.tsx`
- [x] Deno sandbox — `internal/miniapp/sandbox.go`
- [x] Mini app API bridge + manifest — `internal/miniapp/{bridge,manifest}.go`
- [x] Etkinlik/QR check-in — `api/event_handlers.go`
- [x] Konum keşfi — `frontend/app/location/page.tsx`

### FAZ 3 — Tamamlananlar (Audit doğruladı)
- [x] Permissionless node kaydı — `internal/federation/federation.go`
- [x] BFT konsensüs — `internal/consensus/bft.go`
- [x] Post-quantum Kyber-768 — `internal/pqcrypto/kyber.go`
- [x] Dilithium3 (ML-DSA) — `internal/pqcrypto/dilithium.go`
- [x] Cross-chain bridge — `internal/bridge/bridge.go` + `contracts/bridge/OBSBridge.sol`
- [x] ZK-ML moderation — `internal/moderation/zkml.go`
- [x] recursive_proof + zkml_moderation circuit — derlenmiş
- [x] Mobile Bridge — `mobile/app/(main)/bridge.tsx`

### FAZ 4 — Tamamlananlar (Audit doğruladı)
- [x] Tam DAO + timelock + guardian veto — `internal/dao/dao.go`
- [x] AI node optimizasyonu — `internal/ai/optimizer.go`
- [x] Decentralized sequencer (VRF) — `internal/sequencer/sequencer.go`
- [x] GPS + ZK location proof — `circuits/location_proof.circom` + `frontend/app/location/`
- [x] Frontend DAO + Sequencer UI — `frontend/app/{dao,sequencer}/page.tsx`
- [x] Mobile DAO + Sequencer — `mobile/app/(main)/{dao,sequencer}.tsx`
- [x] WASM ZK 7 prover — `frontend/lib/zk.ts`
- [x] MLS CLI binary — `crypto/target/release/mls-cli` (2.3MB, production-ready)

---

## ❌ KALAN EKSIKLER (2026-06-21 itibariyle)

### Stub / Partial (Kod var, production-grade değil)
- [ ] **ENS resolver** — `backend/internal/p2p/ens_stub.go` hardcoded IP; gerçek Ethereum RPC gerekiyor (FAZ 3+)
- [ ] **zk-Rollup settlement** — ADR-0010 planlama var, token layer standalone; gerçek rollup katmanı yok (FAZ 3+)
- [ ] **GPU/FPGA ZK hızlandırma** — `crypto/src/gpu.rs` bellperson feature flag arkasında; binding eksik (FAZ 3+)

### Production GA Koşulları (Kod değil, operasyon)
- [ ] 7-gün kesintisiz uptime kanıtı
- [ ] 10k kullanıcı smoke test
- [ ] 11 deferred medium/low güvenlik item giderilmesi (ADR-0009 audit raporu)
- [ ] Multi-party trusted setup ceremony (production ZK güveni — bkz ADR-0006)
- [ ] Resmi pen test / formal ZK circuit verification

### Tasarım Skill'leri (UI işlerinde çağır)
Her UI işinde sırayla çağır:
1. `.claude/skills/frontend-design-obscura/SKILL.md` — tasarım tokenleri, theme, Telegram/Element referansı
2. `.claude/skills/motion-principles-obscura/SKILL.md` — animasyon, easing, reduced-motion
3. `.claude/skills/impeccable-obscura/SKILL.md` — done demeden önce polish/critique

---

## Kodlama Kuralları

### Go
- Paket adları: `internal/xxx` altında küçük harf
- Error handling: her hata `writeError(w, http.StatusXxx, "mesaj")` ile
- DB sorguları: `db.DB.QueryRow(...)` — global DB instance
- Auth: `middleware.go`'daki `RequireAuth` middleware
- CGO_ENABLED=0 — hiçbir zaman CGO bağımlılığı ekleme

### TypeScript/React
- `"use client"` direktifi: browser state kullanan her component'te
- API çağrıları: `lib/api.ts` üzerinden — direkt fetch yazma
- Store: `lib/store.ts` (Zustand) — global state buraya
- Token: `localStorage.getItem("obscura_token")` (web), `SecureStore` (mobile)

### Rust (Desktop/Crypto)
- Tauri 2.x API kullan — 1.x API tamamen farklı
- `get_window` → `get_webview_window`
- `SystemTray` → `TrayIconBuilder`
- `CustomMenuItem` → `MenuItem`

### Genel
- Türkçe yorum/log kabul edilir
- `SUCCESS` response: `{"success": true, "data": ...}`
- `ERROR` response: `{"success": false, "error": "mesaj"}`

---

## Denetim ve Topluluk Katmanı — Anayasa (DEĞİŞMEZ)

Tasarım dokümanı: `docs/spec/obscura_denetim_topluluk_katmani.md` (vault: `vault/03_Resources/Spec/obscura_denetim_topluluk_katmani.md`). Bu katman subscriber store ve sealed-sender'ın **üstüne** gelir — onlar bitmiş ve testlerle kilitlenmiştir, bu doküman onlara dokunmaz.

**DURUM (2026-08-01 audit ile düzeltildi — "henüz kod yazılmadı" ARTIK YANLIŞ, aşağıdaki üç parça çoktan yazılmış ve test edilmiş):**
- **Moderasyon** — `internal/moderation` paketi: şikayet akışı, TIER A kanıt doğrulama (sealed + legacy), kademeli ceza, brigading koruması, complainant credibility. **40+ test PASS** (`go test ./internal/moderation/...`).
- **Panik butonu** (Madde 13, tamamlandı) — `mobile/lib/panic.ts` + `panic-flow.ts` + `mobile/app/(main)/panic.tsx`. **23/23 test PASS** (`panic.test.ts` + `panic-flow.test.ts`, jest).
- **Self-destruct** — backend `internal/messaging/expiry.go` (+ `expiry_test.go`) ve mobile `lib/ws-handlers.ts` + `lib/plaintext-cache.ts` (client-side purge). **Backend + mobile testleri PASS** (`expiry_test.go` messaging paketiyle; mobile `plaintext-cache.test.ts` + `ws-handlers.test.ts`, 6/6).

Aşağıdaki bölüm hâlâ geçerli — bu üçü YAZILDI ama doküman Bölüm 12'deki "açık kalan kod-öncesi kararlar" (aşağıda) hâlâ kod-öncesi.

Aşağıdaki 6 ilke dokümanın Bölüm 0'ından türer. Herhangi bir özellik bu ilkelerle çelişirse özellik değil ilke kazanır — kod bu sınırları ihlal edemez:

1. **Özel alan asla taranmaz.** Birebir mesaj, arama, davetle girilen özel gruplar — hiçbir tarama/moderasyon kodu buraya bakmaz. "Kamusal" yalnızca gerçekten herkese açık kanal/ilan/yayın demektir; davetle girilen grup kaç kişilik olursa olsun özeldir.
2. **Davranış denetlenir, ahlak değil.** Kamusal alan moderasyonu yalnızca kapalı listedeki somut ihlallere bakar: spam, dolandırıcılık, taciz/tehdit, telif ihlali, yasadışı satış, çocuk güvenliği. Öznel "uygunsuzluk"/"ahlaksızlık" kategorisi eklenmez.
3. **Kural şeffaftır.** Kredi/güven puanı, ceza kademeleri, ihlal listesi kullanıcıya açık olmalı. Gizli puanlama/gizli ceza mantığı koda giremez.
4. **Konum operatörde tutulmaz.** Panik butonu ve buluşma onayı doğrudan kullanıcının önceden seçtiği güven kişisine gider; operatör aracı olmaz, loglamaz, saklamaz.
5. **Anahtar/pepper repo'da olmaz.** Yalnızca production ortam değişkeninde bulunur, hiçbir yerde (operatör dahil) loglanmaz — subscriber store anahtarıyla aynı disiplin.
6. **CGO_ENABLED=0, subprocess-CLI deseni, yerel-model-birincil + cloud-fallback korunur.** Tarama motoru (Ollama/Mistral yerel birincil, Groq yalnızca kararsızlıkta fallback) ve her yeni kripto/imza işi mevcut `obscura-crypto-cli` subprocess köprüsü üzerinden gider (bkz. `internal/signal/crypto_cli.go`) — cgo eklenmez.

**Açık kalan kod-öncesi kararlar** (doküman Bölüm 12) — bunlar çözülmeden ilgili özellik koda geçmez: kanıt doğrulama (mesajlar imzalı hash ile mi saklanıyor?), Sybil direnci (marketplace öncesi), kredi eşik değerleri, marketplace kuralları.

---

## Sık Yapılan Hatalar — Yapma

- `keys/bundle/{did}` yazma — doğrusu `keys/{did}`
- `/v1/webrtc/turn-credentials` yazma — doğrusu `/v1/rtc/turn-credentials`
- Tauri 1.x API kullanma (`SystemTray`, `CustomMenuItem`, `get_window`)
- CGO bağımlılığı ekleme (alpine'de gcc/sqlite-dev)
- `nodeStatus` fonksiyonunu api.ts'e iki kez ekleme (duplicate var, kaldırılmalı)
- Gossip relay'de NODE_ID karşılaştırması yapmayı unutma (sonsuz döngü)
- ZK kanıtı sunucu tarafında üretmeye çalışma — sadece client'ta üretilir

---

## DID Şeması (2026-08-01'de düzeltildi — commit 1aca6c3)

`GenerateDID` (`backend/internal/auth/auth.go`) daha önce `uuid.New()` ile rastgele DID üretiyordu — bu, mobile'ın sealed-sender cert'inde bağımsız hesapladığı hash-DID (`"did:obs:" + SHA256(dh_public)[:16]`, bkz. `mobile/lib/sealed-sender.ts:didFromDhPublic`) ile HİÇ örtüşmüyordu. Sonuç: her mesajda sahte-yeni X3DH bootstrap tetikleniyor, alıcının OPK havuzu israf ediliyordu (confidentiality kırılmıyordu, sadece forward-secrecy katmanı kayboluyordu).

**Fix:** `GenerateDID` artık `identityKey` parametresini (zaten alıyordu, kullanmıyordu) base64 decode edip SHA256'lıyor, mobile'daki formülle birebir aynı. `backend/cmd/did-backfill/` — mevcut kullanıcılar için tek seferlik backfill aracı (47 tablo/kolon, dry-run varsayılan). Local dev DB'de doğrulandı (eski/yeni DID + ODI eşleşmesi Python'da bağımsız cross-check edildi), Railway prod'da (test/dev ortamı, gerçek kullanıcı yoktu) da uygulandı. `backend/internal/auth/auth_test.go` — determinizm + 2 bilinen vektör + format testleri, 6/6 PASS.

---

## Ortam Değişkenleri (backend)

```
PORT=8080
NODE_ID=node-1
NODE_PEERS=node-2:8082,node-3:8083
JWT_SECRET=<random>
SMS_PROVIDER=stub          # veya twilio
FCM_PROJECT_ID=            # prod için gerekli
FCM_SERVICE_ACCOUNT_JSON=  # prod için gerekli
MINIO_ENDPOINT=minio:9000
MINIO_ACCESS_KEY=obscura
MINIO_SECRET_KEY=<secret>
MINIO_BUCKET=obscura-media
MINIO_USE_SSL=false
TURN_SECRET=<secret>
INTERNAL_SECRET=<secret>   # gossip relay auth
```

---

## Projeyi Çalıştırma

```bash
# Tüm stack (Docker)
make docker-up

# Sadece backend
make dev-backend

# Frontend
make dev-web

# Mobile
make dev-mobile

# Testler
make test

# ZK devreleri (circom + snarkjs kurulu olmalı)
make circuits-build
```

---

## Bir Göreve Başlamadan Önce Kontrol Listesi

1. Bu görev yukarıdaki YAPILDI listesinde var mı? → Tekrar yazma
2. Bu görev YAPILMADI listesinde var mı? → Hangi faza ait, sıra geldi mi?
3. Spec'te bu özelliğin detayı var mı? → `docs/spec/obscura_spec_v3.txt` (TAM SPEC)
4. Mevcut kod buna benzer bir şey yapıyor mu? → Önce `grep` ile ara
5. Plan Mode'da onayla → Sonra yaz
6. Yazdıktan sonra `code-reviewer` ajanını çağır
7. Auth/crypto/network'e dokunduysa `security-auditor` çağır
8. "Bitti" demeden önce `spec-checker` çağır
9. CLAUDE.md'deki YAPILDI/YAPILMADI listesini güncelle
10. `docs/sessions/YYYY-MM-DD.md` oluştur veya güncelle

---

## Sub-Agent Kayıt (`.claude/agents/`)

Her özel görev için ayrı bir alt-ajan var. Karmaşık iş geldiğinde `orchestrator`'ı çağır, o dağıtsın.

**Genel amaçlı:**
- `code-reviewer` — Bağımsız kod incelemesi (her kod değişikliğinden sonra)
- `security-auditor` — Güvenlik açığı taraması (auth/crypto/network değişikliğinde)
- `spec-checker` — Spec uyumu kontrolü ("bitti" demeden önce)
- `architect` — ADR yazımı, tasarım kararları, trade-off analizi
- `performance-analyst` — Profiling, latency, N+1 avı
- `tester` — Unit/integration/e2e/load/fuzz testleri
- `docs-writer` — README, API docs, runbook, ADR
- `dependency-auditor` — CVE, lisans, supply chain
- `release-manager` — Versioning, changelog, deploy
- `orchestrator` — Çoklu domain işlerini koordine eder

**Mühendislik:**
- `backend-engineer` (Go), `crypto-engineer` (Rust+Circom), `frontend-engineer` (Next.js), `mobile-engineer` (Expo), `desktop-engineer` (Tauri 2.x), `network-engineer` (libp2p+nginx+coturn), `devops-engineer` (Docker+CI), `database-engineer` (SQLite+migrations), `ui-ux-designer`

**Domain uzmanı:**
- `zk-circuit-engineer` (Circom devre tasarımı), `token-economist` (OBS ekonomisi), `mls-engineer` (grup şifreleme), `p2p-engineer` (libp2p migrasyonu), `blockchain-engineer` (zk-Rollup contracts), `mini-app-engineer` (Deno sandbox), `event-coordinator` (Bölüm 11 fiziksel), `dao-engineer` (governance), `quantum-cryptographer` (PQ), `ai-optimizer` (ZK-ML)

**Özel:** `migration-runner` (DB migration güvenliği)

**Denetim/Topluluk katmanı** (bkz. yukarıdaki Anayasa bölümü):
- `security-crypto` (Fable, sıralı/tek) — imzalı hash, kanıt bütünlüğü, sealed-sender sınırı; darboğaz kabul edilir, paralel çağrılmaz
- `core-worker` (Opus, paralel olabilir) — CRUD, şema, iş mantığı, test, docs, tarama boru hattı, UI

---

## Skill Kütüphanesi (`.claude/skills/external/`)

Topluluk skill repo'ları klonlandı (379+ SKILL.md, ~2200 markdown):
- `anthropics/` — frontend-design, skill-creator, webapp-testing, pdf, docx, pptx, xlsx, vb.
- `obra-superpowers/` — TDD, systematic-debugging, requesting-code-review, vb.
- `vercel-agent-skills/` + `vercel-next-skills/` — Next.js best practices
- `expo-skills/` — RN/Expo
- `wshobson-agents/` — 153 skill (backend pattern'leri)
- `microsoft-azure-skills/` — 62 skill (sadece referans)
- `firebase-agent-skills/`, `neon-agent-skills/`, `supabase-skills/`
- `mattpocock-skills/`, `xixu-skills/`, `currents-playwright/`, `remotion-skills/`, `coreyhaines-marketing/`

İhtiyaç olduğunda ilgili skill dosyasına bak; aktivasyon Claude Code'un skill loader'ı tarafından otomatik.

---

## Doc Yapısı (`docs/`)

```
docs/
├── spec/obscura_spec_v3.txt    ← TAM SPEC (oku, özete güvenme)
├── spec/obscura_denetim_topluluk_katmani.md  ← Denetim/Topluluk katmanı tasarımı (henüz kod yok)
├── adr/NNNN-*.md                ← Architecture Decision Records
├── api/*.md + openapi.yaml      ← Endpoint referansı
├── architecture/*.md            ← Sistem diyagramları (Mermaid)
├── circuits/*.md                ← ZK devre matematiği
├── design/*.md                  ← UI/UX kararları
├── economics/*.md               ← FAZ 2 token ekonomisi
├── postmortems/*.md             ← Incident raporları
├── protocols/*.md               ← Signal/MLS/ZK akış diyagramları
├── runbooks/*.md                ← Operasyon prosedürleri
└── sessions/YYYY-MM-DD.md       ← Her oturum logu (cross-session memory)
```

---

## Spec Bölüm Haritası (Hızlı Referans)

- **Bölüm 1-5** (PARÇA 1): Mimari, çekirdek modüller, network, kripto, kimlik
- **Bölüm 6-9** (PARÇA 2): Mesajlaşma, kredi, token, client
- **Bölüm 10-12** (PARÇA 3): Mini app, fiziksel entegrasyon, fazlar
- **Bölüm 13-16** (PARÇA 4): Eksikler dosyası (dış servisler), API'ler, diller, test
- **Bölüm 17-20** (PARÇA 5): ZK circuit kodları, deployment, güvenlik, sonuç

**Sık başvurulan:**
- Bölüm 4.5 — 7 KESİN güvenlik kuralı
- Bölüm 7.2 — Tier ayrıcalıkları (kim ne yapabilir)
- Bölüm 12.1-12.4 — Faz deliverable listesi
- Bölüm 13 — Dış servisler kurulum
- Bölüm 15.2 — Performans hedefleri
- Bölüm 17 EK B — Tüm API endpoint'leri (hedef)

---

**Son güncelleme:** 2026-08-01 (BFT/moderasyon/DID şema düzeltmeleri, gerçek koda karşı doğrulandı) — önceki: 2026-06-21 (audit + YAPILMADI listesi revize edildi)
**Spec versiyonu:** v3.0-FINAL (2026-04-26)
