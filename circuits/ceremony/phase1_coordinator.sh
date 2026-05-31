#!/usr/bin/env bash
# ─── Phase 1: Powers of Tau — COORDINATOR ───────────────────────────────────
#
# Bu script ceremony koordinatörü (örn. Obscura Foundation) tarafından çalıştırılır.
# Üç sub-komut: init, beacon, finalize.
#
# Kullanım:
#   bash phase1_coordinator.sh init           → fresh pot14_0000.ptau üret
#   bash phase1_coordinator.sh beacon         → son katılımcıdan sonra beacon attach
#   bash phase1_coordinator.sh finalize       → prepare phase 2 → pot14_final.ptau
#
# Çıktı: ceremony/output/pot14_*.ptau
#
# Spec: BN254 curve, power 14 (16K constraint capacity yeterli — biggest is 944).
# Snarkjs sürümü: 0.7.x (deterministik build için).

set -e

CEREMONY_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUTPUT_DIR="$CEREMONY_DIR/output"
CIRCUITS_DIR="$(cd "$CEREMONY_DIR/.." && pwd)"
SNARKJS="$CIRCUITS_DIR/node_modules/.bin/snarkjs"

POWER=14
CURVE="bn128"

# Cross-platform snarkjs path
if [ ! -f "$SNARKJS" ] && [ -f "$SNARKJS.cmd" ]; then
    SNARKJS="$SNARKJS.cmd"
fi
if [ ! -f "$SNARKJS" ]; then
    SNARKJS="npx snarkjs"
fi

mkdir -p "$OUTPUT_DIR"

sha256() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{print $1}'
    else
        shasum -a 256 "$1" | awk '{print $1}'
    fi
}

cmd="${1:-}"

case "$cmd" in
    init)
        echo "🔑 Phase 1 — Coordinator INIT"
        echo "   Curve: $CURVE, Power: $POWER"
        echo ""
        echo "⚠️  KORUMA: bu komut TOXIC WASTE üretir."
        echo "   Çalıştırılan makine ceremony bitince WIPE edilmeli."
        read -p "Devam? (yes/NO): " confirm
        [ "$confirm" = "yes" ] || { echo "İptal."; exit 1; }

        POT0="$OUTPUT_DIR/pot${POWER}_0000.ptau"

        $SNARKJS powersoftau new "$CURVE" "$POWER" "$POT0" -v

        HASH=$(sha256 "$POT0")
        echo ""
        echo "✅ Initial ptau hazır: $POT0"
        echo "   SHA-256: $HASH"
        echo ""
        echo "Sonraki adım: katılımcılara duyur, participant 1 ile başla."
        echo "  bash phase1_participant.sh 1 $POT0"
        ;;

    beacon)
        # Son katılımcı çıktısını al, public randomness ile beacon ekle.
        # Beacon, kamuya açık doğrulanabilir entropi (örn. Bitcoin block hash).
        LAST_PTAU="${2:-}"
        BEACON_HEX="${3:-}"
        if [ -z "$LAST_PTAU" ] || [ -z "$BEACON_HEX" ]; then
            echo "Kullanım: bash phase1_coordinator.sh beacon <son_ptau> <beacon_hex_64char>"
            echo ""
            echo "Beacon kaynağı önerileri:"
            echo "  1. Bitcoin block hash (örn. block #X = randomness source)"
            echo "  2. NIST randomness beacon: https://beacon.nist.gov/beacon/2.0/pulse/last"
            echo "  3. League of Entropy (drand) random output"
            echo ""
            echo "BEACON_HEX 64 karakter hex (32 byte) olmalı."
            exit 1
        fi

        if [ ${#BEACON_HEX} -ne 64 ]; then
            echo "✗ BEACON_HEX 64 karakter olmalı (32 byte)"
            exit 1
        fi

        OUT="$OUTPUT_DIR/pot${POWER}_beacon.ptau"
        echo "🔆 Beacon contribute (public randomness, 10 round)..."
        $SNARKJS powersoftau beacon \
            "$LAST_PTAU" \
            "$OUT" \
            "$BEACON_HEX" 10 \
            -n="Obscura Ceremony Beacon" \
            -v

        HASH=$(sha256 "$OUT")
        echo ""
        echo "✅ Beacon eklendi: $OUT"
        echo "   SHA-256: $HASH"
        echo "   Beacon source ($BEACON_HEX) participants.json'a ekle."
        ;;

    finalize)
        # Beacon ptau → prepare phase 2 → final ptau (universal phase 2 hazır)
        BEACON_PTAU="${2:-$OUTPUT_DIR/pot${POWER}_beacon.ptau}"
        if [ ! -f "$BEACON_PTAU" ]; then
            echo "✗ Beacon ptau bulunamadı: $BEACON_PTAU"
            exit 1
        fi

        echo "🔍 Phase 1 transcript verify (full chain)..."
        $SNARKJS powersoftau verify "$BEACON_PTAU"

        OUT="$OUTPUT_DIR/pot${POWER}_final.ptau"
        echo "🎯 Prepare Phase 2..."
        $SNARKJS powersoftau prepare phase2 "$BEACON_PTAU" "$OUT" -v

        HASH=$(sha256 "$OUT")
        echo ""
        echo "✅ Phase 1 tamamlandı: $OUT"
        echo "   SHA-256: $HASH"
        echo ""
        echo "Sonraki adım: her circuit için Phase 2 başlat:"
        echo "  bash phase2_circuit.sh init <circuit_name>"
        ;;

    *)
        echo "Obscura Phase 1 Coordinator"
        echo ""
        echo "Kullanım:"
        echo "  bash phase1_coordinator.sh init"
        echo "  bash phase1_coordinator.sh beacon <son_ptau> <hex_64>"
        echo "  bash phase1_coordinator.sh finalize [beacon_ptau]"
        exit 1
        ;;
esac
