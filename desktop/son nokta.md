# Obscura — Son Nokta (2026-06-21)

## Nerede kaldık
Desktop client (Tauri v2) başlatılmaya çalışıldı → hata: `dlltool.exe: program not found`

## Neden hata
Rust, Chocolatey üzerinden **GNU toolchain** (`x86_64-pc-windows-gnu`) olarak kurulu.
GNU toolchain MinGW'den `dlltool.exe` bekler ama MSYS2 / MinGW yüklü değil.

## Çözüm (bir sonraki oturumda)
Admin PowerShell aç, tek komut:
```
choco install mingw -y
```
Sonra:
```
cd E:\obscura\desktop
npx tauri dev
```
Frontend (port 3003) + backend (port 8090) ayrı terminallerde çalışıyor olmalı önce.

---

## Bu oturumda yapılanlar

### Backend / Başlatma
- `start-backend.ps1` — Go backend 8090 portunda, dev modda başlatır
- `start-frontend.ps1` — Next.js 3003 portunda başlatır
- `start-desktop.ps1` — `npx tauri dev` çalıştırır
- `start-mobile.ps1` — Expo (android/ios/web/tunnel/device parametreli)
- `build-desktop.ps1` — Tauri production build pipeline

### Login sayfası (`frontend/app/login/page.tsx`)
- Telefon alanı placeholder kaldırıldı (boş)
- Dev modda OTP otomatik doldurulup doğrulanıyor (`res.dev_otp`)
- `doVerify` / `sendOTP` sırası düzeltildi (initialization hatası fix)

### Alt navigasyon (`frontend/components/GravityWell.tsx`)
14 item → 6 item:
- Sohbetler, Aramalar, Finans, Yönetim, Uygulamalar, Ayarlar
- Yönetim ikonu: `Building2` (Vote ikonu checkbox gibi görünüyordu)

### Cüzdan sayfası (`frontend/app/wallet/page.tsx`)
Finance tab bar eklendi: Cüzdan / Staking / Bridge / DAO

### Ayarlar sayfası (`frontend/app/settings/page.tsx`)
- Shimmer skeleton (null user için "Yükleniyor..." yerine)
- Etkinlikler menü item eklendi

### Profil düzenleme (`frontend/app/settings/profile/page.tsx`)
Telegram tarzı tam yeniden tasarım:
- Hero gradient + 88px avatar + kamera butonu
- Status badge (tap ile 3 durum döngüsü)
- `EditRow` bileşeni — borderless input, alt çizgi
- Bio karakter sayacı (n/70)
- Save butonu opacity fix: `style={{ opacity: saving || !isDirty ? 0.35 : 1 }}`

### Desktop client (`desktop/src-tauri/tauri.conf.json`)
- devUrl: `localhost:3003`
- frontendDist: `../../frontend/out`
- CSP: port 8090

### Mobile client
- `mobile/lib/api.ts`: port 8090, Android emulator `10.0.2.2`
- `mobile/app.json`: apiUrl + wsUrl eklendi

---

## Bekleyen
- [ ] `choco install mingw -y` (admin) → Tauri build çalışacak
- [ ] Staking / Bridge / DAO sayfalarına finance tab bar eklenmesi
