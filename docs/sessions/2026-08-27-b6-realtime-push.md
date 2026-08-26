# Oturum — 2026-08-27 — B6 real-time push (TAMAMLANDI, + 2 aylık kritik yan-bulgu)

BLOK A + B7 + B9 kapalıydı. Bu oturumun tek işi: grup mesajlaşmada 4sn
polling'i gerçek WS push ile değiştirmek. B5/B8/B10 kapsam dışı,
MLS/ratchet kriptosuna dokunulmadı.

## Faz 0 — envanter

- Backend `mls_message` WS event'i `internal/api/mls_handlers.go:435` —
  grup mesajı POST handler'ında, önce `mls_messages` tablosuna INSERT
  (`:409-416`), SONRA online üyelere `messaging.GlobalHub.SendTo` ile
  broadcast. **1:1'in `"new_message"` push'uyla AYNI hub** (`handlers.go:987`).
  Best-effort — offline'sa drop edilir ama mesaj zaten DB'de, kayıp değil.
- Mobile: WS dispatch tek noktada, `app/_layout.tsx:64-112`. `"mls_message"`
  case'i YOKTU — event sessizce düşüyordu. Grup polling:
  `app/(main)/chat/[id].tsx:199-228`, `setInterval(tick, 4000)`,
  `fetchAndDecryptGroupMessages(groupId)` (sinceEpoch kullanılmıyor, her
  tick'te tam liste + cache-diff).
- 1:1 deseni: WS push VAR, periyodik fallback YOK — sadece mount'ta tam
  geçmiş çekimi + `createWS`'in kendi 3sn reconnect'i.
- Kontrol edilen 5 diğer MLS WS event'i (`mls_welcome`/`mls_commit`/
  `mls_removed`/`mls_key_update`/`key_package_rotation_needed`) mobile'da
  HİÇ bağlı değil — B6 kapsamı dışı, dokunulmadı, Master-Liste'ye ayrı
  madde düşüldü.

**Kullanıcı kararı (1. tur):** periyodik fallback TAMAMEN kaldırılsın
(1:1 ile birebir aynı desen), şartıyla: reconnect-yakalama ZORUNLU ve
KANITLI (WS kasıtlı koparılıp reconnect sonrası kaçan mesajın geldiği
canlı gösterilecek).

## Faz 1 kodu — ilk hâli

`lib/store.ts`: `mlsMessageNudge` (decrypt YOK, sadece "bir şey geldi"
sinyali) + `setMlsMessageNudge`. `app/_layout.tsx`: `"mls_message"` case'i
nudge'ı set ediyor. `chat/[id].tsx`: 4sn `setInterval` yerine nudge veya
`wsConnected` false→true geçişiyle tetiklenen `fetchAndDecryptGroupMessages`
çağrısı (E1'in aynı fonksiyonu, decrypt yolu değişmedi).

## Kritik keşif — WS auth 2 aydır tamamen kırık

Canlı kanıt testi yazılırken (`mls-b6-realtime-push.smoke.test.ts`) WS her
denemede 3sn'de bir kapanıp yeniden deniyordu, `onopen` HİÇ ateşlenmiyordu.
Kök neden:

- `lib/api.ts:361-378` `createWS()` — token'ı bağlantı AÇILDIKTAN SONRA bir
  WS mesajı olarak gönderiyordu (`ws.send({type:"auth",token})`).
- Backend `/v1/stream` (`cmd/node/main.go:567-578`) token'ı SADECE query
  param veya `Authorization` header'dan, upgrade ANINDA okuyor —
  `internal/messaging/hub.go` `ReadPump`→`HandleMessage`'da `"auth"`
  case'i HİÇ yok.
- Sonuç: her bağlantı 401 → `CLOSE 1006`, `onopen` asla ateşlenmiyor,
  `wsConnected` asla `true` olmuyor. **1:1/grup/call/presence real-time
  push'un TAMAMI ölüydü.**
- `git log -S'ws.send(JSON.stringify({ type: "auth"'` → regresyon commit'i:
  **`1873709`, 2026-06-23** ("feat(mobile): welcome screen, 11 missing
  pages, eas.json, session log"). Öncesinde kod `?token=${token}` query
  param kullanıyordu (çalışıyordu). 2 ay boyunca kimse fark etmedi — app
  tamamen HTTP polling/mount-refetch fallback'ine dayanmış.

**DUR edildi, kullanıcıya raporlandı.** Karar: query param DEĞİL, header —
`nginx/nginx.conf:17-21` `log_format main` `$request` query string'i
plaintext JWT olarak `access.log`'a yazar (kanıtlandı, gerçek risk); header
aynı riski taşımıyor (nginx `$request` sadece istek satırını yakalar) ve
backend zaten destekliyor (`main.go:571-572`, sıfır backend değişikliği).
RN'nin `WebSocket`'i (`node_modules/react-native/Libraries/WebSocket/
WebSocket.js:98-148`) 3. argüman `{headers}`'ı native seviyede destekliyor.

## FAZ A — WS auth fix (`24aec07`)

`lib/api.ts:createWS()` → `Authorization: Bearer <token>` header (mesaj
bazlı auth kaldırıldı). Test tarafı ayrı bir keşif daha çıkardı: Node'un
native `WebSocket`'i sadece 2-argümanlı `(url,{headers})` destekliyor,
RN'nin 3-argümanlı `(url,protocols,{headers})` şeklini DEĞİL (kanıtlandı,
`ws-arg-shape-probe`) — `createWS()` GERÇEK RN şeklinde bırakıldı
(production kodu değişmedi), `test-utils/nodeWebSocketShim.ts` sadece
Jest/Node smoke testleri için şekil çevirici.

**Canlı kanıt** (`ws-auth-header-fix.smoke.test.ts`, gerçek backend):
1:1 `"new_message"` push **97ms**'de geldi (2 aydır hiç gelmiyordu).
Kasıtlı WS kopması → reconnect (createWS'in 3sn mekanizması) → push
**98ms**'de yine geldi — header reconnect'te de taşınıyor.

## FAZ B — B6'nın kendisi (`49b1653`)

FAZ A fix'i üzerine, Faz 1'in kodu aynen. **Canlı kanıt**
(`mls-b6-realtime-push.smoke.test.ts`, gerçek MLS grup, gerçek backend):
grup mesajı **73ms**'de geldi (eski 4000ms poll'un ~1/55'i). Kasıtlı WS
kopması → Alice kopukken 2. mesaj gönderdi → reconnect (~3sn) →
reconnect-catchup fetch'i kopukken gönderilen mesajı **kayıpsız** yakaladı.

## Regresyon kontrolü

Tüm mobile jest suite (267 test, 42 suite) + E1 canlı smoke — ikisi de her
adımdan sonra tekrar tekrar çalıştırıldı, hepsi yeşil. `tsc --noEmit`
temiz her commit öncesi.

## Commit'ler

- `24aec07` — fix(ws): auth regresyonu (1873709, 2 aylık) — header-based
- `49b1653` — feat(mobile): B6 — grup mesajlaşmada 4sn polling yerine WS push
- `0aef3bf` — docs(vault): B6 kapanış kaydı — Master-Liste

## Kalan / kapsam dışı

- 5 diğer MLS WS event'i (`mls_welcome`/`mls_commit`/`mls_removed`/
  `mls_key_update`/`key_package_rotation_needed`) mobile'da hâlâ bağlı
  değil — üyelik/epoch/key-rotation değişiklikleri real-time gelmiyor.
  Ayrı iş, Master-Liste'de not edildi.
