// msg_count_proof end-to-end smoke — pattern birebir smoke.js/vote_proof_smoke.js'ten
// kopyalandı.
//
// İki bölüm:
//   1. GEÇERLİ input → fullProve → snarkjs.verify → test/msg_count_proof_smoke_proof.json
//      (backend TestVerifyGroth16_MsgCountProof bunu okur).
//   2. GEÇERSİZ input (conv_count > total_count, circuit'in "3. conv_count <= total_count
//      (tutarlılık)" hard constraint'ini — consistency.out === 1 — ihlal ediyor) →
//      fullProve'un HATA VERMESİ beklenir (witness calculator constraint'i çözemez).
//      Beklenen hatayı almazsak (yani proof üretimi BAŞARILI olursa) bu bir hata —
//      circuit'in tutarlılık kısıtı delinmiş demektir.
//
// Run: cd circuits && node test/msg_count_proof_smoke.js

const path = require("path");
const fs = require("fs");
const snarkjs = require("snarkjs");
const circomlibjs = require("circomlibjs");

async function main() {
  const buildDir = path.join(__dirname, "..", "build", "msg_count_proof");
  const wasmPath = path.join(buildDir, "msg_count_proof_js", "msg_count_proof.wasm");
  const zkeyPath = path.join(buildDir, "msg_count_proof_final.zkey");
  const vkeyPath = path.join(buildDir, "verification_key.json");

  for (const p of [wasmPath, zkeyPath, vkeyPath]) {
    if (!fs.existsSync(p)) throw new Error("missing build artifact: " + p);
  }

  const poseidon = await circomlibjs.buildPoseidon();
  const F = poseidon.F;

  function commitmentFor(total_count, conv_count, user_secret, epoch) {
    return F.toString(poseidon([total_count, conv_count, user_secret, epoch]));
  }

  // ─── 1. GEÇERLİ senaryo ─────────────────────────────────────────────────────
  const epoch = 1700000000n;
  const user_secret = 8888888888n;
  const total_count = 150n;
  const conv_count = 20n; // <= total_count, tutarlı
  const min_count = 100n; // total_count >= min_count → is_above_threshold=1

  const validCommitment = commitmentFor(total_count, conv_count, user_secret, epoch);
  const validInputs = {
    min_count: min_count.toString(),
    epoch: epoch.toString(),
    count_commitment: validCommitment,
    total_count: total_count.toString(),
    conv_count: conv_count.toString(),
    user_secret: user_secret.toString(),
  };

  console.log("→ [GEÇERLİ] Inputs:", validInputs);
  console.log("→ [GEÇERLİ] Generating proof (groth16.fullProve)");
  const t0 = Date.now();
  const { proof, publicSignals } = await snarkjs.groth16.fullProve(
    validInputs,
    wasmPath,
    zkeyPath
  );
  console.log(`✓ [GEÇERLİ] Proof generated in ${Date.now() - t0}ms`);

  const vkey = JSON.parse(fs.readFileSync(vkeyPath));
  const ok = await snarkjs.groth16.verify(vkey, publicSignals, proof);
  if (!ok) throw new Error("[GEÇERLİ] snarkjs verify FAILED — beklenen PASS");
  console.log("✓ [GEÇERLİ] snarkjs verify OK");

  const payload = {
    proof_json: JSON.stringify(proof),
    circuit_id: "msg_count_proof",
    public_inputs: publicSignals,
  };
  fs.writeFileSync(
    path.join(__dirname, "msg_count_proof_smoke_proof.json"),
    JSON.stringify(payload, null, 2)
  );
  console.log("✓ [GEÇERLİ] Saved test/msg_count_proof_smoke_proof.json");

  // ─── 2. GEÇERSİZ senaryo — conv_count > total_count (tutarlılık ihlali) ────
  const bad_total_count = 150n;
  const bad_conv_count = 200n; // > total_count — consistency.out === 1 ihlali
  const invalidCommitment = commitmentFor(bad_total_count, bad_conv_count, user_secret, epoch);
  const invalidInputs = {
    min_count: min_count.toString(),
    epoch: epoch.toString(),
    count_commitment: invalidCommitment,
    total_count: bad_total_count.toString(),
    conv_count: bad_conv_count.toString(),
    user_secret: user_secret.toString(),
  };

  console.log("\n→ [GEÇERSİZ] conv_count(200) > total_count(150) — tutarlılık ihlali, fullProve HATA VERMELİ");
  let invalidThrew = false;
  try {
    await snarkjs.groth16.fullProve(invalidInputs, wasmPath, zkeyPath);
  } catch (e) {
    invalidThrew = true;
    console.log("✓ [GEÇERSİZ] fullProve beklendiği gibi HATA VERDİ:", e.message);
  }
  if (!invalidThrew) {
    throw new Error(
      "[GEÇERSİZ] fullProve HATA VERMEDİ — conv_count > total_count kısıtı DELİNMİŞ (KRİTİK)"
    );
  }

  console.log("\n✓✓ msg_count_proof smoke: GEÇERLİ PASS + GEÇERSİZ REDDEDİLDİ");
  process.exit(0);
}

main().catch((e) => {
  console.error("✗", e);
  process.exit(1);
});
