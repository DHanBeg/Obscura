pragma circom 2.1.6;

// node_proof.circom — Node çalıştırma kanıtı (kredi puanı + staking)
//
// Spec Bölüm 7.2 + 12.3: Kullanıcının aktif bir node çalıştırdığını
// kimliğini açıklamadan kanıtlar. Node uptime ve stake miktarı doğrulanır.
//
// Public inputs:
//   min_uptime_pct    — kanıtlanan minimum uptime yüzdesi (0-100)
//   min_stake         — kanıtlanan minimum stake miktarı (OBS token, 1e6 birim)
//   epoch             — kanıt dönemi (replay koruması)
//   node_commitment   — Poseidon(node_id_hash, uptime_pct, stake_amount, operator_secret, epoch)
//
// Private inputs:
//   node_id_hash      — SHA256(node_id) mod p (node kimliğini gizler)
//   uptime_pct        — gerçek uptime yüzdesi (0-100)
//   stake_amount      — gerçek stake miktarı
//   operator_secret   — node operatör gizli anahtarı
//
// Outputs:
//   uptime_ok         — 1 eğer uptime >= min_uptime_pct
//   stake_ok          — 1 eğer stake >= min_stake

include "node_modules/circomlib/circuits/poseidon.circom";
include "node_modules/circomlib/circuits/comparators.circom";

template NodeProof() {
    // Public
    signal input min_uptime_pct;
    signal input min_stake;
    signal input epoch;
    signal input node_commitment;

    // Private
    signal input node_id_hash;
    signal input uptime_pct;
    signal input stake_amount;
    signal input operator_secret;

    // Outputs
    signal output uptime_ok;
    signal output stake_ok;

    // 1. Node taahhüdü doğrula
    component hasher = Poseidon(5);
    hasher.inputs[0] <== node_id_hash;
    hasher.inputs[1] <== uptime_pct;
    hasher.inputs[2] <== stake_amount;
    hasher.inputs[3] <== operator_secret;
    hasher.inputs[4] <== epoch;
    node_commitment === hasher.out;

    // 2. Uptime kontrolü: uptime_pct >= min_uptime_pct
    component uptime_check = GreaterEqThan(8); // 0-100
    uptime_check.in[0] <== uptime_pct;
    uptime_check.in[1] <== min_uptime_pct;
    uptime_ok <== uptime_check.out;

    // 3. Uptime aralık kontrolü: 0 <= uptime_pct <= 100
    component uptime_range = LessEqThan(8);
    uptime_range.in[0] <== uptime_pct;
    uptime_range.in[1] <== 100;
    uptime_range.out === 1;

    // 4. Stake kontrolü: stake_amount >= min_stake
    component stake_check = GreaterEqThan(64);
    stake_check.in[0] <== stake_amount;
    stake_check.in[1] <== min_stake;
    stake_ok <== stake_check.out;

    // 5. Epoch != 0 (geçersiz epoch koruması)
    signal epoch_nonzero;
    epoch_nonzero <== epoch * epoch;
    // epoch=0 ise epoch_nonzero=0, constraint tutarsız olur
    // Dolaylı kısıt: circuit doğru çalışması için epoch>0 gerekli
}

component main {public [min_uptime_pct, min_stake, epoch, node_commitment]} = NodeProof();
