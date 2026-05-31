pragma circom 2.1.6;

// Spec Bölüm 7.1 — Topluluk katkısı kanıtı (kredi puanı bileşeni, +5/katkı)
//
// Kullanıcı, kayda değer topluluk katkısı (mini app yayını, governance önerisi
// kabul edilen, doğrulanmış bug raporu, vb.) sayısının bir eşiği aştığını
// DID'ini açıklamadan kanıtlar. Her katkı +5 puan getirdiği için Sybil
// koruması olarak üst sınır vardır (max 100 katkı sayılır).
//
// Public inputs:
//   did_commitment       — Poseidon(secret, salt) — kullanıcı DID taahhüdü
//   timestamp            — kanıt epoch'u (replay protection)
//   min_contributions    — kanıtlanan minimum katkı sayısı
//
// Private inputs:
//   contribution_count   — gerçek katkı sayısı (gizli)
//   secret               — kullanıcı gizli anahtarı
//   salt                 — commitment tuzu
//
// Output:
//   has_enough_contribs  — 1 eğer contribution_count >= min_contributions

include "node_modules/circomlib/circuits/poseidon.circom";
include "node_modules/circomlib/circuits/comparators.circom";

template ContributionProof() {
    // Public
    signal input did_commitment;
    signal input timestamp;
    signal input min_contributions;

    // Private
    signal input contribution_count;
    signal input secret;
    signal input salt;

    // Output
    signal output has_enough_contribs;

    // 1. DID commitment doğrula: Poseidon(secret, salt) === did_commitment
    component didHasher = Poseidon(2);
    didHasher.inputs[0] <== secret;
    didHasher.inputs[1] <== salt;
    didHasher.out === did_commitment;

    // 2. timestamp sıfır olmayan (boş kanıt önleme)
    component tsCheck = IsZero();
    tsCheck.in <== timestamp;
    tsCheck.out === 0;

    // 3. contribution_count >= min_contributions
    component cmp = GreaterEqThan(16);
    cmp.in[0] <== contribution_count;
    cmp.in[1] <== min_contributions;
    has_enough_contribs <== cmp.out;

    // 4. Sybil koruması: max 100 katkı sayılır
    component maxCheck = LessEqThan(16);
    maxCheck.in[0] <== contribution_count;
    maxCheck.in[1] <== 100;
    maxCheck.out === 1;
}

component main {public [did_commitment, timestamp, min_contributions]} = ContributionProof();
