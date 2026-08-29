#!/bin/bash
# Obscura ZK Devre Kurulum Scripti
# Circom 2.1.6 + snarkJS 0.7.x gerektirir
#
# Kurulum:
#   curl -L https://github.com/iden3/circom/releases/latest/download/circom-linux-amd64 -o circom
#   chmod +x circom && sudo mv circom /usr/local/bin/
#   npm install -g snarkjs

set -euo pipefail

CIRCUITS_DIR="$(dirname "$0")/../circuits"
BUILD_DIR="$(dirname "$0")/../build"
KEYS_DIR="$(dirname "$0")/../keys"
POT="$KEYS_DIR/pot_final.ptau"

mkdir -p "$BUILD_DIR" "$KEYS_DIR"

# ─── Powers of Tau (Phase 1 — Devre bağımsız) ───────────────────────────────
# Eğer yoksa indir veya üret
if [ ! -f "$POT" ]; then
    echo "⏳ Powers of Tau hazırlanıyor (bu birkaç dakika sürebilir)..."
    # Gerçekte: Hermez ceremony'den indir (güvenilir kurulum)
    # snarkjs powersoftau new bn128 15 "$KEYS_DIR/pot_0000.ptau" -v
    # snarkjs powersoftau contribute "$KEYS_DIR/pot_0000.ptau" "$KEYS_DIR/pot_0001.ptau" --name="Obscura" -v
    # snarkjs powersoftau prepare phase2 "$KEYS_DIR/pot_0001.ptau" "$POT" -v
    #
    # Development için hızlı (güvensiz) setup:
    snarkjs powersoftau new bn128 15 "$KEYS_DIR/pot_0000.ptau" -v
    snarkjs powersoftau contribute "$KEYS_DIR/pot_0000.ptau" "$KEYS_DIR/pot_0001.ptau" \
        --name="Obscura-Dev" --entropy="$(openssl rand -hex 32)" -v
    snarkjs powersoftau prepare phase2 "$KEYS_DIR/pot_0001.ptau" "$POT" -v
    echo "✅ Powers of Tau hazır: $POT"
fi

# ─── Devre Derleme + Kurulum Fonksiyonu ────────────────────────────────────

compile_circuit() {
    local name="$1"
    local circuit="$CIRCUITS_DIR/${name}.circom"
    local build="$BUILD_DIR/$name"

    mkdir -p "$build"

    echo ""
    echo "═══════════════════════════════════════"
    echo "🔧 Devre: $name"
    echo "═══════════════════════════════════════"

    # Derle: R1CS + WASM witness hesaplayıcı
    echo "⚙️  Derleniyor..."
    circom "$circuit" \
        --r1cs "$build/${name}.r1cs" \
        --wasm "$build" \
        --sym "$build/${name}.sym" \
        --O2 \
        -l "$CIRCUITS_DIR/../node_modules"

    echo "📊 R1CS istatistikleri:"
    snarkjs r1cs info "$build/${name}.r1cs"

    # Phase 2 kurulum (devre-spesifik)
    echo "🔑 Phase 2 kurulumu..."
    snarkjs groth16 setup \
        "$build/${name}.r1cs" \
        "$POT" \
        "$KEYS_DIR/${name}_0000.zkey"

    # Contribution (gerçekte çok taraflı ceremony)
    snarkjs zkey contribute \
        "$KEYS_DIR/${name}_0000.zkey" \
        "$KEYS_DIR/${name}_final.zkey" \
        --name="Obscura-1" \
        --entropy="$(openssl rand -hex 32)" -v

    # Verification key export
    snarkjs zkey export verificationkey \
        "$KEYS_DIR/${name}_final.zkey" \
        "$KEYS_DIR/${name}_vkey.json"

    echo "✅ $name hazır: $KEYS_DIR/${name}_vkey.json"
}

# ─── circomlib bağımlılıkları ─────────────────────────────────────────────────
if [ ! -d "$CIRCUITS_DIR/../node_modules/circomlib" ]; then
    echo "📦 circomlib yükleniyor..."
    cd "$(dirname "$0")/.."
    cat > package.json <<'EOF'
{
  "name": "obscura-zk",
  "version": "1.0.0",
  "dependencies": {
    "circomlib": "^2.0.5",
    "snarkjs": "^0.7.0"
  }
}
EOF
    npm install
    cd -
fi

# ─── Devreleri derle ve kur ──────────────────────────────────────────────────
compile_circuit "identity_proof"
compile_circuit "credit_threshold"
compile_circuit "token_transfer"

echo ""
echo "╔══════════════════════════════════════╗"
echo "║  ✅ Tüm ZK devreleri hazır!          ║"
echo "║                                      ║"
echo "║  Verification keys:                  ║"
echo "║    keys/identity_proof_vkey.json     ║"
echo "║    keys/credit_threshold_vkey.json   ║"
echo "║    keys/token_transfer_vkey.json     ║"
echo "╚══════════════════════════════════════╝"
