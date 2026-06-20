/**
 * Obscura ZK Proof Client
 *
 * Browser'da Circom/snarkjs ile Groth16 kanıt üretimi.
 * WASM dosyaları public/ klasöründen serve edilir.
 *
 * Kullanım:
 *   const proof = await proveCredit({ actualScore: 85, threshold: 70 });
 *   await submitZKProof(proof);
 */

import { api } from "@/lib/api";

// ─── Tipler ────────────────────────────────────────────────────────────────────

export interface ZKProof {
  circuit: string;
  proof: object;
  publicSignals: string[];
  commitment?: string;
  prover_did?: string;
  proof_type?: string;
  timestamp?: number;
}

// ─── snarkjs lazy import ──────────────────────────────────────────────────────

async function getSnarkjs() {
  // snarkjs sadece istemcide yüklenir (SSR'da Node.js'te çalışmaz)
  if (typeof window === "undefined") throw new Error("ZK kanıtlama sadece tarayıcıda çalışır");
  const sjs = await import("snarkjs" as any).catch(() => null);
  if (!sjs) throw new Error("snarkjs yüklenemedi — npm install snarkjs gerekli");
  return sjs;
}

// ─── Credit Threshold Kanıtı ──────────────────────────────────────────────────

/**
 * Kredi eşiği kanıtı üret
 * "Puanım threshold'dan büyük, ama tam değerimi açıklamıyorum"
 */
export async function proveCredit(params: {
  actualScore: number;
  threshold: number;
  userDID: string;
}): Promise<ZKProof> {
  const { actualScore, threshold, userDID } = params;

  if (actualScore < threshold) {
    throw new Error(`Kanıt üretilemez: puan (${actualScore}) < eşik (${threshold})`);
  }

  // Offset: +20 (negatif puanları 0..120 aralığına taşı)
  const scoreWithOffset = actualScore + 20;
  const thresholdWithOffset = threshold + 20;

  // Rastgele tuz
  const saltBytes = crypto.getRandomValues(new Uint8Array(32));
  const salt = BigInt("0x" + Array.from(saltBytes).map(b => b.toString(16).padStart(2, "0")).join(""));

  // Poseidon hash (circomlibjs ile)
  const commitment = await poseidonHash([BigInt(scoreWithOffset), salt]);

  const input = {
    actual_score: scoreWithOffset.toString(),
    salt: salt.toString(),
    threshold: thresholdWithOffset.toString(),
    commitment: commitment,
  };

  const snarkjs = await getSnarkjs();
  const { proof, publicSignals } = await snarkjs.groth16.fullProve(
    input,
    "/zk/credit_threshold.wasm",
    "/zk/credit_threshold_final.zkey"
  );

  return {
    circuit: "credit_threshold",
    proof_type: "credit_threshold",
    proof,
    publicSignals,
    commitment: commitment.toString(),
    prover_did: userDID,
    timestamp: Date.now(),
  };
}

// ─── Identity Kanıtı ──────────────────────────────────────────────────────────

/**
 * Kimlik kanıtı üret
 * "Bu DID benim, gizli anahtarımı açıklamıyorum"
 */
export async function proveIdentity(params: {
  privateKeyHex: string;
  didHashHex: string;
  userDID: string;
}): Promise<ZKProof> {
  const { privateKeyHex, didHashHex, userDID } = params;

  const pk = BigInt("0x" + privateKeyHex);
  const did = BigInt("0x" + didHashHex);

  const nonceBytes = crypto.getRandomValues(new Uint8Array(16));
  const nonce = BigInt("0x" + Array.from(nonceBytes).map(b => b.toString(16).padStart(2, "0")).join(""));

  const commitment = await poseidonHash([pk, nonce]);
  const timestamp = Math.floor(Date.now() / 60000); // Dakikalık granularite (epoch)

  const input = {
    private_key: pk.toString(),
    nonce: nonce.toString(),
    did_hash: did.toString(),
    timestamp: timestamp.toString(),
    commitment: commitment.toString(),
  };

  const snarkjs = await getSnarkjs();
  const { proof, publicSignals } = await snarkjs.groth16.fullProve(
    input,
    "/zk/identity_proof.wasm",
    "/zk/identity_proof_final.zkey"
  );

  return {
    circuit: "identity_proof",
    proof_type: "identity",
    proof,
    publicSignals,
    commitment: commitment.toString(),
    prover_did: userDID,
    timestamp: Date.now(),
  };
}

// ─── Backend'e Gönder ─────────────────────────────────────────────────────────

/**
 * Üretilen kanıtı backend'e gönder ve doğrulat
 */
export async function submitZKProof(zkProof: ZKProof): Promise<{ verified: boolean; id: string }> {
  const proofJSON = JSON.stringify({
    proof_type: zkProof.proof_type || zkProof.circuit,
    proof: zkProof.proof,
    public_signals: zkProof.publicSignals,
    commitment: zkProof.commitment,
    prover_did: zkProof.prover_did,
    timestamp: zkProof.timestamp,
  });

  return api.verifyZKProof?.({
    proof_json: proofJSON,
    circuit_id: zkProof.circuit,
    public_inputs: zkProof.publicSignals,
  });
}

// ─── Hesap Yaşı Kanıtı ────────────────────────────────────────────────────────

export async function proveAge(params: {
  createdEpoch: number;
  currentEpoch: number;
  minAgeDays: number;
  accountSecret: bigint;
}): Promise<ZKProof> {
  const { createdEpoch, currentEpoch, minAgeDays, accountSecret } = params;
  const commitment = await poseidonHash([BigInt(createdEpoch), accountSecret]);
  const input = {
    min_age_days: minAgeDays.toString(),
    current_epoch: currentEpoch.toString(),
    age_commitment: commitment,
    created_epoch: createdEpoch.toString(),
    account_secret: accountSecret.toString(),
  };
  const snarkjs = await getSnarkjs();
  const { proof, publicSignals } = await snarkjs.groth16.fullProve(
    input, "/zk/age_proof.wasm", "/zk/age_proof_final.zkey"
  );
  return { circuit: "age_proof", proof_type: "age_proof", proof, publicSignals, commitment, timestamp: Date.now() };
}

// ─── Aktivite Kanıtı ──────────────────────────────────────────────────────────

export async function proveActivity(params: {
  activeDays: number;
  msgCount: number;
  minActiveDays: number;
  minMsgCount: number;
  userSecret: bigint;
}): Promise<ZKProof> {
  const { activeDays, msgCount, minActiveDays, minMsgCount, userSecret } = params;
  const epoch = Math.floor(Date.now() / 86400000);
  const root = await poseidonHash([BigInt(activeDays), BigInt(msgCount), userSecret, BigInt(epoch)]);
  const input = {
    min_active_days: minActiveDays.toString(),
    min_msg_count: minMsgCount.toString(),
    epoch: epoch.toString(),
    activity_root: root,
    active_days: activeDays.toString(),
    msg_count: msgCount.toString(),
    user_secret: userSecret.toString(),
  };
  const snarkjs = await getSnarkjs();
  const { proof, publicSignals } = await snarkjs.groth16.fullProve(
    input, "/zk/activity_proof.wasm", "/zk/activity_proof_final.zkey"
  );
  return { circuit: "activity_proof", proof_type: "activity_proof", proof, publicSignals, commitment: root, timestamp: Date.now() };
}

// ─── Mesaj Sayısı Kanıtı ──────────────────────────────────────────────────────

export async function proveMsgCount(params: {
  totalCount: number;
  convCount: number;
  minCount: number;
  userSecret: bigint;
}): Promise<ZKProof> {
  const { totalCount, convCount, minCount, userSecret } = params;
  const epoch = Math.floor(Date.now() / 86400000);
  const commitment = await poseidonHash([BigInt(totalCount), BigInt(convCount), userSecret, BigInt(epoch)]);
  const input = {
    min_count: minCount.toString(),
    epoch: epoch.toString(),
    count_commitment: commitment,
    total_count: totalCount.toString(),
    conv_count: convCount.toString(),
    user_secret: userSecret.toString(),
  };
  const snarkjs = await getSnarkjs();
  const { proof, publicSignals } = await snarkjs.groth16.fullProve(
    input, "/zk/msg_count_proof.wasm", "/zk/msg_count_proof_final.zkey"
  );
  return { circuit: "msg_count_proof", proof_type: "msg_count_proof", proof, publicSignals, commitment, timestamp: Date.now() };
}

// ─── Node Kanıtı ──────────────────────────────────────────────────────────────

export async function proveNode(params: {
  nodeIdHash: bigint;
  uptimePct: number;
  stakeAmount: number;
  minUptimePct: number;
  minStake: number;
  operatorSecret: bigint;
}): Promise<ZKProof> {
  const { nodeIdHash, uptimePct, stakeAmount, minUptimePct, minStake, operatorSecret } = params;
  const epoch = Math.floor(Date.now() / 86400000);
  const commitment = await poseidonHash([nodeIdHash, BigInt(uptimePct), BigInt(stakeAmount), operatorSecret, BigInt(epoch)]);
  const input = {
    min_uptime_pct: minUptimePct.toString(),
    min_stake: minStake.toString(),
    epoch: epoch.toString(),
    node_commitment: commitment,
    node_id_hash: nodeIdHash.toString(),
    uptime_pct: uptimePct.toString(),
    stake_amount: stakeAmount.toString(),
    operator_secret: operatorSecret.toString(),
  };
  const snarkjs = await getSnarkjs();
  const { proof, publicSignals } = await snarkjs.groth16.fullProve(
    input, "/zk/node_proof.wasm", "/zk/node_proof_final.zkey"
  );
  return { circuit: "node_proof", proof_type: "node_proof", proof, publicSignals, commitment, timestamp: Date.now() };
}

// ─── Onay Kanıtı ──────────────────────────────────────────────────────────────

export async function proveEndorsement(params: {
  endorsementCount: number;
  minEndorsements: number;
  endorsedDIDHash: bigint;
  userSecret: bigint;
}): Promise<ZKProof> {
  const { endorsementCount, minEndorsements, endorsedDIDHash, userSecret } = params;
  const epoch = Math.floor(Date.now() / 86400000);
  // Circuit enforces: endorsement_root === Poseidon(endorsed_hash, endorsement_count, user_secret, epoch)
  const endorsementRoot = await poseidonHash([endorsedDIDHash, BigInt(endorsementCount), userSecret, BigInt(epoch)]);
  const input = {
    min_endorsements: minEndorsements.toString(),
    endorsed_hash: endorsedDIDHash.toString(),
    endorsement_root: endorsementRoot.toString(),
    epoch: epoch.toString(),
    endorsement_count: endorsementCount.toString(),
    user_secret: userSecret.toString(),
  };
  const snarkjs = await getSnarkjs();
  const { proof, publicSignals } = await snarkjs.groth16.fullProve(
    input, "/zk/endorsement_proof.wasm", "/zk/endorsement_proof_final.zkey"
  );
  return { circuit: "endorsement_proof", proof_type: "endorsement_proof", proof, publicSignals, timestamp: Date.now() };
}

// ─── Streak Kanıtı ────────────────────────────────────────────────────────────

export async function proveStreak(params: {
  streakStart: number;
  streakLength: number;
  minStreakDays: number;
  userSecret: bigint;
}): Promise<ZKProof> {
  const { streakStart, streakLength, minStreakDays, userSecret } = params;
  const currentEpoch = Math.floor(Date.now() / 86400000);
  const commitment = await poseidonHash([BigInt(streakStart), BigInt(streakLength), userSecret, BigInt(currentEpoch)]);
  const input = {
    min_streak_days: minStreakDays.toString(),
    current_epoch: currentEpoch.toString(),
    streak_commitment: commitment,
    streak_start: streakStart.toString(),
    streak_length: streakLength.toString(),
    user_secret: userSecret.toString(),
  };
  const snarkjs = await getSnarkjs();
  const { proof, publicSignals } = await snarkjs.groth16.fullProve(
    input, "/zk/streak_proof.wasm", "/zk/streak_proof_final.zkey"
  );
  return { circuit: "streak_proof", proof_type: "streak_proof", proof, publicSignals, commitment, timestamp: Date.now() };
}

// ─── Konum Kanıtı ─────────────────────────────────────────────────────────────

export async function proveLocation(params: {
  latRaw: number;
  lonRaw: number;
  regionLatMin: number;
  regionLatMax: number;
  regionLonMin: number;
  regionLonMax: number;
  deviceSecret: bigint;
}): Promise<ZKProof> {
  const { latRaw, lonRaw, regionLatMin, regionLatMax, regionLonMin, regionLonMax, deviceSecret } = params;
  const epoch = Math.floor(Date.now() / 3600000);
  const gpsTimestamp = Math.floor(Date.now() / 1000);
  // Circuit: Poseidon(lat_raw, lon_raw, device_secret, epoch) — 4 inputs, epoch must match circuit
  const commitment = await poseidonHash([BigInt(latRaw), BigInt(lonRaw), deviceSecret, BigInt(epoch)]);
  const input = {
    region_lat_min: regionLatMin.toString(),
    region_lat_max: regionLatMax.toString(),
    region_lon_min: regionLonMin.toString(),
    region_lon_max: regionLonMax.toString(),
    epoch: epoch.toString(),
    location_commitment: commitment,
    lat_raw: latRaw.toString(),
    lon_raw: lonRaw.toString(),
    gps_timestamp: gpsTimestamp.toString(),
    device_secret: deviceSecret.toString(),
  };
  const snarkjs = await getSnarkjs();
  const { proof, publicSignals } = await snarkjs.groth16.fullProve(
    input, "/zk/location_proof.wasm", "/zk/location_proof_final.zkey"
  );
  return { circuit: "location_proof", proof_type: "location_proof", proof, publicSignals, commitment, timestamp: Date.now() };
}

// ─── Token Balance (Shielded) Kanıtı ─────────────────────────────────────────

/**
 * Shielded token balance ZK kanıtı üret (token_balance.circom, depth=20).
 *
 * Prover, bir note'un mevcut Merkle tree'ye dahil olduğunu ve balance >= amount
 * olduğunu secret/path açıklamadan kanıtlar.
 *
 * @param leafIndex   - Harcanacak note'un shielded tree'deki yaprak indeksi
 * @param balance     - Note'un gerçek bakiyesi (bigint)
 * @param amount      - Transfer miktarı (bigint)
 * @param secret      - Kullanıcı gizli anahtarı (bigint)
 * @param salt        - Not'a bağlı rastgele tuz (bigint)
 */
export async function proveTokenBalance(params: {
  leafIndex: number;
  balance: bigint;
  amount: bigint;
  secret: bigint;
  salt: bigint;
}): Promise<ZKProof> {
  const { leafIndex, balance, amount, secret, salt } = params;

  // Fetch Merkle proof from backend
  const { api } = await import("@/lib/api");
  const merkle = await api.walletMerkleProof(leafIndex);

  const timestamp = Math.floor(Date.now() / 1000);

  // Compute public signals client-side so we can pass them with the proof
  const balanceCommitment = await poseidonHash([secret, balance, salt]);
  const nullifier = await poseidonHash([secret, salt]);

  const input = {
    // Private inputs
    balance: balance.toString(),
    amount: amount.toString(),
    secret: secret.toString(),
    salt: salt.toString(),
    path_elements: merkle.path_elements,
    path_indices: merkle.path_indices.map(String),
    // Public inputs
    balance_commitment: balanceCommitment,
    nullifier,
    root: merkle.root,
    timestamp: timestamp.toString(),
    leaf_index: leafIndex.toString(),
  };

  const snarkjs = await getSnarkjs();
  const { proof, publicSignals } = await snarkjs.groth16.fullProve(
    input,
    "/zk/token_balance.wasm",
    "/zk/token_balance_final.zkey"
  );

  return {
    circuit: "token_balance",
    proof_type: "token_balance",
    proof,
    publicSignals,
    commitment: balanceCommitment,
    timestamp: Date.now(),
  };
}

// ─── Shield Note Oluşturma ────────────────────────────────────────────────────

/**
 * Yeni bir shielded note için commitment = Poseidon(secret, balance, salt) üret.
 * secret ve salt dışarıya çıkmaz — caller localStorage'da saklamalı.
 *
 * Returns: { commitment, secret, salt } — secret/salt yedeklenmezse note harcanamaz.
 */
export async function createShieldNote(balanceStr: string): Promise<{
  commitment: string;
  secret: string;
  salt: string;
}> {
  const balanceWei = BigInt(Math.round(parseFloat(balanceStr) * 1e9)); // 9 decimal OBS
  const secretBytes = crypto.getRandomValues(new Uint8Array(31)); // 248-bit < BN254 field
  const saltBytes   = crypto.getRandomValues(new Uint8Array(31));
  const secret = BigInt("0x" + Array.from(secretBytes).map(b => b.toString(16).padStart(2, "0")).join(""));
  const salt   = BigInt("0x" + Array.from(saltBytes).map(b => b.toString(16).padStart(2, "0")).join(""));
  const commitment = await poseidonHash([secret, balanceWei, salt]);
  return { commitment, secret: secret.toString(), salt: salt.toString() };
}

// ─── Yardımcılar ──────────────────────────────────────────────────────────────

async function poseidonHash(inputs: bigint[]): Promise<string> {
  // circomlibjs lazy import
  try {
    const { buildPoseidon } = await import("circomlibjs" as any);
    const poseidon = await buildPoseidon();
    const hash = poseidon(inputs.map(n => n));
    return poseidon.F.toString(hash);
  } catch {
    // Fallback: circomlibjs yüklü değilse SHA-256 kullan (test amaçlı)
    const data = inputs.map(n => n.toString()).join("|");
    const buf = new TextEncoder().encode(data);
    const hashBuf = await crypto.subtle.digest("SHA-256", buf);
    return BigInt("0x" + Array.from(new Uint8Array(hashBuf))
      .map(b => b.toString(16).padStart(2, "0")).join("")).toString();
  }
}
