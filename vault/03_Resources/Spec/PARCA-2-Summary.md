# PARÇA 2 — Mesajlaşma, Kredi, Token, Client (Bölüm 6-9)

**Tam metin:** [[full/PARCA-2-mesajlasma-kredi-token-client|PARCA-2 raw]]

## Bölüm 6 — Mesajlaşma (satır 521-634)

**Mesaj tipleri:** Text 4KB / Image 10MB / Voice 50MB / File 100MB / Location / Call invite/accept/end / Group invite / ZK proof

**6.2 Birebir akışı (Signal):** X3DH + Double Ratchet ✅
**6.3 Grup akışı (MLS):** Welcome/Commit/Add/Remove — tam akış ✅ (Bölüm 6.3 her satırı openmls wrapper'da)
**6.4 Mesaj durumları:** sending/sent/delivered/read/failed/expired
**6.5 Offline:** Shard storage + push + senkron — push ✅ (FCM/APNs), shard ❌ FAZ 3
**6.6 Silme:** TTL 30 gün + recall + ZK silme kanıtı

## Bölüm 7 — Kredi (satır 635-779)

**Bölüm 7.1 — Davranış matrisi (12 maddey):**
| Davranış | Etki | Frekans | ZK Circuit |
|---|---|---|---|
| Hesap yaşı | +1/ay | +24 max | age_proof ❌ |
| Günlük giriş | +0.5 | +15 | activity_proof ❌ |
| Mesaj | +0.1 | +10 | msg_count_proof ❌ |
| Sesli arama | +0.2 | +5 | call_proof ❌ |
| Grup oluşturma | +2 | +10 | group_proof ❌ |
| Spam (alma) | -5 | -50 | spam_victim_proof ❌ |
| Spam (yanlış verme) | -3 | -30 | spam_false_proof ❌ |
| Dolandırıcılık | -20 | -100 | fraud_proof ❌ |
| Topluluk katkı | +5 | +25 | contribution_proof ❌ |
| Node çalıştırma | +10/ay | +60 | node_proof ❌ |
| Onay | +1 | +20 | endorsement_proof ❌ |
| İyi davranış streak | +2/7gün | +20 | streak_proof ❌ |

**Bunlardan hiçbiri henüz circuit olarak yazılmadı.** Şu an credit score Go'da hesaplanıp `credit_threshold` ile tier upgrade ediliyor.

**Bölüm 7.2 — Tier ayrıcalıkları:**
| Tier | Puan | Yetki |
|---|---|---|
| Bronz | 0-59 | 1-1 mesaj, 1-1 arama 5dk, 50 msg/gün |
| Gümüş | 60-69 | + 50 kişilik Signal grup, limitsiz arama, 100 kişi MLS, 200 msg/gün |
| Altın | 70-79 | + 500 kişi MLS, dosya 50MB, mini app kullan, OBS wallet, 1000 msg/gün |
| Platin | 80-89 | + 5000 kişi MLS, 50 grup görüntülü, mini app oluştur, governance vote, staking |
| Elmas | 90-100 | + Limitsiz MLS, governance veto 1/5, revenue share |

**Bölüm 7.3 — ZK proof akış 6 adım:** ✅ tamamen çalışıyor (credit_upgrade.go)

## Bölüm 8 — OBS Token (satır 776-866)

**Token:** 1B arz, %40 topluluk / %20 ekip / %15 yatırımcı / %15 ekosistem / %10 rezerv
**Enflasyon:** %2/yıl, %50 fee burn
**Privacy:** zk-Rollup ile gizli transfer
**Status:** ✅ ADR-0010 + transparent ledger + shielded (FAZ 2)

**Bölüm 8.2 — zk-Rollup karşılaştırma:** StarkNet (Cairo) vs zkSync Era (Solidity) vs **Aztec (Noir, native privacy — SEÇİLDİ ADR-0005)**

**Bölüm 8.3 — Gizli transfer:** ✅ shielded.go (FAZ 2 simplified, FAZ 3'te tam Merkle)

**Bölüm 8.5 — Staking:** Min 1000 OBS user, 10000 OBS node, 30 gün lock, APY 5-15%, slash kötü davranış. ✅ staking.go

**Bölüm 8.6 — Fee tablosu:**
| İşlem | Ücret | ZK |
|---|---|---|
| Mesaj | 0 | Hayır |
| Dosya | 0.01 OBS | Hayır |
| Mini app deploy | 10 OBS | Hayır |
| Gizli transfer | 0.05 OBS | Evet |
| Stake | 0.1 OBS | Hayır |
| Vote | 0.01 OBS | Evet |
| Tier yükseltme | 0 | Evet |

## Bölüm 9 — Client (satır 869-1010)

**Spec önerisi:** Flutter 3.19+
**Obscura sapması:** Next.js (web) + Expo (mobile) + Tauri 2.x (desktop) — ADR-0002 kalıcı sapma
- Spec mantığı: tek codebase, flutter_rust_bridge ile Rust FFI
- Sapma sebebi: TS ekosistemi tanıdık, libsignal-protocol-typescript browser'da çalışıyor

**Bölüm 9.6 — WebRTC:** STUN/TURN (coturn), SRTP ✅
**Bölüm 9.7 — Platform min versiyonları:** iOS 14+, Android 8+, Web Chrome 90+
