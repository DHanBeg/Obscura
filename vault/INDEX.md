# Obscura Vault — Index

Knowledge base for the **Obscura** project. Open this folder in Obsidian to navigate visually.

## Hızlı Erişim

- 🗺️ [[01_Projects/Obscura/README|Obscura Projesi (aktif)]]
- 📜 [[03_Resources/Spec/Index|Spec v3.0 — bölüm rehberi]]
- 🏛️ [[03_Resources/ADRs/Index|ADR kayıtları]]
- 📅 [[01_Projects/Obscura/Sessions/Index|Oturum logları]]
- 🔐 [[02_Areas/Security/Index|Güvenlik denetim sonuçları]]
- 📊 [[01_Projects/Obscura/Phase-Status|FAZ ilerleme dashboard]]

## Klasör Mantığı (PARA)

- `00_Inbox/` — fikir / not / link kayıt — sonra işlenir
- `01_Projects/Obscura/` — aktif faz işi (FAZ 1 ✅, FAZ 2 implementation ✅, audit pending)
- `02_Areas/` — sürekli alanlar: Backend, Crypto, Frontend, Mobile, Desktop, Security, DevOps
- `03_Resources/` — spec, ADR, referans dokümanlar (Signal, MLS, openmls, Aztec, Circom)
- `04_Archive/` — tamamlanmış fazlar, eski tasarımlar
- `05_Attachments/` — diagrams, ekran görüntüleri
- `06_Metadata/` — şablonlar (ADR, session log)

## Claude Code ile Kullanım

Bu vault, kod repo'sunun **bilgi katmanı**. Kod `E:\obscura\` altında, vault `E:\obscura\vault\` altında. İkisi aynı git repo'da.

Her oturum başında:
1. `E:\obscura\CLAUDE.md` (zorunlu okuma)
2. `E:\obscura\vault\01_Projects\Obscura\Phase-Status.md` (mevcut durum)
3. Çalışacağın domain — `02_Areas/<Backend|Crypto|...>/Index.md`

Yeni karar verirken: `06_Metadata/Templates/ADR.md` şablonu, `docs/adr/NNNN-*.md`'ye yaz.
Oturum sonunda: `06_Metadata/Templates/Session.md` şablonu, `docs/sessions/YYYY-MM-DD.md`'ye yaz, bu vault'ta da link bırak.
