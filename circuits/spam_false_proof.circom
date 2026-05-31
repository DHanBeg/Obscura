pragma circom 2.1.6;

// Spec Bölüm 7.1 — Yanlış spam raporu kanıtı (kredi puanı bileşeni, eksi cezaya tabi)
//
// Kullanıcının kendisi tarafından yapılan ve sonradan haksız bulunan spam
// raporlarının sayısının cezanın tetiklendiği eşiğin altında kaldığını
// kanıtlar. Bu, kötü niyetli rapor sallayarak başka kullanıcılara zarar
// veren aktörlere karşı bir caydırıcıdır.
//
// Public inputs:
//   did_commitment   — Poseidon(secret, salt) — kullanıcı DID taahhüdü
//   timestamp        — kanıt epoch'u (replay protection)
//   max_false        — izin verilen maksimum yanlış rapor sayısı (ceza eşiği)
//
// Private inputs:
//   false_reports    — gerçek yanlış rapor sayısı (gizli)
//   secret           — kullanıcı gizli anahtarı
//   salt             — commitment tuzu
//
// Output:
//   below_threshold  — 1 eğer false_reports <= max_false

include "node_modules/circomlib/circuits/poseidon.circom";
include "node_modules/circomlib/circuits/comparators.circom";

template SpamFalseProof() {
    // Public
    signal input did_commitment;
    signal input timestamp;
    signal input max_false;

    // Private
    signal input false_reports;
    signal input secret;
    signal input salt;

    // Output
    signal output below_threshold;

    // 1. DID commitment doğrula: Poseidon(secret, salt) === did_commitment
    component didHasher = Poseidon(2);
    didHasher.inputs[0] <== secret;
    didHasher.inputs[1] <== salt;
    didHasher.out === did_commitment;

    // 2. timestamp sıfır olmayan (boş kanıt önleme)
    component tsCheck = IsZero();
    tsCheck.in <== timestamp;
    tsCheck.out === 0;

    // 3. false_reports <= max_false (LessThan ile: false_reports < max_false + 1)
    component cmp = LessThan(16);
    cmp.in[0] <== false_reports;
    cmp.in[1] <== max_false + 1;
    below_threshold <== cmp.out;

    // 4. Üst sınır: max 65535 (16-bit aralık)
    component rangeCheck = LessThan(16);
    rangeCheck.in[0] <== false_reports;
    rangeCheck.in[1] <== 65535;
    rangeCheck.out === 1;
}

component main {public [did_commitment, timestamp, max_false]} = SpamFalseProof();
