# zk-prototypes (arşiv)

`circuits/`'in (aktif set) güvenlik-öncesi öncülleri. Tarihsel referans,
build zinciri DIŞI — hiçbir Makefile hedefi, CI job'ı ya da `//go:embed`
direktifi buraya dokunmaz (bkz. `backend/internal/zk/verifier.go`, sadece
`backend/internal/zk/keys/*.json` gömüyor).

C11 envanterinde dosya-dosya diff'lendi, "çöp kopya" DEĞİL — aynı isimli
ama tamamen farklı/eski tasarımlar:

- **credit_threshold.circom**: C3-öncesi hali. `user_hash` binding YOK —
  `circuits/credit_threshold.circom`'daki (security audit 2026-05-10,
  finding C3) düzeltme burada uygulanmamış. Saldırgan bu eski tasarımda
  keyfi `user_hash` yazabilirdi.
- **identity_proof.circom**: Baby-Jubjub/X25519 private_key+did_hash+timestamp
  yaklaşımı. `circuits/identity_proof.circom` tamamen farklı bir tasarıma
  (identity_secret+phone_hash+nullifier+epoch) geçmiş.
- **token_transfer.circom**: basit commitment prototip ("Aztec-style").
  `circuits/token_balance.circom` FAZ 3'te gerçek Merkle inclusion proof +
  nullifier + double-spend korumasına evrilmiş, isim de değişmiş.

`scripts/prove.js` + `scripts/setup.sh`: bu prototip circuit'lere özel
yardımcı script'ler, aynı şekilde build zinciri dışı.

2026-08-29'da `archive/`'e taşındı (C11, git mv — history `git log --follow`
ile erişilebilir). Silinmedi, sadece git-HEAD'den çalışan kod yolundan
çıkarıldı.
