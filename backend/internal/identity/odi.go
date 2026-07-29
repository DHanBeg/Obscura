// Package identity — ODI (Obscura Display Identifier) türetimi.
//
// ODI, kullanıcıya gösterilen kimlik kodudur: DID'in SHA-256 hash'inden
// deterministik ve tek yönlü türetilir (ODI'den DID'e dönülemez). DID
// hiçbir ekranda ham gösterilmez — bu paket sadece gösterim katmanı
// içindir, backend/API/state DID'i kullanmaya devam eder.
//
// Format: ODI-XXXX-XXXX-XXXX (Crockford Base32 alfabesi — I, L, O, U yok,
// karışabilecek karakterler elenmiş).
package identity

import (
	"crypto/sha256"
	"strings"
)

const crockfordAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// DeriveODI, verilen DID'den deterministik, tek yönlü ODI kodu üretir.
func DeriveODI(did string) string {
	sum := sha256.Sum256([]byte(did))

	var symbols strings.Builder
	symbols.Grow(12)
	bitBuf := uint32(0)
	bitCount := 0
	for _, b := range sum {
		bitBuf = (bitBuf << 8) | uint32(b)
		bitCount += 8
		for bitCount >= 5 {
			bitCount -= 5
			idx := (bitBuf >> uint(bitCount)) & 0x1F
			symbols.WriteByte(crockfordAlphabet[idx])
			if symbols.Len() >= 12 {
				break
			}
		}
		if symbols.Len() >= 12 {
			break
		}
	}

	s := symbols.String()
	return "ODI-" + s[0:4] + "-" + s[4:8] + "-" + s[8:12]
}
