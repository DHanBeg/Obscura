pragma circom 2.1.6;

// Spec Bölüm 7.1 — Sesli arama kanıtı (kredi puanı bileşeni, +0.2/arama)
//
// Kullanıcı bir epoch içinde gerçekleştirdiği toplam sesli arama sayısının
// belirli bir eşiği aştığını DID kimliğini açıklamadan kanıtlar.
//
// DID binding: did_commitment = Poseidon(secret, salt) — backend kayıtlı commitment ile
// karşılaştırır. Saldırgan başka bir kullanıcının arama sayısını kullanamaz.
//
// Public inputs:
//   did_commitment   — Poseidon(secret, salt) — kullanıcı DID taahhüdü
//   timestamp        — kanıt epoch'u (replay protection)
//   min_calls        — kanıtlanan minimum arama sayısı
//
// Private inputs:
//   call_count       — gerçek arama sayısı (gizli)
//   secret           — kullanıcı gizli anahtarı
//   salt             — commitment tuzu
//
// Output:
//   has_enough_calls — 1 eğer call_count >= min_calls

include "node_modules/circomlib/circuits/poseidon.circom";
include "node_modules/circomlib/circuits/comparators.circom";

template CallProof() {
    // Public
    signal input did_commitment;
    signal input timestamp;
    signal input min_calls;

    // Private
    signal input call_count;
    signal input secret;
    signal input salt;

    // Output
    signal output has_enough_calls;

    // 1. DID commitment doğrula: Poseidon(secret, salt) === did_commitment
    component didHasher = Poseidon(2);
    didHasher.inputs[0] <== secret;
    didHasher.inputs[1] <== salt;
    didHasher.out === did_commitment;

    // 2. timestamp sıfır olmayan (boş kanıt önleme)
    component tsCheck = IsZero();
    tsCheck.in <== timestamp;
    tsCheck.out === 0;

    // 3. call_count >= min_calls
    component cmp = GreaterEqThan(16);
    cmp.in[0] <== call_count;
    cmp.in[1] <== min_calls;
    has_enough_calls <== cmp.out;

    // 4. Makul üst sınır (Sybil koruması: max 10000 arama sayılır)
    component maxCheck = LessEqThan(16);
    maxCheck.in[0] <== call_count;
    maxCheck.in[1] <== 10000;
    maxCheck.out === 1;
}

component main {public [did_commitment, timestamp, min_calls]} = CallProof();
