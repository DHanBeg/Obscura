---
proje: Obscura
tip: master-liste
guncellendi: 2026-08-26
durum: aktif
kullanim: CC bu listeyi tek kaynak alır — biten kalemi [x] yapar, yeniden DENETLEMEZ
etiketler: [obscura, roadmap, checklist]
---

# Obscura — Master Liste

> **CC için kural:** Bu liste tek kaynak. Bir kalem bitince `[ ]` → `[x]`, bir satır
> kanıt (commit hash + dosya:satır) ekle. Yeniden denetim YAPMA — durum burada.
> Her kalem kendi commit'i, kendi push'u. Kanıtsız `[x]` yok.

---

## Vizyon → Kapsama (5 madde, mevcut gerçek durum)

| # | Madde | Durum | Neye bağlı |
|---|---|---|---|
| 1 | Gizlilik | ✅ karşılanıyor (en olgun) | E2E gerçek+kanıtlı |
| 2 | Hız | 🟡 kısmi | real-time yok (polling) → B6 |
| 3 | Güvenlik | ✅ mesaj + trustless çekirdeği kanıtlı (2026-08-26, A bloğu kapandı) | — |
| 4 | Kesintisiz iletişim + veri | 🟡 çekirdek trustless kanıtlı, geniş dağıtım/ops ayrı (bkz. C12) | C bloğu |
| 5 | Gelişim-ödülü + anti-bağımlılık sosyalleşme | ❌ yok | yeni katman, önce tasarım |

**Not:** 2-3-4 aynı çekirdeğe (A bloğu) bağlı — A biterse üç madde birden ilerler.
5. madde tamamen ayrı: token/marketplace onun yakıtı, motoru değil. Motor henüz yok.

---

## ✅ BİTTİ

- [x] **E1 — grup E2E mesajlaşma** · canlı smoke, 2 AEAD bug fix · *(madde 1)*
- [x] **#30 — marketplace UI (mobile)** · escrow uçtan uca canlı · *(altyapı)*
- [x] **#30 — marketplace UI (web)** · 13-adım canlı akış, auth guard · *(altyapı)*
- [x] **CI dürüstlüğü** · silent-skip kapandı, dürüst ○ skip
- [x] **A1 — libp2p wiring** · discovery→host, SavePeer bağlandı, "0 çağrı" bitti · *(madde 3,4)*
- [x] **B7 — grup türleri (E2)** · TAMAMEN KAPANDI (2026-08-25, commit `57c47a9`+`313210e`+`832f6a4`+`30b71d3`+`a552d76`+`f2c6dba`+`72995b4`) — backend yetki gate'i (send-path + invite güvenlik deliği kapandı, kanal=admin-only yazar, topluluk=herkes davet eder) + web UI (kanal/topluluk/grup oluşturma gerçek backende bağlandı, tür-farkındalı composer/davet gating, is_public keşif+self-join dürüst mls_synced:false ile). Gerçek backend'e karşı canlı doğrulandı (3 kullanıcı, tam akış). MLS'e dokunulmadı — açık sorular A4'e taşındı (aşağıda). *(madde 1,2,3)*
- [x] **A2 — bootstrap otomasyonu (davetli-ağ)** · KARAR MÜHÜRLÜ, KAPANDI (2026-08-26, commit `6257627`) — `centralDiscoverySourcesEnabled=false` sabiti DNS-TXT/ENS'i pasifledi (kod silinmedi, derleme-zamanı sabit), env+peer_cache tek aktif kaynak. Gerçek iki-node kanıtı: env-tohum bir kez → cache'ten otomatik devam (BOOTSTRAP_PEERS olmadan restart, buldu+bağlandı). *(madde 3,4)*

---

## 🔴 BLOK A — Trustless çekirdeği · *(madde 3, 4)*
> v1 launch şartı. Bitmeden "trustless" yazılamaz. En ağır blok.

- [x] **A3 — BFT güvenlik-kasları (imza)** · KAPANDI (2026-08-26, commit `05d2b82`+`c3e3b88`) — Vote.Sig (A3.1) + Block/proposal Sig (A3.2) stub→gerçek Ed25519, federation registry (P2P identity anahtarı) doğrulama kaynağı, 4+4 test (pozitif/sahte/tampering/yanlış-anahtar) + mutasyon testi ikisinde de. **Kalan (A3 dışı, kasıtlı ayrı bırakıldı):** quorum hâlâ düz peer-sayısı (stake-ağırlıklı değil), token yazması konsensüse bağlanmadı (ADR-0017).
  - *Model: CC (Opus) — bitti*
- [x] **A4 — iki-node doğrulama harness'i** · TAMAMEN KAPANDI (2026-08-26, commit `064bdf4`+`9b27785`) — Faz 1 (iyi senaryo): iki gerçek node, iki bağımsız height'te birebir aynı hash commit etti. Faz 2 (kötü senaryo, trustless'ın asıl ispatı): `cmd/a4attacker/main.go` — gerçek P2P kimliğiyle ağa giren ama kendi BFT motoru olmayan saldırgan, 3 saldırı (sahte imzalı oy, sahte proposer bloğu, kimlik taklidi) gerçek GossipSub üzerinden denendi, **üçü de dürüst node tarafından reddedildi**. Mutasyon testiyle doğrulandı (kapı bozulunca sessizce kabul, geri konunca tekrar red). Harness+attacker kalıcı, ikisi de commit'li.
  - *Model: CC (Opus) — bitti*
  - **Açık soru 1 (B7 Faz 0'dan, 2026-08-25):** Kanal broadcast kripto modeli —
    MLS mi ayrı fanout mu (sender-keys/imzalı server-fanout)? Broadcast
    one-to-many, MLS grup değil (MLS her üye için Welcome+TreeKEM commit ister).
    B7 kanalı şimdilik HTTP-yetki seviyesinde admin-only yazar olarak sınırladı,
    kriptoya dokunmadı — bu soru çözülmeden kanal "gerçek" broadcast olamaz.
  - **Açık soru 2 (B7 Faz 0'dan, 2026-08-25):** MLS ölçek / spec 7.2 çelişkisi —
    Platin 10000 / Elmas limitsiz üye vaadi vs RFC 9420 (üye başı
    Welcome+TreeKEM commit). Doğrulanmadı, A4 sıfır çok-node testi bu ölçekte
    hiç denenmedi. A4 kapsamında gerçek ölçek testiyle netleştirilmeli.
- [x] **A5 — BFT liveness: proposer/oy self-echo bug'ı** · KAPANDI (2026-08-26, commit `064bdf4`) — A4 Faz 1'de bulundu: proposer gerçek P2P transport'ta kendi prevote'unu hiç üretmiyordu, oy üreten hiçbir node kendi oyunu kendi tablosuna yazmıyordu (sadece yayınlıyor, P2P'nin engellediği self-mesaj echo'suna güveniyordu) — iki dürüst node asla mutabakata varamıyordu. `LocalTransport` (test-only) yayıncıya kendi mesajını geri verdiği için 27 eski test bunu hiç yakalamamıştı. Fix: `vrf_broadcast.go PublishOwnProof` deseni (önce yerel say, sonra yayınla), 3 nokta. Kanıt: 39 birim test + canlı iki-node (2 bağımsız height'te birebir aynı hash) + mutasyon disiplini (fix geri alınca gerçekten tıkanıyor, geri koyunca düzeliyor).
  - *Model: CC (Opus) — bitti*
  - **Not — A4 kapandı ama şu 2 soru hâlâ açık (B7 Faz 0'dan, 2026-08-25), A4'ün test ettiği BFT'nin değil MLS'in konusu:**
    - Kanal broadcast kripto modeli — MLS mi ayrı fanout mu (sender-keys/imzalı server-fanout)? Broadcast one-to-many, MLS grup değil. B7 kanalı HTTP-yetki seviyesinde admin-only yazar olarak sınırladı, kriptoya dokunmadı — bu soru çözülmeden kanal "gerçek" broadcast olamaz.
    - MLS ölçek / spec 7.2 çelişkisi — Platin 10000 / Elmas limitsiz üye vaadi vs RFC 9420 (üye başı Welcome+TreeKEM commit). Doğrulanmadı, hiç test edilmedi. B5 (grup medya, MLS'e dokunan iş) kapsamında ele alınabilir.

---

## 🟠 BLOK B — Ürün tamlığı · *(madde 1, 2)*
> Kullanıcının hissedeceği eksikler. A'dan bağımsız, paralel gidebilir.

- [x] **B11 — blob E2E açığı** · KAPANDI (2026-08-27) — video/dosya/ses artık upload
  öncesi client-side şifreleniyor. Model: her upload için rastgele 32-byte blob-key
  (`crypto.getRandomValues`, CSPRNG — `mobile/lib/media-crypto.ts:20-24`), mevcut
  `encryptBlob`/`decryptBlob` primitifi (AES-256-GCM, `session-store.ts:67-86`,
  ratchet-at-rest şifrelemesiyle AYNI kod, persistent master key yerine rastgele
  medya-anahtarı) ile blob şifrelenip `expo-file-system` geçici dosyaya yazılıyor,
  mevcut `api.uploadMedia` yoluyla (değişmedi) MinIO'ya öyle gidiyor. Anahtar MEVCUT
  mesaj payload'ında taşınıyor — `[video]<url>|<keyB64>`, `[voice]<url>|<keyB64>`,
  `[file]<name>|<url>|<keyB64>` (1:1=ratchet, grup=MLS, sıfır kripto-yapı dokunuşu).
  Resme dokunulmadı (zaten inline+tam E2E). Geriye uyumluluk: eski mesajlarda `|keyB64`
  segmenti yok → `parseMediaKey` `keyB64:null` döner → client legacy/şifresiz olarak
  direkt kullanır (`media-crypto.ts:29-33`), eski blob'lar kırılmadı. Ses gerçek
  çalınıyor (`decryptToTempFile` ile indirilip yerel dosyaya yazılıp `Audio.Sound`'a
  veriliyor); video/dosya UI'sı hâlâ placeholder (B5 öncesi durum, bilerek
  dokunulmadı) ama encrypt/decrypt yolu round-trip testiyle kanıtlı, ölü kod değil.
  Kanıt: `media-crypto.test.ts` (6 birim test — CSPRNG kullanımı, farklı upload'larda
  farklı anahtar, round-trip, yanlış-anahtar GCM hatası, legacy format) + canlı smoke
  `mls-b11-blob-encryption.smoke.test.ts` (gerçek backend+gerçek MinIO — MinIO'da duran
  ham byte ORİJİNALLE EŞLEŞMİYOR/çözülemiyor, doğru anahtar GERÇEK MLS mesajından
  gelip orijinali veriyor, eski şifresiz upload hâlâ düz okunuyor). Regresyon: E1
  + B5 + B6 canlı smoke'lar PASS, 65 birim test PASS, `tsc` temiz. · *(madde 1)*
  - **Ek bulgu 1 (B11 Faz 0, 2026-08-27):** MinIO upload'ı `x-amz-acl: public-read`
    ile gidiyor (`backend/internal/media/minio.go:79`) — blob URL'ini bilen
    **HERKES**, kimlik doğrulamasız çekebiliyordu (public, "sunucu görür"den
    beter — backend'in kendi `media.Download()`'ı chat medyasında hiç
    kullanılmıyor, `minio.go:132`, sadece mini-app kod paketlerinde). B11'in
    client-side blob şifrelemesi bunu zararsızlaştırıyor (anahtarsız ciphertext
    çözülemez) ama ACL'in kendisi hâlâ yanlış yapılandırma — C10'da ayrıca
    gözden geçirilmeli (private-read + presigned URL/backend-proxy download
    düşünülebilir, B11 kapsamı dışı).
  - **Ek bulgu 2 (B11 kanıt fazı, 2026-08-27, ÖNEMLİ — ayrıca launch-blocker
    olabilir):** `x-amz-acl: public-read` header'ı MinIO'da TEK BAŞINA hiçbir
    şey yapmıyor — gerçek anonim erişim `docker-compose.yml`'daki `minio-init`
    servisinin bucket policy'sine bağlı, o da SADECE `obscura-media/avatar`
    prefix'ini `mc anonymous set download` ile açıyor (satır ~364). Chat medyası
    (`mediaType="media"`, TÜM video/dosya/ses upload'ları) `media/` prefix'inde —
    bu prefix'te anonim okuma policy'si YOK. Canlı test sırasında doğrulandı:
    şifresiz haliyle bile `media/` altındaki bir objeye anonim GET **403
    AccessDenied** döndü (docker-compose'un şu anki haliyle). Yani bu compose
    dosyasını kullanan bir ortamda video/dosya/ses **hiç indirilemiyor/çalmıyor
    olabilir** — encryption'dan bağımsız, ayrı bir kırıklık ihtimali (production
    ortamı farklı bir bucket-policy/CDN kullanıyorsa geçerli olmayabilir,
    doğrulanmadı). Kanıt ortamını tamamlamak için YEREL `obscura-minio`
    konteynerine (kalıcı, mayıs 2026'dan beri var — B11 için OLUŞTURULMADI)
    `mc anonymous set download obs/obscura-media/media` ve `.../voice` policy'si
    EKLENDİ (kod/compose dosyası değiştirilmedi, sadece çalışan konteynerin
    canlı policy'si) — bu B11 kapsamı dışı ama C10'da `docker-compose.yml`
    `minio-init` servisine `media`+`voice` prefix'lerinin de eklenmesi (ya da
    bilinçli olarak private tutulup presigned URL/backend-proxy'ye geçilmesi)
    gerekiyor.
  - *Model: CC (Opus) — client-side blob encryption + anahtar dağıtımı, tamamlandı*
- [x] **B5 — grup medya** · KAPANDI (2026-08-27) — 5 call-site (`chat/[id].tsx`: resim/video
  satır ~419-461, dosya ~469-490, konum ~499-520, ses `startRecording`/`stopRecording`
  ~528-565) `classifyConv`'a göre dallandı, grup ise yeni `sendGroupMedia` helper'ı
  (sendGroupText'in optimistic-echo/rollback desenini izler) üzerinden
  `sendGroupTextMessage`'a gider — mevcut payload formatları (`[img]/[video]/[file]/
  [location]/[voice]`) AYNEN taşındı, MLS/ratchet/epoch'a dokunulmadı (encryptGroupMessage
  plaintext:string alıyor, medya text'ten kripto olarak farksız). Kanıt: yeni canlı smoke
  (`mls-b5-group-media.smoke.test.ts`, gerçek backend) — Alice 5 medya tipini gruba
  gönderdi, Bob hepsini MLS akışından çözdü, PASS. Regresyon: E1 grup metin smoke PASS,
  B6 real-time push smoke PASS (WS push 107ms, reconnect-catchup kayıpsız), 33 birim test
  PASS, `tsc --noEmit` temiz. Blob şifreleme modeli 1:1'den birebir taşındı (resim inline
  tam E2E, video/dosya/ses referans-E2E + düz blob) — **açık bulunan gerçek gizlilik
  açığı yeni üst-sıra maddeye taşındı: [[B11]]**. · *(madde 1)*
  - *Model: CC (Opus) — kripto-dokunuşlu, E1 ratchet'ına dokunmadan bağlandı*
- [x] **B6 — real-time push** · 2026-08-27, WS `mls_message` grup mesajlarına bağlandı, 4sn polling kaldırıldı · *(madde 2)*
  - FAZ 0 keşfinde kapsamın çok ötesinde bir bulgu çıktı → önce ayrı acil fix: **WS auth 2 aydır tamamen kırıktı** (`24aec07`) — `createWS()` token'ı mesaj olarak gönderiyordu, backend hiç okumuyordu (regresyon: `1873709`, 2026-06-23), her bağlantı 401→1006, `onopen` hiç ateşlenmiyordu. **1:1/grup/call/presence real-time push'un TAMAMI ölüydü**, app sessizce tam HTTP-polling'e dayanıyordu. Fix: Authorization HEADER (query param DEĞİL — `nginx.conf`'un `$request` log formatı query string'i plaintext JWT olarak access.log'a yazardı, header'ı yazmaz; backend zaten header destekliyordu, RN'nin WS'i native destekli). Canlı kanıt: 1:1 push 97ms'de geldi (2 aydır 0), reconnect sonrası da çalıştı.
  - B6'nın kendisi (`49b1653`): bu fix ÜZERİNE — `mlsMessageNudge` (store) + `_layout.tsx` WS case + `chat/[id].tsx`'te polling→WS-tetikli decrypt, periyodik fallback YOK (1:1'in deseniyle aynı: WS+reconnect, mesaj WS broadcast'ten önce DB'ye yazıldığı için kaçan event veri kaybı değil). Canlı kanıt: grup push 73ms (eski 4000ms poll'un ~1/55'i), kasıtlı WS-kopma → reconnect-catchup kaçan mesajı kayıpsız yakaladı.
  - **AYRI MADDE (kapsam dışı bırakıldı, dokunulmadı):** 5 diğer MLS WS event'i (`mls_welcome`/`mls_commit`/`mls_removed`/`mls_key_update`/`key_package_rotation_needed`) mobile'da HÂLÂ bağlı değil — üyelik/epoch/key-rotation değişiklikleri real-time gelmiyor, kendi (varsa) polling/queue mekanizmalarına dayanıyor (örn. `mls_welcome_queue`). B6 sadece `mls_message`'ı bağladı.
  - *Model: CC (Sonnet) — mevcut event'i bağlama*
- [ ] **B8 — gerçek ödeme** · sadece OBS iç transfer var, fiat/kripto giriş yok · *(altyapı)*
  - *Model: CC (Opus) — para yolu, yüksek dikkat*
- [x] **B9 — #30 kuyruğu** · 2026-08-26/27, üç parça + yan-bulgu fix, hepsi push'landı
  - Parça 2 (`fb161ff`): satıcı ilan düzenle/sil ekranı — canlı doğrulandı (PATCH kalıcı, non-owner 403, DELETE kalıcı)
  - Parça 3 (`27770a4`): admin dispute-resolve ekranı — sessiz admin-probe (yeni endpoint yok), canlı doğrulandı (non-admin 403, admin 200 + kalıcı)
  - Parça 1 (`354a8b5`): gerçek eksik "dispute local-only" değil, "transaction→dispute ID eşlemesi local-only" çıktı — `TransactionInfo.DisputeID` (query-time subquery, migration/backfill yok) + web localStorage cache kaldırıldı, canlı doğrulandı (buyer+seller aynı dispute_id, GET dispute eşleşiyor)
  - Yan-bulgu (`a250a3b`, AYRI commit, B9 kapsamı dışı ama öncelikli çözüldü): `middleware.go:34` phone COALESCE eksikliği → migrate edilmemiş kullanıcılar restart sonrası yanlışlıkla "askıya alınmış" 403 alıyordu. İki yönlü regresyon testiyle doğrulandı.
  - *Model: CC (Sonnet) — plumbing*
- [ ] **B10 — web grup mesajlaşma (E1'in web tarafı yok)** · keşif (B7 Faz 2, 2026-08-25): `frontend/app/chats/[id]/page.tsx` `sendMessage` (satır ~250) ve diğer 3 gönderim call-site'ı HER ZAMAN `to_id: conv.peer_did` kullanıyor, `is_group` hiç geçirilmiyor — grup/kanal/topluluk mesaj gönderme web'de conv_type'tan BAĞIMSIZ olarak zaten çalışmıyor (peer_did grup için undefined → backend'e boş to_id gider). Web kendi Signal-tarzı ratchet'ini kullanıyor (`lib/e2ee-session.ts`), MLS değil — E1 mobile'ı MLS'e bağladı, web'i bağlamadı. B7 Faz 2 composer/davet gating'i bu kırıklığı DEĞİŞTİRMEDİ, sadece kanal/grup ayrımını doğru yansıtıyor (backend zaten 403'ü doğru veriyor, ama grup'ta bile üye yazamaz çünkü to_id boş gider). Düzeltmek MLS wiring gerektirir (guardrail nedeniyle B7'de dokunulmadı). · *(madde 1,2)*
  - *Model: CC (Opus) — kripto-dokunuşlu, E1'in web muadili*

---

## 🟡 BLOK C — Launch hijyeni
> En son. Görünmez ama şart.

- [ ] **C10 — #12 güvenlik denetimi** · 22/23 madde doğrulanmadı
  - B11'den devralınan 2 kalem: (1) MinIO `x-amz-acl:public-read` genel gözden
    geçirmesi (`minio.go:79`, private-read + presigned/proxy düşünülsün); (2)
    `docker-compose.yml` `minio-init`'in `media`/`voice` prefix'lerine anonim
    okuma policy'si eklenmesi (ya da bilinçli private + backend-proxy kararı) —
    şu an `media/` prefix'inde policy YOK, gerçek ortamda video/dosya/ses
    indirilemiyor olabilir, bkz. B11 kanıt fazı notu.
  - *Model: CC (Opus) — güvenlik*
- [ ] **C11 — ölü kütle temizliği** · her silmeden önce "çağrılmıyor" kanıtı
  - umay (~850 satır) · packages/e2ee boş kabuk · desktop (derlenmemiş) · zk/aztec sınıfla
  - *Model: CC (Sonnet) — trivial ama kanıt şart*
- [ ] **C12 — ops/deploy + bilinmeyenler**
  - deploy/ops belirsiz · bridge canlı zincir-üstü doğrula · #39 keşfi (iz yok)
  - *Model: karışık*

---

## 🔵 BLOK D — Vizyon motoru (madde 5) · *TASARIM ÖNCE, KOD SONRA*
> ❌ Şu an YOK. Token/marketplace altyapısı hazır ama "motor" yok.
> **Bu bloğa kod yazılmaz — önce tasarım belgesi.** A+B bitmeden inşa edilmez.

- [ ] **D1 — Ürün kimliği manifestosu** (kod değil, belge)
  - Çelişki çözümü: ödül = EYLEM/gelişim, ekran-süresi DEĞİL (bu yüzden bağımlılık kırar)
  - "Geliştirdikçe kazan" tam olarak neyi ödüllendirir? Hangi eylem → hangi token?
- [ ] **D2 — Anti-bağımlılık mekaniği tasarımı**
  - Uygulama "beni çok kullan" demez, "git yaşa, kanıtını getir" der (Pokémon Go dersi, düzeltilmiş)
- [ ] **D3 — Gelişim-ödül sistemi** (token'ı motora bağla — mevcut OBS/escrow üstüne)
  - *Model: tasarım = chat, kod = A+B bitince CC*

---

## Kritik yol
**A (çekirdek) → B (tamlık) → C (hijyen) → LAUNCH.** D (vizyon motoru) launch'tan
sonra ya da B ile paralel tasarlanır — ama A bitmeden inşa edilmez.

**Şu an (2026-08-27):** 🔴 **BLOK A TAMAMEN KAPANDI** — A1(bulma)+A2(davetli-ağ)+A3(imza)+A4(iki-node kanıt, iyi+kötü senaryo)+A5(liveness) hepsi commit'li, trustless çekirdeği canlı ağda ispatlandı. B7 + B9 + B6 + B5 + B11 de kapandı (B6 sırasında 2 aylık kritik bir yan-bulgu da kapandı: WS auth tamamen kırıktı, real-time push'un tamamı ölüydü; B5 sırasında grup+1:1 ortak bir gizlilik açığı bulundu → B11 açtı; B11 kapanışında da AYRI bir olası kırıklık bulundu — MinIO bucket policy `media/` prefix'ini hiç açmıyor, C10'da netleştirilmeli). Sırada: 🟠 BLOK B (ürün tamlığı — B5✅/B6✅/B7✅/B9✅/B11✅/B8/B10, A'dan çok daha hafif) ve 🟡 BLOK C (launch hijyeni — MinIO bucket-policy netleştirmesi de C10'a eklendi). Kritik yolda bir sonraki gerçek adım B (B8/B10) veya C'den başlar.
