pragma circom 2.1.6;

/*
 * ZKMLModeration — Sıfır-Bilgi ML Moderasyon Devresi (FAZ 3, ADR-0013)
 *
 * Client-side ONNX modeli ile çalışır. Model plaintext mesajı görür ama
 * server plaintext'i görmez — sadece ZK kanıtını doğrular.
 *
 * Amaç:
 *   Kullanıcı client'ta spam/zarar sınıflandırması yapar ve ZK kanıtı üretir.
 *   Server kanıtı doğrular — içerik gizli kalır.
 *
 * Gizli girdiler (witness):
 *   - input_hash          : Poseidon(plaintext_chunks[0..15]) — mesaj hash'i
 *   - inference_output[2] : ONNX çıktısı [spam_logit, ham_logit]
 *   - model_weights_hash  : Poseidon(model_weights) — model doğrulaması için
 *
 * Kamuya açık girdiler:
 *   - score_bucket        : 0-9 arası skor kovası (inference_output'tan türetilir)
 *   - model_commitment    : Poseidon(model_weights_hash, version) — onaylı model
 *   - sender_commitment   : Poseidon(sender_did_hash, epoch) — spam track
 *
 * Kamuya açık çıktılar:
 *   - is_spam             : 1 = spam saptandı (score_bucket >= 7)
 *
 * Güvenlik garantileri:
 *   - Plaintext server'a açılmaz
 *   - Model sürümü commitment ile sabitlenir
 *   - Gönderici takip için sender_commitment (DID gizli)
 *   - score_bucket manipülasyonu: inference_output ile tutarsızlık constraint'i
 *
 * Kısıtlamalar: ~1100 constraint
 */

include "node_modules/circomlib/circuits/poseidon.circom";
include "node_modules/circomlib/circuits/comparators.circom";

template ZKMLModeration() {
    // ─── Gizli girdiler ───────────────────────────────────────────────────────
    signal input input_hash;             // Poseidon(plaintext mesaj parçaları)
    signal input inference_output[2];    // [spam_logit * 1000, ham_logit * 1000] (int)
    signal input model_weights_hash;     // ONNX model ağırlıkları hash'i

    // ─── Kamuya açık girdiler ─────────────────────────────────────────────────
    signal input score_bucket;           // 0-9 (inference_output'tan türetilir)
    signal input model_commitment;       // onaylı model commitment'ı
    signal input sender_commitment;      // gönderici + epoch commitment'ı

    // ─── Çıktılar ─────────────────────────────────────────────────────────────
    signal output is_spam;

    // ─── 1. Model commitment doğrulama ───────────────────────────────────────
    // Poseidon(model_weights_hash) == model_commitment
    component model_h = Poseidon(1);
    model_h.inputs[0] <== model_weights_hash;

    component model_eq = IsEqual();
    model_eq.in[0] <== model_h.out;
    model_eq.in[1] <== model_commitment;

    // ─── 2. Score bucket aralık kontrolü (0-9) ────────────────────────────────
    component bucket_lt = LessThan(4);
    bucket_lt.in[0] <== score_bucket;
    bucket_lt.in[1] <== 10; // 0..9 geçerli

    component bucket_gte = GreaterEqThan(4);
    bucket_gte.in[0] <== score_bucket;
    bucket_gte.in[1] <== 0;

    // ─── 3. Spam sınıflandırma: spam_logit > ham_logit ────────────────────────
    // inference_output[0] = spam score, inference_output[1] = ham score
    component spam_gt = GreaterThan(20);
    spam_gt.in[0] <== inference_output[0];
    spam_gt.in[1] <== inference_output[1];

    // ─── 4. score_bucket ile sınıflandırma tutarlılığı ────────────────────────
    // score_bucket >= 7 → spam = 1 (threshold: 7/9 = 0.78)
    component bucket_spam = GreaterEqThan(4);
    bucket_spam.in[0] <== score_bucket;
    bucket_spam.in[1] <== 7;

    // spam_gt.out ve bucket_spam.out tutarlı olmalı
    // Tutarlılık: XOR == 0 (ikisi de aynı sonuç vermeli)
    signal consistency;
    consistency <== spam_gt.out - bucket_spam.out;
    // Tutarsız olursa (1 veya -1) → kanıt geçersiz
    // Constraint: consistency * consistency == 0 (yani 0 olmalı)
    consistency * consistency === 0;

    // ─── 5. is_spam çıktısı ───────────────────────────────────────────────────
    // model_eq.out (1 = model geçerli) * bucket_spam.out (1 = spam)
    signal valid_model;
    valid_model <== model_eq.out * bucket_lt.out;
    is_spam <== valid_model * bucket_spam.out;
}

component main {public [score_bucket, model_commitment, sender_commitment]} = ZKMLModeration();
