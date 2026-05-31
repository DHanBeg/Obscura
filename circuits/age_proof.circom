pragma circom 2.1.6;

// age_proof.circom — Hesap yaşı kanıtı (Obscura Kredi Puanı bileşeni)
//
// Spec Bölüm 7.2: Hesap yaşı kredi puanını etkiler.
// Kullanıcı hesap açılış epoch'unu ve mevcut epoch'u açıklamadan
// minimum yaşı (gün cinsinden) kanıtlar.
//
// Public inputs:
//   min_age_days    — kanıtlanan minimum yaş (gün)
//   current_epoch   — mevcut zaman damgası (Unix timestamp / 86400)
//   age_commitment  — Poseidon(created_epoch, account_secret)
//
// Private inputs:
//   created_epoch   — hesap açılış epoch'u (gün)
//   account_secret  — hesap gizli anahtarı (kimlik gizliliği)
//
// Output:
//   is_old_enough   — 1 eğer hesap yaşı >= min_age_days

include "node_modules/circomlib/circuits/poseidon.circom";
include "node_modules/circomlib/circuits/comparators.circom";

template AgeProof() {
    // Public
    signal input min_age_days;
    signal input current_epoch;
    signal input age_commitment;

    // Private
    signal input created_epoch;
    signal input account_secret;

    // Output
    signal output is_old_enough;

    // 1. Taahhüt doğrula: Poseidon(created_epoch, account_secret) == age_commitment
    component hasher = Poseidon(2);
    hasher.inputs[0] <== created_epoch;
    hasher.inputs[1] <== account_secret;
    age_commitment === hasher.out;

    // 2. Hesap yaşını hesapla
    signal account_age;
    account_age <== current_epoch - created_epoch;

    // 3. Yaşın negatif olmadığını kontrol et (created_epoch <= current_epoch)
    component not_future = LessEqThan(32);
    not_future.in[0] <== created_epoch;
    not_future.in[1] <== current_epoch;
    not_future.out === 1;

    // 4. account_age >= min_age_days
    component old_enough = GreaterEqThan(32);
    old_enough.in[0] <== account_age;
    old_enough.in[1] <== min_age_days;
    is_old_enough <== old_enough.out;
}

component main {public [min_age_days, current_epoch, age_commitment]} = AgeProof();
