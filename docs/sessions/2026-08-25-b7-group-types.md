# Session 2026-08-25 — B7: Grup türleri (kanal/topluluk/grup) uçtan uca

## Summary
`conv_type`/`is_public` önceden sadece etiketti (davranış farkı yoktu); şimdi backend'de gerçek yetki mantığına, web'de gerçek UI'ya bağlı — B7 tümden kapandı, 7 commit.

## Tasks completed
- **Faz 0 (doğrulama, kod yok):** `conv_type`/`is_public` kullanım haritası çıkarıldı (dosya:satır). Bulgular: mesaj yazma/okuma/join/invite/rol yönetimi `conv_type`'a hiç bakmıyordu; `HandleSendMessage` üyelik kontrolü yapmıyordu (herkes herhangi bir grup conv_id'sine yazabiliyordu); `HandleCreateConvInvite` requester'ın üye olduğunu bile kontrol etmiyordu; join sadece invite-code ile mümkündü, `is_public=1` konuşmalar Discover'da listelenmesine rağmen self-join endpoint'i yoktu. Semantik tablo önerildi, onaylandı.
- **Faz 1 — güvenlik deliği (commit `57c47a9`):** `handlers.go` `HandleSendMessage`'a `isConvMember` gate eklendi (üye olmayan yazamaz). `extra_handlers.go` `HandleCreateConvInvite`'a admin-only kontrolü eklendi. Mevcut bir test (`sealed_policy_test.go`) sahte conv_id kullanıyordu, gerçek grup+üyelikle düzeltildi. Yeni regresyon testleri: `group_membership_gate_test.go`.
- **Faz 1 — tür dallanması (commit `313210e`):** kanal=admin-only yazar (HTTP seviyesi, MLS'e dokunulmadı), grup/topluluk=tüm üyeler yazar, davet grup/kanal=admin-only topluluk=herkes. Testler: `conv_type_authz_test.go`.
- **Faz 1 — self-join (commit `832f6a4`):** `POST /v1/conversations/{id}/join` — `is_public=1` konuşmalara invite'sız katılma. Yanıt her zaman `mls_synced:false` — MLS üyeliği ayrı, senkron değil. Testler: `self_join_test.go`.
- **Faz 2 — web oluşturma ekranları (commit `30b71d3`):** `new-channel`/`new-community` sayfaları ÖNCEDEN VARDI ama `api.createChannel`/`createCommunity` hiç var olmayan metodlardı (demo-fallback stub) — hiçbir zaman gerçek backend'e ulaşmıyordu. Gerçek `api.createConversation({type:...})`'a bağlandı.
- **Faz 2 — tür-farkındalı davranış (commit `a552d76`):** `lib/store.ts` `Conversation` tipine `conv_type`/`is_public`/`my_role` eklendi (backend zaten döndürüyordu, web tipi eksikti). Kanal'da admin değilse composer yerine salt-okur banner'ı. Davet butonu yetkisiz kullanıcıda hiç render edilmiyor. `components/Toast.tsx` (`ToastProvider`) ÖNCEDEN VARDI ama hiçbir yerde mount edilmemişti (grep: sıfır kullanım) — `app/layout.tsx`'e bağlandı.
- **Faz 2 — öz-düzeltme (commit `f2c6dba`):** İlk turda (`30b71d3`) `chats/page.tsx` header'ına kendi "+" menümü eklemiştim — ama `components/AppShell.tsx` zaten her ekranda `GravityWell` (alt nav) üzerinden `NewChatSheet`'i açıyordu; o sheet'in "Yeni Grup Oluştur" butonu vardı ama `onClick`'i yoktu (dead button), kanal/topluluk hiç yoktu. Kendi menümü geri aldım, gerçek mekanizmayı tamamladım: `NewChatSheet.tsx`'e Kanal/Topluluk/Keşfet eklendi, `new-group/page.tsx`'in aynı stub deseni (`api.createGroup` yoktu) aynı fix'le düzeltildi.
- **Faz 2 — keşif + self-join UI (commit `72995b4`):** `app/chats/discover/page.tsx` (yeni) — `is_public=1` konuşmaları listeler, "Katıl" butonu. `mls_synced:false` dürüstlüğü UI'da korunuyor: sayfa üstünde sabit bilgi notu + katılım sonrası toast'ta "şifreli mesajlar ayrıca senkronize edilecek, hemen görünmeyebilir".

## Decisions made
- Kanal broadcast kripto modeli (MLS mi ayrı fanout mu) ve MLS ölçek/spec-7.2 çelişkisi (Platin 10000/Elmas limitsiz vaadi vs RFC 9420 üye-başı Welcome+TreeKEM) — B7 kapsamı DIŞI bırakıldı, A4'e açık soru olarak taşındı (Master-Liste, A4 altında).
- Faz 1'in davet-gate varsayılanı admin-only seçildi (Faz 0 tablosunda "tercihen admin" önerisi netleştirildi), Faz 2'de topluluk için gevşetildi — iki aşamalı, her adımı ayrı test edilebilir tutuldu.
- packages/theme (`@obscura/theme`, `#C9A24B`/`#30D158`) SADECE marketplace ekranları için — chat ekranları kendi mevcut `var(--em)`/`var(--surface-*)` token sistemini kullanmaya devam etti (kod incelemesiyle doğrulandı: `app/layout.tsx:55-59` yorumu + `packages/theme/src/index.ts:1-10` bunu açıkça söylüyor).

## Files changed
- backend/internal/api/handlers.go, extra_handlers.go, group_handlers.go
- backend/internal/api/{group_membership_gate,conv_type_authz,self_join}_test.go (yeni)
- backend/internal/api/integration_test.go, sealed_policy_test.go
- backend/cmd/node/main.go
- frontend/lib/api.ts, lib/store.ts, app/layout.tsx
- frontend/app/chats/page.tsx, new-channel/page.tsx, new-community/page.tsx, new-group/page.tsx, discover/page.tsx (yeni), [id]/page.tsx
- frontend/components/NewChatSheet.tsx

## Spec gaps closed
- conv_type/is_public artık davranışsal (Spec Bölüm 5.2 grup/kanal/topluluk ayrımı) — önceden sadece etiketti.
- HTTP-katmanı yetki deliği kapandı (üye-olmayan yazma, herkes-davet-üretebilir).
- Web'de grup/kanal/topluluk oluşturma, keşif, self-join uçtan uca çalışıyor (önceden hiçbiri gerçek backend'e bağlı değildi, bazıları tamamen erişilemezdi).

## Spec gaps remaining (bu çalışma alanında)
- **B10 (yeni madde, Master-Liste'ye eklendi):** `app/chats/[id]/page.tsx` `sendMessage` (ve 3 diğer call-site) her zaman `to_id: conv.peer_did` kullanıyor, `is_group` hiç geçmiyor — grup/kanal/topluluk mesaj gönderme web'de `conv_type`'tan BAĞIMSIZ olarak zaten çalışmıyordu (B7'den önce de böyleydi, B7 bunu değiştirmedi). Web MLS değil kendi Signal-ratchet'ini kullanıyor (`lib/e2ee-session.ts`) — E1 sadece mobile'ı MLS'e bağladı. Düzeltmek MLS wiring gerektirir, guardrail nedeniyle bu oturumda dokunulmadı.
- Kanal broadcast kripto modeli + MLS ölçek çelişkisi — A4'e taşındı (yukarıda).
- ENS stub, DNS-TXT domain yayınlanmamış — A2'nin konusu, B7 ile ilgisiz.

## CLAUDE.md updates needed
- Yok

## Open questions for next session
- B10 ne zaman ele alınacak — MLS wiring gerektirdiği için Opus önerildi (Master-Liste).
- A2 kararı (davetli-ağ mı açık-katılım mı) hâlâ bekliyor — B7 bunu etkilemedi.

## Notes
- **Kanıt disiplini:** Her Faz 1 alt-adımı kendi Go testiyle kanıtlandı (`go build`/`go vet`/`go test ./internal/api/...` her commit öncesi temiz). Faz 2'de `tsc --noEmit` + `next build` (46/46 sayfa) her commit öncesi temiz.
- **Gerçek backend'e karşı canlı doğrulama (son commit `72995b4` öncesi, bu oturumda):** `DATA_DIR=/tmp/obscura-b7-verify OBSCURA_ENV=development P2P_ENABLED=false` ile backend ayağa kaldırıldı, 3 gerçek kullanıcı (alice/bob/carol) gerçek OTP akışıyla kaydedildi. Web kodunun ürettiği TAM istek gövdeleriyle (to_id/is_group/encryption_type/type/is_public alanları page.tsx'lerdeki ile birebir) tüm akış çalıştırıldı:
  - BOB (tier3) `is_public` kanal oluşturdu (ALICE üye) → `conv_type:channel, is_public:true` — `GET /v1/conversations` aynı şekilde döndü (lib/store.ts tip eklentisi doğrulandı)
  - ALICE (üye, admin değil) kanala yazmayı denedi → 403 "Bu kanalda sadece yöneticiler mesaj yazabilir" — composer banner metniyle BİREBİR aynı çıktı
  - BOB (admin) yazdı → 201
  - ALICE davet oluşturmayı denedi → 403; BOB (admin) oluşturdu → 200
  - `GET /v1/conversations/discover` → kanal listede
  - CAROL invite'siz join etti → 200 `{mls_synced:false, status:"joined"}`; tekrar join → `{status:"already_member"}` (idempotent)
  - BOB topluluk oluşturdu (ALICE üye) → ALICE (admin değil) davet oluşturdu → 200 (topluluk=herhangi üye) VE mesaj yazdı → 201 (topluluk=tüm üyeler yazar)
  - Test backend + geçici DB oturum sonunda kapatıldı/silindi, iz yok.
- Master-Liste.md güncellendi (B7 ✅BİTTİ'ye taşındı, B10 eklendi, A4 altına 2 açık soru eklendi) — henüz git'e commit edilmedi.
