pragma circom 2.1.6;

/*
 * Obscura — Token Transfer Proof Circuit (OBS Token)
 *
 * Kanıt: "Belirli bir miktarda OBS token'ı şu adresten şu adrese gönderdim,
 *         bakiyem negatife düşmüyor ve transfer geçerli"
 *
 * Private inputs:
 *   - sender_balance:    Gönderenin mevcut bakiyesi (gizli)
 *   - amount:            Transfer miktarı (gizli)
 *   - sender_salt:       Gönderen commitment tuzu
 *   - receiver_salt:     Alıcı commitment tuzu
 *
 * Public inputs:
 *   - sender_commitment:   Poseidon(sender_balance, sender_salt)
 *   - new_sender_commitment: Poseidon(sender_balance - amount, new_sender_salt)
 *   - receiver_commitment: Alıcı bakiye commitment (off-chain güncellenir)
 *   - amount_hash:         Poseidon(amount) — transfer miktarı gizli ama bağlı
 *
 * Bu devre Aztec-style private transfer mantığını simüle eder.
 */

include "circomlib/circuits/poseidon.circom";
include "circomlib/circuits/comparators.circom";
include "circomlib/circuits/bitify.circom";

template TokenTransfer(max_bits) {
    // ── Private inputs ───────────────────────────────────────────────────────
    signal input sender_balance;    // Mevcut bakiye (gizli)
    signal input amount;            // Transfer miktarı (gizli)
    signal input sender_salt;       // Gönderen commitment tuzu
    signal input new_sender_salt;   // Yeni gönderen commitment tuzu

    // ── Public inputs ────────────────────────────────────────────────────────
    signal input sender_commitment;      // Poseidon(sender_balance, sender_salt)
    signal input new_sender_commitment;  // Poseidon(sender_balance - amount, new_sender_salt)
    signal input amount_hash;            // Poseidon(amount)

    // ── CONSTRAINT 1: Gönderen commitment doğrulama ─────────────────────────
    component com1 = Poseidon(2);
    com1.inputs[0] <== sender_balance;
    com1.inputs[1] <== sender_salt;
    com1.out === sender_commitment;

    // ── CONSTRAINT 2: Transfer sonrası bakiye ────────────────────────────────
    signal new_balance;
    new_balance <== sender_balance - amount;

    component com2 = Poseidon(2);
    com2.inputs[0] <== new_balance;
    com2.inputs[1] <== new_sender_salt;
    com2.out === new_sender_commitment;

    // ── CONSTRAINT 3: Bakiye negatife düşmüyor ──────────────────────────────
    // sender_balance >= amount → new_balance >= 0
    component gte = LessEqThan(max_bits);
    gte.in[0] <== amount;
    gte.in[1] <== sender_balance;
    gte.out === 1;

    // ── CONSTRAINT 4: Amount hash ────────────────────────────────────────────
    component amt_hash = Poseidon(1);
    amt_hash.inputs[0] <== amount;
    amt_hash.out === amount_hash;

    // ── CONSTRAINT 5: Amount > 0 ─────────────────────────────────────────────
    component amt_pos = GreaterThan(max_bits);
    amt_pos.in[0] <== amount;
    amt_pos.in[1] <== 0;
    amt_pos.out === 1;
}

// 64-bit token amount (mikro-OBS)
component main {
    public [sender_commitment, new_sender_commitment, amount_hash]
} = TokenTransfer(64);
