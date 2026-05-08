#!/usr/bin/env bash
# ─── Obscura ZK Devre Derleme Scripti ────────────────────────────────────────
#
# Gereksinimler:
#   npm install -g circom snarkjs
#
# Kullanım:
#   chmod +x build.sh && ./build.sh
#
# Çıktılar (her devre için):
#   build/<circuit>/
#     ├── <circuit>.r1cs       — Kısıtlamalar (R1CS formatı)
#     ├── <circuit>.wasm       — WebAssembly ispat üretici
#     ├── <circuit>_final.zkey — Groth16 ispat anahtarı (trusted setup sonrası)
#     └── verification_key.json— Doğrulama anahtarı (backend & frontend kullanır)

set -e

CIRCUITS_DIR="$(dirname "$0")"
BUILD_DIR="$CIRCUITS_DIR/build"
PTAU_FILE="$BUILD_DIR/powersOfTau28_hez_final_12.ptau"

echo "🔧 Obscura ZK devre derleme başlıyor..."

# Build dizini oluştur
mkdir -p "$BUILD_DIR"

# Powers of Tau dosyasını indir (yalnızca 1 kez)
if [ ! -f "$PTAU_FILE" ]; then
    echo "📥 Powers of Tau indiriliyor (Hermez Ceremony 2^12)..."
    curl -Lo "$PTAU_FILE" \
        "https://hermez.s3-eu-west-1.amazonaws.com/powersOfTau28_hez_final_12.ptau"
    echo "✅ Powers of Tau hazır"
fi

# Her devreyi derle
CIRCUITS=("credit_threshold" "identity_proof" "message_integrity")

for CIRCUIT in "${CIRCUITS[@]}"; do
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "🔨 Derleniyor: $CIRCUIT"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

    OUT_DIR="$BUILD_DIR/$CIRCUIT"
    mkdir -p "$OUT_DIR"

    # 1. Circom → R1CS + WASM
    circom "$CIRCUITS_DIR/$CIRCUIT.circom" \
        --r1cs \
        --wasm \
        --sym \
        --output "$OUT_DIR" \
        -l "$CIRCUITS_DIR/node_modules"

    # 2. Groth16 Trusted Setup (dev: zkey generate_final)
    #    Production'da gerçek multi-party ceremony gerekir!
    echo "🔑 Groth16 anahtarı üretiliyor (dev ceremony)..."
    snarkjs groth16 setup \
        "$OUT_DIR/$CIRCUIT.r1cs" \
        "$PTAU_FILE" \
        "$OUT_DIR/${CIRCUIT}_0000.zkey"

    # 3. Contribution (dev: random entropy — prod'da çok taraflı olmalı)
    snarkjs zkey contribute \
        "$OUT_DIR/${CIRCUIT}_0000.zkey" \
        "$OUT_DIR/${CIRCUIT}_final.zkey" \
        --name="Obscura Dev Contribution" \
        -v \
        -e="$(openssl rand -hex 32)"

    # 4. Doğrulama anahtarını export et (backend doğrulama için)
    snarkjs zkey export verificationkey \
        "$OUT_DIR/${CIRCUIT}_final.zkey" \
        "$OUT_DIR/verification_key.json"

    # 5. Solidity verifier üret (opsiyonel — zincir doğrulama için)
    snarkjs zkey export solidityverifier \
        "$OUT_DIR/${CIRCUIT}_final.zkey" \
        "$OUT_DIR/${CIRCUIT}_verifier.sol"

    echo "✅ $CIRCUIT hazır → $OUT_DIR/"
    echo "   R1CS: $(ls -lh $OUT_DIR/$CIRCUIT.r1cs | awk '{print $5}')"
    echo "   WASM: $(ls -lh $OUT_DIR/${CIRCUIT}_js/$CIRCUIT.wasm | awk '{print $5}')"
done

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ Tüm ZK devreleri hazır!"
echo ""
echo "📁 Build çıktıları: $BUILD_DIR"
echo ""
echo "Frontend entegrasyonu:"
echo "  snarkjs groth16 prove <zkey> <wtns> proof.json public.json"
echo "  snarkjs groth16 verify verification_key.json public.json proof.json"
echo ""
echo "⚠️  Production için trusted setup ceremony yeniden yapılmalı!"
