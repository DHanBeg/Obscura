# PARÇA 1 — Mimari, Çekirdek, Network, Kripto, Kimlik (Bölüm 1-5)

**Tam metin:** [[full/PARCA-1-mimari-cekirdek-network-kripto-kimlik|PARCA-1 raw]]
**Boyut:** 514 satır

## Bölüm 1 — Felsefe (satır 11-62)

- **Protocol-first:** Backend protokol sabit, client herhangi bir dilde
- **Zero-Knowledge first:** Kullanıcı verisi cihazda, sunucu sadece ZK proof doğrular
- 5 katman: Transport / Framing / E2EE (Signal+MLS) / **ZK Layer** / App Logic
- 5 ZK katman: Kimlik (ZK-ID), Kredi (ZK-Proof), Token (zk-Rollup), İçerik (ZK-ML), Yönetim (ZK-Vote)

## Bölüm 2 — Çekirdek (satır 63-242)

**Node 5 modul:**
- MODUL A (Go): libp2p, DHT, QUIC, GossipSub, MLS
- MODUL B (Go+Rust): Shard storage, Reed-Solomon 4/6, ZK storage proof
- MODUL C (Rust): Signal + MLS + ZK proof üretim (`obscura-crypto` crate)
- MODUL D (Go): Multisig, ZK governance vote
- MODUL E (Rust): ZK circuit'ler (Circom/Noir)

**Node tipleri:** Bootstrap / Relay / Storage / ZK Prover

**Status (Obscura):**
- libp2p ❌ (HTTP gossip — ADR-0003 sapma)
- Shard storage ❌ (FAZ 3'e)
- Rust crypto crate 🟡 (MLS ✅, Signal ❌ Go'da — ADR-0004)
- Multisig + ZK vote ✅ (governance)
- 6 circuit ✅

## Bölüm 3 — Network (satır 243-293)

Senaryo 1: Aynı node <10ms
Senaryo 2: Farklı node <100ms yerel, <300ms küresel
Senaryo 3: Offline alıcı → storage shard + push
Senaryo 4 (YENI): MLS grup mesaj broadcast

**Discovery:** Bootstrap DNS + IPFS + cache + ENS fallback

## Bölüm 4 — Kriptografi (satır 294-380)

- **Bölüm 4.5 — 7 KESIN KURAL:** Private key sunucuya gitmez / Rust'ta şifrele / Hiçbir node tam mesaj çözmez / Metadata minimum / 30 gün sonra sil / ZK proof detay açıklamaz / Audit edilmiş circuit
- X3DH + Double Ratchet (Signal) ✅
- MLS KeyPackage 90 gün rotasyon (Bölüm 4.2) — kod var, cron yok
- ZK kredi puanı akışı (Bölüm 4.4) — credit_upgrade.go'da yapıldı

## Bölüm 5 — Kimlik (satır 381-507)

**3 katman kimlik:**
- Login: +90XXXX (telefon)
- Display: @kullaniciadi
- Protocol: did:obs:sha256(pubkey)
- ZK-ID: zk:obs:proof (gizli)

**Kayıt akışı 7 adım** (Bölüm 5.2):
1. Telefon
2. SMS OTP
3. TOTP üretimi
4. Kimlik oluşturma (Ed25519 + DID + PreKey + ZK-ID secret)
5. ZK-ID kaydı (identity_proof)
6. Sunucu kaydı (POST /v1/register)
7. **12 kelime mnemonic** + social recovery

**Status:** Tüm akış ✅ (BIP39 mnemonic crypto/src/mnemonic.rs)

**Bölüm 5.4 — Cross-signing:**
- Birincil (Ed25519) / İkincil (cross-signed) / Kurtarma (timelock + email)
- **Status:** ✅ cross_signing.go (audit C5 fix sonrası)

**Bölüm 5.5 — Kredi puanı (ZK tabanlı):**
- Başlangıç 20-100 (Sybil)
- 6 davranış matrisi (yaş, aktivite, spam, dolandırıcılık, katkı, node)
- 5 tier (Bronz 0-59 / Gümüş 60-69 / Altın 70-79 / Platin 80-89 / Elmas 90-100)
- Her tier yükseltme ZK proof gerektirir
- **Status:** ✅ credit_upgrade.go + ZK binding
