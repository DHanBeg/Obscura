// storage_proof end-to-end smoke
const path = require("path");
const fs = require("fs");
const snarkjs = require("snarkjs");
const circomlibjs = require("circomlibjs");

(async () => {
  const buildDir = path.join(__dirname, "..", "build", "storage_proof");
  const wasm = path.join(buildDir, "storage_proof_js", "storage_proof.wasm");
  const zkey = path.join(buildDir, "storage_proof_final.zkey");
  const vkey = path.join(buildDir, "verification_key.json");
  for (const p of [wasm, zkey, vkey]) {
    if (!fs.existsSync(p)) throw new Error("missing: " + p);
  }

  const poseidon = await circomlibjs.buildPoseidon();
  const F = poseidon.F;

  const data_hash = 999111n;
  const node_secret = 12345n;
  const data_commitment = F.toString(poseidon([data_hash, node_secret]));

  const inputs = {
    data_hash: data_hash.toString(),
    node_secret: node_secret.toString(),
    data_commitment,
    timestamp: "1700000000",
    ttl: "2592000",
    shard_id: "42",
  };

  console.log("→ Generating storage_proof...");
  const t0 = Date.now();
  const { proof, publicSignals } = await snarkjs.groth16.fullProve(inputs, wasm, zkey);
  console.log(`✓ Generated in ${Date.now() - t0}ms`);

  const v = JSON.parse(fs.readFileSync(vkey));
  const ok = await snarkjs.groth16.verify(v, publicSignals, proof);
  if (!ok) throw new Error("snarkjs verify failed");
  console.log("✓ snarkjs verified");

  const payload = {
    proof_json: JSON.stringify(proof),
    circuit_id: "storage_proof",
    public_inputs: publicSignals,
  };
  fs.writeFileSync(path.join(__dirname, "storage_smoke_proof.json"), JSON.stringify(payload, null, 2));
  console.log("✓ saved test/storage_smoke_proof.json");
  console.log("public signals:", publicSignals);
})();
