#!/usr/bin/env bash
# build-new-circuits.sh — FAZ 3-4 yeni circuit'lerin dev trusted setup'ı
# Mevcut pot14_final.ptau kullanır (FAZ 1-2 ile aynı ceremony).
# DEV ONLY: Production için multi-party ceremony gereklidir.

set -e

CIRCUITS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BUILD_DIR="$CIRCUITS_DIR/build"
PTAU_FILE="$BUILD_DIR/pot14_final.ptau"
CIRCOM="circom"

# Windows: PATH'te circom.exe olabilir
if ! command -v circom >/dev/null 2>&1; then
    if command -v circom.exe >/dev/null 2>&1; then
        CIRCOM="circom.exe"
    else
        echo "✗ circom bulunamadı."
        exit 1
    fi
fi

SNARKJS="snarkjs"
if ! command -v snarkjs >/dev/null 2>&1; then
    SNARKJS="npx snarkjs"
fi

if [ ! -f "$PTAU_FILE" ]; then
    echo "✗ pot14_final.ptau bulunamadı. Önce circuits/build.sh çalıştırın."
    exit 1
fi

if [ ! -d "$CIRCUITS_DIR/node_modules" ]; then
    echo "→ npm dependencies kuruluyor..."
    cd "$CIRCUITS_DIR" && npm install --save-dev circomlib snarkjs
fi

mkdir -p "$BUILD_DIR"

# Yeni FAZ 3-4 circuit'leri + Spec Bölüm 7.1 kredi matrisi circuit'leri
NEW_CIRCUITS=(
    "recursive_proof"
    "zkml_moderation"
    "location_proof"
    "age_proof"
    "activity_proof"
    "msg_count_proof"
    "node_proof"
    "endorsement_proof"
    "streak_proof"
    "call_proof"
    "group_proof"
    "spam_victim_proof"
    "spam_false_proof"
    "fraud_proof"
    "contribution_proof"
)

build_circuit() {
    local CIRCUIT="$1"
    local CIRCOM_FILE="$CIRCUITS_DIR/$CIRCUIT.circom"

    if [ ! -f "$CIRCOM_FILE" ]; then
        echo "⚠️  $CIRCUIT.circom bulunamadı, atlanıyor"
        return
    fi

    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "🔨 Derleniyor: $CIRCUIT"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

    OUT_DIR="$BUILD_DIR/$CIRCUIT"
    mkdir -p "$OUT_DIR"

    # 1. Circom → R1CS + WASM + SYM
    $CIRCOM "$CIRCOM_FILE" \
        --r1cs --wasm --sym \
        --output "$OUT_DIR" \
        -l "$CIRCUITS_DIR/node_modules"

    # 2. R1CS istatistikleri
    $SNARKJS r1cs info "$OUT_DIR/$CIRCUIT.r1cs" 2>/dev/null || true

    # 3. Groth16 Phase 2 setup (mevcut ptau ile)
    echo "🔑 Groth16 setup ($CIRCUIT)..."
    $SNARKJS groth16 setup \
        "$OUT_DIR/$CIRCUIT.r1cs" \
        "$PTAU_FILE" \
        "$OUT_DIR/${CIRCUIT}_0000.zkey"

    # 4. Dev contribution
    RANDOM_HEX=$(node -e "console.log(require('crypto').randomBytes(32).toString('hex'))")
    $SNARKJS zkey contribute \
        "$OUT_DIR/${CIRCUIT}_0000.zkey" \
        "$OUT_DIR/${CIRCUIT}_final.zkey" \
        --name="Obscura Dev Contribution" \
        -v \
        -e="$RANDOM_HEX" 2>/dev/null

    # 5. Verification key export
    $SNARKJS zkey export verificationkey \
        "$OUT_DIR/${CIRCUIT}_final.zkey" \
        "$OUT_DIR/verification_key.json"

    # Temizlik
    rm -f "$OUT_DIR/${CIRCUIT}_0000.zkey"

    echo "✅ $CIRCUIT hazır → $OUT_DIR/verification_key.json"
}

echo "🚀 FAZ 3-4 ZK circuit'leri derleniyor (${#NEW_CIRCUITS[@]} adet)..."
echo "   Powers of Tau: $PTAU_FILE"
echo "   ⚠️  DEV ceremony (tek katkıcı) — production multi-party gerektirir"
echo ""

for CIRCUIT in "${NEW_CIRCUITS[@]}"; do
    build_circuit "$CIRCUIT"
done

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ Tüm yeni circuit'ler tamamlandı"
echo ""
echo "Sonraki adım: bash distribute-new.sh"
echo "  (vkey.json dosyalarını backend/internal/zk/keys/'e kopyalar)"
