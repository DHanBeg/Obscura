---
proje: Obscura
tip: tasarim-referansi
olusturuldu: 2026-08-26
durum: onaylandı (yön), inşa: tema turu (C13), işlev oturunca
etiketler: [obscura, tasarim, apple-design, tema]
---

# Obscura — Arayüz Yönü (Onaylı Referans)

> **KURAL:** Her Obscura tasarım işinde `apple-design` skill'i kullanılır.
> Kaynak: github.com/emilkowalski/skills → skill: apple-design
> (Emil Kowalski — Sonner/Vaul/animations.dev. Apple "Designing Fluid
> Interfaces" prensiplerini web'e çevirir.) CC tarafında npx skills add ile
> kurulmalı; chat tarafında referans çekilerek uygulanır.

## Onaylı yön (mockup: Obscura-Arayuz-Yonu.html)
- Apple karanlık-mod mantığı, katmanlı grafit: #0A0A0C → #141417 → #1D1D21 → #26262B
- Törensel altın aksan #C9A24B — CİMRİ (aktif sekme, mühür, bakiye, birincil aksiyon)
- İşlevsel yeşil #30D158 — SADECE durum (çevrimiçi/şifreli), marka değil
- SF Pro (UI) + SF Mono (kriptografik veri: DID, adres, parmak izi, hash)
- Vibrancy nav/tabbar/composer (blur 24px saturate 140%)
- **İmza öğe — MÜHÜR/tamga:** gizlilik mimarisini GÖRÜNÜR kılar. Her sohbette
  altın kilit = E2E. Ayrı "Mühür" ekranı: DID + anahtar parmak izi + verini
  taşıyan node'lar. Tez'in kanıtı, pazarlaması değil. Göktürk/Orhun kökü.
- Kaynak: tokens.ts (tek kaynak) — hex/px sabiti hiçbir bileşende yok

## Dört referans ekran (mockup'ta)
Sohbetler (mühürlü liste) · Topluluk kanalı (üstte sabit şifre satırı, metin/
ses/arama tek yerde) · Cüzdan (altın=bakiye, mono=adres/emanet, köprü ilk sıra)
· Mühür (DID/anahtar/node — tezin görünür hali)

---

## C13 — Tema yayma turu (launch-öncesi cila)
- [ ] tokens.ts altın/grafit temasını + yukarıdaki mockup'ı TÜM eski ekranlara
      yay: sohbet listesi, 1:1 sohbet, ayarlar, profil, cüzdan (şu an sadece
      yeni ekranlarda: marketplace, grup türleri).
- **ŞART:** apple-design skill'i kullanılır, mockup birebir hedef.
- **ZAMANLAMA:** işlev oturunca (B kalanı + C). Eski ekranların ALTINDA bozuk
  mantık var (WS auth, dead button geçmişi) — tema yaymadan önce işlev sağlam
  olmalı, yoksa "güzel ama bozuk ekran". CC tema-turuna girerken Demir'e HABER
  VERİLİR (yeni tasarımla gidildiği bildirilecek).

## D4 — Logo/tema özelleştirme (vizyon, launch-sonrası)
Kullanıcı logo/tema özelleştirebilir AMA "modlu WhatsApp" kaosu olmadan.
İlke: **YEREL + KOZMETİK serbest, PAYLAŞILAN/KOD yasak.**
- [ ] **App-icon + kişisel tema:** SERBEST — sadece kendi cihazı, kendi gördüğü.
      Kimseyi etkilemez.
- [ ] **Kendi avatarı (başkaları görür):** serbest AMA moderasyon-yüzeyi —
      yükleme + rapor mekanizması (uygunsuz avatar bildirimi).
- [ ] **Resmi Obscura mührü/doğrulama işareti:** TAKLİT-KORUMALI (kod). Kullanıcı
      kendi icon'unu değiştirebilir ama "resmi Obscura" mührünü avatar yapıp
      hesap taklidi YAPAMAZ.
- [ ] **Kod/protokol/kripto:** ASLA özelleştirilemez (anti-modlu-APK). Özelleştirme
      SADECE asset katmanı (resim/renk), asla kod. Resmi build imzalı;
      değiştirilmiş build "resmi Obscura" diye görünemez.
- **Gerekçe:** Özelleştirme başkalarının GÖRDÜĞÜ bir güven-yüzeyine dokunursa
  (mühür taklidi, kod değişimi) trustless/E2E garantileri kırılır. Yerel+kozmetik
  kalırsa güvenli.

---

**Bağlantılı:** [[Master-Liste]] (C13 + D4 buradan da izlenir, kanonik durum
oradaki checkbox'lar — bu dosya tasarım gerekçesi/referansı, Master-Liste ilerleme
kaynağı).
