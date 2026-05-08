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
