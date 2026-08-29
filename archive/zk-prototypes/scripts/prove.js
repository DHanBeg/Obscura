#!/usr/bin/env node
/**
 * Obscura ZK Proof Generator
 *
 * Kullanım:
 *   node prove.js identity    --key <private_key_hex> --did <did_hash_hex>
 *   node prove.js credit      --score <actual> --threshold <min>
 *   node prove.js token       --balance <bal> --amount <amt>
 *
 * Çıktı: JSON proof (stdout)
 */

const snarkjs = require("snarkjs");
const path = require("path");
const crypto = require("crypto");

const KEYS_DIR = path.join(__dirname, "../keys");
const BUILD_DIR = path.join(__dirname, "../build");

// Poseidon hash (circomlib uyumlu — BN254 alanında)
// Gerçekte: poseidon.js kütüphanesini kullan
async function poseidonHash(inputs) {
    const { buildPoseidon } = require("circomlibjs");
    const poseidon = await buildPoseidon();
    const hash = poseidon(inputs);
    return poseidon.F.toString(hash);
}

// ─── Identity Proof ───────────────────────────────────────────────────────────
async function proveIdentity({ privateKey, didHash }) {
    const nonce = BigInt("0x" + crypto.randomBytes(16).toString("hex"));
    const pk = BigInt("0x" + privateKey);
    const did = BigInt("0x" + didHash);

    // Commitment: Poseidon(privateKey, nonce)
    const commitment = await poseidonHash([pk, nonce]);

    const input = {
        private_key: pk.toString(),
        nonce: nonce.toString(),
        did_hash: did.toString(),
        timestamp: Math.floor(Date.now() / 60000).toString(),
        commitment: commitment,
    };

    console.error("📐 Witness hesaplanıyor...");
    const { proof, publicSignals } = await snarkjs.groth16.fullProve(
        input,
        path.join(BUILD_DIR, "identity_proof/identity_proof_js/identity_proof.wasm"),
        path.join(KEYS_DIR, "identity_proof_final.zkey")
    );

    return {
        circuit: "identity_proof",
        proof,
        publicSignals,
        commitment,
        nonce: nonce.toString(),
    };
}

// ─── Credit Threshold Proof ───────────────────────────────────────────────────
async function proveCredit({ actualScore, threshold }) {
    // Offset: +20 (negatif puanları handle et)
    const scoreWithOffset = actualScore + 20;
    const thresholdWithOffset = threshold + 20;

    if (scoreWithOffset < thresholdWithOffset) {
        throw new Error(`Kanıt üretilemez: ${actualScore} < ${threshold} eşiği`);
    }

    const salt = BigInt("0x" + crypto.randomBytes(32).toString("hex"));
    const commitment = await poseidonHash([BigInt(scoreWithOffset), salt]);

    const input = {
        actual_score: scoreWithOffset.toString(),
        salt: salt.toString(),
        threshold: thresholdWithOffset.toString(),
        commitment: commitment,
    };

    console.error("📐 Witness hesaplanıyor...");
    const { proof, publicSignals } = await snarkjs.groth16.fullProve(
        input,
        path.join(BUILD_DIR, "credit_threshold/credit_threshold_js/credit_threshold.wasm"),
        path.join(KEYS_DIR, "credit_threshold_final.zkey")
    );

    return {
        circuit: "credit_threshold",
        proof,
        publicSignals,
        commitment,
        threshold,
    };
}

// ─── Token Transfer Proof ─────────────────────────────────────────────────────
async function proveToken({ balance, amount }) {
    if (balance < amount) {
        throw new Error(`Yetersiz bakiye: ${balance} < ${amount}`);
    }

    const senderSalt = BigInt("0x" + crypto.randomBytes(32).toString("hex"));
    const newSenderSalt = BigInt("0x" + crypto.randomBytes(32).toString("hex"));
    const newBalance = balance - amount;

    const senderCommitment = await poseidonHash([BigInt(balance), senderSalt]);
    const newSenderCommitment = await poseidonHash([BigInt(newBalance), newSenderSalt]);
    const amountHash = await poseidonHash([BigInt(amount)]);

    const input = {
        sender_balance: balance.toString(),
        amount: amount.toString(),
        sender_salt: senderSalt.toString(),
        new_sender_salt: newSenderSalt.toString(),
        sender_commitment: senderCommitment,
        new_sender_commitment: newSenderCommitment,
        amount_hash: amountHash,
    };

    console.error("📐 Witness hesaplanıyor...");
    const { proof, publicSignals } = await snarkjs.groth16.fullProve(
        input,
        path.join(BUILD_DIR, "token_transfer/token_transfer_js/token_transfer.wasm"),
        path.join(KEYS_DIR, "token_transfer_final.zkey")
    );

    return {
        circuit: "token_transfer",
        proof,
        publicSignals,
        senderCommitment,
        newSenderCommitment,
        amountHash,
    };
}

// ─── Verification ─────────────────────────────────────────────────────────────
async function verify(circuit, proof, publicSignals) {
    const vKey = require(path.join(KEYS_DIR, `${circuit}_vkey.json`));
    const valid = await snarkjs.groth16.verify(vKey, publicSignals, proof);
    return valid;
}

// ─── CLI ──────────────────────────────────────────────────────────────────────
async function main() {
    const args = process.argv.slice(2);
    const command = args[0];
    const opts = {};
    for (let i = 1; i < args.length - 1; i += 2) {
        opts[args[i].replace("--", "")] = args[i + 1];
    }

    let result;
    switch (command) {
        case "identity":
            result = await proveIdentity({
                privateKey: opts.key,
                didHash: opts.did,
            });
            break;
        case "credit":
            result = await proveCredit({
                actualScore: parseInt(opts.score),
                threshold: parseInt(opts.threshold),
            });
            break;
        case "token":
            result = await proveToken({
                balance: parseInt(opts.balance),
                amount: parseInt(opts.amount),
            });
            break;
        case "verify":
            const proof = JSON.parse(require("fs").readFileSync(opts.proof, "utf8"));
            const valid = await verify(opts.circuit, proof.proof, proof.publicSignals);
            console.log(JSON.stringify({ valid }));
            return;
        default:
            console.error("Kullanım: node prove.js [identity|credit|token|verify] [seçenekler]");
            process.exit(1);
    }

    // Doğrula
    const valid = await verify(result.circuit, result.proof, result.publicSignals);
    result.verified = valid;

    console.log(JSON.stringify(result, null, 2));
}

main().catch(err => {
    console.error("❌ Hata:", err.message);
    process.exit(1);
});
