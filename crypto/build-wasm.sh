#!/usr/bin/env bash
# ─── Obscura Crypto WASM Build ──────────────────────────────────────────────
#
# Browser ZK proof generation için wasm-pack build.
# Çıktı doğrudan frontend public/ altına yazılır.
#
# Gereksinimler:
#   - Rust 1.75+ (rustup target add wasm32-unknown-unknown)
#   - wasm-pack (cargo install wasm-pack)
#
# Kullanım:
#   bash build-wasm.sh           # release build
#   bash build-wasm.sh --dev     # debug (hızlı iter, büyük binary)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT_DIR="$SCRIPT_DIR/../frontend/public/wasm-zk"

PROFILE="--release"
if [ "${1:-}" = "--dev" ]; then
    PROFILE="--dev"
    echo "→ DEV build (debug symbols, no LTO)"
fi

echo "🦀 wasm-pack build başlıyor..."
echo "   crate:  $SCRIPT_DIR"
echo "   out:    $OUT_DIR"
echo "   target: web"
echo "   feat:   wasm"

if ! command -v wasm-pack >/dev/null 2>&1; then
    echo "✗ wasm-pack bulunamadı."
    echo "  Kurulum: cargo install wasm-pack"
    exit 1
fi

mkdir -p "$OUT_DIR"

cd "$SCRIPT_DIR"
wasm-pack build \
    $PROFILE \
    --target web \
    --out-dir "$OUT_DIR" \
    --out-name obscura_crypto \
    -- --features wasm

echo ""
echo "✅ WASM build hazır: $OUT_DIR"
echo ""
echo "Üretilen dosyalar:"
ls -lh "$OUT_DIR" 2>/dev/null | grep -E "\.(wasm|js|ts)$" || true

echo ""
echo "Frontend kullanımı:"
echo "  import init, { generate_credit_proof_wasm } from '/wasm-zk/obscura_crypto.js';"
echo "  await init();"
echo "  const proof = generate_credit_proof_wasm(75, 50, userSecret, null, did);"
echo ""
echo "⚠️  PRODUCTION uyarısı:"
echo "   stub Groth16 yerine snarkjs/arkworks entegrasyonu hâlâ kalan iş."
echo "   Bkz: docs/adr/0006-dev-trusted-setup.md"
