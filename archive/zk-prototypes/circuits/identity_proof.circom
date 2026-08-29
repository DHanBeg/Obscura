pragma circom 2.1.6;

/*
 * Obscura — Identity Proof Circuit
 *
 * Kanıt: "Ben bu DID'e sahip private key'i biliyorum"
 *
 * Private inputs (witness):
 *   - private_key[2]:  Alice'in X25519 private key (iki 128-bit limb)
 *   - nonce[2]:        Replay koruması için nonce
 *
 * Public inputs:
 *   - did_hash[2]:     SHA-256(X25519 public key) ilk 128 bit (iki 128-bit limb)
 *   - timestamp:       Unix timestamp (dakika bazlı, replay window = 5 dk)
 *   - commitment:      Pedersen(private_key, nonce)
 *
 * Kanıt:
 *   1. private_key'den public key türet (Scalar Mult üzerinde Baby-Jubjub)
 *   2. SHA-256(public_key) == did_hash
 *   3. commitment == Pedersen(private_key, nonce)
 *
 * NOT: X25519 (Curve25519) Circom'da uygulamak pahalı.
 *      Pratik implementasyon: Baby-Jubjub üzerinde keypair + Groth16.
 *      Bu devre Baby-Jubjub'ı kullanır (zk-SNARK dostu).
 *
 * Gerçek dağıtım için:
 *   1. circom identity_proof.circom --r1cs --wasm --sym
 *   2. snarkjs groth16 setup identity_proof.r1cs pot_final.ptau identity_proof_0000.zkey
 *   3. snarkjs zkey contribute ...
 *   4. snarkjs zkey export verificationkey
 */

include "circomlib/circuits/poseidon.circom";
include "circomlib/circuits/babyjub.circom";
include "circomlib/circuits/bitify.circom";
include "circomlib/circuits/comparators.circom";

/*
 * Poseidon Hash (ZK-dostu hash fonksiyonu — SHA-256'dan 100x daha verimli)
 * Obscura'da DID türetimi için SHA-256 yerine Poseidon kullanılır.
 */
template IdentityProof() {
    // ── Private inputs (witness) ─────────────────────────────────────────────
    signal input private_key;        // Baby-Jubjub private scalar
    signal input nonce;              // 128-bit replay nonce

    // ── Public inputs ────────────────────────────────────────────────────────
    signal input did_hash;           // Poseidon(public_key) — DID'in on-chain hash'i
    signal input timestamp;          // Unix timestamp / 60 (dakika)
    signal input commitment;         // Poseidon(private_key, nonce)

    // ── Outputs ──────────────────────────────────────────────────────────────
    signal output valid;             // Her zaman 1 (constraint'ler kanıtı zorunlar)

    // ── CONSTRAINT 1: Commitment doğrulama ──────────────────────────────────
    // commitment == Poseidon(private_key, nonce)
    component commit_hasher = Poseidon(2);
    commit_hasher.inputs[0] <== private_key;
    commit_hasher.inputs[1] <== nonce;
    commit_hasher.out === commitment;

    // ── CONSTRAINT 2: Public key türetme (Baby-Jubjub scalar mult) ──────────
    // (Basitleştirilmiş: tam implementasyonda Edwards curve kullanılır)
    component pub_key_gen = BabyPbk();
    pub_key_gen.in <== private_key;

    // ── CONSTRAINT 3: DID hash doğrulama ─────────────────────────────────────
    // did_hash == Poseidon(pub_key_x, pub_key_y)
    component did_hasher = Poseidon(2);
    did_hasher.inputs[0] <== pub_key_gen.Ax;
    did_hasher.inputs[1] <== pub_key_gen.Ay;
    did_hasher.out === did_hash;

    // ── CONSTRAINT 4: Timestamp geçerlilik penceresi ─────────────────────────
    // Timestamp, şu andan en fazla 5 dakika önce olmalı (replay önleme)
    // Bu constraint off-chain doğrulanır (circuit timestamp'i bilmez)
    // On-chain: verifier contract timestamp kontrolü yapar
    _ <== timestamp;  // Public input olarak kabul et

    // ── OUTPUT ───────────────────────────────────────────────────────────────
    valid <== 1;
}

component main {public [did_hash, timestamp, commitment]} = IdentityProof();
