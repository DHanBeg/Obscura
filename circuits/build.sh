#!/usr/bin/env bash
# ─── Obscura ZK Devre Derleme Scripti ────────────────────────────────────────
#
# Gereksinimler:
#   - circom 2.1.6+ (PATH'te)
#   - Node.js + npm (cd circuits && npm install)
#
# Kullanım:
#   bash build.sh
#
# Çıktılar (her devre için):
#   build/<circuit>/
#     ├── <circuit>.r1cs
#     ├── <circuit>_js/<circuit>.wasm
#     ├── <circuit>_final.zkey
#     ├── verification_key.json
#     └── <circuit>_verifier.sol  (opsiyonel)
#
# Sonra dağıtım: bash distribute.sh

set -e

CIRCUITS_DIR="$(dirname "$0")"
BUILD_DIR="$CIRCUITS_DIR/build"
PTAU_FILE="$BUILD_DIR/pot14_final.ptau"
SNARKJS="$CIRCUITS_DIR/node_modules/.bin/snarkjs"
NODE_MODULES="$CIRCUITS_DIR/node_modules"

# Cross-platform snarkjs path
if [ ! -f "$SNARKJS" ] && [ -f "$SNARKJS.cmd" ]; then
    SNARKJS="$SNARKJS.cmd"
fi
if [ ! -f "$SNARKJS" ]; then
    SNARKJS="npx snarkjs"
fi

echo "🔧 Obscura ZK devre derleme başlıyor..."

# circom kontrol
if ! command -v circom >/dev/null 2>&1; then
    echo "✗ circom bulunamadı. Kurulum:"
    echo "  Windows: https://github.com/iden3/circom/releases"
    echo "  Linux/Mac: cargo install --git https://github.com/iden3/circom.git"
    exit 1
fi

# node_modules kontrol
if [ ! -d "$NODE_MODULES" ]; then
    echo "→ npm dependencies kuruluyor..."
    cd "$CIRCUITS_DIR" && npm install --save-dev circomlib snarkjs circom_tester chai mocha
fi

mkdir -p "$BUILD_DIR"

# ─── Powers of Tau (LOKAL DEV CEREMONY) ─────────────────────────────────────
# DEV ONLY: Tek katılımcı, deterministik. PRODUCTION için multi-party gerekir.
if [ ! -f "$PTAU_FILE" ]; then
    echo "🔑 Powers of Tau (BN128, power 14) lokal üretiliyor..."
    echo "   ⚠️  Bu DEV ceremony — production için multi-party şart!"

    POT0="$BUILD_DIR/pot14_0000.ptau"
    POT1="$BUILD_DIR/pot14_0001.ptau"

    $SNARKJS powersoftau new bn128 14 "$POT0" -v
    $SNARKJS powersoftau contribute "$POT0" "$POT1" \
        --name="Obscura Dev Ceremony" -v -e="$(openssl rand -hex 32)"
    $SNARKJS powersoftau prepare phase2 "$POT1" "$PTAU_FILE" -v

    rm -f "$POT0" "$POT1"
    echo "✅ Powers of Tau hazır: $PTAU_FILE"
fi

# ─── Her devreyi derle ──────────────────────────────────────────────────────
CIRCUITS=("credit_threshold" "identity_proof" "message_integrity")

for CIRCUIT in "${CIRCUITS[@]}"; do
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "🔨 Derleniyor: $CIRCUIT"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

    OUT_DIR="$BUILD_DIR/$CIRCUIT"
    mkdir -p "$OUT_DIR"

    # 1. Circom → R1CS + WASM + SYM
    circom "$CIRCUITS_DIR/$CIRCUIT.circom" \
        --r1cs --wasm --sym \
        --output "$OUT_DIR" \
        -l "$CIRCUITS_DIR/node_modules"

    # 2. Groth16 Phase 2 setup
    echo "🔑 Groth16 setup..."
    $SNARKJS groth16 setup \
        "$OUT_DIR/$CIRCUIT.r1cs" \
        "$PTAU_FILE" \
        "$OUT_DIR/${CIRCUIT}_0000.zkey"

    # 3. Contribution (dev ceremony — production multi-party olmalı)
    $SNARKJS zkey contribute \
        "$OUT_DIR/${CIRCUIT}_0000.zkey" \
        "$OUT_DIR/${CIRCUIT}_final.zkey" \
        --name="Obscura Dev Contribution" \
        -v \
        -e="$(openssl rand -hex 32)"

    # 4. Verification key export
    $SNARKJS zkey export verificationkey \
        "$OUT_DIR/${CIRCUIT}_final.zkey" \
        "$OUT_DIR/verification_key.json"

    # 5. Solidity verifier (opsiyonel)
    $SNARKJS zkey export solidityverifier \
        "$OUT_DIR/${CIRCUIT}_final.zkey" \
        "$OUT_DIR/${CIRCUIT}_verifier.sol" 2>/dev/null || echo "  (sol verifier atlandı)"

    rm -f "$OUT_DIR/${CIRCUIT}_0000.zkey"

    echo "✅ $CIRCUIT hazır"
done

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ Tüm ZK devreleri derlendi"
echo "📁 Build: $BUILD_DIR"
echo ""
echo "Sonraki adım: bash distribute.sh  (artifact'leri client/server'a kopyalar)"
echo ""
echo "⚠️  PRODUCTION için trusted setup ceremony multi-party olarak yeniden yapılmalı!"
