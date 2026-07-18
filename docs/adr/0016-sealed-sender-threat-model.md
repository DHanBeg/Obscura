# ADR 0016: Sealed-sender tehdit modeli netleştirmesi

Date: 2026-07-17
Status: Accepted
Decider: project lead
Spec ref: Madde 15 (sealed-sender), Adım 6-7 denetimi sırasında bulundu

## Context

Madde 15 Adım 7 (yetkilendirme kontrollerinin sealed mesajlara taşınması)
üzerinde çalışırken şu yanlış varsayım ortaya çıktı: "sealed-sender sunucunun
göndereni bilmesini engeller" (Signal'in "unidentified sender" / anonim
gönderim modeliyle karıştırılmış).

`backend/internal/api/handlers.go` — `HandleSendMessage` incelemesi:

- Route `RequireAuth` middleware'inin ardında çalışır → `getUser(r)` çağrısı
  JWT'den `user.DID`'i **istek işlenirken** çözer.
- Sealed mesajlarda (`req.IsSealedSender()`) sunucu bu `user.DID`'i **DB'ye
  yazmıyor** (`storedFromDID = ""`, satır 728-731) ve **WS payload'ına
  eklemiyor** (satır 780-789).
- Yani sunucu, isteği o an işlerken göndereni gerçekten biliyor — sadece bu
  bilgiyi kalıcı hale getirmemeyi ve başka taraflara (alıcı, relay,
  DB dump/backup) sızdırmamayı seçiyor.

Bu, Signal'in gerçek "sealed sender" tasarımından farklı: Signal'de gönderim
*anonim erişim token'ı* ile yapılır, sunucu istek anında bile göndereni
tanımlamaz. Obscura'da gönderim her zaman kimliği doğrulanmış (JWT) bir
istektir — sealed-sender burada yalnızca **kalıcılaştırma ve iletim**
katmanında devreye giriyor.

## Decision

Kanonik çerçeveleme:

> **YANLIŞ:** "Sunucu göndereni bilmez."
> **DOĞRU:** "Sunucu gönderen kimliğini kalıcı olarak saklamaz ve alıcıya/
> üçüncü taraflara iletmez."

Sealed-sender'ın gerçekte koruduğu şeyler:
- DB dump / yedek / mahkeme celbi ile `messages` tablosundan gönderen kimliği
  çıkarılamaz (from_did boş).
- Alıcının client'ı gönderen kimliğini sunucu beyanından değil kendi
  kriptografik `Unseal()`'ından öğrenir.
- WS relay'i dinleyen başka bir süreç/log gönderen kimliğini göremez.

Sealed-sender'ın koru**mad**ığı şey:
- Canlı sunucu sürecine (veya sunucuyu işleten operatöre) karşı — istek
  anında `user.DID` zaten JWT'den elde ediliyor, sealed olsun olmasın aynı.
  Buna karşı tek gerçek koruma uçtan uca şifrelemenin kendisi (içerik için)
  — kimlik için böyle bir uçtan uca "sunucudan gizli" mekanizma yok, çünkü
  gönderim protokolü baştan kimlik doğrulamalı.

UI/kod yorumu kuralı: hiçbir metin "sunucu bilmez/göremez" (gönderen kimliği
için) yazmamalı. Doğru ifade: "sunucu kalıcı saklamaz / iletmez."

Düzeltilen dosyalar (bu ADR ile aynı tarihte):
- `mobile/lib/e2e.ts:56-61` — yorum düzeltildi.
- `mobile/lib/sealed-sender.ts:8-19` — zarf-seviyesi iddia ile istek-seviyesi
  gerçeklik arasındaki fark netleştirildi.
- `mobile/lib/panic.ts:7-11` — bayat "sealed-sender bağlı değil" notu
  güncellendi (Adım 5'te bağlandı; panik özelinde `encryption_type: "sealed"`
  hiç gönderilmediği için panik mesajları zaten sealed yoluna girmiyor).

Kullanıcıya görünen ekranlarda (`privacy.tsx`, `panic.ts` içindeki
`PANIC_PRIVACY_NOTE`) bu yanlış iddia bulunmadı — mevcut metinler zaten
sadece **içerik** gizliliği iddia ediyor, gönderen kimliği hakkında yanlış
beyan yok. `panic.test.ts` bunu zaten regresyona karşı kilitliyor.

## Consequences

Madde 15 Adım 7 (yetkilendirme taşıma) tasarımı bu çerçeveye göre revize
edildi: sunucu göndereni istek anında zaten meşru şekilde bildiğinden, delete/
recall/status endpoint'leri için yeni bir kriptografik "kanıt" mekanizmasına
(imza tabanlı, Adım 8 kapsamı) gerek yok — sunucu-içi, dışa asla sızmayan bir
owner-bağlama alanı yeterli ve yeterince gerekçeli.

**Uygulanan çözüm (Adım 7, bu ADR'den sonraki turda karara bağlandı):**
`messages.owner_hash = HMAC-SHA256(pepper, DID+":"+msgID)` — gönderim
anında hesaplanır, sealed mesajlarda saklanır (bkz.
`backend/internal/api/owner_hash.go`). Delete/recall/status handler'ları
`fromDID == ""` olduğunda (sealed) `owner_hash`'i yeniden hesaplayıp
`hmac.Equal` ile karşılaştırır; eski (zarfsız) mesajlarda mevcut
`fromDID != user.DID` yolu AYNEN kalır. Pepper `OBSCURA_MESSAGE_OWNER_PEPPER`
env değişkeninden yüklenir — `OBSCURA_PHONE_PEPPER`'dan bilinçli olarak
AYRI (alan sızıntıları birbirini etkilemesin, rotasyon bağımsız kalsın),
`subscriber.PepperFromEnv()` ile aynı desen: prod'da eksikse FATAL, dev'de
uyarı + fallback.

msgID'nin HMAC girdisine dahil edilmesi kasıtlı: aynı gönderenin farklı
mesajları farklı `owner_hash` üretir, bu yüzden bir DB dump'a bakan biri
"bu mesajlar aynı kişiden" diye KÜMELEYEMEZ bile (korelasyon direnci —
`TestSealedOwnerHashDiffersAcrossMessages` bunu kilitliyor).

**Artık risk (kabul edildi, şimdi ele alınmadı):** Pepper sızarsa VE
saldırgan hedef mesajın konuşmasındaki üye listesini (`conv_members`) de
biliyorsa, küçük aday kümesini (1:1 sohbette 2 kişi) tek tek deneyip
`owner_hash`'i kırabilir. Bu, karşılaştırılabilir/exact-match çalışması
gereken HER şemanın matematiksel sınırı — subscriber store'un `phone_hash`'i
de aynı sınıf riski taşıyor (bkz. `subscriber/phone_hash.go` + ek
`EncryptField` zarf-şifreleme katmanı, tam da bu senaryoyu bir kademe
zorlaştırmak için var). owner_hash için eşdeğer bir ikinci-anahtar zarf
şifreleme katmanı **şimdi eklenmedi** — Adım 7'nin kapsamı "karşılaştırılabilir
ama okunamaz + korelasyonsuz" bar'ını HMAC+pepper tek başına karşılıyor,
ek katman gelecekte ayrı bir sertleştirme adımı (post-Faz sertleştirme
listesi) olarak değerlendirilmeli, bugünkü tasarımı bloke etmemeli.

## Adım 10 eki: kademeli geçiş

**Sunucu zaten dual-format:** `SendMessageRequest.EffectiveEncryptionType()`
(`models.go`) eski/yeni format ayrımını client'ın `encryption_type` alanı
gönderip göndermemesine göre yapıyor — göndermeyen "signal" varsayılanına
düşüyor. Bu ayrımdan BAĞIMSIZ, ek bir "sealed zorunlu" reddetme yolu YOKTU;
Adım 10 bunu operatörün elle açacağı bir anahtar olarak ekledi
(`backend/internal/api/sealed_policy.go`,
`OBSCURA_SEALED_SENDER_REQUIRED`, varsayılan **kapalı**). Açıldığında grup
mesajları muaf — sealed zarfı tek-alıcı X25519 DH'ına dayanır, grup
fanout'una mimari olarak uygulanamaz.

**Önemli bulgu — "karışık dönem" bugün fiilen YOK:** `mobile/lib/e2e.ts`
`sealAndEncryptMessage`/`receiveMessage` fonksiyonları (Adım 1-9'da
inşa edildi, test edildi) gerçek sohbet ekranına
(`mobile/app/(main)/chat/[id].tsx`) HENÜZ BAĞLANMADI — ekran hâlâ eski
`encryptMessage`/`decryptMessage`'ı çağırıyor (grep ile doğrulandı, tek
kullanım yeri `e2e.ts`'in kendisi ve testler). Yani bugün prod'da (v1.4.7
dahil) HİÇBİR client sealed göndermiyor — "biri sealed gönderirken karşı
taraf eski client'sa" senaryosu şu an tetiklenemez. Bu durum, UI kablolama
(ayrı, henüz yapılmamış bir adım) gerçekleşene kadar geçerli.

**UI kablolandığında (gelecek) beklenen davranış — belgelendi, tam
çözülmedi:** Gönderen client karşı tarafın sealed'ı çözebileceğini ÖNCEDEN
bilmiyor (kapasite anlaşması yok). Bir NEW client bir OLD client'a sealed
gönderirse:
- Mesaj KAYBOLMAZ — sunucu her zamanki gibi kalıcı yazar (`messages` satırı,
  30 gün TTL, WS/push iletimi) — sealed olması saklama/iletim davranışını
  değiştirmiyor.
- OLD client zarfı (rastgele bayt/base64) `JSON.parse` edemez, ne v1/v2
  yolunu ne de (henüz sahip olmadığı) `unseal()` yolunu deneyebilir →
  mevcut "çözülemeyen mesaj" yedek davranışına düşer. Bu YENİ bir hata
  sınıfı değil — Double Ratchet'in zaten sahip olduğu (oturum dışı/atlanmış
  anahtar) çözülemez-mesaj durumuyla aynı kategori.
- Kalıcı çözüm (gönderenin alıcının sealed desteğini önceden bilmesi) bir
  kapasite-anlaşması özelliği gerektirir (ör. `users` tablosunda
  `supports_sealed_sender` veya client build numarası bazlı sinyal) — bu,
  UI kablolama adımının kapsamına girmeli, Adım 10'un değil. Bilinçli
  ertelendi.

**Test kapsamı (`sealed_policy_test.go`, `sealed_send_test.go`):** eski
mesaj hâlâ gidiyor, sealed mesaj çalışıyor, anahtar kapalıyken ikisi bir
arada, anahtar açıkken eski 1:1 mesaj reddediliyor, grup mesajı her koşulda
muaf.

## Related

- `backend/internal/api/handlers.go` (HandleSendMessage, satır 719-789)
- `backend/internal/api/sealed_policy.go` (Adım 10)
- Adım 6 hub.go denetimi (typing/call sinyalleri, kalıcı değil — ayrı konu)
