# Oturum — 2026-08-24 — E1 Grup E2E Chat Wiring (kapanış)

Devamı: [[2026-08-24-ground-truth-audit]] (aynı gün, aynı oturum zinciri). Denetim E1'i
"kapanmadı" işaretlemişti (`chat/[id].tsx`'te MLS fonksiyonlarına 0 çağrı). Bu oturumda
kapatıldı — kanıt aşağıda.

Kalem 0 (hijyen) bu oturumda ayrıca yapıldı: denetim dokümanları commit `dd74ee5`,
push `9c3302e..dd74ee5`. CI silent-skip kalemi (crypto-cli/mls-cli/deno) AÇIK KALDI —
bu oturuma girmedi, sıradaki kalem.

## Hedef

`mobile/app/(main)/chat/[id].tsx`'i gerçek MLS akışına bağlamak:
`createOwnKeyPackage → uploadKeyPackage → getKeyPackage → createGroupWithMember →
getWelcomes → joinFromWelcomeWire → encryptGroupMessage → sendGroupMessage →
getGroupMessages → decrypt`. Kapanış kriteri ADR-0019 (satır 283-287): "Mobile,
`/v1/mls/*`'e gerçek MLS mesajı gönderip başka bir üyenin bunu ts-mls ile çözdüğü an
kapanmış sayılır."

## Faz 1 — Envanter (kanıt)

- `chat/[id].tsx` (1109 satır öncesi): grup gönderim 6 call-site, hepsi
  `isGroupSendBlocked(conv)` → Alert+return (satır 189,257,305,327,351,369).
- Geçmiş çekme (satır 141-157 öncesi): HER ZAMAN `api.getMessages(convId)` (1:1 REST) —
  grup mesajları hiç MLS'ten çekilmiyordu.
- Decrypt effect (159-178 öncesi): tüm `convMsgs`'i `receiveMessage` (1:1 ratchet) ile
  çözmeye çalışıyordu, grup guard'ı yoktu (zararsızdı çünkü grup mesajı hiç fetch
  edilmiyordu).
- Zaten bağlı, dokunulmadı: `createGroupFlow.ts`→`new-group.tsx` (grup kurma),
  `joinGroupFlow.ts`→`mls-invites.tsx` (davet kabul) — Tuğla 5b-2/5c'de hazırdı.
- Backend'de WS push var (`mls_handlers.go:435`, `messaging.MsgTypeMlsMessage` =
  `"mls_message"`) ama mobile `_layout.tsx` switch'inde case yoktu. Görev zinciri WS
  içermiyordu (sadece `getGroupMessages`) — bu oturumda WS BİLEREK atlandı, **4sn
  polling** kullanıldı (kapsam dışı, flag edildi).

## Faz 2 — Wire

`mobile/app/(main)/chat/[id].tsx`:
- `sendMessage`: `classifyConv(conv)` dalına ayrıldı — `"unknown"` bloklu kalır,
  `"group"` yeni `sendGroupText` callback'ine yönlenir, `"direct"` eski 1:1 kod
  değişmeden akar.
- Fetch-history effect: grup dalında `fetchAndDecryptGroupMessages` çağırır, sonucu
  `Message[]`'e map'ler + `decrypted` state'ini doldurur.
- 1:1 decrypt effect (`receiveMessage`): `classifyConv(conv) !== "direct"` guard'ı
  eklendi — grup mesajları artık bu yoldan hiç geçmiyor.
- Yeni polling effect: grup konuşmasıysa 4sn'de bir `fetchAndDecryptGroupMessages`,
  yeni mesajları `addMessage`/`decrypted`'e merge eder (id bazlı dedupe, mevcut
  girdileri asla ezmez).
- 5 diğer gönderim call-site'ı (resim/video/dosya/konum/ses, satır ~257-369) BİLEREK
  dokunulmadı — `isGroupSendBlocked` Alert+return grup için hâlâ geçerli. Görev
  zinciri (`encryptGroupMessage(plaintext: string)`) sadece metni kapsıyor.

Yeni dosya `mobile/lib/mls/groupChat.ts` (102 satır) — `createGroupFlow.ts`/
`joinGroupFlow.ts` ile AYNI mimari desen (saf, UI'sız orkestrasyon):
- `sendGroupTextMessage(groupId, plaintext, stores?)` — encrypt → **saveGroupState
  (ağdan ÖNCE)** → `sendGroupMessage`.
- `fetchAndDecryptGroupMessages(groupId, sinceEpoch?, stores?, cacheStores?)` —
  `getGroupMessages` → sadece `content_type==="application"` → created_at sırayla
  çöz → başarılı her decrypt'i `plaintext-cache.ts`'e yaz (1:1'in zaten kullandığı
  desen) → state'i döngü sonunda kalıcı hale getirir.

## Bulunan 2 bug (mock-relay testleri yakalamamıştı, smoke'un amacı buydu)

**Bug 1 — gönderen taraf (`group.ts:248`, `encryptGroupMessage`):** ts-mls'in
döndürdüğü `newState`'i atıyordu. Kanıt: `vendor/ts-mls/.../createMessage.js:31-39`,
her `createApplicationMessage` çağrısı secretTree generation'ı ilerletir. Persist
edilmezse art arda 2 grup mesajı AYNI ratchet generation/nonce ile şifrelenir (AEAD
reuse). **Fix:** `EncryptedGroupMessage`'a `newState: ClientState` eklendi (yeni kripto
DEĞİL, zaten hesaplanan değeri dışa açmak) — `groupChat.ts` her send sonrası
`saveGroupState` ile kaydediyor.

**Bug 2 — alıcı taraf (`group.ts:340`, `decryptApplicationMessageWire`), CANLI
smoke'ta yakalandı:** `bobInbox2` testinde 2. mesaj `"aes/gcm: invalid ghash tag"` ile
çözülemedi. Kök neden izole edildi (`_debug-two-msgs.smoke.test.ts`, geçici, silindi):
ts-mls'in `processMessage`'ı **state referansının secretTree'sini yerinde mutasyona
uğratıyor** — aynı `ciphertext_b64`'ü aynı state referansıyla İKİNCİ kez çözmek bile
2. denemede aynı hatayla patlıyor (kanıtlandı, izole test). `mls-store.test.ts`'in
"KRİTİK" testi bunu hiç yakalamamış çünkü her reload'dan sonra SADECE BİR mesaj
çözüyordu. **Fix (2 katmanlı):**
1. Yeni `decryptApplicationMessageWireWithState` (group.ts) — `decryptApplicationMessageWire`
   İMZASI DEĞİŞMEDİ (5 mevcut test dosyası düz string dönüşüne güveniyor) — ayrı
   fonksiyon, sadece `groupChat.ts` kullanıyor, state'i döngüde açıkça ileri taşıyor.
2. `plaintext-cache.ts` entegrasyonu (`groupChat.ts`) — bir mesaj BİR KEZ çözülür,
   önbelleğe yazılır; sonraki poll'lar ciphertext'e hiç dokunmadan önbellekten okur.
   Neden gerekli: chat ekranı her poll'da TÜM geçmişi tekrar çeker (sinceEpoch yok,
   bilinçli MVP kararı) — forward-secret ratchette ESKİ bir generation'ı state ileri
   gittikten SONRA tekrar çözmeye çalışmak retention penceresi dışındaysa başarısız
   olur (bu bir hata değil, forward secrecy'nin ta kendisi). Önbellek olmadan chat
   ekranı 2+ mesajlı gruplarda eski mesajları "…" (çözülemedi) göstermeye başlardı.

## Kanıt

**(a) Yerel jest, tüm suite:**
```
Test Suites: 39 passed, 39 total
Tests:       1 skipped, 264 passed, 265 total
```
(1 skip = CANLI smoke dosyası, `OBSCURA_API_BASE` set değilken — `test.skip`,
"○ skipped" jest çıktısında görünür, sahte yeşil DEĞİL. Kalem 0'ın bulduğu
CI-silent-skip hatasına burada düşülmedi — bkz. aşağıdaki "araç hatası" notu.)

**(b) CANLI SMOKE (ADR-0019 kapanış kriteri) — `mls-e1-real-backend.smoke.test.ts`:**
```
√ Alice grup kurar, Bob katılır, iki yönlü ardışık mesajlaşma gerçek backend'de
  çalışır (generation-persistence dahil)
```
Koşum: `cd backend && OBSCURA_ENV=development DATA_DIR=... PORT=8099 go run ./cmd/node`
(yerel, gerçek Go node — production Railway'e DOKUNULMADI, ayrı localhost instance).
Sonra `cd mobile && OBSCURA_API_BASE=http://localhost:8099 npx jest mls-e1-real-backend.smoke`.

Zincir: gerçek `/v1/auth/request-otp`→`/v1/dev/otp` (dev-only, `OBSCURA_ENV=development`
gerekli, `dev_handlers.go:12`)→`/v1/auth/verify-otp` (gerçek JWT, `auth.GenerateToken`)
→ gerçek `/v1/mls/key-package`, `/v1/mls/group`, `/v1/mls/group/{id}/add`,
`/v1/mls/welcomes`, `/v1/mls/group/{id}/message`, `/v1/mls/group/{id}/messages` —
`chat/[id].tsx`'in fiilen çağırdığı `sendGroupTextMessage`/`fetchAndDecryptGroupMessages`
üzerinden (el ile `encryptGroupMessage`/`sendGroupMessage` tekrar yazılmadı). İki yön
(Alice→Bob, Bob→Alice), 3 mesaj (Alice x2 ardışık + Bob x1) — Bug 1 ve Bug 2 ikisi de
bu smoke'ta yakalandı ve düzeltildi.

**Araç hatası (kendi kendine düzeltildi, not düşülüyor):** Smoke dosyasında ilk yazımda
`process.env.OBSCURA_API_BASE = process.env.OBSCURA_API_BASE;` ("no-op, netlik için")
satırı vardı — Node'da `process.env.X = undefined` ATAMASI değeri `"undefined"` STRING'İNE
çevirir (process.env her zaman string coerce eder), bu da `REAL_BACKEND` kontrolünü
HER ZAMAN truthy yapıp testin production Railway'e (yanlışlıkla) gitmesine + JSON parse
hatasıyla FAIL olmasına (sessiz skip değil ama yanlış hedefe gitme) neden oluyordu. Satır
silindi, doğrulandı: env yokken artık dürüst `test.skip`.

## Kapsam dışı (bilerek, flag edildi)

- WS real-time push (`mls_message` event, backend'de var) — 4sn polling kullanıldı.
- Grup için resim/video/dosya/konum/ses gönderimi — hâlâ Alert-bloklu.
- 1:1 mesajlaşma bozulmadı — `x3dh`/`sealed-sender`/`ratchet`/`message-send`/
  `session-store` suite'leri hâlâ yeşil (regresyon yok, tam suite kanıtı yukarıda).

## Yeni dosyalar

- `mobile/lib/mls/groupChat.ts` (102 satır) — orkestrasyon, prod kod.
- `mobile/test-utils/nodeXhrPolyfill.ts` (66 satır) — test-only, Jest'in node
  testEnvironment'ında `XMLHttpRequest` hiç yok (doğrulandı) — Node `http` modülü
  üzerinden minimal polyfill, `api.ts`'in gerçek `apiFetch`'ini smoke test'te
  kullanılabilir kılıyor.
- `mobile/lib/__tests__/mls-e1-real-backend.smoke.test.ts` (168 satır) — CANLI smoke,
  `OBSCURA_API_BASE` yokken `test.skip`.

## Sıradaki

Kalem 0 kalan: CI silent-skip (crypto-cli/mls-cli/deno env). Sonra Kalem 2 (#30
marketplace UI). E1 artık launch-blocker listesinden düşebilir — vault
[[Phase-Status]] güncellendi.
