#!/usr/bin/env bash
# ─── Phase 2: Circuit-Specific Setup ────────────────────────────────────────
#
# Phase 1 bittikten sonra her circuit için ayrı phase 2 ceremony koşulur.
# Bu da multi-party olabilir (önerilen ≥ 5 katılımcı) ama Phase 1'den hızlı:
# her contribute ~30 sn - 2 dk.
#
# Kullanım:
#   bash phase2_circuit.sh init       <circuit>
#   bash phase2_circuit.sh contribute <circuit> <participant_num>
#   bash phase2_circuit.sh beacon     <circuit> <hex_64>
#   bash phase2_circuit.sh finalize   <circuit>
#
# Örnek (credit_threshold için tam akış):
#   bash phase2_circuit.sh init       credit_threshold
#   bash phase2_circuit.sh contribute credit_threshold 1
#   bash phase2_circuit.sh contribute credit_threshold 2
#   ... (5 katılımcı)
#   bash phase2_circuit.sh beacon     credit_threshold abc...64hex
#   bash phase2_circuit.sh finalize   credit_threshold

set -e

CEREMONY_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUTPUT_DIR="$CEREMONY_DIR/output"
CIRCUITS_DIR="$(cd "$CEREMONY_DIR/.." && pwd)"
SNARKJS="$CIRCUITS_DIR/node_modules/.bin/snarkjs"
PTAU_FINAL="$OUTPUT_DIR/pot14_final.ptau"

# Cross-platform snarkjs
if [ ! -f "$SNARKJS" ] && [ -f "$SNARKJS.cmd" ]; then
    SNARKJS="$SNARKJS.cmd"
fi
if [ ! -f "$SNARKJS" ]; then
    SNARKJS="npx snarkjs"
fi

sha256() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{print $1}'
    else
        shasum -a 256 "$1" | awk '{print $1}'
    fi
}

cmd="${1:-}"
circuit="${2:-}"

if [ -z "$cmd" ] || [ -z "$circuit" ]; then
    cat <<'EOF'
Obscura Phase 2 Circuit Setup

Kullanım:
  bash phase2_circuit.sh init       <circuit>
  bash phase2_circuit.sh contribute <circuit> <participant_num>
  bash phase2_circuit.sh beacon     <circuit> <hex_64>
  bash phase2_circuit.sh finalize   <circuit>

Bilinen circuit'ler:
  credit_threshold, identity_proof, message_integrity, storage_proof,
  token_balance, vote_proof, age_proof, activity_proof, msg_count_proof,
  node_proof, endorsement_proof, streak_proof, location_proof,
  recursive_proof, zkml_moderation
EOF
    exit 1
fi

CIRCUIT_DIR="$OUTPUT_DIR/$circuit"
mkdir -p "$CIRCUITS_DIR/build/$circuit" "$CIRCUIT_DIR"

R1CS="$CIRCUITS_DIR/build/$circuit/$circuit.r1cs"

case "$cmd" in
    init)
        if [ ! -f "$PTAU_FINAL" ]; then
            echo "✗ Phase 1 final ptau bulunamadı: $PTAU_FINAL"
            echo "  Önce: bash phase1_coordinator.sh finalize"
            exit 1
        fi
        if [ ! -f "$R1CS" ]; then
            echo "✗ R1CS bulunamadı: $R1CS"
            echo "  Önce circuit derle: cd circuits && circom $circuit.circom --r1cs --output build/$circuit"
            exit 1
        fi

        OUT="$CIRCUIT_DIR/0000.zkey"
        echo "🔑 Phase 2 INIT — $circuit"
        $SNARKJS groth16 setup "$R1CS" "$PTAU_FINAL" "$OUT"

        HASH=$(sha256 "$OUT")
        echo "✅ Initial zkey: $OUT"
        echo "   SHA-256: $HASH"
        echo ""
        echo "Sonraki: bash phase2_circuit.sh contribute $circuit 1"
        ;;

    contribute)
        PARTICIPANT_NUM="${3:-}"
        if [ -z "$PARTICIPANT_NUM" ]; then
            echo "Kullanım: bash phase2_circuit.sh contribute $circuit <participant_num>"
            exit 1
        fi
        PADDED=$(printf "%04d" "$PARTICIPANT_NUM")
        PREV_NUM=$((PARTICIPANT_NUM - 1))
        PREV_PADDED=$(printf "%04d" "$PREV_NUM")

        INPUT="$CIRCUIT_DIR/${PREV_PADDED}.zkey"
        OUTPUT="$CIRCUIT_DIR/${PADDED}.zkey"

        if [ ! -f "$INPUT" ]; then
            echo "✗ Önceki zkey bulunamadı: $INPUT"
            echo "  participant_num bir önceki olmalı (önceki: $PREV_NUM)"
            exit 1
        fi

        INPUT_HASH=$(sha256 "$INPUT")
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo " Phase 2: $circuit — Participant #$PARTICIPANT_NUM"
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo "Girdi:          $INPUT"
        echo "Girdi SHA-256:  $INPUT_HASH"
        echo ""

        # Entropy
        echo "🎲 Klavye entropy (ENTER'a basana kadar yaz):"
        read -r KBD
        SYS_RAND=$(openssl rand -hex 32)
        ENTROPY_HEX=$(printf '%s|%s|%s|p%d|%s' "$KBD" "$SYS_RAND" "$(date +%s%N)" "$PARTICIPANT_NUM" "$circuit" \
            | openssl dgst -sha256 -hex | awk '{print $NF}')

        echo "🔨 Contribute..."
        $SNARKJS zkey contribute \
            "$INPUT" \
            "$OUTPUT" \
            --name="Obscura $circuit P#$PARTICIPANT_NUM" \
            -v \
            -e="$ENTROPY_HEX"

        OUTPUT_HASH=$(sha256 "$OUTPUT")
        echo ""
        echo "✅ Tamam: $OUTPUT"
        echo "   SHA-256: $OUTPUT_HASH"

        # Attestation
        ATTEST="$CIRCUIT_DIR/attestation_p${PADDED}.json"
        cat > "$ATTEST" <<EOF
{
  "ceremony": "obscura-phase2-${circuit}",
  "circuit": "$circuit",
  "participant_num": $PARTICIPANT_NUM,
  "input_zkey": "${PREV_PADDED}.zkey",
  "input_sha256": "$INPUT_HASH",
  "output_zkey": "${PADDED}.zkey",
  "output_sha256": "$OUTPUT_HASH",
  "timestamp_utc": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF
        echo "📝 Attestation: $ATTEST"
        ;;

    beacon)
        BEACON_HEX="${3:-}"
        if [ -z "$BEACON_HEX" ] || [ ${#BEACON_HEX} -ne 64 ]; then
            echo "Kullanım: bash phase2_circuit.sh beacon $circuit <hex_64>"
            exit 1
        fi

        # Find latest contribute zkey
        LAST=$(ls "$CIRCUIT_DIR"/[0-9][0-9][0-9][0-9].zkey 2>/dev/null | sort | tail -n1)
        if [ -z "$LAST" ]; then
            echo "✗ Hiç katılımcı zkey'i bulunamadı: $CIRCUIT_DIR"
            exit 1
        fi

        OUT="$CIRCUIT_DIR/beacon.zkey"
        echo "🔆 Beacon ekleniyor: $LAST → $OUT"
        $SNARKJS zkey beacon \
            "$LAST" \
            "$OUT" \
            "$BEACON_HEX" 10 \
            -n="Obscura $circuit Beacon" \
            -v

        HASH=$(sha256 "$OUT")
        echo "✅ Beacon zkey: $OUT  (SHA-256: $HASH)"
        ;;

    finalize)
        BEACON="$CIRCUIT_DIR/beacon.zkey"
        if [ ! -f "$BEACON" ]; then
            echo "✗ Beacon zkey bulunamadı: $BEACON"
            echo "  Önce: bash phase2_circuit.sh beacon $circuit <hex>"
            exit 1
        fi

        # Verify full chain
        echo "🔍 Zkey transcript verify..."
        $SNARKJS zkey verify "$R1CS" "$PTAU_FINAL" "$BEACON"

        # Final + vkey
        FINAL="$CIRCUIT_DIR/${circuit}_final.zkey"
        VKEY="$CIRCUIT_DIR/verification_key.json"

        cp "$BEACON" "$FINAL"
        $SNARKJS zkey export verificationkey "$FINAL" "$VKEY"

        FINAL_HASH=$(sha256 "$FINAL")
        VKEY_HASH=$(sha256 "$VKEY")

        echo ""
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo "✅ Phase 2 tamamlandı: $circuit"
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo "Final zkey:  $FINAL"
        echo "             SHA-256: $FINAL_HASH"
        echo "Vkey:        $VKEY"
        echo "             SHA-256: $VKEY_HASH"
        echo ""
        echo "Dağıtım: cp $FINAL ../../build/$circuit/${circuit}_final.zkey"
        echo "         cp $VKEY  ../../build/$circuit/verification_key.json"
        echo "         bash ../distribute.sh"
        echo ""
        echo "⚠️  Vkey SHA-256'yı on-chain (Aztec contract) yayımla → manipülasyon koruması."
        ;;

    *)
        echo "Bilinmeyen komut: $cmd"
        exit 1
        ;;
esac
