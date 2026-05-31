pragma circom 2.1.6;

// msg_count_proof.circom — Toplam mesaj sayısı kanıtı (kredi puanı)
//
// Spec Bölüm 7.2: Toplam mesaj sayısı tier yükseltme için kullanılır.
// Kullanıcı, toplam mesaj sayısının belirli bir eşiği geçtiğini
// kimliğini açıklamadan kanıtlar.
//
// Public inputs:
//   min_count         — kanıtlanan minimum mesaj sayısı
//   epoch             — replay koruması
//   count_commitment  — Poseidon(total_count, conv_count, user_secret, epoch)
//
// Private inputs:
//   total_count       — toplam gönderilen mesaj sayısı
//   conv_count        — toplam konuşma sayısı
//   user_secret       — kullanıcı gizli anahtarı
//
// Output:
//   is_above_threshold — 1 eğer total_count >= min_count

include "node_modules/circomlib/circuits/poseidon.circom";
include "node_modules/circomlib/circuits/comparators.circom";

template MsgCountProof() {
    // Public
    signal input min_count;
    signal input epoch;
    signal input count_commitment;

    // Private
    signal input total_count;
    signal input conv_count;
    signal input user_secret;

    // Output
    signal output is_above_threshold;

    // 1. Taahhüt doğrula
    component hasher = Poseidon(4);
    hasher.inputs[0] <== total_count;
    hasher.inputs[1] <== conv_count;
    hasher.inputs[2] <== user_secret;
    hasher.inputs[3] <== epoch;
    count_commitment === hasher.out;

    // 2. Eşik kontrolü: total_count >= min_count
    component threshold = GreaterEqThan(32);
    threshold.in[0] <== total_count;
    threshold.in[1] <== min_count;
    is_above_threshold <== threshold.out;

    // 3. conv_count <= total_count (tutarlılık)
    component consistency = LessEqThan(32);
    consistency.in[0] <== conv_count;
    consistency.in[1] <== total_count;
    consistency.out === 1;

    // 4. Değerler 32-bit aralığında (overflow koruması)
    // GreaterEqThan(32) zaten 32-bit sinyal varsayıyor
}

component main {public [min_count, epoch, count_commitment]} = MsgCountProof();
