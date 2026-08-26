# Session 2026-08-26 — A2: davetli-ağ bootstrap

## Summary
Bootstrap kaynak seti davetli-ağ kararına kesinleştirildi (env+peer_cache aktif, DNS-TXT/ENS pasif); iki gerçek node ile "env-tohum, bir kez → cache-devam, otomatik" akışı canlı kanıtlandı.

## Tasks completed
- `p2p/discovery.go`: `centralDiscoverySourcesEnabled = false` sabiti eklendi, DNS-TXT+ENS blokları bu sabitle sarıldı (kod silinmedi, v1'de hiç çalışmıyor — env var değil derleme-zamanı sabit, prod'da yanlışlıkla flip edilemez). Kapalıyken tek log satırı: "DNS-TXT/ENS v1'de pasif (davetli-ağ kararı, bkz. A2)".
- İlk-tohum + cache-devam akışı zaten A1'de kurulmuştu (env→host, SavePeer→cache) — bu oturum sadece kaynak setini kesinleştirdi + iki-node ile kanıtladı.
- peer_cache olgunluğu: `LoadPeers` zaten 72s + `successful_connections>=1` filtreli — yeni prune job/tablo eklenmedi (mevcut filtre yeterli, kapsam sınırı gereği).
- Commit `6257627`, push edildi (kullanıcı onayı + tam mesajı verdi).

## Decisions made
- DNS-TXT/ENS kapatma yöntemi: env var değil derleme-zamanı `const` — "karar mühürlü" ile tutarlı, runtime'da yanlışlıkla açılamasın diye.
- peer_cache için ek DELETE/prune job eklenmedi — `LoadPeers`'ın WHERE filtresi zaten bayat/başarısız peer'ları sonuca hiç sokmuyor, ayrı temizlik mekanizması gerekmedi (yeni tablo/entity guardrail'ine takılmadı).

## Files changed
- backend/internal/p2p/discovery.go (+32/-16, tek dosya)

## Spec gaps closed
- Spec Bölüm 3.3 bootstrap kaynak önceliği artık davetli-ağ kararıyla uyumlu: sadece env+peer_cache aktif, merkezi/harici bağımlılık yok.

## Spec gaps remaining (bu çalışma alanında)
- Yok — A2 kapsamı tam kapandı. A3 (BFT güvenlik-kasları) ve A4 (iki-node doğrulama harness'i) ayrı, kapsam dışı bırakıldı.

## CLAUDE.md updates needed
- Yok

## Open questions for next session
- A3'e ne zaman geçilecek — Master-Liste'de "EN KRİTİK, Opus, taze kafa şart" olarak işaretli.

## Notes
- **Kanıt (bu oturumda, gerçek iki-node, aynı makine, QUIC):**
  - Geçici bir smoke-test binary'si (`cmd/p2ptwo_main.go`, gerçek `internal/db`+`internal/p2p` kullanarak) yazıldı, build edildi, test sonunda repodan silindi — iz yok.
  - node-1 (kararlı kimlik, `P2P_KEY_PATH`) ayağa kalktı → log: `"DNS-TXT/ENS v1'de pasif"`.
  - node-2 `BOOTSTRAP_PEERS=<node-1 addr>` ile ilk kez başladı → `[env]` kaynağından buldu → bağlandı → `SavePeer` cache'e yazdı (DB'den doğrudan `LoadPeers` ile doğrulandı).
  - **Karar noktası:** node-2 BOOTSTRAP_PEERS OLMADAN yeniden başlatıldı → `[peer_cache]` kaynağından node-1'i buldu (env boş, DNS/ENS pasif) → otomatik bağlandı, `PeerCount=1`. Manuel tohum bir kereydi, restart tamamen otomatikti.
  - Önceki test turundan kalma bayat bir cache girdisi de listedeydi — dial denendi, temiz başarısız oldu, akışı bozmadı (`LoadPeers` filtresinin canlıda da doğru çalıştığının kanıtı).
  - `go build`/`go vet` repo geneli temiz, tüm test artefaktları (geçici DB'ler, log'lar, binary, smoke-test kaynak dosyası) temizlendi.
