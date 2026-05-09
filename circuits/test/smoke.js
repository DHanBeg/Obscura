// End-to-end smoke test:
// 1. Generate witness for credit_threshold
// 2. Create Groth16 proof
// 3. Verify with snarkjs (sanity)
// 4. Print proof + publicSignals JSON for backend test
//
// Run: cd circuits && node test/smoke.js

const path = require("path");
const fs = require("fs");
const snarkjs = require("snarkjs");
const circomlibjs = require("circomlibjs");

async function main() {
  const buildDir = path.join(__dirname, "..", "build");
  const wasmPath = path.join(buildDir, "credit_threshold/credit_threshold_js/credit_threshold.wasm");
  const zkeyPath = path.join(buildDir, "credit_threshold/credit_threshold_final.zkey");
  const vkeyPath = path.join(buildDir, "credit_threshold/verification_key.json");

  for (const p of [wasmPath, zkeyPath, vkeyPath]) {
    if (!fs.existsSync(p)) throw new Error("missing build artifact: " + p);
  }

  console.log("→ Building Poseidon hash for commitment");
  const poseidon = await circomlibjs.buildPoseidon();
  const F = poseidon.F;

  // Inputs
  const credit_score = 75n;
  const score_salt = 42n;
  const threshold = 60n;
  const user_hash = 12345n;
  const score_commitment_field = poseidon([credit_score, score_salt]);
  const score_commitment = F.toString(score_commitment_field);

  const inputs = {
    credit_score: credit_score.toString(),
    score_salt: score_salt.toString(),
    threshold: threshold.toString(),
    score_commitment,
    user_hash: user_hash.toString(),
  };

  console.log("→ Inputs:", inputs);

  console.log("→ Generating proof (groth16.fullProve)");
  const t0 = Date.now();
  const { proof, publicSignals } = await snarkjs.groth16.fullProve(
    inputs,
    wasmPath,
    zkeyPath
  );
  const dt = Date.now() - t0;
  console.log(`✓ Proof generated in ${dt}ms`);

  console.log("→ Verifying with snarkjs (sanity)");
  const vkey = JSON.parse(fs.readFileSync(vkeyPath));
  const ok = await snarkjs.groth16.verify(vkey, publicSignals, proof);
  if (!ok) throw new Error("snarkjs verify FAILED");
  console.log("✓ snarkjs verify OK");

  console.log("\n→ Proof JSON (for backend POST /v1/zk/verify):");
  const payload = {
    proof_json: JSON.stringify(proof),
    circuit_id: "credit_threshold",
    public_inputs: publicSignals,
  };
  console.log(JSON.stringify(payload, null, 2));

  // Save for backend test
  fs.writeFileSync(
    path.join(__dirname, "smoke_proof.json"),
    JSON.stringify(payload, null, 2)
  );
  console.log("\n✓ Saved test/smoke_proof.json");

  process.exit(0);
}

main().catch((e) => {
  console.error("✗", e);
  process.exit(1);
});
