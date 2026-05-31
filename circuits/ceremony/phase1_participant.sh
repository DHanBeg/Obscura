#!/usr/bin/env bash
# ─── Phase 1: Powers of Tau — PARTICIPANT ───────────────────────────────────
#
# Bu script bağımsız bir katılımcı tarafından (air-gapped makinede) çalıştırılır.
#
# Kullanım:
#   bash phase1_participant.sh <katılımcı_no> <önceki_ptau_dosyası>
#
# Örnek (1. katılımcı):
#   bash phase1_participant.sh 1 output/pot14_0000.ptau
#
# Örnek (5. katılımcı):
#   bash phase1_participant.sh 5 output/pot14_0004.ptau
#
# Çıktı: output/pot14_NNNN.ptau (sonraki katılımcıya teslim edilir)
# Katılımcı VIDEO + ekran kaydı tutmalı, attestation imzalamalı.

set -e

CEREMONY_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUTPUT_DIR="$CEREMONY_DIR/output"
CIRCUITS_DIR="$(cd "$CEREMONY_DIR/.." && pwd)"
SNARKJS="$CIRCUITS_DIR/node_modules/.bin/snarkjs"

POWER=14

# Cross-platform snarkjs path
if [ ! -f "$SNARKJS" ] && [ -f "$SNARKJS.cmd" ]; then
    SNARKJS="$SNARKJS.cmd"
fi
if [ ! -f "$SNARKJS" ]; then
    SNARKJS="npx snarkjs"
fi

PARTICIPANT_NUM="${1:-}"
INPUT_PTAU="${2:-}"

if [ -z "$PARTICIPANT_NUM" ] || [ -z "$INPUT_PTAU" ]; then
    echo "Kullanım: bash phase1_participant.sh <numara> <önceki_ptau>"
    echo ""
    echo "Örnek: bash phase1_participant.sh 1 output/pot14_0000.ptau"
    exit 1
fi

if [ ! -f "$INPUT_PTAU" ]; then
    echo "✗ Girdi ptau bulunamadı: $INPUT_PTAU"
    exit 1
fi

mkdir -p "$OUTPUT_DIR"

sha256() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{print $1}'
    else
        shasum -a 256 "$1" | awk '{print $1}'
    fi
}

# Padded participant number: 1 → 0001, 12 → 0012
PADDED=$(printf "%04d" "$PARTICIPANT_NUM")
OUTPUT_PTAU="$OUTPUT_DIR/pot${POWER}_${PADDED}.ptau"

INPUT_HASH=$(sha256 "$INPUT_PTAU")
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo " Obscura Ceremony — Phase 1 Participant #$PARTICIPANT_NUM"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Girdi ptau:       $INPUT_PTAU"
echo "Girdi SHA-256:    $INPUT_HASH"
echo "Çıktı ptau:       $OUTPUT_PTAU"
echo ""
echo "⚠️  ÖNEMLİ:"
echo "  1. VIDEO KAYIT açık olsun (ekran + ortam)"
echo "  2. Bu makine ceremony bitince DİSK WIPE edilecek"
echo "  3. Entropy hazırlığı: klavye/fare 60+ sn rastgele hareket"
echo "  4. Bu çıktı dosyası bir sonraki katılımcıya HTTPS+imza ile aktarılacak"
echo ""

# 1. Verify input ptau (önceki katılımcıdan gelen)
echo "🔍 Girdi ptau doğrulanıyor (chain integrity)..."
$SNARKJS powersoftau verify "$INPUT_PTAU"
echo ""

# 2. Entropy toplama — kullanıcıya rastgele veri girdir
echo "🎲 Entropy girişi:"
echo "   - Klavye RANDOM input (ENTER'a basana kadar yaz):"
read -r ENTROPY_KEYBOARD
echo ""
echo "   - Sistem random + timestamp ekleniyor..."

# Combine multiple entropy sources
SYS_RAND=$(openssl rand -hex 32)
TIMESTAMP=$(date +%s%N)
ENTROPY_COMBINED="${ENTROPY_KEYBOARD}|${SYS_RAND}|${TIMESTAMP}|p${PARTICIPANT_NUM}"

# Final entropy = SHA-256 of combined (snarkjs ister 64 char hex)
ENTROPY_HEX=$(printf '%s' "$ENTROPY_COMBINED" | openssl dgst -sha256 -hex | awk '{print $NF}')
echo "   ✓ Combined entropy hash: ${ENTROPY_HEX:0:16}... (full 64 char)"
echo ""

# 3. Contribute
echo "🔨 Powers of Tau contribute (~5-15 dk, power $POWER)..."
$SNARKJS powersoftau contribute \
    "$INPUT_PTAU" \
    "$OUTPUT_PTAU" \
    --name="Obscura Ceremony Participant #$PARTICIPANT_NUM" \
    -v \
    -e="$ENTROPY_HEX"

# 4. Hash + cleanup
OUTPUT_HASH=$(sha256 "$OUTPUT_PTAU")
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ Contribution tamamlandı"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Çıktı:        $OUTPUT_PTAU"
echo "Boyut:        $(du -h "$OUTPUT_PTAU" | awk '{print $1}')"
echo "Çıktı SHA-256: $OUTPUT_HASH"
echo ""

# 5. Toxic waste imha uyarısı
echo "🔥 TOXIC WASTE İMHA:"
echo "   1. Bu shell history TEMİZLE:  history -c && rm -f ~/.bash_history"
echo "   2. Süreç belleği temizle:     sync && echo 3 > /proc/sys/vm/drop_caches (sudo)"
echo "   3. Disk wipe (ceremony sonu): shred -vfz -n 7 <disk>"
echo "   4. ASLA $INPUT_PTAU dosyasını arşivleme — yalnız output yüklenir"
echo ""

# 6. Attestation JSON oluştur
ATTEST_FILE="$OUTPUT_DIR/attestation_p${PADDED}.json"
cat > "$ATTEST_FILE" <<EOF
{
  "ceremony": "obscura-phase1-pot${POWER}",
  "participant_num": $PARTICIPANT_NUM,
  "input_ptau": "$(basename "$INPUT_PTAU")",
  "input_sha256": "$INPUT_HASH",
  "output_ptau": "$(basename "$OUTPUT_PTAU")",
  "output_sha256": "$OUTPUT_HASH",
  "timestamp_utc": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "hostname": "$(hostname)",
  "snarkjs_version": "$($SNARKJS --version 2>&1 | head -n1 || echo 'unknown')",
  "entropy_sources": ["keyboard", "openssl_rand", "timestamp_ns"],
  "TODO_participant_fills": {
    "name": "<doldur>",
    "country": "<doldur>",
    "hardware": "<doldur>",
    "video_url": "<doldur>",
    "witnesses": [],
    "attestation_sig_ed25519_hex": "<imzala>"
  }
}
EOF
echo "📝 Attestation: $ATTEST_FILE"
echo "   Dosyayı doldurup imzala, participants.json'a ekle."
echo ""
echo "Sonraki katılımcıya (P$((PARTICIPANT_NUM+1))) HTTPS+sig ile $OUTPUT_PTAU yolla."
