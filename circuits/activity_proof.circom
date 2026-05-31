pragma circom 2.1.6;

// activity_proof.circom — Aktivite kanıtı (kredi puanı bileşeni)
//
// Spec Bölüm 7.2: Son 30 günlük aktivite kredi puanını etkiler.
// Kullanıcı, belirli bir aktivite eşiğini (gün sayısı, mesaj sayısı)
// aştığını kimliğini açıklamadan kanıtlar.
//
// Public inputs:
//   min_active_days    — kanıtlanan minimum aktif gün sayısı
//   min_msg_count      — kanıtlanan minimum mesaj sayısı
//   epoch              — hesaplama epoch'u (replay protection)
//   activity_root      — Poseidon(active_days, msg_count, user_secret, epoch)
//
// Private inputs:
//   active_days        — son 30 günde kaç gün aktif olundu
//   msg_count          — son 30 günde kaç mesaj gönderildi
//   user_secret        — kullanıcı gizli anahtarı
//
// Outputs:
//   meets_day_threshold  — 1 eğer active_days >= min_active_days
//   meets_msg_threshold  — 1 eğer msg_count >= min_msg_count

include "node_modules/circomlib/circuits/poseidon.circom";
include "node_modules/circomlib/circuits/comparators.circom";

template ActivityProof() {
    // Public
    signal input min_active_days;
    signal input min_msg_count;
    signal input epoch;
    signal input activity_root;

    // Private
    signal input active_days;
    signal input msg_count;
    signal input user_secret;

    // Outputs
    signal output meets_day_threshold;
    signal output meets_msg_threshold;

    // 1. Activity root doğrula
    component hasher = Poseidon(4);
    hasher.inputs[0] <== active_days;
    hasher.inputs[1] <== msg_count;
    hasher.inputs[2] <== user_secret;
    hasher.inputs[3] <== epoch;
    activity_root === hasher.out;

    // 2. Gün eşiği: active_days >= min_active_days
    component day_check = GreaterEqThan(32);
    day_check.in[0] <== active_days;
    day_check.in[1] <== min_active_days;
    meets_day_threshold <== day_check.out;

    // 3. Mesaj eşiği: msg_count >= min_msg_count
    component msg_check = GreaterEqThan(32);
    msg_check.in[0] <== msg_count;
    msg_check.in[1] <== min_msg_count;
    meets_msg_threshold <== msg_check.out;

    // 4. Değerler makul aralıkta (30 gün, 10000 mesaj üst sınır)
    component days_range = LessEqThan(32);
    days_range.in[0] <== active_days;
    days_range.in[1] <== 30;
    days_range.out === 1;

    component msg_range = LessEqThan(32);
    msg_range.in[0] <== msg_count;
    msg_range.in[1] <== 10000;
    msg_range.out === 1;
}

component main {public [min_active_days, min_msg_count, epoch, activity_root]} = ActivityProof();
