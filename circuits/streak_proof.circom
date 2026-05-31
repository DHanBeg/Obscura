pragma circom 2.1.6;

// streak_proof.circom — Kesintisiz aktivite serisi kanıtı (kredi puanı)
//
// Spec Bölüm 7.2: Kesintisiz günlük aktivite streak'i kredi puanını artırır.
// Kullanıcı, N gün kesintisiz aktif olduğunu açıklamadan kanıtlar.
//
// Public inputs:
//   min_streak_days    — kanıtlanan minimum streak uzunluğu (gün)
//   current_epoch      — mevcut gün epoch'u
//   streak_commitment  — Poseidon(streak_start, streak_length, user_secret, current_epoch)
//
// Private inputs:
//   streak_start       — streak başlangıç epoch'u
//   streak_length      — streak uzunluğu (gün)
//   user_secret        — kullanıcı gizli anahtarı
//
// Output:
//   streak_ok          — 1 eğer streak_length >= min_streak_days

include "node_modules/circomlib/circuits/poseidon.circom";
include "node_modules/circomlib/circuits/comparators.circom";

template StreakProof() {
    // Public
    signal input min_streak_days;
    signal input current_epoch;
    signal input streak_commitment;

    // Private
    signal input streak_start;
    signal input streak_length;
    signal input user_secret;

    // Output
    signal output streak_ok;

    // 1. Streak taahhüdü doğrula
    component hasher = Poseidon(4);
    hasher.inputs[0] <== streak_start;
    hasher.inputs[1] <== streak_length;
    hasher.inputs[2] <== user_secret;
    hasher.inputs[3] <== current_epoch;
    streak_commitment === hasher.out;

    // 2. streak_start + streak_length <= current_epoch (streak gerçek zamanda olmuş)
    signal streak_end;
    streak_end <== streak_start + streak_length;

    component time_check = LessEqThan(32);
    time_check.in[0] <== streak_end;
    time_check.in[1] <== current_epoch;
    time_check.out === 1;

    // 3. streak_start <= current_epoch (geçmişte başlamış)
    component start_check = LessEqThan(32);
    start_check.in[0] <== streak_start;
    start_check.in[1] <== current_epoch;
    start_check.out === 1;

    // 4. streak_length >= min_streak_days
    component length_check = GreaterEqThan(16);
    length_check.in[0] <== streak_length;
    length_check.in[1] <== min_streak_days;
    streak_ok <== length_check.out;

    // 5. Makul üst sınır: max 365 günlük streak
    component max_check = LessEqThan(16);
    max_check.in[0] <== streak_length;
    max_check.in[1] <== 365;
    max_check.out === 1;
}

component main {public [min_streak_days, current_epoch, streak_commitment]} = StreakProof();
