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
| **FAZ 2** Çekirdek | zk-Rollup, OBS wallet, mini app, ZK-ML, governance, MLS 5000+, staking | Bölüm 12.2 | %0 |
| **FAZ 3** Federasyon | Permissionless nodes, BFT, recursive ZK, post-quantum prep, cross-chain | Bölüm 12.3 | %0 |
| **FAZ 4** Otonomi | Full DAO, quantum crypto, AI optimization, sequencer decentralization, GPS+ZK | Bölüm 12.4 | %0 |

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

### Mobile (React Native/Expo)
- [x] Login (SMS OTP)
- [x] Sohbet listesi + detayı
- [x] Ayarlar (profil düzenleme modal)
- [x] Aramalar

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

## ❌ YAPILMADI — Kritik Eksikler

### P2P Network (Spec Bölüm 2-3) — FAZ 1 için gerekli
- [ ] libp2p host + DHT (Distributed Hash Table)
- [ ] QUIC transport
- [ ] GossipSub (gerçek pub/sub — şu an basit HTTP relay var)
- [ ] Bootstrap DNS / ENS fallback
- [ ] ZK proof ile node yetki doğrulaması

### MLS Grup Şifreleme — FAZ 1 için gerekli (kısmen tamam — 2026-05-10)
- [x] openmls 0.6 entegrasyonu (RFC 9420, ADR-0007)
- [x] Ciphersuite: MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519
- [x] Identity, KeyPackage, create_group, add_member, encrypt/decrypt
- [x] 2-party + 3-party tam akış testi geçiyor
- [ ] FFI exports (crypto/src/ffi.rs'e ekle)
- [ ] Backend Go entegrasyonu (subprocess + JSON RPC)
- [ ] DB schema: mls_key_packages, mls_groups, mls_pending_proposals
- [ ] API endpoints: POST /v1/mls/{group,key-package,join,commit}, GET /v1/mls/group/{id}/state
- [ ] WebSocket message types: mls_welcome, mls_commit, mls_message
- [ ] Frontend WASM build of openmls
- [ ] Persistent storage backend (SQLite — şu an in-memory)
- [ ] KeyPackage rotation (90 gün, spec Bölüm 4.2)
- [ ] 1000+ üyeli grup performans testi (spec Bölüm 15.2)
- [ ] Remove member + key update operations

### Shard Storage — FAZ 1 için gerekli
- [ ] Mesajları 256KB shard'lara bölme
- [ ] Reed-Solomon 4-of-6 erasure coding
- [ ] Dağıtık depolama (farklı node'lara)
- [ ] ZK storage proof (storage_proof.circom)
- [ ] 30 gün TTL + otomatik silme

### Rust Crypto Crate — FAZ 1 için gerekli
- [ ] obscura-crypto: X3DH, Double Ratchet, SRTP (şu an Go'da stub)
- [ ] obscura-zk: ZK proof üretimi native (şu an snarkjs ile tarayıcıda)
- [ ] flutter_rust_bridge FFI (spec Flutter istiyor, biz RN yaptık)

### Tasarım skill'leri (frontend/mobile/desktop için)
Her UI işinde sırayla çağır:
1. `.claude/skills/frontend-design-obscura/SKILL.md` — tasarım tokenleri, theme, Telegram/Element referansı
2. `.claude/skills/motion-principles-obscura/SKILL.md` — animasyon, easing, reduced-motion
3. `.claude/skills/impeccable-obscura/SKILL.md` — done demeden önce polish/critique
Kaynak library'ler: `.claude/skills/external/{anthropics,pbakaus-impeccable,leonxlnx-taste,arvindrk-design-system,vercel-agent-skills,wshobson-agents/plugins/ui-design}`

### ZK Devreleri (Hâlâ Eksik) — FAZ 2 için
- [x] token_balance.circom — ✅ (944 constraints, pipeline tam)
- [x] vote_proof.circom — ✅ (733 constraints, pipeline tam)
- [x] storage_proof.circom — ✅ (FAZ 1'de bitti)
- [ ] age_proof.circom — hesap yaşı (kredi puanı)
- [ ] activity_proof.circom — aktivite (kredi puanı)
- [ ] msg_count_proof.circom — mesaj sayısı (kredi puanı)
- [ ] node_proof.circom — node çalıştırma kanıtı
- [ ] endorsement_proof.circom, streak_proof.circom — kredi bileşenleri
- [ ] Multi-party trusted setup ceremony (production öncesi — bkz ADR-0006)
- [ ] `frontend/lib/zk.ts` smoke.js input formatına göre revize

### Kimlik & Cihaz Yönetimi — FAZ 1 için gerekli
- [ ] 12 kelime mnemonic (BIP39) yedekleme
- [ ] Social recovery
- [ ] Cross-signing (çoklu cihaz QR onayı)
- [ ] ZK-ID secret türetme (mnemonic'den)

### Token Ekonomisi — FAZ 2
- [ ] OBS Token (1 milyar arz)
- [ ] zk-Rollup (StarkNet / zkSync / Aztec)
- [ ] Shielded wallet (gizli transfer)
- [ ] Staking mekanizması
- [ ] Revenue sharing

### Governance / DAO — FAZ 2
- [ ] ZK vote (oy tercihi gizli)
- [ ] 3/5 multisig node kararları
- [ ] Proposal + 48 saat zaman kilidi
- [ ] Governance portal

### Mini App Motoru — FAZ 2
- [ ] Deno sandbox runtime
- [ ] Mini app API bridge (identity, messaging, wallet, zk, ui)
- [ ] İzin sistemi (manifest + kullanıcı onayı)

### Fiziksel Entegrasyon — FAZ 2
- [ ] Etkinlik oluşturma/katılım
- [ ] QR kod check-in
- [ ] Konum bazlı keşif (1km grid, ZK proof ile)

### FAZ 3 — Federasyon
- [ ] Açık node (herkes kurabilir)
- [ ] Recursive ZK proof
- [ ] Cross-chain bridge
- [ ] PLONK / STARK
- [ ] GPU/FPGA ZK hızlandırma
- [ ] Formal verification

### FAZ 4 — Otonomi
- [ ] Tam DAO yönetimi
- [ ] Kuantum dayanıklı kripto (CRYSTALS-Kyber/Dilithium)
- [ ] AI node optimizasyonu
- [ ] GPS + ZK location proof

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

## Sık Yapılan Hatalar — Yapma

- `keys/bundle/{did}` yazma — doğrusu `keys/{did}`
- `/v1/webrtc/turn-credentials` yazma — doğrusu `/v1/rtc/turn-credentials`
- Tauri 1.x API kullanma (`SystemTray`, `CustomMenuItem`, `get_window`)
- CGO bağımlılığı ekleme (alpine'de gcc/sqlite-dev)
- `nodeStatus` fonksiyonunu api.ts'e iki kez ekleme (duplicate var, kaldırılmalı)
- Gossip relay'de NODE_ID karşılaştırması yapmayı unutma (sonsuz döngü)
- ZK kanıtı sunucu tarafında üretmeye çalışma — sadece client'ta üretilir

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

**Son güncelleme:** 2026-05-09
**Spec versiyonu:** v3.0-FINAL (2026-04-26)
