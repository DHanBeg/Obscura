# Oturum — 2026-08-26/27 — B9 #30 kuyruğu (TAMAMLANDI)

BLOK A (A1-A5) + B7 önceki oturumda kapandı, push'landı. Kullanıcı durmadan
B9'a geçti.

## Kapsam (kullanıcı talimatı, tam)

Üç parça: (1) web dispute local-only → gerçek endpoint'e bağla, (2) satıcı
ilan düzenle/sil ekranı, (3) admin dispute-resolve ekranı. Guardrail: yeni
backend/route/tablo yok (gerekirse DUR sor), gerçek endpoint zorunlu (mock
yasak), BFT/A-blok/MLS'e dokunma, her commit ayrı+push, her iddia dosya:satır.

## Faz-0 keşif → kullanıcı kararı

Parça 1'in premise'i yanlış çıktı: dispute durumu zaten sunucudan çekiliyor,
gerçek eksik daha dar (transaction→dispute ID keşfi local-only). İki kez
soruldu (`AskUserQuestion`, kullanıcı "tekrar sor" dedi, aynı soru tekrar
soruldu). Nihai karar: **Parça 2 → Parça 3 → sonra Parça 1**, Parça 1 için
şart — migration/backfill/dispute-yaratma yolu değişikliği gerekiyorsa
"küçük" sayılmaz, DUR ve kapsamı yeniden çiz.

## Parça 2 — TAMAMLANDI, commit+push edildi (`fb161ff`)

`app/marketplace/[id]/page.tsx` (Düzenle/Kaldır butonları, `handleDelete`),
yeni `app/marketplace/[id]/edit/page.tsx` (create formunun birebir muadili,
`getListing`→prefill, `updateListing`→PATCH). Canlı doğrulandı: gerçek PATCH
kalıcı (GET ile re-confirm), non-owner PATCH/DELETE'te 403, DELETE
`status:"removed"` kalıcı.

## Parça 3 — KOD TAMAM, CANLI DOĞRULANDI, commit BEKLİYOR (kesinti sırasında)

`lib/marketplace-api.ts`'e `resolveMarketplaceDispute`/`ResolveDisputeResult`
eklendi. Yeni `app/admin/marketplace/disputes/page.tsx`: admin tespiti YENİ
endpoint eklemeden, var olan admin-gated `api.adminListReviewQueue({limit:1})`
ile SESSİZ PROBE — 403 ise form hiç render edilmiyor (guardrail: yetkisiz
kullanıcıda ekran görünmesin, `admin/review/page.tsx` emsalinden daha katı).
Önizleme yok (GetDispute buyer/seller-only, admin-görüntüleme endpoint'i
kapsam dışı) — sadece dispute ID gir + karar ver + sonucu gör.
`npx tsc --noEmit` + `npx next build` temiz (route 2.75 kB).

**CANLI KANIT (gerçek backend, `localhost:8098`, `DATA_DIR=/tmp/b9-verify`):**
- Non-admin (seller token) → `POST /v1/admin/marketplace-disputes/{id}/resolve`
  → `{"error":"Bu işlem için yönetici yetkisi gerekli","code":403}` ✅
- Admin (doğru `OBSCURA_ADMIN_DIDS` DID'i) → aynı endpoint, `upheld:true`
  → `{"dispute_id":...,"transaction_id":"8d9c7932...","upheld":true,
  "paid_to":"did:obs:63c142..."}`, 200 ✅
- Kalıcılık doğrulandı: `GET /v1/marketplace/transactions/{id}` sonrasında
  `status:"refunded"`, `resolved_by:"did:obs:f8a4537..."` (admin'in DID'i). ✅

Commit atılmadı — kullanıcı `git add` adımında durdurdu, oturum kesintiye
uğradı. **Sıradaki adım: Parça 3'ü commit+push et** (dosyalar zaten stage
edilmemiş halde duruyor: `frontend/lib/marketplace-api.ts` (M),
`frontend/app/admin/marketplace/` (yeni)).

## Yan-keşif: gerçek prod bug bulundu, B9 kapsamı DIŞINDA, DOKUNULMADI

Canlı doğrulama sırasında test hesapları her backend restart'ında
"Hesabınız askıya alınmıştır" (403) almaya başladı, DB'de `is_active=true`
görünmesine rağmen. Kök neden bulundu:

- `internal/api/middleware.go:34` — `AuthMiddleware`'in `SELECT`'i `phone`
  kolonunu `COALESCE` OLMADAN çekiyor.
- `internal/subscriber/migrate.go:105` — her boot'ta çalışan
  `MigratePhoneToSubscriberStore`, migrate edilmemiş (`phone_migrated=0`)
  satırların `phone`'unu `NULL`'a çeviriyor.
- `middleware.go:41-49` — `Scan()` hatasını sadece `err == sql.ErrNoRows`
  için kontrol ediyor; `phone=NULL` → tip hatası → `err != nil` ama
  `ErrNoRows` değil → kontrol atlanıyor → `user` sıfır-değer struct'ta
  kalıyor (`IsActive=false`) → yanlışlıkla "askıya alınmıştır" 403.

Etki: restart öncesi var olan (henüz migrate edilmemiş) her kullanıcı, bir
sonraki restart'ta bu bug'a çarpar — gerçek suspension DEĞİL, bir DB-scan
hatasının yanlış sınıflandırılması. Doğrulama, restart-sonrası hesabı
ÖNCEDEN hesaplanmış deterministik DID (`auth.GenerateDID` = sha256(identity
_key) ilk 16 byte) ile restart ÖNCESİNDE `OBSCURA_ADMIN_DIDS`'e koyup,
kaydı restart SONRASI (migration bir daha çalışmadan) yaparak bypass edildi.

**Fix B9 kapsamı dışı (marketplace şeması değil, core auth) — kullanıcıya
raporlanacak, düzeltilmedi.** Olası düzeltme: `COALESCE(phone,'')` +
middleware'e `else if err != nil { respond(w,500,...) ; return }` bloğu.

## Temizlik durumu

`cmd/b9check`, `cmd/b9tierbump2`, `cmd/b9fund` (geçici doğrulama
helper'ları) silindi. Dev backend süreci durduruldu. `git status` temiz
(sadece Parça 3'ün gerçek diff'i kaldı, stage edilmedi).

## Devam (yeni oturum, kesinti sonrası) — hepsi TAMAMLANDI

1. **Parça 3 commit+push edildi** (`27770a4`) — yukarıdaki durum aynen kod.

2. **Middleware bug'ı kullanıcıya raporlandı**, kullanıcı kararı: Parça 1'den
   ÖNCE düzelt, ayrı iş/ayrı commit. Fix (`a250a3b`, `backend/internal/api/
   middleware.go:34,41-49`): SELECT'e `COALESCE(phone,'')` + `Scan` hatası
   `ErrNoRows` dışında da artık 500'e düşüyor (sessiz sıfır-değer
   fallthrough kapatıldı). İki yönlü regresyon testi
   (`auth_middleware_phone_test.go`, internal/api `subscriber`'ı import
   edemediği için — `layer_boundary_test.go` — `ensurePhoneNullable` +
   `MigratePhoneToSubscriberStore`'un NULL/phone_migrated adımları yerelde
   tekrarlandı, mock değil):
   a. phone NULL + is_active=1 → 200 (fix'siz aynı test 403 ile FAIL —
      regresyon gerçek bug'ı yakalıyor).
   b. is_active=0 (gerçek suspension) → hâlâ 403.
   Yol boyunca `+905559994001` telefon numarası çakışması bulundu/düzeltildi
   (başka test dosyasıyla), 2 masum test daha kurtarıldı. `go build/vet/test
   ./...` (tüm backend) temiz.

3. **Parça 1 tamamlandı** (`354a8b5`). Kontrol sonucu: `TransactionInfo`'ya
   `dispute_id` eklemek migration/backfill/dispute-yaratma-yolu değişikliği
   GEREKTİRMEDİ — query-time subquery yeterli (disputes tablosu zaten
   transaction_id kolonuna sahip; OpenDispute sadece "held" transaction'ı
   kabul eder, resolveHeld sadece released/refunded'a götürür → disputed bir
   transaction asla held'e dönmez → en fazla bir dispute/transaction garanti,
   backfill'e gerek yok). `marketplace.go`: `TransactionInfo.DisputeID`,
   `loadTransaction` + `ListTransactionsForUser` ikisi de güncellendi. Web:
   `app/marketplace/orders/[id]/page.tsx`'teki `obscura_mp_dispute_for_tx_*`
   localStorage cache mekanizması TAMAMEN kaldırıldı, `txn.dispute_id`
   kullanılıyor. Kanıt: yeni backend testleri
   (`transaction_dispute_id_test.go`) + canlı doğrulama (localhost:8099,
   gerçek register+DB satırı, mock değil): `GET /v1/marketplace/
   transactions/{id}` buyer VE seller token'ıyla aynı `dispute_id`'yi
   döndürüyor, `GET .../disputes/{dispute_id}` eşleşiyor.

4. **Master-Liste.md B9 satırı kapatıldı** (`[x]`, üç parça + yan-bulgu fix
   commit hash'leriyle listelendi), "Şu an" özet satırı güncellendi.

## Sonuç

B9'un üç parçası + öncelik verilen yan-bulgu (auth 403 bug'ı) hepsi
commit+push edildi: `27770a4`, `a250a3b`, `354a8b5` (+ bu oturumun ilk
commit'i `fb161ff` önceki oturumdan). BFT/A-blok/MLS'e dokunulmadı. Mock
data yok — her parça gerçek backend'e karşı canlı doğrulandı.
