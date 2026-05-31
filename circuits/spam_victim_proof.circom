pragma circom 2.1.6;

// Spec Bölüm 7.1 — Spam mağduru ceza kanıtı (kredi puanı bileşeni, eksi cezaya tabi)
//
// Kullanıcı hakkında açılan spam raporu sayısının izin verilen eşiğin altında
// olduğunu kanıtlar. Backend bu kanıtı, kredi cezası uygulanmadan önce
// "spam reports <= max_allowed" koşulunun sağlandığını teyit etmek için
// kullanır. Aksi halde tier düşürme yapılır.
//
// Public inputs:
//   did_commitment   — Poseidon(secret, salt) — kullanıcı DID taahhüdü
//   timestamp        — kanıt epoch'u (replay protection)
//   max_allowed      — izin verilen maksimum spam raporu sayısı
//
// Private inputs:
//   spam_reports     — kullanıcı hakkında açılmış gerçek spam raporu sayısı
//   secret           — kullanıcı gizli anahtarı
//   salt             — commitment tuzu
//
// Output:
//   within_limit     — 1 eğer spam_reports <= max_allowed

include "node_modules/circomlib/circuits/poseidon.circom";
include "node_modules/circomlib/circuits/comparators.circom";

template SpamVictimProof() {
    // Public
    signal input did_commitment;
    signal input timestamp;
    signal input max_allowed;

    // Private
    signal input spam_reports;
    signal input secret;
    signal input salt;

    // Output
    signal output within_limit;

    // 1. DID commitment doğrula: Poseidon(secret, salt) === did_commitment
    component didHasher = Poseidon(2);
    didHasher.inputs[0] <== secret;
    didHasher.inputs[1] <== salt;
    didHasher.out === did_commitment;

    // 2. timestamp sıfır olmayan (boş kanıt önleme)
    component tsCheck = IsZero();
    tsCheck.in <== timestamp;
    tsCheck.out === 0;

    // 3. spam_reports <= max_allowed (LessThan ile: spam_reports < max_allowed + 1)
    component cmp = LessThan(16);
    cmp.in[0] <== spam_reports;
    cmp.in[1] <== max_allowed + 1;
    within_limit <== cmp.out;

    // 4. Sybil koruması: max 65535 rapor sayılır (16-bit aralık)
    component rangeCheck = LessThan(16);
    rangeCheck.in[0] <== spam_reports;
    rangeCheck.in[1] <== 65535;
    rangeCheck.out === 1;
}

component main {public [did_commitment, timestamp, max_allowed]} = SpamVictimProof();
