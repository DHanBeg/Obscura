# Oturum — 2026-08-01 — Bridge DOT tarafı (PARÇA 1 + PARÇA 2)

## Özet

ETH relayer tamamlandı (3 bug düzeltildi, testler PASS — `bridge/relayer.go`).
Ardından DOT tarafının en alt katmanı kuruldu: RPC bağlantısı, sr25519
imzalama, SS58 adres kodlama (PARÇA 1), sonra extrinsic oluşturma + SCALE
encoding + author_submitExtrinsic (PARÇA 2). **Gerçek zincire hiçbir şey
gönderilmedi — yalnızca hazırlık + kanıtlama.**

## Yapılanlar

### PARÇA 1 — RPC bağlantısı + sr25519 + SS58

- `backend/internal/bridge/dot.go` (yeni): `Client.CheckDotConnection` —
  system_chain/system_health/chain_getBlockHash(0), mevcut `rpcCallWithRetry`
  deseni korunarak (WSS değil, HTTP JSON-RPC).
- Gerçek çalışan public Paseo RPC bulundu: `https://paseo-rpc.n.dwellir.com`
  (kullanıcının verdiği `wss://paseo.rpc.amforc.com` DNS'te yoktu, güncel
  değildi — polkadot-js/apps kaynağından doğru liste çekildi).
- sr25519: `github.com/ChainSafe/go-schnorrkel` (pure Go, CGO_ENABLED=0
  temiz) direct dependency yapıldı. `Sr25519KeypairFromSeed`,
  `Sr25519DevKeypair("Alice"/"Bob")`, `Sign`/`Sr25519Verify`.
- SS58: `EncodeSS58`/`DecodeSS58` (blake2b-512 checksum + base58, prefix<64).
- **Kanıt:** //Alice pubkey `d43593c715fdd31c61141abd04a99fd6822c8558854ccde39a5684e7a56da27d`
  → SS58 `5GrwvaEF5zXb26Fz9rcQpDWS57CtERHpNehXCPcNoHGKutQY` — Substrate
  kanonik sabitiyle birebir eşleşti. Kullanıcının gerçek adresi
  `5GZVKNQhXGfWWmAYy5psxjm3E2g9J9aYTbGK86qWYG9LnxSC` decode edilip
  round-trip'te birebir üretildi.
- Dosyalar: `dot.go`, `dot_test.go`. `.env`/`.env.example` → `DOT_RPC_URL` eklendi.

### PARÇA 2 — Extrinsic + SCALE + author_submitExtrinsic

- `scale.go` (yeni): compact int encode/decode (mode 0-3, u128'e kadar).
  **Kanıt:** 1→0x04, 42→0xa8, 69→0x1501, 65535→0xfeff0300 — hepsi eşleşti,
  ayrıca Python `scalecodec` kütüphanesiyle bağımsız cross-check edildi
  (birebir aynı hex).
- `metadata.go` (yeni): RuntimeMetadataV14 decoder — frame-metadata +
  scale-info GitHub kaynağından birebir struct sırası alındı (tahmin YOK).
  Gerçek Paseo metadata'sı (845KB, `testdata/paseo_metadata_v14.hex`) HİÇ
  ARTIK BAYT BIRAKMADAN tam çözüldü.
  **Kanıt:** Balances pallet index=**5**, `transfer_keep_alive` call
  index=**3** — HARDCODE DEĞİL, canlı metadata'dan. `substrate-interface`
  (Python, bağımsız 3. parti) ile `compose_call()` çağrılıp aynı index'ler
  (`0x0503...`) doğrulandı.
- `extrinsic.go` (yeni): `EraMortal`/`EraImmortal`, call builder, signed
  extra + additional_signed (Paseo'nun gerçek 11 signed extension'ı sırayla:
  AuthorizeCall, CheckNonZeroSender, CheckSpecVersion, CheckTxVersion,
  CheckGenesis, CheckMortality, CheckNonce, CheckWeight,
  ChargeTransactionPayment, PrevalidateAttests, CheckMetadataHash — sonuncusu
  Disabled modda, RFC-78 merkle hash doğrulaması kapsam dışı), imzalama,
  `DecodeExtrinsicForReview` (göndermeden önce insanca gösterim).
  **Kanıt:** substrate-interface ile üretilen gerçek referans extrinsic'le
  imza HARİÇ birebir byte eşleşti; Go'nun ürettiği imza substrate-interface'in
  `Keypair.verify()` fonksiyonuyla bağımsız doğrulandı (`True`).
- `dot_submit.go` (yeni): `FetchMetadata`, `FetchRuntimeVersion`,
  `FetchGenesisHash`, `FetchAccountNonce`, `SubmitExtrinsic`.
  **`SubmitExtrinsic` hiçbir yerden otomatik çağrılmıyor.**
- Toplam 20 test, hepsi PASS. `go build`/`go vet` CGO_ENABLED=0 tüm repo'da
  temiz.

## Kaldığımız Yer

**DUR noktası — kullanıcı onayı bekleniyor.** Extrinsic hazır, decode ile
insanca gösterildi, ama **gerçek gönderim yapılmadı.**

| # | İş | Durum |
|---|---|---|
| 1 | Hangi hesaptan gönderilecek? | ⏳ Kullanıcıya soruldu — kendi gerçek adresi değil, ayrı test hesabı istendi. //Alice'in Paseo'da bakiyesi olduğu bulundu (~44.97 PAS, herkesçe bilinen dev anahtarı, düşük risk) — karar bekleniyor |
| 2 | İlk gönderim miktarı | Kullanıcı "çok küçük, örn 0.1 PAS" dedi — teyit gerek |
| 3 | Era seçimi (mortal/immortal) | Kod ikisini de destekliyor; mortal için gerçek current block gerekir (RPC'den çekilecek, henüz çekilmedi) |
| 4 | `SubmitExtrinsic` çağrısı | Kod hazır, **çağrılmadı**. Onay gelince: nonce+era canlı çekilip extrinsic kurulacak → decode ile tekrar gösterilecek → onay → gönder |

## Kararlar

- CheckMetadataHash → Disabled mod (RFC-78 merkleized metadata hash
  implement edilmedi, kapsam dışı bırakıldı — Disabled geçerli bir mod).
- Yalnızca SS58 prefix<64 (tek bayt) destekleniyor — Paseo (42) bunun
  içinde, 2-baytlık prefix aralığı YAGNI ile kapsam dışı bırakıldı.
- Yalnızca metadata V14 destekleniyor (Paseo'nun döndürdüğü sürüm) — V15
  gelirse kod HATA verir, sessizce yanlış varsaymaz.

## Dosyalar

- `backend/internal/bridge/dot.go`, `dot_test.go`, `dot_submit.go`
- `backend/internal/bridge/scale.go`, `scale_test.go`
- `backend/internal/bridge/metadata.go`, `metadata_test.go`
- `backend/internal/bridge/extrinsic.go`, `extrinsic_test.go`
- `backend/internal/bridge/testdata/paseo_metadata_v14.hex`
- `backend/go.mod`/`go.sum` (go-schnorrkel direct dependency)
- `.env`, `.env.example` (`DOT_RPC_URL`)

## Açık sorular (sıradaki oturum)

- Test gönderimi hangi hesaptan? (//Alice mi, ayrı bir test cüzdanı mı)
- Gönderim onaylanınca: canlı nonce + mortal era (current block) çekilip
  gerçek extrinsic kurulacak, tekrar decode ile gösterilecek, "gönder"
  onayından SONRA `SubmitExtrinsic` çağrılacak.
