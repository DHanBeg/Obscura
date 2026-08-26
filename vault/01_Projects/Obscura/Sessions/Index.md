# Oturum Logları

Tüm session log'ları `E:\obscura\docs\sessions\` altında. Bu sayfa hızlı erişim için liste.

## 2026-05

- 2026-05-09 — Skill + agent setup + ZK pipeline (commit `58c95a2`)
- 2026-05-10 — MLS openmls foundation (commit `3e5d556`)
- 2026-05-10 — FAZ 1 completion sprint (commit `9877064`)
- 2026-05-10 — FAZ 1 post-audit hardening (commit `5e10fd7`)
- 2026-05-13 — FAZ 2 batch 1: foundation + ADR-0010/0011/0012 + token/vote circuits + MLS 5000 (commit `8e0ee79`)
- 2026-05-13 — FAZ 2 batch 2: token layer + wallet + mini app
- 2026-05-13 — FAZ 2 batch 3: staking + governance + airdrop + shielded + moderation + aztec
- 2026-05-16 — Design skills (frontend-design + impeccable + motion-principles) commit `d006840`
- 2026-05-16 — Claudesidian vault setup

## 2026-08

- 2026-08-01 — Bridge DOT tarafı: RPC+sr25519+SS58 (PARÇA 1) + extrinsic/SCALE/author_submitExtrinsic (PARÇA 2), gönderim onay bekliyor (uncommitted)
- 2026-08-24 — **Tam yer-gerçeği denetimi** (E1-E5 + teknik borç + harita-dışı mimari, HEAD `6e05e24`) — bkz. [[Ground-Truth-Audit-2026-08-24]] (şematik özet) + `E:\obscura\docs\sessions\2026-08-24-ground-truth-audit.md` (tam kanıt tablosu). 3 net launch-blocker doğrulandı (E1 chat-wiring, marketplace UI, #40 bootstrap) + CI'nin crypto-cli/mls-cli/deno testlerini sessizce skip ettiği bulundu. Phase-Status.md'de 2 bayat/yanlış madde düzeltildi (BFT wiring, libp2p-vs-gossip).
- 2026-08-25 — **#30 Marketplace UI KAPANDI** (mobile+web) — E1'in ertesi denetim zincirinde. `packages/theme` token katmanı + logo altın recolor, Faz 1 envanterinde 1 backend boşluğu bulunup kapatıldı (GET transaction/dispute), 5 ekran grubu iki platformda (6 mobile+web commit toplam). İki BAĞIMSIZ canlı smoke (mobile Jest, web Node fetch) gerçek backende karşı — auth guard dahil (yabancı kullanıcı başkasının işlemini göremiyor). Bkz. [[2026-08-25-30-marketplace-web]] + `E:\obscura\docs\sessions\2026-08-25-30-marketplace-web.md`.
- 2026-08-25 — **BLOK A / A1: libp2p discovery wiring** — `DiscoverBootstrapPeers` artık `host.go:Start()`'ta fiilen çağrılıyor (önceden 0 çağrı), `connectBootstrap` sonucundan besleniyor; başarılı bootstrap bağlantısında `SavePeer` peer_cache'e yazıyor (önceden iki taraflı ölüydü). BOOTSTRAP_PEERS env (kaynak 1) geriye uyumlu doğrulandı. 3 dosya, +30/-2, uncommitted — kullanıcı onayı bekleniyor. Bkz. [[2026-08-25-a1-libp2p-wiring]] + `E:\obscura\docs\sessions\2026-08-25-a1-libp2p-wiring.md`.
- 2026-08-26 — **BLOK A TAMAMEN KAPANDI** — A3 (imza: Vote.Sig + Block.Sig, stub→gerçek Ed25519, federation registry doğrulama kaynağı) + A4 (iki-node canlı kanıt: iyi senaryo — 2 bağımsız height'te birebir aynı hash; kötü senaryo — sahte imza/sahte proposer/kimlik taklidi 3 saldırı, üçü de reddedildi) + A5 (kritik liveness bug'ı: proposer/oy self-echo — node kendi oyunu kendi tablosuna hiç yazmıyordu, LocalTransport 27 testte maskeliyordu, gerçek P2P'de iki dürüst node asla mutabakata varamıyordu — bulundu+düzeltildi+kanıtlandı). Trustless çekirdeği (A1-A5) canlı ağda ispatlandı. Commit `05d2b82`+`c3e3b88`+`064bdf4`+`9b27785`. Bkz. `E:\obscura\docs\sessions\2026-08-26-a-blok-kapanisi.md`.
- 2026-08-26 — **A2 KAPANDI** — bootstrap davetli-ağ kararına kesinleştirildi: `centralDiscoverySourcesEnabled=false` sabitiyle DNS-TXT/ENS pasif (kod silinmedi, derleme-zamanı sabit), env+peer_cache tek aktif kaynak. Gerçek iki-node kanıtı: node-2 env-tohum ile ilk bağlanıp `SavePeer` ile cache'e yazdı, sonra BOOTSTRAP_PEERS OLMADAN yeniden başlayıp `peer_cache`'ten otomatik buldu+bağlandı (`PeerCount=1`) — manuel tohum bir kereydi. Commit `6257627`. Bkz. `E:\obscura\docs\sessions\2026-08-26-a2-invited-bootstrap.md`.
- 2026-08-25 — **B7 KAPANDI** — grup türleri (kanal/topluluk/grup) uçtan uca: backend yetki gate'i (üye-olmayan yazma + herkes-davet delikleri kapandı, kanal=admin-only yazar, topluluk=herkes davet eder, is_public self-join `mls_synced:false` dürüstlüğüyle) + web UI (oluşturma ekranları gerçek backende bağlandı, tür-farkındalı composer/davet gating, keşif+self-join). 7 commit. Yol boyunca kendi hatamı buldum+düzelttim (redundant "+" menü yerine mevcut `NewChatSheet`'i tamamladım) ve `new-group`'un da aynı stub bug'ını taşıdığını görüp düzelttim. Gerçek backend'e karşı 3-kullanıcılı canlı akışla doğrulandı. Kalan (flag, B10): web grup mesaj GÖNDERME hâlâ çalışmıyor (`to_id: conv.peer_did` her zaman, `is_group` hiç geçmiyor — B7'den önce de böyleydi, MLS wiring gerektirir, dokunulmadı). Bkz. `E:\obscura\docs\sessions\2026-08-25-b7-group-types.md`.
- 2026-08-24 — **E1 KAPANDI** — `chat/[id].tsx` gerçek MLS akışına bağlandı (aynı gün, denetimin devamı). CANLI SMOKE PASS (gerçek yerel backend + gerçek auth + gerçek `/v1/mls/*`, Alice↔Bob iki yönlü). Wiring sırasında 2 AEAD nonce-reuse bug'ı bulundu+düzeltildi (gönderen+alıcı taraf ratchet state persistence) — mock-relay jest'lerinin yakalayamayacağı türden gerçek kripto hatası. Kalan (flag): grup medya yok (metin-only), WS real-time yok (4sn polling), çok-üyeli/sıra-dışı ratchet senaryoları henüz denenmedi (E2 kapsamı). Bkz. [[2026-08-24-e1-chat-wiring]] + `E:\obscura\docs\sessions\2026-08-24-e1-chat-wiring.md`.

## Şablon
Yeni oturum logu: `06_Metadata/Templates/Session.md` kopyala → `E:\obscura\docs\sessions\YYYY-MM-DD-kisa-baslik.md`
