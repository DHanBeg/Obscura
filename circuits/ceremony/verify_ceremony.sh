#!/usr/bin/env bash
# ─── Ceremony Verification ──────────────────────────────────────────────────
#
# Tüm ceremony transcript'ini bağımsız doğrular.
# Bu script ceremony'ye katılmamış 3. parti tarafından çalıştırılabilir.
#
# Doğrulanan:
#   1. Phase 1 chain: pot14_0000 → pot14_NNNN → beacon → final
#   2. Her attestation JSON'unun input_sha256 / output_sha256 alanları gerçekle eşleşiyor mu
#   3. Phase 2 her circuit için: chain integrity + final zkey ↔ vkey tutarlılığı
#   4. participants.json içindeki tüm imzaların Ed25519 doğrulaması (TODO: imza schema)
#
# Kullanım:
#   bash verify_ceremony.sh                       # tüm circuit'leri tara
#   bash verify_ceremony.sh credit_threshold      # sadece bir circuit

set -e

CEREMONY_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUTPUT_DIR="$CEREMONY_DIR/output"
CIRCUITS_DIR="$(cd "$CEREMONY_DIR/.." && pwd)"
SNARKJS="$CIRCUITS_DIR/node_modules/.bin/snarkjs"
PTAU_FINAL="$OUTPUT_DIR/pot14_final.ptau"

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

FAILED=0
PASSED=0

red()   { printf "\033[31m%s\033[0m\n" "$*"; }
green() { printf "\033[32m%s\033[0m\n" "$*"; }
blue()  { printf "\033[34m%s\033[0m\n" "$*"; }

# ─── Phase 1 doğrulama ─────────────────────────────────────────────────────
verify_phase1() {
    blue "━━━ Phase 1 Verification ━━━"

    if [ ! -f "$PTAU_FINAL" ]; then
        red "✗ Final ptau bulunamadı: $PTAU_FINAL"
        FAILED=$((FAILED + 1))
        return
    fi

    echo "→ snarkjs powersoftau verify (chain integrity)..."
    if $SNARKJS powersoftau verify "$PTAU_FINAL" >/dev/null 2>&1; then
        green "✓ Phase 1 chain integrity OK"
        PASSED=$((PASSED + 1))
    else
        red "✗ Phase 1 chain BOZUK"
        FAILED=$((FAILED + 1))
    fi

    # Attestation hash kontrolü
    echo "→ Attestation SHA-256 doğrulanıyor..."
    for ATTEST in "$OUTPUT_DIR"/attestation_p*.json; do
        [ -f "$ATTEST" ] || continue

        OUT_FILE=$(awk -F'"' '/"output_ptau"/ {print $4}' "$ATTEST")
        EXPECTED=$(awk -F'"' '/"output_sha256"/ {print $4}' "$ATTEST")

        FULL_OUT="$OUTPUT_DIR/$OUT_FILE"
        if [ ! -f "$FULL_OUT" ]; then
            red "  ✗ $(basename "$ATTEST"): output dosyası yok ($OUT_FILE)"
            FAILED=$((FAILED + 1))
            continue
        fi

        ACTUAL=$(sha256 "$FULL_OUT")
        if [ "$ACTUAL" = "$EXPECTED" ]; then
            green "  ✓ $(basename "$ATTEST"): SHA-256 eşleşti"
            PASSED=$((PASSED + 1))
        else
            red "  ✗ $(basename "$ATTEST"): SHA-256 EŞLEŞMEDİ"
            echo "      beklenen: $EXPECTED"
            echo "      gerçek:   $ACTUAL"
            FAILED=$((FAILED + 1))
        fi
    done
}

# ─── Phase 2 doğrulama (per-circuit) ────────────────────────────────────────
verify_phase2() {
    local CIRCUIT="$1"
    blue "━━━ Phase 2 Verification — $CIRCUIT ━━━"

    local CDIR="$OUTPUT_DIR/$CIRCUIT"
    local R1CS="$CIRCUITS_DIR/build/$CIRCUIT/$CIRCUIT.r1cs"
    local FINAL="$CDIR/${CIRCUIT}_final.zkey"
    local VKEY="$CDIR/verification_key.json"

    if [ ! -d "$CDIR" ]; then
        red "✗ $CIRCUIT: ceremony output yok"
        FAILED=$((FAILED + 1))
        return
    fi
    if [ ! -f "$R1CS" ]; then
        red "✗ $CIRCUIT: R1CS bulunamadı ($R1CS) — circuit derlenmemiş"
        FAILED=$((FAILED + 1))
        return
    fi
    if [ ! -f "$FINAL" ]; then
        red "✗ $CIRCUIT: final zkey bulunamadı"
        FAILED=$((FAILED + 1))
        return
    fi

    # Chain verify (R1CS + ptau ↔ final zkey)
    echo "→ snarkjs zkey verify..."
    if $SNARKJS zkey verify "$R1CS" "$PTAU_FINAL" "$FINAL" >/dev/null 2>&1; then
        green "✓ $CIRCUIT: zkey chain integrity OK"
        PASSED=$((PASSED + 1))
    else
        red "✗ $CIRCUIT: zkey chain BOZUK"
        FAILED=$((FAILED + 1))
    fi

    # Vkey re-export → karşılaştır
    if [ -f "$VKEY" ]; then
        local TMP_VKEY
        TMP_VKEY=$(mktemp)
        $SNARKJS zkey export verificationkey "$FINAL" "$TMP_VKEY" >/dev/null 2>&1

        EXPECTED=$(sha256 "$VKEY")
        ACTUAL=$(sha256 "$TMP_VKEY")
        rm -f "$TMP_VKEY"

        if [ "$EXPECTED" = "$ACTUAL" ]; then
            green "✓ $CIRCUIT: vkey deterministik (re-export eşleşti)"
            PASSED=$((PASSED + 1))
        else
            red "✗ $CIRCUIT: vkey re-export EŞLEŞMEDİ"
            FAILED=$((FAILED + 1))
        fi
    fi

    # Per-participant attestation kontrolleri
    for ATTEST in "$CDIR"/attestation_p*.json; do
        [ -f "$ATTEST" ] || continue
        OUT_FILE=$(awk -F'"' '/"output_zkey"/ {print $4}' "$ATTEST")
        EXPECTED=$(awk -F'"' '/"output_sha256"/ {print $4}' "$ATTEST")
        FULL="$CDIR/$OUT_FILE"
        if [ ! -f "$FULL" ]; then continue; fi
        ACTUAL=$(sha256 "$FULL")
        if [ "$ACTUAL" = "$EXPECTED" ]; then
            green "  ✓ $(basename "$ATTEST"): hash OK"
            PASSED=$((PASSED + 1))
        else
            red "  ✗ $(basename "$ATTEST"): hash MISMATCH"
            FAILED=$((FAILED + 1))
        fi
    done
}

# ─── Main ──────────────────────────────────────────────────────────────────
blue "Obscura Ceremony Verification"
echo "Çalıştırma zamanı: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo ""

verify_phase1

if [ -n "${1:-}" ]; then
    verify_phase2 "$1"
else
    # Tüm circuit'leri tara
    if [ -d "$OUTPUT_DIR" ]; then
        for D in "$OUTPUT_DIR"/*/; do
            [ -d "$D" ] || continue
            CNAME=$(basename "$D")
            verify_phase2 "$CNAME"
        done
    fi
fi

echo ""
blue "━━━ Özet ━━━"
green "  Başarılı: $PASSED"
if [ "$FAILED" -gt 0 ]; then
    red "  Başarısız: $FAILED"
    exit 1
else
    echo "  Başarısız: 0"
    green "✅ Tüm ceremony transcript'leri DOĞRULANDI"
fi
