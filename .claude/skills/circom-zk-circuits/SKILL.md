---
name: circom-zk-circuits
description: Build, test, contribute trusted setup, and distribute Circom 2.1.6 circuits with snarkjs Groth16. Use when writing or compiling any *.circom file or generating .wasm/.zkey artifacts for Obscura.
---

# Circom ZK Circuits — Build & Distribute

## When to use

- Writing a new circuit
- Modifying an existing circuit
- Generating new .wasm / .zkey artifacts after circuit changes
- Running trusted setup ceremony

## Prerequisites

```bash
# Circom (compiler)
git clone https://github.com/iden3/circom.git /tmp/circom
cd /tmp/circom && cargo build --release
sudo cp target/release/circom /usr/local/bin/

# snarkjs (proving + verification)
npm install -g snarkjs

# circomlib (primitives)
cd D:/obscura/circuits && npm install circomlib
```

## Standard circuit template

```circom
pragma circom 2.1.6;

include "circomlib/poseidon.circom";
include "circomlib/comparators.circom";

template MyCircuit() {
    // Private inputs
    signal input secret;

    // Public inputs
    signal input commitment;

    // Constraints
    component hasher = Poseidon(1);
    hasher.inputs[0] <== secret;
    commitment === hasher.out;
}

component main {public [commitment]} = MyCircuit();
```

## Build pipeline (single circuit)

```bash
cd D:/obscura/circuits
NAME=my_circuit
mkdir -p build

# 1. Compile
circom $NAME.circom --r1cs --wasm --sym -l node_modules -o build/

# 2. Powers of Tau (one-time per power; share across circuits)
# Power = ceil(log2(constraint_count)) + 1
# Power 14 → up to ~16k constraints; power 16 → ~64k; power 18 → ~262k
POWER=14
if [ ! -f "build/pot${POWER}_final.ptau" ]; then
  snarkjs powersoftau new bn128 $POWER build/pot${POWER}_0000.ptau -v
  snarkjs powersoftau contribute build/pot${POWER}_0000.ptau build/pot${POWER}_0001.ptau \
    --name="OBS contrib 1" -v -e="$(head -c 32 /dev/urandom | xxd -p)"
  snarkjs powersoftau prepare phase2 build/pot${POWER}_0001.ptau build/pot${POWER}_final.ptau -v
fi

# 3. Phase 2 setup (per circuit)
snarkjs groth16 setup build/$NAME.r1cs build/pot${POWER}_final.ptau build/${NAME}_0000.zkey

# 4. Contribute (each contributor adds entropy)
snarkjs zkey contribute build/${NAME}_0000.zkey build/${NAME}_final.zkey \
  --name="contributor name" -v -e="$(head -c 32 /dev/urandom | xxd -p)"

# 5. Export verification key
snarkjs zkey export verificationkey build/${NAME}_final.zkey build/${NAME}_vkey.json

# 6. Distribute artifacts
mkdir -p ../frontend/public/zk ../mobile/assets/zk ../backend/internal/zk/keys
cp build/${NAME}_js/${NAME}.wasm ../frontend/public/zk/
cp build/${NAME}_final.zkey ../frontend/public/zk/${NAME}_final.zkey
cp build/${NAME}_js/${NAME}.wasm ../mobile/assets/zk/
cp build/${NAME}_final.zkey ../mobile/assets/zk/${NAME}_final.zkey
cp build/${NAME}_vkey.json ../backend/internal/zk/keys/${NAME}_vkey.json
```

## Test a circuit

Create `circuits/test/$NAME.test.js`:

```js
const { expect } = require("chai");
const wasm_tester = require("circom_tester").wasm;

describe("$NAME", () => {
  it("accepts valid input", async () => {
    const circuit = await wasm_tester("./$NAME.circom");
    const witness = await circuit.calculateWitness({
      secret: "123",
      commitment: "<computed>",
    });
    await circuit.checkConstraints(witness);
  });

  it("rejects invalid input", async () => {
    const circuit = await wasm_tester("./$NAME.circom");
    await expect(
      circuit.calculateWitness({ secret: "123", commitment: "wrong" })
    ).to.be.rejected;
  });
});
```

Run: `cd circuits && npx mocha test/`

## Constraint budget

- Mobile-friendly proof: < 100k constraints (proves in ~3s on phone)
- Server-only proof: < 1M constraints
- Recursive proof aggregation: any size, but adds ~50k overhead

Print constraint count after compile:
```bash
snarkjs r1cs info build/$NAME.r1cs
```

## Browser proof generation (frontend/lib/zk.ts)

```ts
const snarkjs = await import("snarkjs");
const { proof, publicSignals } = await snarkjs.groth16.fullProve(
  input,
  "/zk/my_circuit.wasm",
  "/zk/my_circuit_final.zkey"
);
```

## Backend verification (Go)

```go
import "github.com/iden3/go-rapidsnark/verifier"

func VerifyProof(proof, vkey, publicSignals []byte) (bool, error) {
    return verifier.VerifyGroth16(proof, vkey, publicSignals)
}
```

## Trusted setup ceremony rules

- Production circuits: minimum 5 contributors from 5 organizations
- Each contributor uses TRULY random entropy (hardware RNG, not /dev/urandom)
- Each contribution attestation published with hash + name
- Final zkey hash announced publicly
- Toxic waste destroyed (RAM only, no disk write of contribution randomness)

## Common errors

- **"Non-quadratic constraint"** — using `*` between two non-constant signals; rewrite via intermediate signals
- **"Pragma version mismatch"** — install matching circom version (`circom --version`)
- **"Memory error during witness gen"** — circuit too large for browser, move to server or split circuit
- **"Invalid proof"** — public inputs in wrong order; check circuit `public [...]` declaration
- **".wasm not found"** — copy step missed; rerun distribute step

## Spec-required circuits (Obscura)

See `docs/spec/obscura_spec_v3.txt` Bölüm 17:

- 17.1 `identity_proof.circom`
- 17.2 `credit_threshold.circom`
- 17.3 `token_balance.circom`
- 17.4 `vote_proof.circom`
- 17.5 `storage_proof.circom`

Plus credit components (Bölüm 7.1): age, activity, msg_count, spam_victim, fraud, contribution, node, endorsement, streak.
