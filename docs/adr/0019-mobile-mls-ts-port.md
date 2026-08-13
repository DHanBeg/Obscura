# ADR 0019: Mobile MLS via ts-mls + workspace-external vendor isolation

Date: 2026-08-11
Revised: 2026-08-13 (spike/ts-mls-isolation, 3 kanıtlı soru — bkz. Revizyon bölümü)
Status: Accepted (Revised)
Decider: project lead
Spec ref: Bölüm 6.3 (MLS), Bölüm 4.5 (7 kesin güvenlik kuralı — E2EE zorunlu), ADR-0007 (openmls seçimi — bu ADR onu mobile'a genişletir)

## Revizyon (2026-08-13) — ÖNCE OKU

Bu ADR'nin orijinal kararı (**Option E: elle yazılmış TS port**) yanlış bir öncüle
dayanıyordu: `crypto/src/mls/mod.rs`'in "port edilecek Rust referans implementasyonu"
olduğu varsayımı. `spike/ts-mls-isolation` branch'inde dosya satır satır okunduğunda
bu YANLIŞ çıktı — `mod.rs` (287 satır) **openmls crate'inin (crates.io, `Cargo.toml:48`,
`openmls = "0.6"`) ince wrapper'ı**, TreeKEM/key-schedule/wire-format'ın kendisi
bu repoda değil, `openmls`'in içinde. Port edilecek somut algoritma yoktu — "Rust'tan
port et" cümlesi, `x3dh.rs`/`ratchet.rs` (gerçekten elle yazılmış protokol mantığı)
ile yanlış bir analoji kuruyordu. Ayrıca `crypto/test-vectors/`'ta MLS vektörü YOK
(sadece `sealed_sender_vectors.json` + `x3dh_ratchet_vectors.json`) — "Rust
referansına karşı test-vector doğrulanabilir" iddiası da asset olarak mevcut değildi.

Aynı oturumda **ts-mls@1.6.2'nin önceki reddi** (Option A, "4 npm stratejisi tükendi")
de eksik çıktı: tükenen 4 strateji hepsi **npm workspace İÇİNDE** varyasyondu
(targeted install / dedupe / workspace-scoped install / overrides+temiz kurulum).
**Workspace-dışı vendor izolasyonu hiç denenmemişti.** `spike/ts-mls-isolation`'da
bu strateji 3 soruyla test edildi, üçü de ampirik kanıtla EVET çıktı:

1. İzole ts-mls kendi akışını (2-taraflı grup, gerçek TLS-codec wire round-trip)
   çalıştırdı mı → **EVET**, 7/7 adım (`spike-two-party.mjs`).
2. Mobile suite'i shadow'lamadı mı → **EVET** — sadece "dokunulmadı" değil, mobile'ın
   GERÇEK Jest ortamında izole path'i import eden bir test RED (`Unexpected token
   'export'`, tam vault `#43`'ün öngördüğü hata) → fix (`transformIgnorePatterns`'a
   `ts-mls|@hpke/.*` eklendi) → GREEN, tam suite 24/24 (200/200) regresyonsuz. Ayrıca
   **gerçek Metro bundler**'la da doğrulandı (`expo export:embed`, 158 modül, ts-mls
   içeriği bundle'da doğrulandı) — sadece Jest değil, prod bundling de çalışıyor.
3. Dual-package/sınıf sızıntısı yok mu → **EVET (temiz)** — ts-mls'in dışa açtığı
   7 değer de `instanceof Uint8Array`, hiçbiri noble class instance değil; mobile'ın
   kendi hoisted `@noble/curves@2.2.0`'ı, izole ts-mls'in (`@noble/curves@2.0.1`
   zinciri) ürettiği ham byte'larla gerçek ECDH yaptı — sınır SADECE byte, brand/
   instanceof hiç geçilmiyor.

**Sonuç: Option A ve Option E yer değiştiriyor — orijinal metin, tam alıntı:**

Orijinal ADR'de (2026-08-11) **Option A** şöyleydi:
> ### Option A: ts-mls (npm paketi)
> - Pros: RFC 9420 uyumlu hazır implementasyon
> - Cons: hoisting cehennemi (4 strateji tükendi), Metro/Jest ESM-CJS kırılması
> - Effort: bilinmiyor — paketleme sorunu çözülemedi, kripto işine hiç başlanamadı
> - **Reddedildi**

Orijinal ADR'de **Option E** şöyleydi:
> ### Option E: Hand-written TS port (chosen)
> - Pros: kanıtlanmış desen (x3dh.ts/ratchet.ts/sealed-sender.ts), npm hoisting
>   riski sıfır (paket değil, yazılan kod), aynı primitif seti
>   (@noble/curves/ciphers/hashes zaten mobile'da), Rust referansına karşı
>   test-vector doğrulanabilir
> - Cons: TreeKEM 1:1'de hiç yoktu — bu, mevcut portlardan (x3dh/ratchet)
>   belirgin şekilde büyük bir iş
> - Effort: L (çok-commit'lik, küçümsenmemeli)
> - **Seçildi**

**Bugün (2026-08-13) bu tam tersine döndü: Option A KABUL edildi, Option E
REDDEDİLDİ.** Sebep yukarıdaki 3 kanıt: Option E'nin "Pros"undaki "kanıtlanmış
desen" ve "test-vector doğrulanabilir" iddiaları gerçek değildi (mod.rs port
edilecek algoritma içermiyor, MLS vektörü repoda yok) — E'nin seçilme gerekçesi
çöktü. Option A'nın "Cons"undaki "4 strateji tükendi" ise EKSİK bilgiydi (5.
strateji — workspace-dışı vendor — hiç denenmemişti) — spike'ta o 5. strateji
denendi ve 3/3 kanıtla çalıştı, A'nın reddi de geçersiz oldu. Aşağıdaki `Context`/
`Options` bölümleri **orijinal haliyle bırakıldı** (o anki muhakeme, o anki bilgiyle
tutarlıydı — sessizce silinmiyor), ama artık geçersiz oldukları yerlerde işaretli.
Karar/Scope/Consequences bölümleri revize edildi.

Kanıt: `spike/ts-mls-isolation` branch'i (main'e merge edilmedi, throwaway artıklar
referans olarak duruyor) — `_spike_ts_mls_vendor/`, `mobile/metro.config.js`,
`mobile/lib/__tests__/_spike-ts-mls-isolated.test.ts`.

## Context

*(Orijinal, 2026-08-11 — mod.rs'in "port edilecek referans" olduğu öncülü artık
geçersiz, bkz. Revizyon. Geri kalan gözlemler — backend relay durumu, 1:1 port
precedent'i, ciphersuite eşleşmesi — hâlâ doğru.)*

Grup mesajlaşması backend'de gerçek ve testli (`backend/internal/api/extra_handlers.go`,
`backend/internal/api/mls_handlers.go`, 1306 satır, RFC 9420 relay). Mobile'da
MLS istemcisi **yok** — `mobile/lib` içinde `mls` referansı sıfır, mobile
`package.json`'da ts-mls veya eşdeğeri hiç yok. Grup gönderimi mobile UI'da
zaten kapalı (`chat/[id].tsx:183` — `sendMessage` `peer_did`'e bağlı, grup
konuşmalarında (`peer_did` yok) fonksiyon hiç çalışmıyor). **[Bu satır artık
güncel değil — L1 (Commit 0-4) grup gönderimini `classifyConv`/`isGroupSendBlocked`
ile açıkça fail-closed hale getirdi, görünür Alert'le. Bkz. `mobile/lib/group-send-gate.ts`.]**

Bu oturumda denenen/değerlendirilen yollar ve neden elendikleri:

- **ts-mls (dış paket):** npm monorepo hoisting cehennemi — 4 strateji
  tükendi (workspace izolasyonu, `@noble/curves` optional peer çakışması,
  mobile-nested shadow kopyası, ESM/CJS Jest+Metro kırılması). Reddedildi.
  **[REVİZE: bu 4 strateji hepsi workspace-İÇİ varyasyondu, workspace-DIŞI
  vendor izolasyonu denenmemişti — o strateji spike'ta kanıtlandı, bkz. Revizyon.
  ts-mls artık KABUL EDİLDİ.]**
- **WASM (crypto crate → wasm-bindgen):** `crypto/src/wasm.rs` zaten ZK
  prover için var (frontend/web), ama mobile'a taşımak gereksiz büyük iş —
  RN'de WASM runtime entegrasyonu (Hermes + polyfill) ek bir bağımlılık
  katmanı ekliyor, mevcut ihtiyacı karşılamak için orantısız.
- **Native module (Rust `mls-cli`'yi RN native/JSI'a derle):** büyük iş,
  platform başına (iOS/Android) ayrı derleme+imzalama hattı gerektirir.
- **Backend-side MLS (mevcut `mls-cli` subprocess'i mobile adına
  çalıştırmaya devam):** E2EE vaadini bozar. `mls_handlers.go:6` — "Server
  holds NO group secrets; only encrypted/wire blobs". Bu oturumda ayrıca
  doğrulandı: `HandleMlsRemoveMember` (satır 1017, server-side commit
  fallback) main.go'da route'a bağlı değil (dead code); kardeşi
  `HandleMlsUpdateKey` (satır 1177, `main.go:429` route'lu) aynı fallback
  desenine sahip ama hiçbir çağıranı yok (mobile/frontend/moderation'da
  sıfır referans) — bugün latent, aktif değil. Backend-side MLS'i kalıcı
  çözüm yapmak bu latent borcu aktif hale getirir ve E2EE'yi fiilen çökertir.

**[REVİZE — artık geçersiz:]** ~~Kanıtlanmış üçüncü yol: 1:1 kripto (X3DH, Double
Ratchet, sealed-sender) zaten Rust'tan elle TypeScript'e port edilmiş... aynı desen
MLS'e genişletilebilir.~~ Bu analoji yanlıştı — x3dh.rs/ratchet.rs elle yazılmış
protokol mantığı, `mls/mod.rs` openmls wrapper'ı. "Aynı desen" yoktu. 1:1 portunun
kanıtladığı şey hâlâ geçerli ve değerli: `@noble/curves`/`ciphers`/`hashes`
RN'de kanıtlı çalışıyor, MLS ciphersuite'i (`MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519`)
aynı primitifleri istiyor (X25519, AES-128-GCM, SHA256, Ed25519) — bu eşleşme
spike'ta da doğrulandı (`ciphersuites.MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519 === 1`,
ts-mls'in kendi ciphersuite tablosunda birebir aynı isim/parametre seti mevcut).

## Options considered

### Option A: ts-mls (npm paketi) — **REVİZE: KABUL EDİLDİ, bkz. Revizyon**
- Pros: RFC 9420 uyumlu hazır implementasyon, 104 star, aktif bakım (npm'de hâlâ
  güncel sürüm), spike'ta 3/3 soru kanıtlı EVET
- Cons: hoisting workspace-İÇİ kurulumda çözülemiyor (workspace-DIŞI vendor
  izolasyonuyla çözüldü — Metro `watchFolders`+`extraNodeModules`, Jest
  `transformIgnorePatterns`), formal security audit yok (niş RFC9420 TS
  kütüphaneleri için tipik, openmls da bu ADR kapsamında ayrıca audit edilmedi)
- Effort: M (kurulum+wiring spike'ta ~1 oturumda kanıtlandı; gerçek entegrasyon
  L2'de ayrı iş)
- **Kabul edildi**

### Option B: WASM (crypto crate → wasm-bindgen, RN'de çalıştır)
- Pros: Rust referans koduyla bit-bit aynı davranış garanti
- Cons: RN'de WASM runtime + polyfill katmanı, mevcut zk.ts'in (frontend/web)
  aksine mobile'da hiç kurulu değil, orantısız büyük iş
- Effort: L
- **Reddedildi** (değişmedi)

### Option C: Native module (Rust → iOS/Android native, JSI köprüsü)
- Pros: en hızlı, en güvenilir kripto yürütme
- Cons: platform başına ayrı derleme/imzalama/dağıtım hattı, büyük iş,
  mevcut precedent yok (mobile hiçbir yerde native kripto modülü kullanmıyor)
- Effort: XL
- **Reddedildi** (değişmedi)

### Option D: Backend-side MLS (mevcut `mls-cli` subprocess mobile adına)
- Pros: sıfır mobile iş
- Cons: E2EE'yi bozar — sunucu grup sırrına erişir. Bugün latent olan
  server-side commit fallback'i (`HandleMlsUpdateKey`) kalıcı mimari yapmak
  anlamına gelir
- **Reddedildi** — mimari ilke ihlali (bkz. `mls_handlers.go:6`) (değişmedi)

### Option E: Hand-written TS port — **REVİZE: REDDEDİLDİ**
- Pros (orijinal iddia): kanıtlanmış desen (x3dh.ts/ratchet.ts/sealed-sender.ts),
  npm hoisting riski sıfır, Rust referansına karşı test-vector doğrulanabilir
- **Neden şimdi reddedildi:** öncülü yanlıştı. `mls/mod.rs` port edilecek
  algoritma içermiyor (openmls wrapper'ı), MLS test-vector asset'i repoda yok
  (`crypto/test-vectors/`'ta sadece x3dh/sealed-sender). "Kanıtlanmış desen"
  aslında yoktu — TreeKEM'i RFC 9420 spesifikasyonundan SIFIRDAN yazmak
  anlamına gelirdi, ts-mls'in (104 star, aktif) zaten yaptığı işi tekrarlamak.
- Effort (revize edilmiş gerçek tahmin): XL, orijinal ADR'nin "L (küçümsenmemeli)"
  notu bile iyimserdi
- **Reddedildi**

## Decision (Revised 2026-08-13)

**ts-mls@1.6.2, workspace-dışı vendor izolasyonuyla.** Dış MLS paketi elle
port edilmeyecek — `ts-mls` npm paketi kullanılacak, ama repo'nun npm
workspace'ine (`package.json`: `"workspaces": ["frontend","mobile","packages/*"]`)
ÜYE OLARAK eklenmeyecek.

**Kurulum stratejisi (spike'ta kanıtlandı):**
- Vendor klasörü repo kökünde, workspace glob'unun dışında (`_spike_*` yerine
  L2'de kalıcı isim seçilecek, örn. `vendor/ts-mls/`), kendi `package.json`+
  lockfile'ıyla `ts-mls@1.6.2` kurulu. npm bu klasörü hoisting'e hiç dahil
  etmiyor — mobile'ın kendi `@noble/curves@2.2.0`'ını shadow'lamıyor.
- `mobile/metro.config.js` (repo'da önceden YOKTU, Expo'nun örtük varsayılanı
  işliyordu) — `config.watchFolders` vendor klasörünü, `config.resolver.extraNodeModules`
  `ts-mls`/`@hpke/core`/`@hpke/common`'ı vendor'ın kendi `node_modules`'üne
  haritalıyor.
- `mobile/package.json` `jest.transformIgnorePatterns`'a mevcut `@noble/.*`
  deseninin yanına `ts-mls|@hpke/.*` eklendi (Jest'in CJS `require()`'ının
  ts-mls'in `"type":"module"` dosyalarında patlamasını engelliyor).

**Ciphersuite:** `MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519` — `ts-mls`'in
ciphersuite tablosunda birebir aynı isim ve `id=1`, backend openmls'in
(`crypto/src/mls/mod.rs:41-42`) kullandığıyla eşleşiyor. Crypto provider
**`nobleCryptoProvider` açıkça seçilmeli** — `defaultCryptoProvider` (WebCrypto
tabanlı) X25519'u desteklemiyor, bu suite'te kullanılamaz (spike'ta `TypeError:
Cannot read properties of undefined (reading 'kdf')` ile bulundu).

Doğrulama: `crypto/src/mls/tests.rs`'teki 2 entegrasyon testinin (`full_two_party_flow`,
`three_party_message_flow`) TS eşdeğeri zaten spike'ta yazıldı (`spike-two-party.mjs`,
7/7 yeşil, gerçek TLS-codec wire round-trip). MLS için RFC 9420 resmi test-vektörü
ya da bu 2 testin byte-çıktısından üretilecek bir `mls_vectors.json` — **henüz
yok, L2'nin ilk işlerinden biri.**

**Backend hedefi (tek yol, karara bağlandı):** Mobile, ts-mls ile ürettiği
gerçek MLS mesajlarını **`/v1/mls/*` relay'ine** gönderecek (`mls_handlers.go`,
`mls_messages` tablosu) — bu yüzey zaten mimariden gereği sırsız, tam CRUD'u
(KeyPackage/group/Welcome/message) hazır ve testli. `/v1/messages` (genel
mesaj yolu) grup için **sadece Commit 0'ın fail-closed gate'i olarak kalır**
(`encryption_type:"mls"` etiketi zorunlu), MLS ciphertext'i hiç taşımayacak —
o yolun kapsamı genişletilmeyecek. Gerekçe ve reddedilen alternatif için bkz.
Consequences → E1 kapanış şartı.

## Implementation notes (ts-mls API — spike'ta öğrenilen sürtünmeler)

İlk kripto commit'inde saat kazandırması için (README bazı yerlerde güncel değil,
gerçek API `.d.ts`'lerden doğrulanmalı):
- `getCiphersuiteImpl(cs, provider)` — `cs` bare id (`ciphersuites.XXX`, sayı)
  DEĞİL, `getCiphersuiteFromName("MLS_128_...")` çıktısı (tam ciphersuite objesi).
- `provider` parametresi X25519 suite'i için **`nobleCryptoProvider` olarak elle
  verilmeli** — varsayılan (`defaultCryptoProvider`) çalışmıyor.
- `defaultCapabilities` bir **fonksiyon**, sabit değil — `defaultCapabilities()`
  çağrılmalı (çağrılmazsa `signLeafNodeKeyPackage` içinde `.length` undefined
  hatasıyla patlıyor).
- `emptyPskIndex` bir **sabit**, fonksiyon değil — `emptyPskIndex()` DEĞİL,
  doğrudan `emptyPskIndex`.
- `createCommit`/`createApplicationMessage`/`processMessage` gibi fonksiyonlar
  `{state, cipherSuite}` şeklinde bir `context`/`MLSContext` objesi bekliyor,
  README örneğindeki gibi ayrı pozisyonel parametreler değil (README bu noktada
  eski bir API sürümünü yansıtıyor olabilir, `.d.ts`'e güven).
- Welcome/mesaj gerçek "wire"dan geçmiş gibi taşınacaksa `encodeMlsMessage`/
  `decodeMlsMessage` (TLS-codec) kullanılmalı — `{version:"mls10", wireformat:
  "mls_welcome"|"mls_private_message", ...}` sarmalayıcısı gerekiyor.

## Rationale

- Spike, önceki 4 workspace-içi denemenin çarptığı duvarın npm workspace'in
  KENDİSİNDEN kaynaklandığını gösterdi — workspace'in tamamen dışına çıkmak
  bu duvarı yapısal olarak kaldırıyor, ampirik olarak (3/3 kanıt) doğrulandı.
- ts-mls, hand-written portun iddia ettiği "kanıtlanmış desen" avantajını
  gerçekte TAŞIMIYORDU (mod.rs port edilecek algoritma değil) — bu avantaj
  ortadan kalkınca, sıfırdan RFC 9420 yazmak yerine aktif bakımlı, 104 star'lı
  bir kütüphaneyi kullanmak daha az riskli.
- Ciphersuite primitifleri (X25519/AES-128-GCM/SHA256/Ed25519) hâlâ 1:1 ile
  birebir örtüşüyor — bu gözlem revize edilmedi, hâlâ doğru ve ts-mls'in
  `@noble` tabanlı `nobleCryptoProvider`'ıyla da tutarlı.
- ADR-0007'nin "openmls = RFC 9420 referans implementasyonu" kararını
  değiştirmiyor — backend hâlâ openmls kullanıyor, mobile ayrı bir kütüphane
  (ts-mls) kullanıyor ama AYNI protokolü (RFC 9420) AYNI ciphersuite'le konuşuyor.

## Consequences

- **Positive:** hoisting riski spike'ta 3/3 kanıtla kapatıldı (npm seviyesi +
  gerçek Jest cross-boundary test + gerçek Metro bundle), dual-package hazard
  yok (byte-only sınır doğrulandı), yeni algoritma yazma riski yok (ts-mls
  hazır+aktif bakımlı).
- **Negative:** yeni bir yapılandırma yüzeyi — `metro.config.js` (önceden
  yoktu) + `jest.transformIgnorePatterns` + workspace-dışı vendor klasörü,
  üçü birlikte bakım gerektiriyor (npm audit/update mobile'ın normal akışının
  dışında, vendor klasörü elle güncellenmeli). ts-mls resmi security audit'ten
  geçmemiş (README'de açık uyarı) — prod-öncesi bağımsız inceleme gerekebilir.
- **Tech debt (latent, acil değil, değişmedi):** `backend/internal/api/mls_handlers.go`
  içindeki iki server-side commit fallback'i —
  `HandleMlsRemoveMember` (satır 1017, route'a bağlı değil, dead code) ve
  `HandleMlsUpdateKey` (satır 1177, `main.go:429` route'lu ama hiçbir
  çağıranı yok) — mobile entegrasyonu canlıya çıktığında ya tamamen silinmeli
  ya da açıkça moderation-only yetkiye kilitlenmeli.
- **Trusted setup / MPC:** değişmedi — grup-seviyesinde analog açık soru
  (grup kurucusunun başlangıç tree state'i üzerindeki güven sınırı) hâlâ
  çözülmedi, prod-öncesi açık madde.
- **E1 kapanış şartı — REVİZE, iki mesaj-yüzeyi bulgusu:** Spike'ın envanter
  turu backend'de İKİ AYRI grup-mesaj yüzeyi olduğunu ortaya çıkardı:
  - **`/v1/mls/*`** (`mls_handlers.go`, `mls_messages` tablosu) — 16 fonksiyon,
    KeyPackage/group/Welcome CRUD relay'i **zaten hazır ve testli**, mimariden
    gereği sırsız (`mls_handlers.go:5-6`, ciphertext_b64'ü hiç parse etmeden
    saklayıp broadcast ediyor). Mobile bu endpoint'lere **hiç dokunmuyor.**
  - **Genel `/v1/messages`** (`handlers.go`, `messages` tablosu) — mobile'ın
    BUGÜN kullandığı yol. Commit 0 gate'i (`handlers.go:698`) burada:
    `encryption_type:"mls"` etiketi zorunlu ama içerik doğrulanmıyor
    (`EffectiveEncryptionType()`, `models.go:256-263`, sadece enum normalize
    ediyor) — honesty-contract, kriptografik kanıt değil.

  **E1 kapanış şartı — TEK YOL, karara bağlandı:** Mobile, `/v1/mls/*`
  relay'ine bağlanacak. `/v1/messages` grup mesajı için sadece Commit 0'ın
  fail-closed gate'i olarak kalacak — MLS ciphertext'i taşımayacak, kapsamı
  genişletilmeyecek. E1, mobile `/v1/mls/*`'e gerçek MLS mesajı gönderip
  başka bir üyenin bunu ts-mls ile çözdüğü an kapanmış sayılır.

  **Reddedilen alternatif — `/v1/messages`'a wire-format doğrulaması eklemek**
  (orijinal ADR'nin planı): Bu, zaten doğru ve testli olarak var olan bir
  mimariyi (`/v1/mls/*` relay'i) `/v1/messages` üzerinde İKİNCİ KEZ, daha
  eksik şekilde inşa etmek anlamına gelirdi — `handlers.go:698`'e TLS-codec
  parse'ı eklemek, `mls_handlers.go`'nun zaten yaptığı işi (KeyPackage/group/
  Welcome/epoch takibi, `mls_group_members`/`mls_messages` şeması) genel
  `messages` tablosunda baştan icat etmek olurdu — hem daha çok iş, hem iki
  paralel grup-mesaj şeması riski (`conv_members`/`messages` vs
  `mls_group_members`/`mls_messages`, hangisi "gerçek" belirsizleşir).
  **Reddedildi** — var olan doğru mimariyi tekrar inşa etmek.

## Scope (bu entegrasyonun kapsamı — REVİZE)

**[REVİZE: eski "port edilecek Rust yüzeyi" listesi (`generate_key_package`,
`create_group`, vb. `mls/mod.rs` fonksiyonları) kaldırıldı — port edilmiyor,
ts-mls'in kendi API'si kullanılıyor.]**

Kullanılacak ts-mls yüzeyi (bkz. Implementation notes):
`generateKeyPackage`, `createGroup`, `createCommit` (Add proposal ile), `joinGroup`,
`createApplicationMessage`, `processMessage`, ileride `remove_member`/`self_update`
karşılığı proposal tipleri.

**İlk kapanabilir dilim (önceki envanter turunda belirlendi, bu ADR'de teyit):**
2-üye statik grup state'i + tek mesaj enc/dec. Genel TreeKEM resolution
(keyfi ağaç şekli, blank node, çoklu-add batching) bu dilimde GEREKMİYOR —
ama MLS key schedule'ın TAMAMI (joiner_secret→member_secret→epoch_secret
zinciri), minimal (1 parent + 2 leaf) ratchet-tree hali, HPKE (Welcome
şifrelemesi) ve application-message AEAD framing'i tree boyutundan bağımsız,
atlanamaz — dördü birbirine bağımlı, ayrı teslim edilemez.

Kapsam dışı (değişmedi):
- 10.000+ üyeli grup performans hedefleri (spec Bölüm 15.2).
- `crypto/src/mls/kyber_mls.rs` (post-quantum ciphersuite) — FAZ 3+.
- ts-mls'in kendi post-quantum/hybrid ciphersuite'leri (X-Wing, ML-KEM) —
  Obscura'nın backend'iyle eşleşen tek ciphersuite (`id=1`) hedefleniyor.

## Alternatives rejected (özet — REVİZE)

| Seçenek | Durum | Sebep |
|---|---|---|
| ts-mls | **Kabul edildi** | Workspace-dışı vendor izolasyonu 3/3 kanıtla çalıştı (spike/ts-mls-isolation) |
| Hand-written TS port | **Reddedildi (revize)** | `mod.rs` port edilecek algoritma değil (openmls wrapper), MLS test-vektörü repoda yok, öncül geçersiz |
| WASM | Reddedildi | RN'de gereksiz büyük runtime katmanı |
| Native module | Reddedildi | platform başına büyük derleme/dağıtım işi |
| Backend-side MLS | Reddedildi | E2EE'yi bozar — sunucu sır tutmaz vaadi ihlali |

## Related

- ADR-0007 — openmls seçimi (backend), bu ADR mobile tarafını tamamlıyor
- `backend/internal/api/mls_handlers.go:6` — "Server holds NO group secrets"
- `backend/internal/api/mls_handlers.go:364-438` (`HandleMLSGroupMessage`) —
  gerçek relay davranışının kanıtı, ciphertext_b64'ü hiç parse etmiyor
- `backend/internal/api/handlers.go:698` — Commit 0 honesty-contract gate'i
  (`/v1/messages` yolu, `/v1/mls/*`'ten AYRI yüzey)
- `backend/internal/mls/global.go` — server-side MLS CLI yaşam döngüsü
  (backend-only, mobile için kullanılamaz — subprocess spawn RN'de yok)
- `mobile/lib/x3dh.ts`, `ratchet.ts`, `sealed-sender.ts` — 1:1 port precedent'i
  (MLS için port DEĞİL, ama primitif seçimi/kanıt yöntemi hâlâ referans)
- `crypto/src/mls/mod.rs`, `crypto/src/mls/tests.rs` — backend openmls wrapper'ı
  ve ciphersuite/parametre eşleşme referansı (artık port kaynağı DEĞİL)
- `spike/ts-mls-isolation` branch — kanıt: `_spike_ts_mls_vendor/`,
  `mobile/metro.config.js`, `mobile/lib/__tests__/_spike-ts-mls-isolated.test.ts`
  (main'e merge edilmedi, throwaway referans)
- `mobile/lib/group-send-gate.ts` — L1 (Commit 0-4), grup gönderimini
  `classifyConv`/`isGroupSendBlocked` ile fail-closed yapan mevcut gate
