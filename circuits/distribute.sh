#!/usr/bin/env bash
# Build artifact'lerini client + server'a dağıt
# Çalıştır: bash distribute.sh (build.sh sonrası)

set -e

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BUILD="$ROOT/circuits/build"

# Hedef dizinler
WEB="$ROOT/frontend/public/zk"
MOBILE="$ROOT/mobile/assets/zk"
BACKEND="$ROOT/backend/internal/zk/keys"

mkdir -p "$WEB" "$MOBILE" "$BACKEND"

CIRCUITS=("credit_threshold" "identity_proof" "message_integrity")

for C in "${CIRCUITS[@]}"; do
    if [ ! -d "$BUILD/$C" ]; then
        echo "✗ $C not built — run bash build.sh first"
        exit 1
    fi

    # WASM (prover) → frontend + mobile
    cp "$BUILD/$C/${C}_js/${C}.wasm" "$WEB/${C}.wasm"
    cp "$BUILD/$C/${C}_js/${C}.wasm" "$MOBILE/${C}.wasm"

    # zkey (prover) → frontend + mobile
    cp "$BUILD/$C/${C}_final.zkey" "$WEB/${C}_final.zkey"
    cp "$BUILD/$C/${C}_final.zkey" "$MOBILE/${C}_final.zkey"

    # verification_key (verifier) → backend
    cp "$BUILD/$C/verification_key.json" "$BACKEND/${C}_vkey.json"

    echo "✓ $C → web + mobile + backend"
done

echo ""
echo "✅ Tüm artifact'ler dağıtıldı"
echo "   Web prover:   $WEB/"
echo "   Mobile prover: $MOBILE/"
echo "   Backend vkey: $BACKEND/"
echo ""
echo "Boyutlar:"
du -sh "$WEB" "$MOBILE" "$BACKEND" 2>/dev/null
