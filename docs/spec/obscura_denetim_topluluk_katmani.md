# OBSCURA — DENETİM VE TOPLULUK KATMANI

## Tasarım Dokümanı v1.0

**Tarih:** 2026-07-07
**Kapsam:** Bu doküman yalnızca Denetim ve Topluluk Katmanı'nı kapsar. Mevcut ve tamamlanmış olan subscriber store (kayıt katmanı) ve sealed-sender (mesaj gizliliği) bu dokümanın kapsamı dışındadır — onlar bitmiş ve testlerle kilitlenmiştir. Bu katman onların **üstüne** gelir.

**Amaç cümlesi:** Bu katman, insanları güvenle bir araya getirmek ve birbirine zarar vermelerini engellemek için vardır. İnsanları "iyi olmaya" zorlamaz; sadece somut zararı (spam, dolandırıcılık, taciz, sahtekârlık) engeller.

---

## 0. TEMEL İLKELER (Bütün tasarımın anayasası)

Bu altı ilke, aşağıdaki her mekanizmanın üstünde durur. Herhangi bir özellik bu ilkelerle çelişiyorsa, özellik değil ilke kazanır.

1. **İki alan kesin ayrıdır.**
   - *Özel alan* (birebir mesaj, arama, davetle girilen özel gruplar): Tam E2E, denetimsiz. Operatör dahil kimse okuyamaz. Sistem buraya asla bakmaz.
   - *Kamusal alan* (herkese açık kanallar, marketplace ilanları, yayınlanan içerik): Moderasyon meşru, çünkü zaten açık, gizlilik iddiası yok.
2. **Ahlak değil, davranış denetlenir.** Sistem "toplumu bozar mı", "ahlaksız mı" gibi öznel yargı vermez. Yalnızca somut, ölçülebilir, kültürden bağımsız ihlallere bakar: spam, dolandırıcılık, taciz/tehdit, telif ihlali, yasadışı satış, çocuk güvenliği.
3. **Kural şeffaftır, gizli değildir.** Kredi/güven sistemi, ceza kademeleri, ihlal listesi — hepsi kullanıcıya açıktır. Gizli puanlama yoktur. İnsan neden kısıtlandığını bilirse düzelir; bilmezse yalnızca dışlanmış hisseder. (Ayrıca gizli otomatik profilleme birçok yargı bölgesinde yasadışıdır.)
4. **Güven varsayılandır.** Yeni gelen temiz sayfayla başlar. Geçmiş sorgulanmaz. Güven yalnızca ihlalle kaybedilir. (Anadolu'nun "Tanrı misafiri" ilkesi.)
5. **Sistem hakim değil, ön eleyicidir.** AI/otomasyon içeriği *işaretler*; ciddi cezalarda kararı insan (operatör/kurul) verir. Yalnızca bariz spam otomatik işlenir. Bu hem yanlış-pozitiflerden korur hem "algoritma haksız yere sildi" suçlamasından.
6. **Operatör veri tutmadığı yerde sorumluluk taşımaz.** Konum, içerik gibi hassas veriler mümkün olan her yerde operatörde *toplanmaz*, kullanıcının kendi güven ağına bırakılır. Tutulmayan veri sızamaz.

---

## 1. KAMUSAL İÇERİK TARAMA MOTORU

### 1.1 Ne taranır, ne taranmaz

- **Taranır:** Herkese açık kanallar, marketplace ilanları, herkese açık paylaşımlar/yayınlanan içerik.
- **Taranmaz:** Birebir mesaj, arama, davetle girilen özel gruplar.
- **Sınır kuralı:** "Kamusal" = gerçekten herkese açık. Davetle girilen grup, kaç kişilik olursa olsun, özeldir. Bu çizgi net çekilir; bulanık bırakılmaz.

### 1.2 İhlal kategorileri (kapalı liste — bu liste dışına çıkılmaz)

- Spam (tekrar eden içerik, kitlesel istenmeyen mesaj/link)
- Dolandırıcılık / scam
- Taciz / tehdit
- Telif ihlali
- Yasadışı satış
- Çocuk güvenliği (yasal zorunluluk — hash tabanlı eşleme şart)

Not: "Ahlak", "kültür bozma", "uygunsuzluk" gibi öznel kategoriler bu listede **yoktur** ve eklenmez.

### 1.3 İçerik türüne göre teknik yaklaşım

- **Metin** (kanal mesajı, ilan): Yerel model (Ollama/Mistral) ile sınıflandırma. Ucuz, hızlı, mevcut altyapıya oturur. **Birincil çözüm.**
- **Görsel:** Hash tabanlı bilinen-kötü-içerik eşleme (çocuk güvenliği için PhotoDNA benzeri — yasal zorunluluk) + NSFW/ihlal sınıflandırıcı.
- **Video:** En pahalı. Kare örnekleme + ses transkripsiyon + metin analizi. **Başta ertelenir**; önce metin ve görsel çözülür.

### 1.4 Mimari (Umay deseni)

`monitor → brain → notify` iskeleti aynen kullanılır:

- **monitor:** Kamusal içerik akışını dinler.
- **brain:** Yerel model (Mistral) ile sınıflandırır; kararsız kalırsa cloud (Groq) fallback. **Maliyet ilkesi:** yerel birincil, cloud yalnızca kararsızlıkta — yoksa moderasyon projenin en pahalı parçası olur.
- **notify:** Eşik aşan içeriği işaretler, insan inceleme kuyruğuna atar. Otomatik silme yalnızca bariz spam için.

---

## 2. ŞİKAYET AKIŞI VE KANIT DOĞRULAMA

### 2.1 Temel mantık

Özel alan taranmaz. Ama özel alanda bir ihlal olduysa, sistem bakmaz — **mağdur kendi rızasıyla kanıt getirir.** Sistem yalnızca sunulan kanıtı inceler. (Anadolu köy adaleti: kimse dinlenmez, ama biri "işte kanıt" derse ona bakılır.)

### 2.2 Kanıt formatı

- Her şikayet ekran görüntüsü + anlatım ile gelir. Kanıtsız şikayet işleme alınmaz.

### 2.3 AÇIK PROBLEM — Kanıt doğrulama (çözülmesi gereken mühendislik işi)

Ekran görüntüsü kolayca sahtelenebilir. "Karşı tarafta doğrulama" tek başına yetmez (suçlu onaylamaz). **Gerçek çözüm yönü:** mesajların kriptografik imzalı hash'i.

- Her mesaj gönderilirken imza taşımalı.
- Şikayet, o imzayı/hash'i kanıt gösterir.
- Sistem "bu mesaj gerçekten bu kişiden mi çıkmış" diye imzayı doğrular — **içeriği okumadan.**
- **YAPILACAK:** Mevcut mimaride mesajların bu şekilde imzalı saklanıp saklanmadığı kontrol edilecek. Saklanmıyorsa, imzalı hash mekanizması bu katmanın ön koşuludur.

---

## 3. KADEMELİ YAPTIRIM (ceza sistemi)

**İsim ve ruh:** Bu "süründürme" değil, "kademeli düzeltme fırsatı"dır. Amaç ezmek değil, "bir daha yapma" deyip yol vermek (Anadolu kültürü). Aynı mekanik, farklı ruh.

### 3.1 Kademeler

- 1. ihlal → uyarı
- 2. ihlal → 7 gün kısıt
- 3. ihlal → 30 gün kısıt
- Ağır ihlal (dolandırıcılık, taciz) → doğrudan üst basamak (ara kademeleri atlar)

### 3.2 Şeffaflık

Kullanıcı hangi kademede olduğunu, neden orada olduğunu görür. Gizli değildir.

---

## 4. YALAN ŞİKAYET YAPTIRIMI (şikayeti silah olmaktan çıkarma)

Şikayet mekanizması tek yönlü silah olmamalı; kötüye kullanımı cezalandırılmalı.

- Şikayet incelenir. Kanıt geçerliyse → şikayet edilen kademeli yaptırıma girer.
- Kanıt sahte/asılsız çıkarsa → **şikayet eden aynı kademeli yaptırıma girer.** Yalan şikayet, ihlaldir.
- Aynı kişiyi tekrar tekrar asılsız şikayet eden → "taciz" kategorisi, doğrudan üst basamak.
- **Brigading koruması:** Kısa sürede bir kişiye toplu şikayet gelirse → sistem işaretler, **otomatik ceza uygulanmaz**, elle/kurul inceleme kuyruğuna alınır.
- **Şikayetçi güvenilirliği:** Şikayet sonucu (haklı/haksız) şikayet edenin geçmişine yazılır. Sürekli yanlış şikayet edenin şikayetleri zamanla daha düşük ağırlık taşır.

---

## 5. KREDİ / GÜVEN KATMANI (şeffaf haliyle)

### 5.1 Felsefe

Kredi, insanları "iyi insan" yapmaya zorlamaz; kötüye karşı güvenli bir topluluk kurar. Puan **açıktır** (İlke 3). "Süründürme" değil, kilit-açma ve düzeltme fırsatı sistemidir.

### 5.2 Katmanlar (kilit-açma mantığı)

> NOT: Aşağıdaki basamak/eşik değerleri kullanıcının ilk taslağından alınmıştır. Bunlar **açık** olacaktır. Değerler kod aşamasında ince ayara tabidir.

- **Katman 1 (temel):** Yeni kullanıcı. Kısıtlı grup kurma, sosyalleşme özellikleri (arkadaş bul, konu arama, çevredeki kullanıcılarla buluşma — hepsi opt-in). Yükselmiş kullanıcıların açtığı mekânlar (kafe/restoran) ve indirim buluşmaları bu katmanda görünür.
- **Katman 2 (sağlıklı kullanıcı):** Kendi kanal/topluluk/grup/bot/mini-uygulama kurabilme. Uygulama içi ekonomi ve token/coin transferlerine erişim. Kamusal içerik üreticileri sürekli (kamusal) denetime tabidir.
- **Katman 3 (işletme/marketplace):** İşletme ekleme, satış, reklam. Genç girişimcilere görünürlük desteği.

### 5.3 KRİTİK DÜZELTME — Güven varsayılanı (İlke 4)

Kullanıcının ilk taslağındaki "-10'a düşünce sil, dönünce -7 ile başlat" fikri **güven varsayılanı ilkesiyle çeliştiği için değiştirildi.** Yeni gelen temiz sayfayla başlar. (Ban kaçırma ayrı bir problem — bkz. Bölüm 7.)

### 5.4 Renk uyumu (sosyalleşme)

Buluşma/arkadaş bulma özelliğinde kullanıcılar arası uyum yeşil/sarı/kırmızı ile gösterilebilir. **Kısıt:** Bu bir "iyi/kötü insan" damgası değil, yalnızca eşleşme/uyum göstergesidir ve kullanıcının kendi puanı kendine görünür kalır; başkasının ham puanı ifşa edilmez.

---

## 6. PANİK BUTONU VE BULUŞMA GÜVENLİĞİ

**Tasarım ilkesi (İlke 6): Operatör konum verisi TUTMAZ.** Güvenlik sağlanır ama veri merkezde toplanmaz. Kullanım şartına madde ekleyip sorumluluktan kaçmak **geçersizdir** — konum tutan taraf, sızıntıdan sorumludur.

### 6.1 Mekanizma

- **Kaba konum:** Buluşma/keşif özelliği yalnızca semt/1km-grid gösterir. Sokak/nokta konum asla.
- **Panik butonu:** Konumu operatöre değil, kullanıcının **önceden seçtiği güven kişisine** (aile/arkadaş) gönderir. Operatör aracı bile olmaz; doğrudan gider.
- **Buluşma onayı:** "Buluştum, iyiyim" onayı kullanıcının güven kişisine gider; **operatörde loglanmaz.**
- **Sonuç:** Operatör güvenliği *sağlar* ama veriyi *tutmaz*. (Tinder vb. bu yöne gidiyor: konumu kullanıcının kendi güven ağına bırakmak.)

---

## 7. BAN KAÇIRMA (açıkça kabul edilen kısmi çözüm)

Silinen/kısıtlanan kullanıcının numara değiştirip dönmesi tam çözülemez; yalnızca zorlaştırılır. Üç katman birlikte caydırıcıdır:

1. **SMS + arama doğrulaması:** Sanal numaraların çoğunu eler (hepsini değil — bazı servisler sanal numaraya arama da iletir).
2. **Cihaz kimliği (parmak izi):** Aynı cihazdan ikinci hesap / ban sonrası yeni numarayla dönüş yakalanır.
3. **Davet zinciri (kefalet):** Yeni gelen, mevcut kullanıcının davetiyle girer. Davet eden bir nebze kefildir. Davet edilen kötü çıkıp silinirse, davet edenin de güveni düşer — böylece kimse rastgele davet etmez. (Anadolu kefalet kültürü.)

**Kabul:** Bu %100 çözüm değildir. Bilerek kabul edilen bir kısmi caydırıcılıktır.

---

## 8. TOKEN EKONOMİSİ — AÇIK RİSK (kod öncesi çözülmeli)

### 8.1 Mevcut durum

Token: katkı → kazanç → görünürlük/güçlendirme için harcama. Kazanılan token, yenen şikayetleri/spam geçmişini **silemez** (doğru ilke, korunur). Token yetmezse (56+ kredi ile) piyasadan OBS coin alıp token'a çevirme.

### 8.2 ÇÖZÜLMEMİŞ AÇIK — Sybil direnci

Sahte hesap çiftliği (100 sahte hesap birbirine katkı yaptırıp token basma) bütün "kazanılmış görünürlük" ekonomisini çökertebilir. **Marketplace kurulmadan önce Sybil direnci çözülmeli.** Bölüm 7'deki cihaz kimliği + davet zinciri buna kısmen yardım eder ama tek başına yetmeyebilir. **Bu, kod aşamasından önce netleştirilecek açık bir tasarım boşluğudur.**

### 8.3 Piyasa manipülasyonu uyarısı

Kullanıcının "ben senin reklamını öne çıkarırım, sen indirim yap" fikri (operatörün piyasayı elle yönetmesi) ölçeklenmez ve manipülasyona açıktır. Marketplace kuralları **şeffaf ve otomatik** olmalı, operatörün günlük insafına bağlı olmamalı.

---

## 9. SÖZLEŞME / ŞEFFAFLIK

- Gizli sözleşme maddesi **yoktur** (geçersizdir; ifşa olunca güveni bitirir).
- İki katmanlı sunum: üstte insanın anlayacağı ~5 maddelik sade özet ("mesajını okumuyoruz, konumunu tutmuyoruz, şunu yaparsan kısıtlanırsın"), altında tam yasal metin.
- İlke: **sadeleştir, gizleme.**

---

## 10. ANAHTAR YÖNETİMİ (ertelenmeyecek temel karar)

Subscriber store'un AES-GCM anahtarı/pepper'ı bütün gizliliğin dayandığı tek nokta. Ertelenmez.

- **Şimdi konan ilke:** Anahtar repo'da **olmayacak**, yalnızca production ortamında bulunacak, operatör dahil kimse loglamayacak.
- Rotation (döndürme) ve erişim detayları sonra, ama bu ilke şimdi geçerli.

---

## 11. TOPOLOJİ NOTU (federasyon)

- 5 genesis node = çekirdek donanım katmanı. Harici node'lar üstüne eklenir.
- **Uyarı:** Donanım dağıtıklığı ≠ yetki dağıtıklığı. Genesis node'lar hâlâ kritik yetkiye (protokol güncelleme, node onayı) sahipse, kontrol merkezi kalır. Donanım dağıtık olsa bile yetki merkezi olabilir — ikisi ayrı şeydir. İleride netleştirilecek.

---

## 12. AÇIK KALAN İŞLER (kod öncesi karar gerektirenler)

1. **Kanıt doğrulama:** Mesajlar imzalı hash ile saklanıyor mu? Saklanmıyorsa bu, şikayet sisteminin ön koşulu (Bölüm 2.3).
2. **Sybil direnci:** Token ekonomisi + marketplace öncesi çözülmeli (Bölüm 8.2).
3. **Kredi eşik değerleri:** Basamak sınırları (51, 85, vb.) kod aşamasında ince ayar.
4. **Marketplace kuralları:** Şeffaf/otomatik kural seti tasarlanmalı (Bölüm 8.3).

---

## 13. KOD OTURUMLARINA BÖLÜNME (token disiplini)

Tek gece = bir büyük parça. Önerilen sıra:

1. **Oturum 1:** Şikayet akışı + yalan-şikayet yaptırımı + kademeli ceza (Bölüm 2,3,4). Çekirdek adalet. — Ama önce Bölüm 2.3 (imzalı hash) kontrol edilmeli.
2. **Oturum 2:** Kamusal tarama motoru (Bölüm 1). Umay deseni, yerel model birincil.
3. **Oturum 3:** Kredi/güven katmanı + kilit-açma (Bölüm 5).
4. **Oturum 4:** Panik butonu / buluşma güvenliği (Bölüm 6).
5. **Sybil + marketplace + token:** Ayrı oturum(lar), Bölüm 8 açıkları çözüldükten sonra.

### Model dağılımı (her oturumda)

- **Fable (tek, sıralı, paralel değil):** Yalnızca kripto/güvenlik-kritik iş (imzalı hash doğrulama, kanıt bütünlüğü).
- **Opus (paralel olabilir):** CRUD, şema, iş mantığı, test, docs, tarama motoru boru hattı, UI.
- **Kural:** Fable darboğaz gibi az kullanılır; mekanik yük Opus'a yıkılır. Her subagent bitince kendi çıktısını test eder. Plan onaylanmadan tek satır kod yazılmaz.

---

*Bu doküman bir referanstır, kod değildir. Kararları tek yerde toplar, çelişkileri çözer. Kod oturumları bu dokümandan parça parça beslenir.*
