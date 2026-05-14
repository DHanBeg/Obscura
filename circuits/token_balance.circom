pragma circom 2.1.6;

/*
 * TokenBalance — Obscura Gizli Token Bakiye Kanıtı (spec Bölüm 17.3)
 *
 * Kullanıcı, mevcut bakiyesinin transfer etmek istediği miktardan büyük veya
 * eşit olduğunu, hem bakiyeyi hem de miktarı açıklamadan kanıtlar.
 *
 * Ek özellikler:
 *   - balance_commitment = Poseidon(secret, balance, salt)
 *     Backend bunu state tree'de tutar; prover yalnızca bilen kişi proof
 *     üretebilir.
 *   - nullifier = Poseidon(secret, salt)
 *     Double-spend koruması. Backend kullanılmış nullifier'ları kayıtlı
 *     tutar; aynı (secret, salt) çiftinden ikinci proof reddedilir.
 *   - root: Merkle state root (basitleştirilmiş — şimdilik bağlanır,
 *     gelecek versiyonda gerçek Merkle path eklenecek).
 *   - timestamp != 0 (replay window enforcement).
 *
 * Gizli girdiler (witness):
 *   - balance : Mevcut bakiye (bits-bit unsigned)
 *   - amount  : Transfer miktarı (bits-bit unsigned)
 *   - secret  : Kullanıcı gizli anahtarı
 *   - salt    : Rastgele tuz
 *
 * Kamuya açık girdiler:
 *   - balance_commitment : Poseidon(secret, balance, salt)
 *   - nullifier          : Poseidon(secret, salt)
 *   - root               : Merkle root (state tree, basitleştirilmiş)
 *   - timestamp          : Proof zamanı
 */

include "circomlib/circuits/poseidon.circom";
include "circomlib/circuits/comparators.circom";
include "circomlib/circuits/bitify.circom";

template TokenBalance(bits) {
    // ─── Gizli Girdiler ───────────────────────────────────────────────────────
    signal input balance;
    signal input amount;
    signal input secret;
    signal input salt;

    // ─── Kamuya Açık Girdiler ─────────────────────────────────────────────────
    signal input balance_commitment;
    signal input nullifier;
    signal input root;
    signal input timestamp;

    // ─── Kısıtlamalar ─────────────────────────────────────────────────────────

    // 1. Range check: balance ve amount, bits-bit unsigned olmalı.
    //    Num2Bits aşımda fail eder.
    component balanceBits = Num2Bits(bits);
    balanceBits.in <== balance;

    component amountBits = Num2Bits(bits);
    amountBits.in <== amount;

    // 2. balance >= amount  ↔  amount < balance + 1
    component sufficient = LessThan(bits);
    sufficient.in[0] <== amount;
    sufficient.in[1] <== balance + 1;
    sufficient.out === 1;

    // 3. balance_commitment = Poseidon(secret, balance, salt)
    component balanceHasher = Poseidon(3);
    balanceHasher.inputs[0] <== secret;
    balanceHasher.inputs[1] <== balance;
    balanceHasher.inputs[2] <== salt;
    balanceHasher.out === balance_commitment;

    // 4. nullifier = Poseidon(secret, salt) — double-spend koruması
    component nullifierHasher = Poseidon(2);
    nullifierHasher.inputs[0] <== secret;
    nullifierHasher.inputs[1] <== salt;
    nullifierHasher.out === nullifier;

    // 5. timestamp != 0 (boş/replay proof önleme)
    component tsCheck = IsZero();
    tsCheck.in <== timestamp;
    tsCheck.out === 0;

    // 6. root public input olarak bağlanır (basitleştirilmiş Merkle).
    //    Henüz Merkle path yok; root, balance_commitment ile birlikte
    //    Poseidon'a beslenerek pruning'den korunur. Çıkış kullanılmaz, ama
    //    kısıtlama root'u devreye gerçek anlamda dahil eder. v2'de gerçek
    //    Merkle inclusion proof eklenecek.
    component rootBinder = Poseidon(2);
    rootBinder.inputs[0] <== root;
    rootBinder.inputs[1] <== balance_commitment;
    signal root_binding;
    root_binding <== rootBinder.out;
}

component main {public [balance_commitment, nullifier, root, timestamp]} = TokenBalance(64);
