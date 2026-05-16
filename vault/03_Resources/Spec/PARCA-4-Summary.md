# PARÇA 4 — Eksikler, API, Diller, Test (Bölüm 13-16)

**Tam metin:** [[full/PARCA-4-eksikler-api-diller-test|PARCA-4 raw]]

## Bölüm 13 — Eksikler Dosyası (Dış Servisler)

Bu bölüm "platformun çalışması için gerekli dış servisler". FAZ 1 GA için kurulması zorunlu.

| Servis | Spec sağlayıcı | Maliyet | Öncelik | Status |
|---|---|---|---|---|
| SMS Gateway | Twilio/Vonage/Netgsm/Vatansms | $50-200/ay | Kritik | 🟡 stub var, prod hesap yok |
| TURN | coturn (self) / Twilio TURN | $0 / $50 | Kritik | ✅ config, ❌ SSL cert |
| Push (FCM/APNs/WebPush) | Firebase ücretsiz | $0 | Kritik | 🟡 dev stub |
| S3/MinIO | self-hosted | $20-100 | Orta | ✅ AWS Sig v4 client, ❌ bucket |
| Monitoring | Prometheus + Grafana | $0 | Orta | ✅ Prometheus, ❌ Grafana panel |
| CI/CD | GitHub Actions | $0 | Düşük | ❌ |
| DNS/Domain | obscura.network, api.*, turn.*, zk.*, rollup.* | $10/yıl | Kritik | ❌ |
| SSL | Let's Encrypt | $0 | Kritik | ❌ |
| ZK altyapısı (Circom) | self | $0 | Kritik | ✅ |
| zk-Rollup | Aztec / zkSync provider | $100-500 | Yüksek | 🟡 Aztec stub bridge |

**Toplam başlangıç maliyeti:** $300-800/ay (5 node + servisler + ZK)

**Bölüm 13.8 — ZK altyapı kurulum** (Circom, snarkjs, Powers of Tau): adım adım. ✅ build.sh + distribute.sh tüm akış otomatik.

## Bölüm 14 — Dil/Teknoloji Haritası

**Backend (Çekirdek):**
- Go 1.21+ — libp2p-go, gorilla/websocket, protobuf, prometheus, **go-rapidsnark**
- Rust 1.75+ — **libsignal-protocol**, **openmls** ✅, ed25519-dalek, aes-gcm, **circom-compat**, **arkworks**

**ZK Katmanı:**
- JS/TS + Rust — snarkjs ✅, circomlib ✅, ffjavascript, **noir-lang** (Aztec)
- 5 circuit: identity, credit, token_balance ✅, storage_proof ✅, vote_proof ✅

**Client:**
- Önerilen: Flutter 3.19+ (Dart) — flutter_rust_bridge, sqflite, firebase_messaging, flutter_webrtc
- Obscura: Next.js + Expo + Tauri (ADR-0002 sapma)

**Blockchain:** zkSync Solidity veya **Aztec Noir** ✅, Substrate (governance pallet — FAZ 3?)

**AI/ML:** Python 3.11+, ONNX Runtime, **ZK-ML** ✅ skeleton (heuristic now, ezkl FAZ 3 — ADR-0013)

**Mini App:** TypeScript 5.3+, Deno 1.40+, WASM ZK API ✅ skeleton

## Bölüm 15 — Test ve Kalite

**15.1 Test piramidi:**
- Unit > %80 coverage (`go test`, `cargo test`, `flutter test`, **circom tester** ✅, snarkJS test ✅)
- Integration: node-arası, client-server, e2e mesaj, **ZK proof gen/verify** ✅, **MLS** ✅
- E2E: tam senaryo, Flutter integration_test, gerçek cihaz

**15.2 — PERFORMANS HEDEFLERİ (sürekli başvur):**
| Metrik | Hedef | Obscura ölçüm | Sonuç |
|---|---|---|---|
| Mesaj gecikme | <100ms yerel, <300ms küresel | n/a (prod yok) | ⏳ |
| Sesli arama başlama | <2s | n/a | ⏳ |
| Uygulama açılış | <3s | n/a | ⏳ |
| Bildirim teslim | <5s | n/a | ⏳ |
| Node senkron | <1dk | n/a | ⏳ |
| **ZK proof üretim** | <3s | 494ms (browser) | ✅ 6x |
| **ZK proof doğrulama** | <500ms | ~5ms | ✅ 100x |
| **MLS encrypt @ 1000** | <100ms | 0.13ms | ✅ 770x |
| **MLS encrypt @ 5000** | <100ms (implicit) | 0.315ms | ✅ 317x |
| **MLS decrypt** | <50ms | 0.109ms | ✅ 460x |

**15.3 Güvenlik testleri:**
- Penetrasyon testi (yıllık) — ❌
- Bug bounty programı — ❌
- **Formal verification (kripto modüller)** — ❌ (FAZ 1 GA için)
- Dependency scanning (SNYK, Dependabot) — ❌
- **ZK circuit audit (yıllık)** — ❌
- **Trusted setup ceremony (multi-party)** — ❌ ADR-0006 dev
- **Side-channel attack testi** — ❌

## Bölüm 16 — Sorun Giderme SSS (hızlı referans)

- Node başlatma, client derleme, yeni node ekleme, mesaj gitmiyor, kripto hatası, **ZK proof hatası**, test ağı kurulumu

## EK A — Protobuf Mesaj Formatı (satır ~2167)

```protobuf
message Envelope { string message_id; int64 timestamp; string from_did; string to_did; bytes ciphertext; MessageType type; ZKProof zk_proof; }
enum MessageType { TEXT, IMAGE, VOICE, FILE, CALL_INVITE/ACCEPT/END, GROUP_INVITE, ZK_PROOF }
message MLSMessage { bytes group_id; uint32 epoch; bytes ciphertext; bytes auth_tag; }
message ZKProof { string circuit_id; bytes proof_data; repeated string public_inputs; int64 timestamp; }
```

**Obscura status:** JSON-based today, protobuf migration için ADR yok.

## EK B — API Endpoint Hedef Listesi

POST /v1/register, /v1/login, /v1/verify-otp / GET /v1/keys/{did} / POST /v1/messages / WS /v1/stream / POST /v1/call/invite, /v1/call/answer / GET /v1/nodes / POST /v1/governance/{proposal,vote}

**Yeni (FAZ 1+):**
- POST /v1/zk/{prove,verify} ✅
- POST /v1/credit/upgrade ✅
- POST /v1/wallet/shielded ✅
- POST /v1/mls/{group,join} ✅

Tüm Obscura endpoint listesi: `backend/cmd/node/main.go`
