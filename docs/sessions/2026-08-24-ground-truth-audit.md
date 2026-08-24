# Oturum — 2026-08-24 — Tam Yer-Gerçeği Denetimi (E1-E5 + teknik borç + harita-dışı mimari)

Git HEAD: `6e05e24` (2026-08-15), working tree temiz, origin/master'ın 93 commit ilerisinde.

Metodoloji: TAHMİN YOK, her iddia dosya:satır kanıtı taşıyor. Vault/ADR/yorum/README
status etiketlerine güvenilmedi — hepsi kod karşısında yeniden doğrulandı (bazıları
BAYAT çıktı, aşağıda işaretli). 5 paralel alt-denetim (fork) + ana oturumda kişisel
doğrulama (grep, test çalıştırma, dosya okuma) birleştirildi.

## Özet

Obscura'nın kripto/protokol çekirdeği (1:1 E2EE, MLS relay+client kriptosu, marketplace
escrow, BFT wiring, self-destruct, panik-uyarı) kod seviyesinde büyük ölçüde GERÇEK ve
test edilmiş — ama üç kritik launch-blocker hâlâ açık: (1) grup mesajlaşması mobile'da
kripto+API+create/join akışı tam olsa da chat ekranı hiçbir zaman gerçek gönder/al
çağrısına bağlanmadı (E1 kapanmadı), (2) marketplace'in tam+testli backend'i (escrow,
dispute) hiçbir istemcide (mobile/web) UI'a sahip değil (sıfır kullanıcı erişimi), (3)
P2P bootstrap discovery hâlâ tamamen kopuk ve prod compose'da libp2p zaten kapalı — federasyon
sync'i hâlâ eski HTTP gossip'e dayanıyor. Ayrıca CI'nin "yeşil" göstergesi yanıltıcı:
crypto-cli/mls-cli/deno'ya bağımlı testler CI'da hiç çalışmıyor (env değişkenleri set
edilmemiş, sessizce skip).

---

## E1 — Grup E2E Mesajlaşma

**Kapanış kriteri (ADR-0019, satır 283-287):** "Mobile, `/v1/mls/*`'e gerçek MLS mesajı
gönderip başka bir üyenin bunu ts-mls ile çözdüğü an kapanmış sayılır." **KAPANMADI.**

| Kalem | Sınıf | Kanıt | Not |
|---|---|---|---|
| L1 transport gate (mobile) | GERÇEK | `mobile/lib/group-send-gate.ts` (`classifyConv`/`isGroupSendBlocked`/`resolveSendTarget`), `chat/[id].tsx:191,258,306,328,370` hepsi `conv?.peer_did`'e bağlı | Fail-closed by construction — flag değil, kod yolu hiç yazılmamış. Kasıtlı ve doğru. |
| Backend sealed-sender/MLS honesty-gate | GERÇEK | `handlers.go:698` `encryption_type:"mls"` zorunlu, `models.go:256-263 EffectiveEncryptionType()` | Kriptografik kanıt değil, sadece etiket zorunluluğu (ADR-0019 kendi ifadesi). |
| ts-mls vendor izolasyonu | GERÇEK | ADR-0019 revizyon (2026-08-13), `mobile/metro.config.js`, `vendor/ts-mls/` | 3/3 spike kanıtı: izole akış çalıştı, mobile Jest/Metro shadow'lamadı, dual-package sızıntısı yok. |
| MLS kripto çekirdeği | GERÇEK | `mobile/lib/mls/group.ts` — `encryptGroupMessage:248`, `decryptApplicationMessageWire:324`, `createGroupWithMember:229`, `addMemberToGroup:203`, `joinFromWelcomeWire:298` | RFC9420 passive-client-welcome 8/8, openmls interop epoch_authenticator byte-eşleşme. |
| MLS API client | GERÇEK | `mobile/lib/mls/mlsApi.ts` — 8 endpoint, gerçek `apiFetch` | Mock değil, gerçek `/v1/mls/*` HTTP wrapper. |
| Encrypted state persistence | GERÇEK | `mobile/lib/mls/mls-store.ts` — AES-256-GCM (session-store primitifi), `MLS_RETAIN_EPOCHS=2`, consumed-key zeroize | Tam state her zaman yazılır/okunur, kırpma yok (bilinçli, MLS state kısmi kalırsa üye kopar). |
| Create-group akışı | GERÇEK+bağlı | `mobile/lib/mls/createGroupFlow.ts` → `new-group.tsx` | `new-group-mls-wiring.test.ts` PASS. |
| Join-group akışı | GERÇEK+bağlı | `mobile/lib/mls/joinGroupFlow.ts`, `inviteBootstrap.ts` → `mls-invites.tsx` | `mls-joinGroupFlow.test.ts`, `mls-invite-bootstrap-wiring.test.ts` PASS. |
| Backend `/v1/mls/*` relay | GERÇEK+TAM | `mls_handlers.go` (1306 satır), `mls_messages`/`mls_groups`/`mls_key_packages`/`mls_group_members`/`mls_pending_proposals`/`mls_welcome_queue` (database.go migration 003-010) | "Server holds NO group secrets" — ciphertext_b64 hiç parse edilmiyor. |
| Golden-wire relay testi | GERÇEK, backend-only | `backend/internal/api/mls_relay_golden_test.go` — httptest+gerçek router+AuthMiddleware+SQLite, openmls'e (cargo test subprocess) karşı çapraz doğrulandı | Commit `942802b`'nin kendi mesajı: "**E1 kapanmadı**: commit_b64 golden fixture'da yok... katılım yolu wire fidelity'si henüz hiçbir katmanda test edilmedi." |
| **Chat ekranı gönder/al entegrasyonu (Tuğla 5/6)** | **YOK** | `grep mlsApi\|sendGroupMessage\|encryptGroupMessage\|decryptApplicationMessageWire mobile/app/(main)/chat/[id].tsx` → **0 sonuç** | Kripto+API+create+join hazır ama hiçbir UI kod yolu bunları çağırmıyor. `group-send-gate.ts:8-13` kendi yorumunda bunu itiraf ediyor: "L2 gerçek MLS-encrypt çağrısını yazana kadar" bloklu kalacak. |

**Test kanıtı (kendim çalıştırdım):** `cd mobile && npx jest lib/__tests__/mls-` → **15 suite, 64 test, hepsi PASS.**

**Sınıf ayrımı (vault'un kendi "META-BULGU"suna göre, tek katman skorlamaktan kaçınmak için):**
- **Bileşen tamamlanma:** ~85-90% (7/8 alt-bileşen GERÇEK+test'li, sadece son entegrasyon eksik).
- **Kullanıcı-erişilebilir özellik:** **%0** — bugün hiçbir gerçek kullanıcı grup içinde şifreli mesaj gönderip alamaz. ADR'nin kendi kapanış kriterine göre **E1 KAPALI DEĞİL.**

---

## E2 — Grup Türleri (L3 açık/kapalı, L4 kanal, L5 topluluk)

| Kalem | Sınıf | Kanıt | Not |
|---|---|---|---|
| `conv_type` kolonu | KISMİ | `extra_handlers.go:195,231,296`, migration `database.go:780` | Gerçek DB kolonu ama tek davranış farkı: `extra_handlers.go:267` "group" için min-1-üye şartı. Mesaj/yetki mantığı `conv_type`'a hiç bakmıyor. |
| L3 `is_public` policy | KISMİ/STUB | `database.go:782`, discovery `extra_handlers.go:821`, moderasyon kapsamı `umay/monitor.go:76` | Görünürlük bayrağı olarak çalışıyor ama genel "kendi kendine katıl" endpoint'i yok — sadece tek hardcoded topluluk (`governance_handlers.go:385-412`, `obs-community-v1`). |
| Mobile UI (channel/community) | GERÇEK | `mobile/app/(main)/new-channel.tsx:25`, `new-community.tsx:35`, `group-profile.tsx:264,277` → gerçek `api.updateConversation` | Backend'e gerçek istek, mock değil. |
| Frontend (web) | YOK | sadece bildirim-tercihi ikonu (`frontend/app/settings/notifications/page.tsx:20,75`) | Oluşturma/görüntüleme akışı yok. |

**Sonuç:** L4/L5 backend'de ayrı yetkilendirme/davranış olarak **gerçek değil** — hepsi aynı `is_group=1` kod yolundan geçiyor, sadece string label + kısmi mobile UI farkı var. Kullanıcı promptundaki hipotez doğrulandı.

**Sınıf:** ~35-40% (1 GERÇEK + 2 KISMİ + 2 fiilen-YOK, ağırlıklı).

---

## E3 — Federasyon (L6 sync, #40 P2P bootstrap)

| Kalem | Sınıf | Kanıt | Not |
|---|---|---|---|
| L6 aktif mekanizma: HTTP gossip | GERÇEK, TEST YOK | `gossip.go:80 RelayToPeers`, çağrı: `handlers.go:990`, `recall_handler.go:196` | ADR-0003 "mvp" yolu — hâlâ TEK aktif inter-node sync yolu. `gossip/` içinde `_test.go` yok. |
| libp2p + GossipSub + DHT | KISMİ (kod gerçek, prod'da KAPALI) | `p2p/host.go:40-114` | `docker-compose.yml` 5/5 node'da `P2P_ENABLED: "false"` (satır 138,175,208,241,274) — **vault'un "HTTP gossip'in yerini alıyor" iddiası yanlış**, ikisi yer değiştirmedi. |
| #40 bootstrap discovery | STUB/ölü kod (doğrulandı, değişmemiş) | `discovery.go:45 DiscoverBootstrapPeers` — 0 çağrı noktası; gerçek yol `host.go:111` doğrudan `BOOTSTRAP_PEERS` env | `docker-compose.yml` 5/5 node'da `BOOTSTRAP_PEERS: ""` — taze deploy düğümleri birbirini bulamaz. |
| `peer_cache.go` | YOK, çifte ölü kod | `SavePeer:40` de dahil **sıfır** çağrı noktası (önceki bilgiden daha kötü — sadece okuma değil, yazma da hiç tetiklenmiyor) | |
| Federation node registry | GERÇEK+test'li | `federation.go:178-328` (Register/List/Get/Heartbeat/ProbeHealth), imza doğrulama (ADR-0018), 28 test | Node-kayıt işlevi — "L6 mesaj senkronu" değil, ayrı bir katman. |

**Sınıf:** ~40-45%. **Launch-blocker (#40) hâlâ açık, değişmemiş.**

---

## E4 — Blockchain Güvenliği (#25 BFT)

**Vault iddiası ("İZOLE İSKELET, ENTEGRE DEĞİL, sıfır test") ARTIK YANLIŞ** — durum
commit `98466a4` (2026-08-02) ile değişti, vault notu (2026-08-11) hâlâ eski metni taşıyor.
**Status etiketi bayat/yanlış.**

| Kalem | Sınıf | Kanıt | Not |
|---|---|---|---|
| Wiring | GERÇEK | `main.go:209-259` — engine kuruluyor, `StartProposerLoop` çağrılıyor, gerçek libp2p GossipSub transport | Mock/LocalTransport değil. |
| Token→consensus besleme | GERÇEK+test'li | `token.go:41-53,324,406,465,525 SetOpRecorder` | `token_oprecorder_test.go` 6 test PASS. |
| **Token yazması konsensüsten geçiyor mu** | **HAYIR — sadece raporlanıyor** | `bft.go:47-51`, `store.go:52-54`: "BAKİYEYE DOKUNMAZ, zaten senkron uygulanmıştır" | ADR-0017 kasıtlı "sonradan-tasdik" tasarımı — kusur değil ama "konsensüs para hareketini güvenceye alıyor" iddiası yanlış olur. |
| İmza doğrulama | STUB | `bft.go:61` `Sig string // Ed25519 (stub)`, `handleMsg`/`collectVote` (`bft.go:205-264`) Sig'i hiç okumuyor | `main.go:191` kendi yorumu: "ERTELENMİŞ: oy imzalama/doğrulama". Sahte NodeID'li oy kabul edilir. |
| Quorum | Stake-ağırlıklı DEĞİL | `main.go:203-204` düz `(2*totalNodes)/3+1` | Leader-election ayrı mekanizma (sequencer VRF) stake-ağırlıklı, ama oy/quorum değil. |
| Bütünlük + persistence | GERÇEK+test'li | `bft.go:226-230` (TxRoot↔Ops merkle), `store.go` (idempotency+replay-guard) | 27 test, hepsi PASS (`TestEndToEnd_TwoNodes_ReachQuorumAndBothCommit` dahil). |

**Sınıf:** ~55-60%. Wiring/persistence/audit-trail sağlam, ama iki kritik Bizans-güvenlik
bacağı (imza, stake-ağırlıklı quorum) bilinçli stub — küçük azınlık node ele geçirilirse
sahte oylarla quorum toplanabilir.

---

## E5 — Launch

| Kalem | Sınıf | Kanıt | Not |
|---|---|---|---|
| #30 marketplace mobil UI | **YOK** | `grep -rli marketplace mobile/ frontend/` → **0 sonuç** (hem mobile hem web) | Backend 10 endpoint + escrow/dispute TAM+test'li (`go test ./internal/marketplace/...` → **PASS**, kendim çalıştırdım) — ama sıfır kullanıcı erişimi. |
| #11 ödeme | **YOK** | `grep -rni "stripe\|paypal\|payment"` → hepsi ya OBS-token iç transferi ya NFC-token wrapper'ı ya alakasız placeholder (`apps-api-connect.tsx:142`) | Gerçek fiat/harici ödeme sağlayıcı sıfır satır. |
| #10 ops | KISMİ | `.github/workflows/ci.yml` (vet+build+test-race+govulncheck), `prometheus/`+`grafana/` config gerçek | P2P prod'da kapalı (bkz. E3). |
| #39 | **DOĞRULANAMADI** | `grep -rn "#39"` repo genelinde 0 alakalı sonuç | Ticket'ın ne olduğu bulunamadı. |
| #12 güvenlik denetimi | KISMİ | ADR-0009 (6 critical+10 code-review+13 re-audit, hepsi "FIXED") — spot-check: HMAC timing-safe compare iddiası `gossip.go:161 hmac.Equal(...)` **doğrulandı gerçek** | ADR kendi itirafı: harici pentest hâlâ yapılmadı. 23 maddenin tamamı yeniden doğrulanmadı (kapsam/zaman dışı). |
| Self-destruct (mesaj) | **GERÇEK** (eski "%0" notu YANLIŞ) | `database.go:871-873`, `handlers.go:716-732`, `expiry.go:92-168`, mobile `self-destruct.ts`+`chat/[id].tsx:124-254` | Geniş test kapsamı (`self_destruct_send_test.go` vb). |
| Panik butonu | **GERÇEK** | `mobile/app/(main)/panic.tsx`, `lib/panic.ts`/`panic-flow.ts` → `api.getContacts`/`sendPanicAlert`, `expo-location` | Konum-uyarısı; cihaz-imha DEĞİL (o özellik hiç yok, ama muhtemelen hiç istenmedi). |

**Sınıf:** ~40-45% (marketplace UI + ödeme YOK ağırlıklı düşürüyor; self-destruct/panik + ops kısmi telafi ediyor).

---

## Teknik Borç

| Kalem | Sınıf | Kanıt | Not |
|---|---|---|---|
| `sealed_policy.go` "content-verification" | **Yanlış isimlendirme** | `sealed_policy.go:1-64` | Content-verification değil — kademeli-geçiş POLICY TOGGLE'ı (`OBSCURA_SEALED_SENDER_REQUIRED`). Kendi içinde gerçek+çalışır. İçindeki yorum (satır 14-19, "chat ekranına henüz bağlanmadı") **BAYAT** — chat/[id].tsx artık gerçekten `sendSealedMessage` çağırıyor. |
| #19 sequencer batch | KISMİ | `sequencer.go:615-634 SubmitBatch`, endpoint `faz4_handlers.go:226-247`, test PASS | `s.batches` **sadece bellek-içi** — DB'ye yazılmıyor, süreç yeniden başlarsa kaybolur. Gerçek token/marketplace tx'i bu batch'ten GEÇMİYOR — dekoratif/demo seviyesi. |
| #22 miniapp seccomp+Deno | KISMİ | Deno spawn GERÇEK (`sandbox.go:69-140`, gerçek subprocess) | Seccomp **kendi kendini itiraf eden STUB**: `seccomp_linux.go:38-43` — `log.Printf("...stub...")`, `TODO(FAZ-3)` (satır 7,37,39), gerçek prctl/BPF yok. İzolasyon sadece Deno `--deny-*` + cgroup CPU limitine dayanıyor. |
| Postgres `db.Init()` | STUB (bilinçli) | `database.go:18-25` — `OBSCURA_DB_DRIVER=postgres` → direkt hata | Sadece SQLite aktif (ADR-0001 kararıyla tutarlı). |
| #2 Sybil TOCTOU | **GERÇEK, kapalı** | `sybil.go:29-35 ComputeNullifier`, `airdrop.go:321-363` (BEGIN TX + INSERT + UNIQUE violation → `ErrAlreadyClaimed`), migration `nullifier TEXT NOT NULL UNIQUE` | Doğru desen — SELECT sadece erken-çıkış, gerçek atomiklik TX+UNIQUE'den geliyor. |
| CI race | **KISMİ/YANILTICI YEŞİL** | `.github/workflows/ci.yml:34` `go test -race -cover ./...` var, ama `CRYPTO_CLI_PATH`/`MLS_CLI_PATH`/`deno` hiçbir workflow adımında set edilmiyor | `sealed_sender_test.go:17-24 requireCLI` boşsa `t.Skip`. **CI "PASS" gösteriyor ama en kritik cross-language crypto roundtrip testleri (sealed-sender, mls/client, signal/crypto_cli) hiç ÇALIŞMIYOR, sessizce skip ediyor.** |

---

## Harita-Dışı Bulgular (kodda var, kullanıcı haritasında yok)

- **`backend/internal/umay/`** — ~850 satır tam yazılmış İKİNCİ bir içerik-izleme/moderasyon
  borusu (monitor+notify+classifier+testler), `main.go`'da **hiç başlatılmıyor** — tam ölü
  kod. Spec Bölüm 1.4'te var ama hiç bağlanmamış.
- **`packages/e2ee/`** — boş kabuk: `package.json` var, `src/` dizini **fiziksel olarak
  yok**. Gerçek E2EE kodu `mobile/lib/e2e.ts`'te yaşıyor — biri bu paketin gerçek olduğunu
  sanabilir, değil.
- **`desktop/` (Tauri)** — gerçek Rust kodu var (`commands/lib/main/tray.rs`, 177 satır)
  ama hiç başarıyla derlenmemiş (`dlltool.exe: program not found`, 2026-06-21 tarihli
  çözülmemiş not, sonrasında düzeltme kanıtı yok).
- **`zk/` (üst-seviye, 3 circom)** — muhtemelen ölü/eski circuit seti, aktif build zinciri
  (`circuits/` → `internal/zk/verifier.go`) buna referans vermiyor. Doğrulanmalı/temizlenmeli.
- **`contracts/aztec/`** — 4 Noir contract, **sıfır test dosyası**.
- **`contracts/bridge/`** — Solidity gerçek+derlenmiş (artifacts mevcut), Go relayer'da
  gerçek DOT-taraf wiring kodu (`relayer.go`, `dot.go`, `extrinsic.go`) var. Vault'un
  "Sepolia+Paseo'da gerçek transfer test edildi" iddiası (`docs/sessions/2026-08-01-bridge-dot.md`)
  detaylı kanıtla belgelenmiş ama **bugün zincir-üstü durumu bu denetimde yeniden
  doğrulanamadı** (RPC sorgusu gerektirir, kapsam dışı) — "belirsiz" kategorisinde.

---

## Status Etiketi Yalanı / Bayat Bulgular

1. **Vault `Phase-Status.md` FAZ3 madde 3** — "İZOLE İSKELET, ENTEGRE DEĞİL, sıfır test"
   diyor; gerçekte `98466a4` (2026-08-02) ile wiring+27 test tamamlanmış. Vault notu
   (2026-08-11 tarihli) bunu hiç yansıtmıyor.
2. **Vault `Phase-Status.md` FAZ3 madde 2** — "✅ libp2p... HTTP gossip'in yerini alıyor"
   — yanlış, `docker-compose.yml`'de 5/5 node'da `P2P_ENABLED=false`, HTTP gossip hâlâ
   tek aktif yol.
3. **Marketplace escrow** — vault sadece "KARAR VERİLDİ" (mühürlenmiş plan) diyor
   (2026-08-11); kod planın 5 adımının hepsini (`c53eeac`,`8b17ce6`,`6c24c8d`,`5b89282`,
   `0ad9386`,`7707396`, 2026-08-12) tamamlamış ve test etmiş — vault hiç güncellenmemiş
   (yalan değil ama tehlikeli derecede bayat — biri "hâlâ karar aşamasında" sanabilir).
4. **`sealed_policy.go:14-19` yorumu** — "chat ekranına henüz bağlanmadı" diyor, kod artık
   bağlı (`chat/[id].tsx` gerçekten `sendSealedMessage` çağırıyor).
5. **Eski denetim notu "self-destruct/panik %0"** — artık yanlış, ikisi de gerçek+test'li.
6. **CI yeşili** — `-race` bayrağı var ama crypto-cli/mls-cli/deno bağımlı testler sessizce
   skip ediliyor; "tüm testler PASS" görüntüsü yanıltıcı.

---

## Hedefe Kalan (kritik yol)

**Launch-blocker (sıralı öncelik):**
1. **E1** — chat ekranını `/v1/mls/*`'e bağla (encryptGroupMessage/sendGroupMessage gönderimde,
   getGroupMessages/decryptApplicationMessageWire alımda). Kripto+API+persistence zaten hazır,
   kalan iş net ve sınırlı.
2. **#30 Marketplace UI** — backend tam hazır, mobile (ve istenirse web) ekranı sıfırdan.
3. **#40 P2P bootstrap** — `DiscoverBootstrapPeers`'ı gerçek yola bağla veya (P2P zaten
   prod'da kapalıysa) HTTP gossip'i resmi launch yolu ilan edip libp2p'yi FAZ3+ öteleyerek
   iddiayı düzelt.
4. **CI crypto-cli/mls-cli/deno testleri** — env değişkenlerini CI'da set et (binary build
   adımı ekle), sessiz skip'i kapat.
5. Harici pentest (#12'nin kendi itirafı), multi-party trusted setup, real SMS/FCM — vault'ta
   zaten bilinen GA-blocker'lar, bu denetim onları değiştirmedi.

**Launch-blocker olmayan ama önemli:** BFT imza+stake-quorum (ADR-0017 tasarımı gereği
zaten launch-gating değil), seccomp gerçek implementasyonu, sequencer batch persistence,
E2/E3 kavramsal netlik (L4/L5'in gerçekten ayrı bir şey mi yoksa terk mi edileceği kararı).

---

## Belirsiz / Doğrulanamayan Kalemler

- **#39** — repoda/vault'ta hiçbir iz yok, ne olduğu bilinmiyor.
- **Bridge'in canlı zincir-üstü durumu** (Sepolia/Paseo) — sadece belgeye dayanıyor, bugün
  RPC ile yeniden doğrulanmadı.
- **`backend/internal/{ai,airdrop,auth,bots,credit,dao,governance,identity,media,moderation,
  push,scanner,signal,sms,storage,subscriber,pqcrypto,zk}`** — sadece "wired mi" seviyesinde
  hızlı taranıp (harita-dışı fork), 4-soru testinin derinliğiyle TEK TEK denetlenmedi.
  Sığ doğrulama — gerçek olduğu varsayılmamalı, sadece "muhtemelen gerçek" işaretlendi.
- **`mobile/android`, `frontend/` derinlik**, **`packages/api`/`packages/store` tüketicileri**
  — zaman/kapsam nedeniyle atlandı.
- **#12'nin 23 maddesinin 22'si** — sadece 1 tanesi (HMAC timing-safe) kod-seviyesinde
  spot-check edildi, gerisi ADR metnine dayanıyor.

---

## Yüzde Metodolojisi

Her eşik için: madde sayısı × sınıf ağırlığı (GERÇEK=1, KISMİ=0.5, STUB≈0.15-0.3 bağlama
göre, YOK=0), ağırlıklı ortalama. Detay yukarıdaki tablolarda.

| Eşik | % | Gerekçe (kısa) |
|---|---|---|
| E1 | **bileşen ~85-90% / kullanıcı-erişilebilir %0** | 7/8 alt-bileşen gerçek+test'li ama TEK kapanış kriteri (chat-ekranı gönder/al) karşılanmadı |
| E2 | ~35-40% | 1 GERÇEK, 2 KISMİ, 2 fiilen-YOK |
| E3 | ~40-45% | node-registry gerçek, ama aktif sync yolu (HTTP gossip) test'siz, #40 hâlâ ölü, libp2p prod'da kapalı |
| E4 | ~55-60% | wiring/persistence/audit-trail sağlam, imza+stake-quorum bilinçli stub |
| E5 | ~40-45% | marketplace UI+ödeme YOK ağırlıklı düşürüyor, self-destruct/panik+ops telafi ediyor |

**Tek "Obscura %X" verilmedi** — bu denetim E1-E5+teknik borç+harita-dışı taramaya
odaklandı, FAZ1/FAZ2'nin geri kalanı (zaten çoklu önceki denetimden geçmiş, spot-check
dışında bu oturumda yeniden doğrulanmadı) dahil tam-proje ağırlıklı ortalama uydurma
kesinlik olurdu. Launch açısından en somut sonuç: **3 net launch-blocker (E1 chat-wiring,
marketplace UI, #40 bootstrap) + 1 sessiz-güven-kırığı (CI skip) hâlâ açık.**
