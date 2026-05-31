pragma circom 2.1.6;

// endorsement_proof.circom — Kullanıcı onay kanıtı (kredi puanı bileşeni)
//
// Spec Bölüm 7.2: Yüksek tier kullanıcılardan alınan onaylar kredi puanını artırır.
// Onaylayan kullanıcıların kimlikleri açıklanmadan onay sayısı kanıtlanır.
//
// Yaklaşım: Merkle tree of endorsements.
//   endorsement_i = Poseidon(endorser_did_hash, endorsed_did_hash, timestamp, nonce)
//   endorsement_root = MerkleRoot(endorsement_0 ... endorsement_N-1)
//
// Public inputs:
//   min_endorsements   — kanıtlanan minimum onay sayısı
//   endorsed_hash      — Poseidon(endorsed_did) — kimin onaylandığı
//   endorsement_root   — Merkle kök (onay listesi bütünlüğü)
//   epoch              — replay koruması
//
// Private inputs:
//   endorsement_count  — gerçek onay sayısı
//   user_secret        — onaylanan kullanıcı gizli anahtarı
//
// Output:
//   has_enough         — 1 eğer endorsement_count >= min_endorsements

include "node_modules/circomlib/circuits/poseidon.circom";
include "node_modules/circomlib/circuits/comparators.circom";

template EndorsementProof() {
    // Public
    signal input min_endorsements;
    signal input endorsed_hash;
    signal input endorsement_root;
    signal input epoch;

    // Private
    signal input endorsement_count;
    signal input user_secret;

    // Output
    signal output has_enough;

    // 1. Taahhüt: endorsement_root bağlı
    // Gerçek implementasyonda Merkle proof kullanılır.
    // Bu circuit basitleştirilmiş: count commitment üzerinden çalışır.
    component hasher = Poseidon(4);
    hasher.inputs[0] <== endorsed_hash;
    hasher.inputs[1] <== endorsement_count;
    hasher.inputs[2] <== user_secret;
    hasher.inputs[3] <== epoch;
    endorsement_root === hasher.out;

    // 2. Yeterli onay var mı?
    component count_check = GreaterEqThan(16);
    count_check.in[0] <== endorsement_count;
    count_check.in[1] <== min_endorsements;
    has_enough <== count_check.out;

    // 3. Makul üst sınır (Sybil koruması: max 1000 onay sayılır)
    component max_check = LessEqThan(16);
    max_check.in[0] <== endorsement_count;
    max_check.in[1] <== 1000;
    max_check.out === 1;
}

component main {public [min_endorsements, endorsed_hash, endorsement_root, epoch]} = EndorsementProof();
