pragma circom 2.1.6;

/*
 * VoteProof — Obscura Gizli Oy Kanıtı (spec Bölüm 17.4)
 *
 * Kullanıcı bir oylamada oy kullandığını kanıtlar; ne oy kullandığını
 * (vote_choice) açıklamaz. Bir oylamada ikinci oy nullifier ile engellenir.
 *
 * Gizli girdiler (witness):
 *   - voter_secret : Oy verenin gizli anahtarı
 *   - vote_choice  : Oy tercihi {0,1,2,3} (yes/no/abstain/veto)
 *   - voter_index  : Voter listesindeki index (Merkle path için, basitleştirilmiş)
 *
 * Kamuya açık girdiler:
 *   - poll_id          : Oylama tanımlayıcısı
 *   - vote_commitment  : Poseidon(vote_choice, voter_secret)
 *   - voter_root       : Voter Merkle root (basitleştirilmiş — v2'de path eklenecek)
 *   - nullifier        : Poseidon(voter_secret, poll_id) — bir oylamada tek oy
 *   - timestamp        : Proof zamanı
 */

include "circomlib/circuits/poseidon.circom";
include "circomlib/circuits/comparators.circom";

template VoteProof() {
    // ─── Gizli Girdiler ───────────────────────────────────────────────────────
    signal input voter_secret;
    signal input vote_choice;
    signal input voter_index;

    // ─── Kamuya Açık Girdiler ─────────────────────────────────────────────────
    signal input poll_id;
    signal input vote_commitment;
    signal input voter_root;
    signal input nullifier;
    signal input timestamp;

    // ─── Kısıtlamalar ─────────────────────────────────────────────────────────

    // 1. vote_choice ∈ {0, 1, 2, 3}
    //    vote_choice * (vote_choice-1) * (vote_choice-2) * (vote_choice-3) === 0
    //    Polynomial degree 4 — quadratic constraint'lere bölünür.
    signal vc0;
    signal vc1;
    vc0 <== vote_choice * (vote_choice - 1);
    vc1 <== (vote_choice - 2) * (vote_choice - 3);
    vc0 * vc1 === 0;

    // 2. vote_commitment = Poseidon(vote_choice, voter_secret)
    component voteHasher = Poseidon(2);
    voteHasher.inputs[0] <== vote_choice;
    voteHasher.inputs[1] <== voter_secret;
    voteHasher.out === vote_commitment;

    // 3. nullifier = Poseidon(voter_secret, poll_id) — bir oylamada tek oy
    component nullifierHasher = Poseidon(2);
    nullifierHasher.inputs[0] <== voter_secret;
    nullifierHasher.inputs[1] <== poll_id;
    nullifierHasher.out === nullifier;

    // 4. timestamp != 0 (boş/replay proof önleme)
    component tsCheck = IsZero();
    tsCheck.in <== timestamp;
    tsCheck.out === 0;

    // 5. voter_root + voter_index public/binding — Merkle path v2'de eklenecek.
    //    Şimdilik Poseidon ile gerçek anlamda devreye dahil edilir, böylece
    //    pruning'e uğramaz. Çıktı kullanılmaz fakat kısıtlama korunur.
    component voterBinder = Poseidon(2);
    voterBinder.inputs[0] <== voter_root;
    voterBinder.inputs[1] <== voter_index;
    signal voter_binding;
    voter_binding <== voterBinder.out;
}

component main {public [poll_id, vote_commitment, voter_root, nullifier, timestamp]} = VoteProof();
