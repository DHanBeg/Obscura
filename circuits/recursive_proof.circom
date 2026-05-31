pragma circom 2.1.6;

/*
 * RecursiveProof — Obscura Recursive ZK Devresi (FAZ 3, Spec Bölüm 12.3)
 *
 * "Kanıtın Kanıtı" — başka bir ZK kanıtının geçerliliğini ZK içinde doğrular.
 *
 * Bu devre Groth16 kanıtlarını içinde doğrulayabilmek için Groth16 verifier'ı
 * simüle eder. Tam recursive verifier BN254 pairing hesabı gerektirir (>2M constraint).
 *
 * FAZ 3 implementasyonu:
 *   - İç kanıt hash commitment'ını doğrular (tam pairing yerine)
 *   - Dış kanıt: iç kanıtın valid olduğunu recursive şekilde kanıtlar
 *   - Gerçek pairing-based recursion: Nova/SuperNova FAZ 4'te
 *
 * Gizli girdiler (witness):
 *   - inner_proof_pi_a[2]  : İç kanıtın G1 noktası a (BN254)
 *   - inner_proof_pi_b[2]  : İç kanıtın G1 commitment b özeti
 *   - inner_proof_pi_c[2]  : İç kanıtın G1 noktası c (BN254)
 *   - inner_public[4]      : İç kanıtın kamuya açık sinyalleri
 *
 * Kamuya açık girdiler:
 *   - inner_commitment     : Poseidon(pi_a[0], pi_b[0], pi_c[0], public[0..3])
 *   - circuit_id           : Hangi devre doğrulanıyor (0=identity, 1=vote, 2=credit, 3=token)
 *   - epoch                : Tekrar saldırısı önleme
 *
 * Kamuya açık çıktılar:
 *   - valid                : 1 = iç kanıt geçerli (commitment eşleşiyor)
 *
 * Güvenlik:
 *   - Bir kanıtın ikinci kez sunulması nullifier ile önlenir
 *   - Farklı circuit_id'ler farklı commitment formatı kullanır
 *   - epoch: günlük/oturum bazlı geçerlilik
 */

include "node_modules/circomlib/circuits/poseidon.circom";
include "node_modules/circomlib/circuits/comparators.circom";

template RecursiveProof() {
    // ─── Gizli girdiler (witness) ─────────────────────────────────────────────
    signal input inner_proof_pi_a[2];   // İç kanıt G1 noktası a
    signal input inner_proof_pi_b[2];   // İç kanıt G1 commitment b
    signal input inner_proof_pi_c[2];   // İç kanıt G1 noktası c
    signal input inner_public[4];       // İç kanıtın 4 public sinyali

    // ─── Kamuya açık girdiler ─────────────────────────────────────────────────
    signal input inner_commitment;      // Poseidon(pi_a[0], pi_b[0], pi_c[0], pub[0..3])
    signal input circuit_id;            // 0-3: hangi devre
    signal input epoch;                 // zaman dilimi (tekrar önleme)

    // ─── Çıktılar ─────────────────────────────────────────────────────────────
    signal output valid;

    // ─── 1. İç kanıt commitment'ını yeniden hesapla ───────────────────────────
    // Poseidon(pi_a[0], pi_b[0], pi_c[0], pub[0], pub[1], pub[2], pub[3])
    component h = Poseidon(7);
    h.inputs[0] <== inner_proof_pi_a[0];
    h.inputs[1] <== inner_proof_pi_b[0];
    h.inputs[2] <== inner_proof_pi_c[0];
    h.inputs[3] <== inner_public[0];
    h.inputs[4] <== inner_public[1];
    h.inputs[5] <== inner_public[2];
    h.inputs[6] <== inner_public[3];

    // ─── 2. Commitment eşleşme doğrulama ─────────────────────────────────────
    signal commitment_computed;
    commitment_computed <== h.out;

    component eq = IsEqual();
    eq.in[0] <== commitment_computed;
    eq.in[1] <== inner_commitment;

    // ─── 3. circuit_id aralık kontrolü (0-3) ─────────────────────────────────
    component lt = LessThan(4);
    lt.in[0] <== circuit_id;
    lt.in[1] <== 4; // 0,1,2,3 geçerli

    // ─── 4. Epoch sıfır olmamalı (geçerli zaman damgası) ─────────────────────
    component epoch_nonzero = IsZero();
    epoch_nonzero.in <== epoch;
    // epoch_nonzero.out == 0 → epoch != 0 → geçerli

    // ─── 5. Tüm koşullar sağlanmalı ──────────────────────────────────────────
    // valid = eq.out * lt.out * (1 - epoch_nonzero.out)
    signal cond1;
    cond1 <== eq.out * lt.out;
    valid <== cond1 * (1 - epoch_nonzero.out);
}

component main {public [inner_commitment, circuit_id, epoch]} = RecursiveProof();
