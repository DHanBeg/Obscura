# Oturum — 2026-08-25 — #30 Marketplace UI (mobile + web) kapanış

Devamı: aynı gün başlayan #30 mobile oturumunun web (Faz 3) tamamlanışı.
Mobile kısmı önceki oturumda kapandı (`12b87b8`,`78928b5`,`c2a088b`).

## Faz 0 (bu oturumdan önce, aynı #30 içinde)
`packages/theme` (@obscura/theme) token katmanı + logo altın recolor
(#C9A24B). Commit `c0ee671`.

## Faz 1 (bu oturumdan önce)
Endpoint envanteri, 1 boşluk bulundu+kapatıldı (GET transaction/dispute,
liste sorgusu) — kullanıcı onayıyla küçük backend eki. Commit `ec806bf`.

## Faz 3 — Web (bu oturum)

Mobile'ın (Faz 2) birebir karşılığı, aynı 12 endpoint, saf plumbing.

**Tema:** `frontend/app/layout.tsx:3,53-59` — `@obscura/theme`'in
`cssVars()`'ı `<style>` etiketiyle `:root`'a enjekte edildi. globals.css'in
mevcut `--accent`/`--bg`/`--em` token'larına DOKUNULMADI — yeni değişkenler
`--color-*`/`--spacing-*`/`--radius-*`/`--text-*` önekiyle ayrı isim
uzayında, ADDITIVE. Altın (`var(--color-accent)`, #C9A24B) sadece fiyat +
birincil CTA + aktif kategori chip'te — "cimri" kullanım talimatına uyuldu.
Yapısal sınıflar (`.card`, `.card-interactive`, `.page-header`, `.badge`)
mevcut design system'den yeniden kullanıldı, yeni hex/px yazılmadı.

**Ekranlar (3 commit, mobile'ın grup sırasıyla birebir):**
- `d93bea6` — `app/marketplace/page.tsx` (liste+arama+filtre),
  `app/marketplace/[id]/page.tsx` (detay+satın-al), `lib/marketplace-api.ts`
  (10 fonksiyon), `wallet/page.tsx`'e "Pazar" sekmesi.
- `2dcf1c9` — `app/marketplace/orders/page.tsx` (Siparişlerim),
  `app/marketplace/orders/[id]/page.tsx` (escrow durum + release + dispute
  aç/gör — dispute id mobile ile AYNI gerekçeyle localStorage'da önbelleğe
  alınıyor, backend'de transaction→dispute listesi yok).
- `cc3fa20` — `app/marketplace/new/page.tsx` (satıcı ilan oluşturma).

## Kanıt

**tsc + gerçek build:** `npx tsc --noEmit` temiz (her commit öncesi ayrı
ayrı koşuldu). `npx next build` — 48 route (marketplace/* dahil), hatasız,
mevcut sayfalar (chats/wallet/staking/vb.) değişmeden aynı boyutlarla
çıktı — regresyon yok.

**CANLI SMOKE (Node native fetch, scratchpad'de, commit'lenmedi):**
`frontend/lib/api.ts:12-28`'in `apiFetch`'iyle BİREBİR aynı request şekli
(Content-Type+Authorization Bearer, `{success,data,error}` zarf parse'ı) —
gerçek yerel backend'e (`go run ./cmd/node`, `OBSCURA_ENV=development`)
karşı 13/13 adım PASS:
1. Satıcı ilan açar → 2. gezinme listesinde görünür → 3. detay doğru →
4. alıcı satın alır (escrow "held") → 5. ilan "sold" → 6. Siparişlerim'de
görünür → 7. işlem detayı buyer/seller/status doğru → 8. release →
9. "released" → 10. ikinci alışta dispute aç → 11. GET ile durum gör
(buyer) → 12. seller de aynı dispute'u görebiliyor → 13. **YABANCI
kullanıcı transaction'ı GÖREMİYOR** (403 `ErrNotParticipant` — auth guard
fiilen doğrulandı, sadece "var" değil "çalışıyor").

Funding/tier-bump için mobile'daki AYNI teknik (backend'in kendi
`fundMarketplaceUser` test helper'ıyla aynı mekanizma, `token.Mint` + tier
UPDATE, geçici `cmd/smoke-fund`, commit'lenmeden silindi) — HTTP üzerinden
faucet yok (airdrop ZK-proof istiyor), bu kapsamın dışında.

## Kapsam dışı / bilinen sınır (flag)

- Satıcı ilan düzenle/sil (PATCH/DELETE) mobile'da da yok, web'de de yok
  — backend'de var (`marketplace_handlers.go:146,195`), ekran yok. Ayrı iş.
- Admin dispute resolve ekranı yok (backend admin route var, kullanıcı
  arayüzü kapsam dışı — admin paneli ayrı konu).
- Dispute durumu SADECE localStorage'da — tarayıcı/cihaz değişince kaybolur
  (mobile'da da aynı sınır, backend'de transaction→dispute listesi
  olmadığı için).

## Sonuç

#30 (Marketplace UI, mobile + web) TAMAMEN KAPANDI. Backend zaten
tam+testliydi; mobile+web ekranları artık gerçek endpoint'lere bağlı,
her ikisi de bağımsız canlı smoke ile kanıtlandı (mobile: Jest, web: Node
fetch). E1 ve önceki #30-mobile işine dokunulmadı, regresyon yok.
