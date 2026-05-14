// vote_proof end-to-end smoke
const path = require("path");
const fs = require("fs");
const snarkjs = require("snarkjs");
const circomlibjs = require("circomlibjs");

(async () => {
  const buildDir = path.join(__dirname, "..", "build", "vote_proof");
  const wasm = path.join(buildDir, "vote_proof_js", "vote_proof.wasm");
  const zkey = path.join(buildDir, "vote_proof_final.zkey");
  const vkey = path.join(buildDir, "verification_key.json");
  for (const p of [wasm, zkey, vkey]) {
    if (!fs.existsSync(p)) throw new Error("missing: " + p);
  }

  const poseidon = await circomlibjs.buildPoseidon();
  const F = poseidon.F;

  const voter_secret = 555666777n;
  const vote_choice = 1n; // {0,1,2,3}
  const voter_index = 7n;
  const poll_id = 314159n;

  // vote_commitment = Poseidon(vote_choice, voter_secret)
  const vote_commitment = F.toString(poseidon([vote_choice, voter_secret]));
  // nullifier = Poseidon(voter_secret, poll_id)
  const nullifier = F.toString(poseidon([voter_secret, poll_id]));

  const inputs = {
    voter_secret: voter_secret.toString(),
    vote_choice: vote_choice.toString(),
    voter_index: voter_index.toString(),
    poll_id: poll_id.toString(),
    vote_commitment,
    voter_root: "112233", // simplified voter Merkle root
    nullifier,
    timestamp: "1700000000",
  };

  console.log("→ Generating vote_proof...");
  const t0 = Date.now();
  const { proof, publicSignals } = await snarkjs.groth16.fullProve(inputs, wasm, zkey);
  console.log(`✓ Generated in ${Date.now() - t0}ms`);

  const v = JSON.parse(fs.readFileSync(vkey));
  const ok = await snarkjs.groth16.verify(v, publicSignals, proof);
  if (!ok) throw new Error("snarkjs verify failed");
  console.log("✓ snarkjs verified");

  const payload = {
    proof_json: JSON.stringify(proof),
    circuit_id: "vote_proof",
    public_inputs: publicSignals,
  };
  fs.writeFileSync(path.join(__dirname, "vote_proof_smoke_proof.json"), JSON.stringify(payload, null, 2));
  console.log("✓ saved test/vote_proof_smoke_proof.json");
  console.log("public signals:", publicSignals);
})();
