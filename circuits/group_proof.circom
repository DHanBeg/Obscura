pragma circom 2.1.6;

// Spec Bölüm 7.1 — Grup oluşturma kanıtı (kredi puanı bileşeni, +2/grup)
//
// Kullanıcı oluşturduğu grup sayısının bir eşiği aştığını DID'ini açıklamadan
// kanıtlar. Grup oluşturma, kredi puanına +2 katkı sağladığı için Sybil
// direnci kritiktir; bu nedenle üst sınır vardır (max 1000 grup sayılır).
//
// Public inputs:
//   did_commitment   — Poseidon(secret, salt) — kullanıcı DID taahhüdü
//   timestamp        — kanıt epoch'u (replay protection)
//   min_groups       — kanıtlanan minimum grup sayısı
//
// Private inputs:
//   group_count      — gerçek grup sayısı (gizli)
//   secret           — kullanıcı gizli anahtarı
//   salt             — commitment tuzu
//
// Output:
//   has_enough_groups — 1 eğer group_count >= min_groups

include "node_modules/circomlib/circuits/poseidon.circom";
include "node_modules/circomlib/circuits/comparators.circom";

template GroupProof() {
    // Public
    signal input did_commitment;
    signal input timestamp;
    signal input min_groups;

    // Private
    signal input group_count;
    signal input secret;
    signal input salt;

    // Output
    signal output has_enough_groups;

    // 1. DID commitment doğrula: Poseidon(secret, salt) === did_commitment
    component didHasher = Poseidon(2);
    didHasher.inputs[0] <== secret;
    didHasher.inputs[1] <== salt;
    didHasher.out === did_commitment;

    // 2. timestamp sıfır olmayan (boş kanıt önleme)
    component tsCheck = IsZero();
    tsCheck.in <== timestamp;
    tsCheck.out === 0;

    // 3. group_count >= min_groups
    component cmp = GreaterEqThan(16);
    cmp.in[0] <== group_count;
    cmp.in[1] <== min_groups;
    has_enough_groups <== cmp.out;

    // 4. Sybil üst sınırı: max 1000 grup sayılır
    component maxCheck = LessEqThan(16);
    maxCheck.in[0] <== group_count;
    maxCheck.in[1] <== 1000;
    maxCheck.out === 1;
}

component main {public [did_commitment, timestamp, min_groups]} = GroupProof();
