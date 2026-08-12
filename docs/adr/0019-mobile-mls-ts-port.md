# ADR 0019: Mobile MLS via hand-written TS port (not ts-mls/WASM/native)

Date: 2026-08-11
Status: Accepted
Decider: project lead
Spec ref: Bölüm 6.3 (MLS), Bölüm 4.5 (7 kesin güvenlik kuralı — E2EE zorunlu), ADR-0007 (openmls seçimi — bu ADR onu mobile'a genişletir)

## Context

Grup mesajlaşması backend'de gerçek ve testli (`backend/internal/api/extra_handlers.go`,
`backend/internal/api/mls_handlers.go`, 1306 satır, RFC 9420 relay). Mobile'da
MLS istemcisi **yok** — `mobile/lib` içinde `mls` referansı sıfır, mobile
`package.json`'da ts-mls veya eşdeğeri hiç yok. Grup gönderimi mobile UI'da
zaten kapalı (`chat/[id].tsx:183` — `sendMessage` `peer_did`'e bağlı, grup
konuşmalarında (`peer_did` yok) fonksiyon hiç çalışmıyor).

Bu oturumda denenen/değerlendirilen yollar ve neden elendikleri:

- **ts-mls (dış paket):** npm monorepo hoisting cehennemi — 4 strateji
  tükendi (workspace izolasyonu, `@noble/curves` optional peer çakışması,
  mobile-nested shadow kopyası, ESM/CJS Jest+Metro kırılması). Reddedildi.
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

**Kanıtlanmış üçüncü yol:** 1:1 kripto (X3DH, Double Ratchet, sealed-sender)
zaten Rust'tan (`crypto/src/{x3dh,ratchet,sealed_sender}.rs`) elle
TypeScript'e port edilmiş — `mobile/lib/x3dh.ts`, `ratchet.ts`,
`sealed-sender.ts`. Kullanılan kütüphaneler `@noble/curves`,
`@noble/ciphers`, `@noble/hashes` (`mobile/package.json:16-17`) — saf JS,
native/WASM bağımlılığı sıfır, RN'de kanıtlı çalışıyor (1:1 mesajlaşma
GERÇEK). Doğrulama yöntemi: Rust referans implementasyonuna karşı
test-vector crosscheck (`crypto/test-vectors/sealed_sender_vectors.json` +
`mobile/lib/__tests__/x3dh-vector-crosscheck.test.ts`).

MLS ciphersuite'i (`crypto/src/mls/mod.rs:41-42`):
`MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519` — X25519, AES-128-GCM,
SHA256, Ed25519. 1:1 kriptonun kullandığı **aynı** primitif seti (bkz.
ADR-0007 "Ciphersuite choice" — primitifler kasıtlı olarak paylaşılıyor).

## Options considered

### Option A: ts-mls (npm paketi)
- Pros: RFC 9420 uyumlu hazır implementasyon
- Cons: hoisting cehennemi (4 strateji tükendi), Metro/Jest ESM-CJS kırılması
- Effort: bilinmiyor — paketleme sorunu çözülemedi, kripto işine hiç
  başlanamadı
- **Reddedildi**

### Option B: WASM (crypto crate → wasm-bindgen, RN'de çalıştır)
- Pros: Rust referans koduyla bit-bit aynı davranış garanti
- Cons: RN'de WASM runtime + polyfill katmanı, mevcut zk.ts'in (frontend/web)
  aksine mobile'da hiç kurulu değil, orantısız büyük iş
- Effort: L
- **Reddedildi**

### Option C: Native module (Rust → iOS/Android native, JSI köprüsü)
- Pros: en hızlı, en güvenilir kripto yürütme
- Cons: platform başına ayrı derleme/imzalama/dağıtım hattı, büyük iş,
  mevcut precedent yok (mobile hiçbir yerde native kripto modülü kullanmıyor)
- Effort: XL
- **Reddedildi**

### Option D: Backend-side MLS (mevcut `mls-cli` subprocess mobile adına)
- Pros: sıfır mobile iş
- Cons: E2EE'yi bozar — sunucu grup sırrına erişir. Bugün latent olan
  server-side commit fallback'i (`HandleMlsUpdateKey`) kalıcı mimari yapmak
  anlamına gelir
- **Reddedildi** — mimari ilke ihlali (bkz. `mls_handlers.go:6`)

### Option E: Hand-written TS port (chosen)
- Pros: kanıtlanmış desen (x3dh.ts/ratchet.ts/sealed-sender.ts), npm
  hoisting riski sıfır (paket değil, yazılan kod), aynı primitif seti
  (@noble/curves/ciphers/hashes zaten mobile'da), Rust referansına karşı
  test-vector doğrulanabilir
- Cons: TreeKEM 1:1'de hiç yoktu — bu, mevcut portlardan (x3dh/ratchet)
  belirgin şekilde büyük bir iş
- Effort: L (çok-commit'lik, küçümsenmemeli)
- **Seçildi**

## Decision

MLS'in gerekli alt-kümesini — **KeyPackage üretimi, TreeKEM (ratchet tree:
add/remove/update, path secret türetme, tree hash), Welcome işleme, Commit
işleme, application message encrypt/decrypt (exporter secret ratchet)** —
`crypto/src/mls/mod.rs`'den elle TypeScript'e port et, `@noble/curves`,
`@noble/ciphers`, `@noble/hashes` primitifleriyle. Dış MLS paketi (ts-mls
veya muadili) **kullanılmayacak**.

Doğrulama: 1:1 kriptoda izlenen crosscheck deseni — `crypto/src/mls/tests.rs`
vektörlerine karşı bit-bit doğrulama, aynı `__tests__/*-vector-crosscheck.test.ts`
kalıbı. Önceki oturumdaki ts-mls↔openmls interop spike'ı (7/7 geçti) referans
uyum kanıtı olarak saklanır — port'un doğruluğunu teyit eden bağımsız bir
ikinci kaynak.

## Rationale

- Codebase'de zaten kanıtlanmış, çalışan bir desen var — yeni bir yöntem
  icat etmiyoruz, mevcut olanı MLS'e genişletiyoruz.
- Ciphersuite primitifleri (X25519/AES-128-GCM/SHA256/Ed25519) 1:1 ile
  birebir örtüşüyor — aynı `@noble` paketleri yeterli, yeni bağımlılık
  gerekmiyor.
- npm paket bağımlılığı olmadığı için hoisting/Metro/Jest sınıfı sorunlar
  yapısal olarak imkansız — kod repo içinde, ESM/CJS ihtilafı yok.
- ADR-0007'nin "openmls = RFC 9420 referans implementasyonu" kararını
  değiştirmiyor — backend hâlâ openmls kullanıyor, bu ADR sadece mobile
  tarafının AYNI protokolü hangi araçla konuşacağını belirliyor.

## Consequences

- **Positive:** hoisting riski sıfır, mevcut crypto test altyapısıyla
  (vector crosscheck) tutarlı, yeni npm bağımlılığı yok.
- **Negative:** TreeKEM portu 1:1'deki X3DH/Ratchet portlarından büyük —
  ratchet tree diff/path secret türetme, çok-üyeli commit işleme gibi
  1:1'de hiç karşılaşılmamış karmaşıklık sınıfları var. Bu ADR bunu **büyük,
  çok-commit'lik iş** olarak işaretler, küçümsemez.
- **Tech debt (latent, acil değil):** `backend/internal/api/mls_handlers.go`
  içindeki iki server-side commit fallback'i —
  `HandleMlsRemoveMember` (satır 1017, route'a bağlı değil, dead code) ve
  `HandleMlsUpdateKey` (satır 1177, `main.go:429` route'lu ama hiçbir
  çağıranı yok) — mobile port canlıya çıktığında ya tamamen silinmeli ya da
  açıkça moderation-only yetkiye kilitlenmeli (bugünkü "server-managed group
  senaryoları için (spam kick)" amacı hiçbir kodda kullanılmıyor, kapsamı
  belirsiz). Bu oturumda doğrulandı: bugün latent, prod'da tetiklenmiyor —
  acil aksiyon gerekmiyor, port işinin bir parçası olarak ele alınacak.
- **Trusted setup / MPC:** 1:1 protokolünde (X3DH/Double Ratchet) trusted
  setup veya MPC gereksinimi yoktu. MLS'te de bireysel anahtar üretimi için
  yok, ancak grup-seviyesinde analog bir açık soru var (grup kurucusunun
  başlangıç tree state'i ve üye ekleme yetkisi üzerindeki güven sınırı). Bu
  ADR bunu çözmüyor — **prod-öncesi açık madde** olarak işaretlenir, port
  işini bloke etmez.

## Scope (bu portun kapsamı)

Port edilecek yüzey (`crypto/src/mls/mod.rs` public fonksiyonları temel
alınarak):
- `generate_key_package` — KeyPackage üretimi
- `create_group` — boş grup + kendi leaf
- `add_member` / `add_members` — Welcome + Commit üretimi
- `process_welcome` — Welcome'dan grup state türetme
- `encrypt_message` / `process_message` — application message
- `remove_member` — Remove proposal + Commit
- `self_update` — leaf key rotation Commit

Kapsam dışı (bu ADR'de karara bağlanmadı, ayrı değerlendirme):
- 10.000+ üyeli grup performans hedefleri (spec Bölüm 15.2) — küçük/orta
  grup ölçeğinde önce doğruluk, sonra ölçek.
- `crypto/src/mls/kyber_mls.rs` (post-quantum ciphersuite) — FAZ 3+ kapsamı,
  bu port yalnızca default `MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519`'u
  hedefler.

## Alternatives rejected (özet)

| Seçenek | Red sebebi |
|---|---|
| ts-mls | npm hoisting cehennemi, 4 strateji tükendi |
| WASM | RN'de gereksiz büyük runtime katmanı |
| Native module | platform başına büyük derleme/dağıtım işi |
| Backend-side MLS | E2EE'yi bozar — sunucu sır tutmaz vaadi ihlali |

## Related

- ADR-0007 — openmls seçimi (backend), bu ADR mobile tarafını tamamlıyor
- `backend/internal/api/mls_handlers.go:6` — "Server holds NO group secrets"
- `backend/internal/mls/global.go` — server-side MLS CLI yaşam döngüsü
  (backend-only, mobile için kullanılamaz — subprocess spawn RN'de yok)
- `mobile/lib/x3dh.ts`, `ratchet.ts`, `sealed-sender.ts` — port precedent'i
- `crypto/src/mls/mod.rs`, `crypto/src/mls/tests.rs` — port kaynağı ve
  doğrulama vektörleri
