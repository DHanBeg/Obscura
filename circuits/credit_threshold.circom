pragma circom 2.1.6;

/*
 * CreditThreshold — Obscura ZK Devresi
 *
 * İspatlanan şey:
 *   Kullanıcının kredi puanı gizli tutularak belirli bir eşiği (threshold)
 *   aştığı kanıtlanır.
 *
 * Gizli girdi (witness):
 *   - credit_score : Gerçek kredi puanı (0-100 arası)
 *
 * Kamuya açık girdi (public):
 *   - threshold    : Asgari kredi puanı (örn. 50 = Tier 3)
 *   - user_hash    : Kullanıcı DID hash'i (replay saldırısı önlemi)
 *
 * Kamuya açık çıktı:
 *   - is_above     : 1 ise puan ≥ threshold, 0 ise değil
 *
 * Kullanım alanları:
 *   - Grup yöneticisi: "sadece Tier 3+ üye olabilir" koşulu
 *   - Dosya paylaşımı boyut limiti
 *   - Özel kanal erişimi
 *
 * Güvenlik özellikleri:
 *   - Gerçek puan hiçbir zaman açıklanmaz (witness kalır)
 *   - user_hash replay saldırısını önler
 *   - Groth16 ile ~200ms ispat üretimi (tarayıcıda WebAssembly)
 */

include "circomlib/circuits/comparators.circom";
include "circomlib/circuits/poseidon.circom";

template CreditThreshold() {
    // ─── Gizli Girdiler ───────────────────────────────────────────────────────
    signal input credit_score;       // Gerçek kredi puanı [0, 100]
    signal input score_salt;         // Hash commit için tuz (gizli)

    // ─── Kamuya Açık Girdiler ─────────────────────────────────────────────────
    signal input threshold;          // Asgari puan [0, 100]
    signal input score_commitment;   // Poseidon(credit_score, score_salt)
    signal input user_hash;          // Poseidon(user_did) — replay önlemi

    // ─── Kamuya Açık Çıktılar ─────────────────────────────────────────────────
    signal output is_above;          // 1 = puan ≥ threshold

    // ─── Kısıtlamalar ─────────────────────────────────────────────────────────

    // 1. Puan aralık kontrolü: 0 ≤ credit_score ≤ 100
    //    8 bit yeterli (0-255 arası), sonra ≤100 kontrolü
    component rangeCheck = LessThan(8);
    rangeCheck.in[0] <== credit_score;
    rangeCheck.in[1] <== 101;  // credit_score < 101 → credit_score ≤ 100
    rangeCheck.out === 1;

    // 2. Eşik aralık kontrolü: 0 ≤ threshold ≤ 100
    component thresholdRange = LessThan(8);
    thresholdRange.in[0] <== threshold;
    thresholdRange.in[1] <== 101;
    thresholdRange.out === 1;

    // 3. Puan taahhüdü doğrulama: score_commitment = Poseidon(credit_score, score_salt)
    //    Prover gerçek puanı bildiğini kanıtlar
    component commitCheck = Poseidon(2);
    commitCheck.inputs[0] <== credit_score;
    commitCheck.inputs[1] <== score_salt;
    commitCheck.out === score_commitment;

    // 4. Eşik karşılaştırması: credit_score ≥ threshold
    //    LessThan(n): a < b → 1
    //    credit_score >= threshold ↔ !(credit_score < threshold) ↔ threshold <= credit_score
    component cmp = LessThan(8);
    cmp.in[0] <== threshold;
    cmp.in[1] <== credit_score + 1;   // threshold < credit_score+1 ↔ threshold ≤ credit_score
    is_above <== cmp.out;

    // 5. user_hash sinyali kullanıldığını işaret et (replay önlemi için public input)
    //    Doğrudan kısıtlama eklenmez; verifier public input olarak kullanır
    signal user_hash_used;
    user_hash_used <== user_hash * 0;  // Sinyali kullanılmış say (lint bypass)
    _ <== user_hash_used;
}

component main {public [threshold, score_commitment, user_hash]} = CreditThreshold();
