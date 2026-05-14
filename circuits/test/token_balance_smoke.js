// token_balance end-to-end smoke
const path = require("path");
const fs = require("fs");
const snarkjs = require("snarkjs");
const circomlibjs = require("circomlibjs");

(async () => {
  const buildDir = path.join(__dirname, "..", "build", "token_balance");
  const wasm = path.join(buildDir, "token_balance_js", "token_balance.wasm");
  const zkey = path.join(buildDir, "token_balance_final.zkey");
  const vkey = path.join(buildDir, "verification_key.json");
  for (const p of [wasm, zkey, vkey]) {
    if (!fs.existsSync(p)) throw new Error("missing: " + p);
  }

  const poseidon = await circomlibjs.buildPoseidon();
  const F = poseidon.F;

  const balance = 5000n;
  const amount = 1200n;
  const secret = 987654321n;
  const salt = 4242n;

  // balance_commitment = Poseidon(secret, balance, salt)
  const balance_commitment = F.toString(poseidon([secret, balance, salt]));
  // nullifier = Poseidon(secret, salt)
  const nullifier = F.toString(poseidon([secret, salt]));

  const inputs = {
    balance: balance.toString(),
    amount: amount.toString(),
    secret: secret.toString(),
    salt: salt.toString(),
    balance_commitment,
    nullifier,
    root: "778899", // simplified Merkle state root
    timestamp: "1700000000",
  };

  console.log("→ Generating token_balance...");
  const t0 = Date.now();
  const { proof, publicSignals } = await snarkjs.groth16.fullProve(inputs, wasm, zkey);
  console.log(`✓ Generated in ${Date.now() - t0}ms`);

  const v = JSON.parse(fs.readFileSync(vkey));
  const ok = await snarkjs.groth16.verify(v, publicSignals, proof);
  if (!ok) throw new Error("snarkjs verify failed");
  console.log("✓ snarkjs verified");

  const payload = {
    proof_json: JSON.stringify(proof),
    circuit_id: "token_balance",
    public_inputs: publicSignals,
  };
  fs.writeFileSync(path.join(__dirname, "token_balance_smoke_proof.json"), JSON.stringify(payload, null, 2));
  console.log("✓ saved test/token_balance_smoke_proof.json");
  console.log("public signals:", publicSignals);
})();
