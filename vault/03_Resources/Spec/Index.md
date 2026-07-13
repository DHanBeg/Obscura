# Obscura Spec v3.0 — Bölüm Rehberi

**Tam spec:** `E:\obscura\docs\spec\obscura_spec_v3.txt` (101 KB, 2026-04-26)

**Vault içi parçalanmış:**
- [[PARCA-1-Summary|PARÇA 1 özet (Mimari/Çekirdek/Network/Kripto/Kimlik)]] · [[full/PARCA-1-mimari-cekirdek-network-kripto-kimlik|raw]]
- [[PARCA-2-Summary|PARÇA 2 özet (Mesajlaşma/Kredi/Token/Client)]] · [[full/PARCA-2-mesajlasma-kredi-token-client|raw]]
- [[PARCA-3-Summary|PARÇA 3 özet (Mini App/Fiziksel/Fazlar)]] · [[full/PARCA-3-miniapp-fiziksel-fazlar|raw]]
- [[PARCA-4-Summary|PARÇA 4 özet (Eksikler/API/Diller/Test)]] · [[full/PARCA-4-eksikler-api-diller-test|raw]]
- [[PARCA-5-Summary|PARÇA 5 özet (ZK Circuits/Deployment/Güvenlik)]] · [[full/PARCA-5-zk-circuits-deployment-guvenlik|raw]]
- [[../Claude-Code-Practices/README|Claude Code Practices — kullanıcının ilk rehberi]]
- [[obscura_denetim_topluluk_katmani|Denetim ve Topluluk Katmanı — tasarım dokümanı v1.0 (2026-07-07)]] — subscriber store + sealed-sender'ın üstüne gelen katman, henüz kod yazılmadı

## PARÇA 1 (Bölüm 1-5) — Mimari, Çekirdek, Network, Kripto, Kimlik

- 1: Protocol-first + Zero-Knowledge first felsefe
- 2.1: Node mimarisi — 5 modul (Network/Storage/Crypto/Consensus/ZK)
- 2.2: Node tipleri (Bootstrap/Relay/Storage/ZK Prover)
- 3: Network topolojisi, mesaj yönlendirme, discovery
- 4.1-4.4: Şifreleme yığını, anahtar yönetimi, mesaj akışları, ZK akış
- **4.5: 7 KESIN güvenlik kuralı** (sürekli başvur)
- 5.1-5.5: 3-katmanlı kimlik, kayıt akışı, giriş, **cross-signing**, kredi puanı

## PARÇA 2 (Bölüm 6-9) — Mesajlaşma, Kredi, Token, Client

- 6.1: Mesaj tipleri (text, image, voice, file, location, call_invite, group_invite, zk_proof)
- 6.2: 1-1 Signal akışı
- **6.3: MLS grup akışı** (TreeKEM, Welcome/Commit, üye ekleme/çıkarma)
- 6.4: Mesaj durumları
- 6.5: Offline mesaj yönetimi
- 6.6: Mesaj geçmişi + silme
- **7.1: Kredi puanı matrisi** (12 davranış × etki × frekans × ZK circuit)
- 7.2: Tier ayrıcalıkları (Bronz/Gümüş/Altın/Platin/Elmas)
- 7.3: ZK proof üretim akışı (6 adım)
- 7.4: Sybil koruması
- 8.1: OBS token özellikleri
- 8.2: zk-Rollup mimarisi (Aztec/zkSync/StarkNet karşılaştırma)
- 8.3: Gizli transfer akışı
- 8.4: Cüzdan yapısı
- 8.5: Staking + ödüller
- 8.6: İşlem ücretleri
- 9: Client spec (Flutter — biz sapma yaptık, ADR-0002)

## PARÇA 3 (Bölüm 10-12) — Mini App, Fiziksel, Fazlar

- 10.1-10.5: Mini app motoru, API bridge, izin sistemi, tier kısıtları
- 11.1-11.4: Etkinlik, konum keşfi, QR, NFC
- **12.1-12.4: 4 FAZ deliverable listesi** (en sık başvurulan)
- 12.5: Yol haritası özet

## PARÇA 4 (Bölüm 13-16) — Dış Servisler, API'ler, Diller, Test

- 13.1-13.8: SMS, TURN, Push, S3, DNS, Monitoring, CI/CD, ZK altyapı kurulum detay
- 14.1-14.6: Backend Go, Crypto Rust, ZK katmanı, Client, Blockchain, AI/ML, Mini App dil seçimleri
- **15.1: Test piramidi** (Unit %80 / Integration / E2E)
- **15.2: Performans hedefleri** (mesaj <100ms yerel, ZK proof <3s, MLS encrypt <100ms @ 1000)
- 15.3: Güvenlik testleri (pen test, bug bounty, formal verification)

## PARÇA 5 (Bölüm 17-20) — ZK Circuit Kodları, Deployment, Güvenlik, Sonuç

- 17.1-17.5: 5 örnek circuit kodu (identity, credit, token_balance, vote, storage)
- 18: Deployment scripts
- EK A: Protobuf mesaj formatı (Envelope, SignalMessage, MLSMessage, ZKProof)
- EK B: API endpoint listesi (hedef — `/v1/...`)

## Sık başvurulan sayfalar

- Bölüm 4.5 — 7 güvenlik kuralı
- Bölüm 7.2 — tier ayrıcalıkları
- Bölüm 12.1-12.4 — faz deliverable listesi
- Bölüm 15.2 — performans hedefleri
- EK A + EK B — wire format + API
