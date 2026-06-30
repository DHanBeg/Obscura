# Obscura — Devam Notu (29 Haziran 2026 — Gece)

## Son Versiyon
- **APK: v1.4.7 (versionCode 23)** — Firebase'e dağıtıldı ✅
- Backend Railway'de canlı (migrations 098–107 uygulandı)

---

## Yapılacaklar (Yarın)

### 1. Davet Linki
- Grup / kanal / topluluk için link oluşturma (`obscura://join/{convId}`)
- Backend: `GET /v1/conversations/{id}/invite` → token üretir
- Backend: `POST /v1/conversations/join` → token ile katılım
- Mobile: chat header'da "Davet Linki Paylaş" butonu
- Mobile: deep link handler (`_layout.tsx`)

### 2. Grup / Kanal / Topluluk Keşif Ekranı
- Backend: `GET /v1/conversations/discover?q=...` (is_public=1 olanlar)
- Mobile: yeni `discover.tsx` ekranı — arama + katıl butonu
- GravityWell FAB aksiyonlarına "Keşfet" eklenebilir

### 3. Sohbet İçi Fotoğraf Gönderme
- chat/[id].tsx'te kamera/galeri ikonuna basınca `uploadMedia` çağır
- Yüklenen URL'yi `msg_type: "image"` ile gönder
- Mesaj listesinde image render et

### 4. Font Size Tercihi Uygulamaya Yansıtma
- SecureStore'dan `obscura_pref_font_size` oku
- Zustand store'a `fontSize` state ekle
- chat/[id].tsx mesaj metni + diğer yerlerde `fontSize` kullan

---

## Önemli Bilgiler

| Şey | Değer |
|-----|-------|
| Backend URL | `https://obscura-backend-production-1827.up.railway.app` |
| Firebase App ID | `1:275166414136:android:f87d8954fa4769e8382dcd` |
| JDK | `D:\tools\jdk\jdk-17.0.11+9` |
| Android SDK | `D:\tools\android-sdk` |
| Keystore | `D:\tools\obscura-release.jks` |
| APK çıktı | `E:\obscura\mobile\android\app\build\outputs\apk\release\` |

### Build komutu:
```bash
JAVA_HOME="/d/tools/jdk/jdk-17.0.11+9" /e/obscura/mobile/android/gradlew assembleRelease --no-build-cache --project-dir /e/obscura/mobile/android
```

### Firebase dağıtım:
```bash
export PATH="/d/tools/npm-global:$PATH"
firebase appdistribution:distribute \
  "/e/obscura/mobile/android/app/build/outputs/apk/release/app-release.apk" \
  --app "1:275166414136:android:f87d8954fa4769e8382dcd" \
  --testers "emirhankarabulut112@gmail.com,yaafeveran@gmail.com,daniel.kratoss6666@gmail.com,er4n.akyldzz@gmail.com" \
  --release-notes "vX.X.X: ..."
```

### Testerlar:
- emirhankarabulut112@gmail.com
- yaafeveran@gmail.com
- daniel.kratoss6666@gmail.com
- er4n.akyldzz@gmail.com
