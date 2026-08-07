// streak_proof end-to-end smoke — pattern birebir smoke.js/vote_proof_smoke.js'ten
// kopyalandı.
//
// İki bölüm:
//   1. GEÇERLİ input → fullProve → snarkjs.verify → test/streak_proof_smoke_proof.json
//      (backend TestVerifyGroth16_StreakProof bunu okur).
//   2. GEÇERSİZ input (streak_length=400 > 365 gün üst sınırı — circuit'in
//      "5. Makul üst sınır: max 365 günlük streak" hard constraint'ini — max_check.out
//      === 1 — ihlal ediyor) → fullProve'un HATA VERMESİ beklenir. time_check ve
//      start_check'in bu senaryoda AYRICA kırılmaması için current_epoch bilerek
//      streak_end'i (=streak_start+streak_length) kapsayacak kadar büyük seçildi —
//      test SADECE 365-gün üst sınırını izole etsin diye.
//
// Run: cd circuits && node test/streak_proof_smoke.js

const path = require("path");
const fs = require("fs");
const snarkjs = require("snarkjs");
const circomlibjs = require("circomlibjs");

async function main() {
  const buildDir = path.join(__dirname, "..", "build", "streak_proof");
  const wasmPath = path.join(buildDir, "streak_proof_js", "streak_proof.wasm");
  const zkeyPath = path.join(buildDir, "streak_proof_final.zkey");
  const vkeyPath = path.join(buildDir, "verification_key.json");

  for (const p of [wasmPath, zkeyPath, vkeyPath]) {
    if (!fs.existsSync(p)) throw new Error("missing build artifact: " + p);
  }

  const poseidon = await circomlibjs.buildPoseidon();
  const F = poseidon.F;

  function commitmentFor(streak_start, streak_length, user_secret, current_epoch) {
    return F.toString(poseidon([streak_start, streak_length, user_secret, current_epoch]));
  }

  // ─── 1. GEÇERLİ senaryo ─────────────────────────────────────────────────────
  const user_secret = 9999999999n;
  const streak_start = 1000n;
  const streak_length = 10n;
  const current_epoch = 1015n; // streak_end=1010 <= 1015 ✓, start <= epoch ✓, 10<=365 ✓
  const min_streak_days = 7n; // 10 >= 7 → streak_ok=1

  const validCommitment = commitmentFor(streak_start, streak_length, user_secret, current_epoch);
  const validInputs = {
    min_streak_days: min_streak_days.toString(),
    current_epoch: current_epoch.toString(),
    streak_commitment: validCommitment,
    streak_start: streak_start.toString(),
    streak_length: streak_length.toString(),
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
    circuit_id: "streak_proof",
    public_inputs: publicSignals,
  };
  fs.writeFileSync(
    path.join(__dirname, "streak_proof_smoke_proof.json"),
    JSON.stringify(payload, null, 2)
  );
  console.log("✓ [GEÇERLİ] Saved test/streak_proof_smoke_proof.json");

  // ─── 2. GEÇERSİZ senaryo — streak_length=400 > 365 (üst sınır ihlali) ──────
  const bad_streak_length = 400n;
  const bad_current_epoch = 2000n; // streak_end=1400 <= 2000, time_check/start_check SAĞLAM kalsın
  const invalidCommitment = commitmentFor(
    streak_start,
    bad_streak_length,
    user_secret,
    bad_current_epoch
  );
  const invalidInputs = {
    min_streak_days: min_streak_days.toString(),
    current_epoch: bad_current_epoch.toString(),
    streak_commitment: invalidCommitment,
    streak_start: streak_start.toString(),
    streak_length: bad_streak_length.toString(),
    user_secret: user_secret.toString(),
  };

  console.log("\n→ [GEÇERSİZ] streak_length(400) > 365 gün üst sınırı — fullProve HATA VERMELİ");
  let invalidThrew = false;
  try {
    await snarkjs.groth16.fullProve(invalidInputs, wasmPath, zkeyPath);
  } catch (e) {
    invalidThrew = true;
    console.log("✓ [GEÇERSİZ] fullProve beklendiği gibi HATA VERDİ:", e.message);
  }
  if (!invalidThrew) {
    throw new Error(
      "[GEÇERSİZ] fullProve HATA VERMEDİ — 365 günlük üst sınır kısıtı DELİNMİŞ (KRİTİK)"
    );
  }

  console.log("\n✓✓ streak_proof smoke: GEÇERLİ PASS + GEÇERSİZ REDDEDİLDİ");
  process.exit(0);
}

main().catch((e) => {
  console.error("✗", e);
  process.exit(1);
});
