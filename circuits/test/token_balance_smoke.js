// token_balance end-to-end smoke — depth=20 Merkle, 5 public signals
const path = require("path");
const fs = require("fs");
const snarkjs = require("snarkjs");
const circomlibjs = require("circomlibjs");

(async () => {
  const buildDir = path.join(__dirname, "..", "build", "token_balance");
  const wasm = path.join(buildDir, "token_balance_js", "token_balance.wasm");
  const zkey = path.join(buildDir, "token_balance_final.zkey");
  const vkeyPath = path.join(buildDir, "verification_key.json");
  for (const p of [wasm, zkey, vkeyPath]) {
    if (!fs.existsSync(p)) throw new Error("missing: " + p);
  }

  const poseidon = await circomlibjs.buildPoseidon();
  const F = poseidon.F;

  const balance = 5000n;
  const amount = 1200n;
  const secret = 987654321n;
  const salt = 4242n;
  const DEPTH = 20;
  const leaf_index = 0;

  // Commitments
  const balance_commitment = F.toString(poseidon([secret, balance, salt]));
  const nullifier = F.toString(poseidon([secret, salt]));

  // Merkle path: all zeros (leaf at index 0, left-most path)
  const path_elements = Array(DEPTH).fill("0");
  const path_indices = Array(DEPTH).fill("0");

  // Compute root: nodes[0] = leaf, nodes[i+1] = Poseidon(nodes[i], 0)
  let node = BigInt(balance_commitment);
  for (let i = 0; i < DEPTH; i++) {
    const h = poseidon([node, 0n]);
    node = BigInt(F.toString(h));
  }
  const root = node.toString();

  const inputs = {
    balance: balance.toString(),
    amount: amount.toString(),
    secret: secret.toString(),
    salt: salt.toString(),
    path_elements,
    path_indices,
    balance_commitment,
    nullifier,
    root,
    timestamp: "1700000000",
    leaf_index: leaf_index.toString(),
  };

  console.log("→ Generating token_balance (depth=20 Merkle)...");
  const t0 = Date.now();
  const { proof, publicSignals } = await snarkjs.groth16.fullProve(inputs, wasm, zkey);
  console.log(`✓ Generated in ${Date.now() - t0}ms`);
  console.log("public signals:", publicSignals);

  const vk = JSON.parse(fs.readFileSync(vkeyPath));
  const ok = await snarkjs.groth16.verify(vk, publicSignals, proof);
  if (!ok) throw new Error("snarkjs verify failed");
  console.log("✓ snarkjs verified");

  const payload = {
    proof_json: JSON.stringify(proof),
    circuit_id: "token_balance",
    public_inputs: publicSignals,
  };
  fs.writeFileSync(path.join(__dirname, "token_balance_smoke_proof.json"), JSON.stringify(payload, null, 2));
  console.log("✓ saved test/token_balance_smoke_proof.json");
})();
