# Obscura Multi-Party Trusted Setup Ceremony

> **Status:** PROTOCOL DEFINED — production ceremony pending coordinator nomination.
>
> **Spec reference:** `docs/spec/obscura_spec_v3.txt` Bölüm 19.3 (Trusted Setup), ADR-0006 (Dev Setup → Prod Ceremony).
>
> **Mevcut durum:** Tüm circuit'lerin `_final.zkey` dosyaları DEV ceremony (tek katılımcı) ile üretildi. Production GA öncesi bu ceremony multi-party olarak yeniden koşulmalı.

---

## Neden Multi-Party Setup?

Groth16 SNARK'ları, her circuit için bir *trusted setup* gerektirir. Setup sırasında üretilen "toxic waste" (rastgele τ skaleri) bir kişide kalırsa, o kişi geçersiz kanıt üretebilir.

**Çözüm:** N katılımcı sırayla setup'a katkı verir. Her katılımcı kendi entropy'sini ekler ve toxic waste'i siler. Eğer en az 1 katılımcı dürüstse, setup güvenlidir ("1-of-N honest").

**Hedef katılımcı:** ≥ 10 (spec Bölüm 19.3).
**Coğrafi dağılım:** ≥ 3 kıta (politik bağımsızlık).
**Donanım çeşitliliği:** ≥ 3 farklı CPU mimarisi (Intel x86, AMD x86, ARM).

---

## Ceremony Faz Akışı

```
Phase 1 — Powers of Tau (universal, circuit-agnostic)
  ├── 1.1 Coordinator init      → bash phase1_coordinator.sh init
  ├── 1.2 Participant 1..N      → bash phase1_participant.sh <i> <prev.ptau>
  ├── 1.3 Coordinator beacon    → bash phase1_coordinator.sh beacon
  └── 1.4 Prepare phase 2       → bash phase1_coordinator.sh finalize

Phase 2 — Circuit-Specific Setup (her circuit için ayrı)
  ├── 2.1 Initial zkey          → bash phase2_circuit.sh init <circuit>
  ├── 2.2 Participant 1..N      → bash phase2_circuit.sh contribute <circuit> <i>
  ├── 2.3 Beacon                → bash phase2_circuit.sh beacon <circuit>
  └── 2.4 Verification key      → bash phase2_circuit.sh finalize <circuit>

Verification
  └── bash verify_ceremony.sh   → her transcript'i bağımsız doğrula
```

---

## Katılımcı Olmak için Checklist

1. **Donanım izolasyonu:** Yeni format edilmiş bir laptop / air-gapped makine.
2. **OS:** Ubuntu LTS canlı USB tercih edilir (silinebilir).
3. **Entropy kaynakları (en az 3):**
   - `/dev/urandom` (system)
   - Klavye/fare entropy (kullanıcı hareketleri)
   - Çevresel: web kamera, mikrofon ham veri
   - Donanım RNG: YubiKey, RNG kartı, kuantum RNG bulutu
4. **Video kayıt:** Tüm ceremony oturumu kaydedilir (bash session + ekran + ortam).
5. **Transcript:** Çıktı `participants.json` içine git commit'lenir.
6. **Tanıklar:** En az 2 bağımsız tanık katılımcının session'unu uzaktan canlı izler.
7. **İmha:** Ceremony bittikten sonra disk şifreli silme (`shred -vfz -n 7`).

Bkz: `participants.json` — kayıt formatı.

---

## Güvenlik Sınırları

- Katılımcı `_0001.ptau` dosyasını **siler**, sadece çıktıyı (`_NNNN.ptau`) sunucuya yükler.
- Coordinator (Obscura Foundation) **kendi entropy'sini ekleyemez** Phase 1 başlangıçtan ayrı (sadece koordinasyon).
- Final beacon: kamuya açık random kaynak (örn. Bitcoin block hash + NIST randomness beacon) — manipüle edilemez.
- Vkey hash'ları on-chain (Aztec contract) yayımlanır → ileride değişen vkey reddedilir.

---

## Dosya Yapısı

```
ceremony/
├── README.md                  ← bu dosya
├── participants.json          ← katılımcı kayıt formu + iletişim
├── phase1_coordinator.sh      ← koordinatör scripti (init, beacon, finalize)
├── phase1_participant.sh      ← katılımcı scripti (entropy contribute)
├── phase2_circuit.sh          ← circuit-specific phase 2 (her devre)
└── verify_ceremony.sh         ← bağımsız doğrulama scripti
```

`ceremony/output/` (gitignored) altında transcript dosyaları toplanır:
```
output/
├── pot14_0000.ptau           ← coordinator init
├── pot14_0001.ptau           ← participant 1
├── pot14_0002.ptau           ← participant 2
├── ...
├── pot14_beacon.ptau         ← beacon contribute
├── pot14_final.ptau          ← prepare phase 2 (universal)
└── <circuit>/                ← her circuit için phase 2 transcript
    ├── 0000.zkey
    ├── 0001.zkey
    ├── ...
    ├── beacon.zkey
    └── final.zkey
```

---

## Sıkça Sorulan

**S: Mevcut DEV ceremony zkey'leri geçerli mi?**
C: Hayır. DEV setup tek katılımcılı (Obscura Foundation), production için 1-of-N güvenlik garantisi yok. Yeni vkey'ler dağıtıldığında istemciler güncellenir.

**S: Ceremony ne kadar sürer?**
C: Phase 1 BN254 power 14 için katılımcı başına ~10-15 dk. Phase 2 her circuit ~2-5 dk. 10 katılımcı + 7 circuit ≈ 4-6 saat dağıtık (asenkron).

**S: Hangi snarkjs sürümü?**
C: `snarkjs@0.7.x` — `circuits/package.json` ile aynı versiyon, deterministik build için.

---

## Audit Trail

Her katkı, `participants.json` içinde aşağıdaki alanlarla loglanır:
- participant_id, name, country, hardware, entropy_sources
- input_hash (önceki ptau SHA-256)
- output_hash (kendi ptau çıktısının SHA-256)
- attestation_signature (Ed25519 ile imzalı)
- video_url (ceremony oturum kaydı)
- witnesses[] (en az 2 tanık DID)

Doğrulama: `bash verify_ceremony.sh` — tüm hash'leri zincirde sırayla kontrol eder, anormal contribute reddedilir.

---

**Coordinator iletişim:** TBA (Obscura Foundation ceremony lead nominasyonu sonrası).
**Etik:** [Zcash Powers of Tau ceremony](https://z.cash/technology/paramgen.html) referans alındı.
