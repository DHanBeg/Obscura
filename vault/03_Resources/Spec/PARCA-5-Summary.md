# PARÇA 5 — ZK Circuit Kodları, Deployment, Güvenlik (Bölüm 17-20)

**Tam metin:** [[full/PARCA-5-zk-circuits-deployment-guvenlik|PARCA-5 raw]]

## Bölüm 17 — ZK Circuit Kodları (Circom)

Spec'in örnek devreleri. Hepsi Obscura'da implement edildi:

| # | Spec circuit | Bölüm | Obscura |
|---|---|---|---|
| 17.1 | identity_proof.circom | DID ownership | ✅ 487 constraints |
| 17.2 | credit_threshold.circom | Puan eşik | ✅ 270 constraints (v2 user_hash binding fix) |
| 17.3 | token_balance.circom | Gizli bakiye + nullifier | ✅ 944 constraints |
| 17.4 | vote_proof.circom | Gizli oy + voter Merkle | ✅ 733 constraints |
| 17.5 | storage_proof.circom | Node retention | ✅ 310 constraints (proof_commitment binding) |

**Bonus Obscura circuit'i (spec'te yok ama eklendi):**
- `message_integrity.circom` — anonim grup mesajı, 487 constraints

**Spec'te tanımlı ama implement edilmemiş (FAZ 2-3):**
- age_proof, activity_proof, msg_count_proof, call_proof, group_proof
- spam_victim_proof, spam_false_proof, fraud_proof
- contribution_proof, node_proof, endorsement_proof, streak_proof
- location_proof (FAZ 4, GPS+ZK)
- recursive proof (FAZ 3)

## Bölüm 18 — Deployment Scripts

Spec içinde `obscura-node-setup.sh` örnek script var — Ubuntu 22.04 hedef, Go/Rust/Circom/snarkJS kurulum + obscura repo klonlama + circuit derleme + Docker.

**Obscura:** `docker-compose.yml` ile lokal stack çalışır. Production deployment runbook ❌ (FAZ 1 GA için yazılacak).

## Bölüm 19 — Güvenlik (spec'te explicit)

Spec'in güvenlik bölümü (gördüğüm kadarıyla) Bölüm 4.5'in 7 KESIN kuralı + Bölüm 15.3 test listesi etrafında dönüyor. ZK circuit audit + multi-party ceremony en kritik. Side-channel attack testleri (özellikle proof üretim sırasında timing) FAZ 1 GA için zorunlu.

## Bölüm 20 — Sonuç

Spec'in son sözü:
> "Bu doküman Obscura platformunun eksiksiz teknik spesifikasyonudur (v3.0). Bu dokümanı okuyan geliştirici: çekirdek sistemi kurabilir, ZK altyapısını kurabilir, client üretebilir, dış servisleri tanımlayabilir, fazları takip ederek platformu hayata geçirebilir. Eksikler listesi (Bölüm 13) tamamlandığında platform çalışır durumda olur."

**Obscura geliştirici notu:** FAZ 1 + FAZ 2 implementation tamamen spec'e uyumlu (kabul edilmiş sapmalar ADR'larda). Bölüm 13 eksikleri (SMS, FCM, SSL, DNS, monitoring) FAZ 1 GA için yapılacak.

**Doküman bilgisi:** v3.0-FINAL, 2026-04-26, YarlikHan + AI Assistant, ~50 sayfa, ~2500 satır (gerçek dosya 3678 satır).
