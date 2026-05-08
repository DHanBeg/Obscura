pragma circom 2.1.6;

/*
 * Obscura — Credit Threshold Proof Circuit
 *
 * Kanıt: "Kredi puanım belirli bir eşiğin üzerinde, ama tam değeri açıklamıyorum"
 *
 * Kullanım örneği:
 *   Tier 3 (Altın) gerektiren bir özellik için:
 *   - threshold = 70 (public)
 *   - actual_score = 85 (private, asla açıklanmaz)
 *   - Kanıt: score ≥ 70
 *
 * Private inputs:
 *   - actual_score:  Gerçek kredi puanı (0-120 arası, negatif için offset+20)
 *   - salt:          Commitment için tuz (bağlanabilirlik önleme)
 *
 * Public inputs:
 *   - threshold:     Eşik değer (-20 ile 100 arası, +20 offset ile 0-120)
 *   - commitment:    Poseidon(actual_score + 20, salt) — bağlar ama açıklamaz
 *
 * Devre kısıtları:
 *   1. commitment == Poseidon(actual_score + 20, salt)
 *   2. actual_score + 20 >= threshold + 20  ↔  actual_score >= threshold
 *   3. actual_score + 20 ∈ [0, 120]  (range check)
 */

include "circomlib/circuits/poseidon.circom";
include "circomlib/circuits/comparators.circom";
include "circomlib/circuits/bitify.circom";

/*
 * Range check: 0 <= value <= max_value
 * max_bits: log2(max_value) + 1
 */
template RangeCheck(max_bits) {
    signal input value;
    signal input max_value;

    component bits = Num2Bits(max_bits);
    bits.in <== value;

    component lte = LessEqThan(max_bits);
    lte.in[0] <== value;
    lte.in[1] <== max_value;
    lte.out === 1;
}

template CreditThresholdProof() {
    // ── Private inputs ───────────────────────────────────────────────────────
    signal input actual_score;   // Gerçek puan + 20 offset (0..120 arası)
    signal input salt;           // Poseidon commitment tuzu

    // ── Public inputs ────────────────────────────────────────────────────────
    signal input threshold;      // Eşik + 20 offset (0..120 arası)
    signal input commitment;     // Poseidon(actual_score, salt)

    // ── CONSTRAINT 1: Commitment doğrulama ──────────────────────────────────
    component com = Poseidon(2);
    com.inputs[0] <== actual_score;
    com.inputs[1] <== salt;
    com.out === commitment;

    // ── CONSTRAINT 2: actual_score >= threshold ──────────────────────────────
    // LessEqThan(n): a <= b → out = 1
    // Biz istiyoruz: actual_score >= threshold ↔ threshold <= actual_score
    component gte = LessEqThan(8);  // 8 bit = 0..255 (120 < 256 ✓)
    gte.in[0] <== threshold;
    gte.in[1] <== actual_score;
    gte.out === 1;  // threshold <= actual_score ZORUNLU

    // ── CONSTRAINT 3: Range check (actual_score ∈ [0, 120]) ─────────────────
    component range_lo = LessEqThan(8);
    range_lo.in[0] <== 0;
    range_lo.in[1] <== actual_score;
    range_lo.out === 1;

    component range_hi = LessEqThan(8);
    range_hi.in[0] <== actual_score;
    range_hi.in[1] <== 120;
    range_hi.out === 1;

    // ── CONSTRAINT 4: threshold range check (0..120) ─────────────────────────
    component thr_lo = LessEqThan(8);
    thr_lo.in[0] <== 0;
    thr_lo.in[1] <== threshold;
    thr_lo.out === 1;

    component thr_hi = LessEqThan(8);
    thr_hi.in[0] <== threshold;
    thr_hi.in[1] <== 120;
    thr_hi.out === 1;
}

component main {public [threshold, commitment]} = CreditThresholdProof();
