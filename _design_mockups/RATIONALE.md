# Obscura — Tasarım Yönü Önerisi

## Yön Adı: **Faceted Vigilance** (Fasetli Tetikte-Duruş)

Logo tek bir şey söylüyor: keskin, geometrik, tetikte bir avcı. Şu anki UI ise
yuvarlak köşeli, düz, "her uygulama gibi". Önerdiğim yön bu çelişkiyi kapatıyor —
ürünün ZK/gözcü kimliğini logonun faset dilinden türetiyorum.

**Somut kararlar:**

- **Köşe dili — çift kanal.** Logo yumuşak köşe barındırmıyor; ama parmak dokunan
  her yer keskin köşe olamaz (ergonomi). Çözüm: *kimlik yüzeyleri* (hero bakiye
  kartı, avatar, gönder butonu, sabitlenen sohbet) `clip-path` ile pah kırılmış
  (chamfered) keskin köşe alır — logoyu yankılar. *Etkileşim satırları/baloncuklar*
  mevcut `radius` ölçeğini korur. Böylece keskinlik dekoratif değil, hiyerarşik.

- **Tipografi eşleşmesi.** Gövde: sistem sans (mevcut). Ama tüm *makine verisi*
  (saat, DID, fingerprint, tutar, ZK etiketleri) `ui-monospace`'e taşındı. Bu
  "surveillance/terminal" tonu katıyor ve marka rengi olmadan karakter üretiyor.
  Başlıklar `-0.02em` sıkı tracking + 800 ağırlıkla ölçek kontrastını keskinleştiriyor.

- **Derinlik tekniği — üç katmanlı.** (1) Mevcut yüzey merdiveni void→ground→surface→raised
  gerçek z-ekseni olarak kullanıldı (gölge + gradyan). (2) Kimlik yüzeylerde 135°
  faset parıltısı (logonun elmas göz highlight'ının yankısı). (3) Marka yeşili SADECE
  semantik: "güvenli/doğrulanmış/aktif" durumda border-glow. Dekoratif yeşil yok.

- **Ritim.** Tekdüze padding kırıldı: sabitlenen sohbet ve bakiye kartı nefes alan
  büyük bloklar; işlem/mesaj satırları sıkı. Bento aksiyon grid'i eşit değil (1 geniş
  + 2 dar).

- **Motion notu (statik mockup'ta yok):** Yeni mesaj baloncuğu pah kırık köşeden
  "açılmalı" (scaleX origin köşe). Claw status ikonu sent→delivered→read geçişinde
  çizgiler ∧ formuna *dönmeli* (rotate tween), tik-atma değil. Aksan glow sadece
  aktif/online'da nabız atar.

## Nitelik kontrolü (hedeflenen ≥4, karşılanan)

1. ✅ Ölçek kontrastıyla hiyerarşi (42px bakiye vs 10px mono meta)
2. ✅ Kasıtlı ritim (bento asimetrisi, sabitlenen vs düz satır)
3. ✅ Derinlik/katman (yüzey merdiveni + faset parıltı + gölge)
4. ✅ Karakterli tipografi eşleşmesi (sans gövde + mono makine-veri)
5. ✅ Renk semantiği (yeşil yalnızca güvenli/aktif; mor=shielded; amber=self-destruct)
6. ✅ Tasarlanmış aktif/focus (sabitlenen accent hairline, online pulse, tab underline glow)
7. ✅ Editoryal/bento kompozisyon (cüzdan aksiyon grid'i)

## Dosyalar

| Dosya | İçerik |
|-------|--------|
| `0_logo_variations.html` | 3 SVG varyant: outline · mikro/favicon · tek-vuruş damga |
| `1_chat_list.html` | Sohbet listesi — sabitlenen faset kart + düz satırlar, claw status |
| `2_chat_conversation.html` | Mesajlaşma — faset baloncuklar, ZK banner, claw StatusIcon, self-destruct |
| `3_profile_or_wallet.html` | Cüzdan — faset hero bakiye + bento aksiyon + shielded/tx listesi |

## Kısıt uyumu

- Palet değişmedi — yalnızca mevcut tokenlar (#4ade80 tek aksan). Nötr gri/mono tonları eklendi.
- "Obscura" ismi ve logo formu korundu; yeni sembol icat edilmedi.
- Kod yok — yalnızca self-contained HTML/CSS mockup (harici CDN/font yok).
- Mockup'lar gerçek ekran içeriğine benzer (gerçek DID/tier/shielded/self-destruct semantiği).
