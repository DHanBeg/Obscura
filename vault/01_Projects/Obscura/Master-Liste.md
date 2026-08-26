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

- [ ] **B5 — grup medya** · resim/video/dosya/ses 5 call-site MLS akışına bağlı değil · *(madde 1)*
  - *Model: CC (Opus) — kripto-dokunuşlu, E1 ratchet'ına dokunur*
- [ ] **B6 — real-time push** · WS `mls_message` event backend'de var, mobile'a bağlı değil, 4sn polling · *(madde 2)*
  - *Model: CC (Sonnet) — mevcut event'i bağlama*
- [ ] **B8 — gerçek ödeme** · sadece OBS iç transfer var, fiat/kripto giriş yok · *(altyapı)*
  - *Model: CC (Opus) — para yolu, yüksek dikkat*
- [ ] **B9 — #30 kuyruğu**
  - web dispute durumu local-only → `GetDispute` route'una bağla (veri-kalıcılık borcu)
  - satıcı ilan düzenle/sil ekranı (backend var)
  - admin dispute-resolve ekranı (backend var)
  - *Model: CC (Sonnet) — plumbing*
- [ ] **B10 — web grup mesajlaşma (E1'in web tarafı yok)** · keşif (B7 Faz 2, 2026-08-25): `frontend/app/chats/[id]/page.tsx` `sendMessage` (satır ~250) ve diğer 3 gönderim call-site'ı HER ZAMAN `to_id: conv.peer_did` kullanıyor, `is_group` hiç geçirilmiyor — grup/kanal/topluluk mesaj gönderme web'de conv_type'tan BAĞIMSIZ olarak zaten çalışmıyor (peer_did grup için undefined → backend'e boş to_id gider). Web kendi Signal-tarzı ratchet'ini kullanıyor (`lib/e2ee-session.ts`), MLS değil — E1 mobile'ı MLS'e bağladı, web'i bağlamadı. B7 Faz 2 composer/davet gating'i bu kırıklığı DEĞİŞTİRMEDİ, sadece kanal/grup ayrımını doğru yansıtıyor (backend zaten 403'ü doğru veriyor, ama grup'ta bile üye yazamaz çünkü to_id boş gider). Düzeltmek MLS wiring gerektirir (guardrail nedeniyle B7'de dokunulmadı). · *(madde 1,2)*
  - *Model: CC (Opus) — kripto-dokunuşlu, E1'in web muadili*

---

## 🟡 BLOK C — Launch hijyeni
> En son. Görünmez ama şart.

- [ ] **C10 — #12 güvenlik denetimi** · 22/23 madde doğrulanmadı
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

**Şu an (2026-08-26):** 🔴 **BLOK A TAMAMEN KAPANDI** — A1(bulma)+A2(davetli-ağ)+A3(imza)+A4(iki-node kanıt, iyi+kötü senaryo)+A5(liveness) hepsi commit'li, trustless çekirdeği canlı ağda ispatlandı. B7 de kapandı. Sırada: 🟠 BLOK B (ürün tamlığı — B5/B6/B7✅/B8/B9/B10, A'dan çok daha hafif) ve 🟡 BLOK C (launch hijyeni). Kritik yolda bir sonraki gerçek adım B veya C'den başlar.
