# Obscura — Claude Çalışma Kılavuzu

## Proje Nedir

Obscura, WhatsApp/Telegram/Signal'a rakip, sıfır dış bağımlılıklı, Zero-Knowledge tabanlı federe mesajlaşma platformu.
Protocol-first mimari: backend protokolü sabittir, client sadece protokolü konuşan bir arayüzdür.

**Her yeni oturumda bu dosyayı tamamen oku. Özete güvenme.**

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

### ZK Devreleri
- [x] credit_threshold.circom — puan eşik kanıtı (Poseidon hash)
- [x] identity_proof.circom — DID sahipliği
- [x] message_integrity.circom — anonim grup mesajı

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

### MLS Grup Şifreleme — FAZ 1 için gerekli
- [ ] MLS (Messaging Layer Security) protokolü
- [ ] TreeKEM anahtar dağıtımı
- [ ] 10.000+ üyeli grup desteği
- [ ] mls_create_group, mls_add_member, mls_encrypt_message

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

### ZK Devreleri (Eksik) — FAZ 1-2 için gerekli
- [ ] token_balance.circom — bakiye kanıtı
- [ ] vote_proof.circom — oy gizliliği
- [ ] age_proof.circom — hesap yaşı
- [ ] activity_proof.circom — aktivite
- [ ] node_proof.circom — node çalıştırma
- [ ] storage_proof.circom — veri saklama
- [ ] Trusted setup ceremony (Groth16 için .ptau dosyası)
- [ ] .wasm ve .zkey dosyaları (circuits/build.sh hiç koşulmadı)

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
3. Spec'te bu özelliğin detayı var mı? → `backend/` veya `circuits/` altındaki dosyalara bak
4. Mevcut kod buna benzer bir şey yapıyor mu? → Önce `grep` ile ara
5. Plan Mode'da onayla → Sonra yaz
