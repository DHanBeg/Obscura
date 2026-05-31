pragma circom 2.1.6;

// Spec Bölüm 7.1 — Dolandırıcılık kanıtı (kredi puanı bileşeni, ağır ceza)
//
// Kullanıcı hesabında herhangi bir aktif dolandırıcılık (fraud) bayrağı
// bulunmadığını DID'ini açıklamadan kanıtlar. Dolandırıcılık bayrağı,
// kredi puanını sıfırlayan ve tier'ı düşüren en ağır cezadır; bu nedenle
// devrenin tek kabul ettiği değer fraud_flags === 0'dır.
//
// Public inputs:
//   did_commitment   — Poseidon(secret, salt) — kullanıcı DID taahhüdü
//   timestamp        — kanıt epoch'u (replay protection)
//
// Private inputs:
//   fraud_flags      — dolandırıcılık bayrağı sayısı (kanıt için 0 olmak zorunda)
//   secret           — kullanıcı gizli anahtarı
//   salt             — commitment tuzu

include "node_modules/circomlib/circuits/poseidon.circom";
include "node_modules/circomlib/circuits/comparators.circom";

template FraudProof() {
    // Public
    signal input did_commitment;
    signal input timestamp;

    // Private
    signal input fraud_flags;
    signal input secret;
    signal input salt;

    // 1. DID commitment doğrula: Poseidon(secret, salt) === did_commitment
    component didHasher = Poseidon(2);
    didHasher.inputs[0] <== secret;
    didHasher.inputs[1] <== salt;
    didHasher.out === did_commitment;

    // 2. timestamp sıfır olmayan (boş kanıt önleme)
    component tsCheck = IsZero();
    tsCheck.in <== timestamp;
    tsCheck.out === 0;

    // 3. fraud_flags KESİNLİKLE 0 olmak zorunda.
    //    Bu zorlayıcı kısıtlama: tek bir bayrak varsa kanıt üretilemez.
    fraud_flags === 0;
}

component main {public [did_commitment, timestamp]} = FraudProof();
