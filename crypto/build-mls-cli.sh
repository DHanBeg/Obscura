#!/usr/bin/env bash
# build-mls-cli.sh — Obscura MLS CLI binary oluşturma
#
# Gereksinimler:
#   - Rust toolchain (rustup install stable)
#   - cargo
#
# Kullanım:
#   cd crypto && ./build-mls-cli.sh
#
# Çıktı:
#   crypto/target/release/mls-cli (Linux/macOS)
#   crypto/target/release/mls-cli.exe (Windows)
#
# Go backend bu binary'yi subprocess olarak çalıştırır (CGO_ENABLED=0 uyumlu).
# backend/internal/mls/client.go: NewClient("../crypto/target/release/mls-cli")

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "🔨 Obscura MLS CLI derleniyor..."
echo "   Crate: $SCRIPT_DIR"
echo "   Binary: target/release/mls-cli"
echo ""

# Release build (optimized)
cargo build --release --bin mls-cli 2>&1

BIN="target/release/mls-cli"
if [[ "$OSTYPE" == "msys" || "$OSTYPE" == "cygwin" || "$OS" == "Windows_NT" ]]; then
    BIN="target/release/mls-cli.exe"
fi

if [[ -f "$BIN" ]]; then
    SIZE=$(du -sh "$BIN" | cut -f1)
    echo ""
    echo "✅ MLS CLI hazır: $BIN ($SIZE)"
    echo ""
    echo "Bağlantı:"
    echo "  MLS_CLI_PATH=../crypto/target/release/mls-cli go run ./cmd/node"
    echo "  veya Docker Compose ile crypto crate mount edilmeli"
else
    echo "❌ Binary bulunamadı: $BIN"
    exit 1
fi
