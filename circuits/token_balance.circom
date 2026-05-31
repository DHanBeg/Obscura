pragma circom 2.1.6;

/*
 * TokenBalance — Obscura Gizli Token Bakiye Kanıtı (spec Bölüm 17.3)
 *
 * FAZ 3: Gerçek Merkle inclusion proof (depth=20, 1M+ yaprak kapasitesi).
 *
 * Prover, mevcut bir note'un (balance_commitment) geçerli state tree'ye
 * dahil olduğunu, hem balance'ı hem de Merkle path'i açıklamadan kanıtlar.
 *
 * Kısıtlamalar:
 *   1. balance_commitment = Poseidon(secret, balance, salt)
 *   2. nullifier          = Poseidon(secret, salt)          [double-spend]
 *   3. balance >= amount                                    [yeterli bakiye]
 *   4. Merkle inclusion:  MerkleProof(leaf, path, indices) == root
 *   5. timestamp != 0                                       [replay guard]
 *
 * Gizli girdiler (witness):
 *   - balance               : Mevcut bakiye
 *   - amount                : Transfer miktarı
 *   - secret                : Kullanıcı gizli anahtarı
 *   - salt                  : Rastgele tuz
 *   - path_elements[DEPTH]  : Merkle sibling hash'leri
 *   - path_indices[DEPTH]   : 0=sol, 1=sağ (her seviye için)
 *
 * Kamuya açık girdiler:
 *   - balance_commitment : Poseidon(secret, balance, salt)
 *   - nullifier          : Poseidon(secret, salt)
 *   - root               : Merkle tree root (shielded_root tablosundan)
 *   - timestamp          : Proof zamanı (unix saniye)
 *   - leaf_index         : Harcanan note'un yaprak indeksi
 */

include "circomlib/circuits/poseidon.circom";
include "circomlib/circuits/comparators.circom";
include "circomlib/circuits/bitify.circom";
include "circomlib/circuits/mux1.circom";

// ─── Merkle Path Doğrulama ────────────────────────────────────────────────────
// depth=20: 2^20 = 1,048,576 yaprak — FAZ 2/3 için yeterli.
// Her seviyede sibling ile birlikte Poseidon(left, right) hesaplar.
template MerkleProof(depth) {
    signal input leaf;
    signal input path_elements[depth];
    signal input path_indices[depth];  // 0: leaf sol, 1: leaf sağ
    signal output root;

    component hashers[depth];
    component selectors[depth];
    signal nodes[depth + 1];
    nodes[0] <== leaf;

    for (var i = 0; i < depth; i++) {
        // path_indices[i] ∈ {0,1} kısıtlaması
        path_indices[i] * (1 - path_indices[i]) === 0;

        // Mux1: idx=0 → (nodes[i], sibling), idx=1 → (sibling, nodes[i])
        selectors[i] = MultiMux1(2);
        selectors[i].c[0][0] <== nodes[i];
        selectors[i].c[0][1] <== path_elements[i];
        selectors[i].c[1][0] <== path_elements[i];
        selectors[i].c[1][1] <== nodes[i];
        selectors[i].s <== path_indices[i];

        hashers[i] = Poseidon(2);
        hashers[i].inputs[0] <== selectors[i].out[0];
        hashers[i].inputs[1] <== selectors[i].out[1];
        nodes[i + 1] <== hashers[i].out;
    }

    root <== nodes[depth];
}

// ─── Ana Devre ────────────────────────────────────────────────────────────────
template TokenBalance(bits, depth) {
    // ── Gizli girdiler ───────────────────────────────────────────────────────
    signal input balance;
    signal input amount;
    signal input secret;
    signal input salt;
    signal input path_elements[depth];
    signal input path_indices[depth];

    // ── Kamuya açık girdiler ──────────────────────────────────────────────────
    signal input balance_commitment;
    signal input nullifier;
    signal input root;
    signal input timestamp;
    signal input leaf_index;     // harcanan note'un ağaçtaki konumu (auditable)

    // ── 1. Range check ───────────────────────────────────────────────────────
    component balanceBits = Num2Bits(bits);
    balanceBits.in <== balance;

    component amountBits = Num2Bits(bits);
    amountBits.in <== amount;

    // ── 2. balance >= amount ──────────────────────────────────────────────────
    component sufficient = LessThan(bits);
    sufficient.in[0] <== amount;
    sufficient.in[1] <== balance + 1;
    sufficient.out === 1;

    // ── 3. balance_commitment = Poseidon(secret, balance, salt) ──────────────
    component balanceHasher = Poseidon(3);
    balanceHasher.inputs[0] <== secret;
    balanceHasher.inputs[1] <== balance;
    balanceHasher.inputs[2] <== salt;
    balanceHasher.out === balance_commitment;

    // ── 4. nullifier = Poseidon(secret, salt) ────────────────────────────────
    component nullifierHasher = Poseidon(2);
    nullifierHasher.inputs[0] <== secret;
    nullifierHasher.inputs[1] <== salt;
    nullifierHasher.out === nullifier;

    // ── 5. timestamp != 0 ────────────────────────────────────────────────────
    component tsCheck = IsZero();
    tsCheck.in <== timestamp;
    tsCheck.out === 0;

    // ── 6. Merkle inclusion proof ─────────────────────────────────────────────
    // leaf = balance_commitment (prover note'un var olduğunu kanıtlar)
    component merkle = MerkleProof(depth);
    merkle.leaf <== balance_commitment;
    for (var i = 0; i < depth; i++) {
        merkle.path_elements[i] <== path_elements[i];
        merkle.path_indices[i]  <== path_indices[i];
    }
    // Hesaplanan root, public root ile eşleşmeli
    merkle.root === root;

    // leaf_index: public'te açıklanır; backend hangi yaprağın harcandığını
    // bilir böylece nullifier + leaf_index çiftiyle çift harcamayı engeller.
    // leaf_index'in gerçekten depth-bit unsigned olduğunu kısıtla.
    component leafIdxBits = Num2Bits(depth);
    leafIdxBits.in <== leaf_index;
}

component main {
    public [balance_commitment, nullifier, root, timestamp, leaf_index]
} = TokenBalance(64, 20);
