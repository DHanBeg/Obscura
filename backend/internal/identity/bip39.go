// Package identity — BIP39 mnemonic + Obscura kimlik türetme
//
// Spec Bölüm 4.2 + 5.2:
//   - 12 kelimeli mnemonic (BIP39, İngilizce)
//   - identity_secret = HKDF-SHA256("obscura-id-v1", BIP39 seed)
//   - DID = "did:obs:" + hex(SHA256(identity_secret))
//   - zk_secret = HKDF("obscura-zk-id-v1", identity_secret) — ayrı türetim
//   - Sosyal kurtarma: mnemonic 3-of-5 Shamir paylaşımı (GF(256))
//
// NOT: mnemonic ve herhangi bir türetilmiş secret hiçbir zaman sunucuya
// gönderilmemelidir. Sunucu yalnızca ZK kanıtlarını ve public commitment'ları görür.
package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/tyler-smith/go-bip39"
	"golang.org/x/crypto/hkdf"
)

const (
	hkdfInfo       = "obscura-id-v1"
	hkdfZKIDInfo   = "obscura-zk-id-v1"
	hkdfDeviceInfo = "obscura-device-zk-v1"
	secretLen      = 32
	mnemonicBits   = 128 // 12 kelime
)

// ─── Mnemonic ─────────────────────────────────────────────────────────────────

// GenerateMnemonic — yeni 12 kelimeli BIP39 mnemonic üret
func GenerateMnemonic() (string, error) {
	entropy, err := bip39.NewEntropy(mnemonicBits)
	if err != nil {
		return "", fmt.Errorf("entropy üretimi: %w", err)
	}
	return bip39.NewMnemonic(entropy)
}

// ValidateMnemonic — mnemonic geçerliliğini doğrula
func ValidateMnemonic(phrase string) bool {
	return bip39.IsMnemonicValid(strings.TrimSpace(phrase))
}

// DeriveIdentitySecret — mnemonic'den 32 byte identity_secret türet
func DeriveIdentitySecret(phrase, passphrase string) ([secretLen]byte, error) {
	if !bip39.IsMnemonicValid(strings.TrimSpace(phrase)) {
		return [secretLen]byte{}, fmt.Errorf("geçersiz mnemonic")
	}
	seed := bip39.NewSeed(strings.TrimSpace(phrase), passphrase)
	hk := hkdf.New(sha512.New, seed, []byte("obscura-mnemonic-v1"), []byte(hkdfInfo))
	var secret [secretLen]byte
	if _, err := io.ReadFull(hk, secret[:]); err != nil {
		return [secretLen]byte{}, fmt.Errorf("hkdf expand: %w", err)
	}
	return secret, nil
}

// DIDFromMnemonic — mnemonic'den DID türet
// DID = "did:obs:" + hex(SHA256(identity_secret))
func DIDFromMnemonic(phrase, passphrase string) (string, error) {
	secret, err := DeriveIdentitySecret(phrase, passphrase)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(secret[:])
	return "did:obs:" + hex.EncodeToString(hash[:]), nil
}

// ─── ZK-ID Secret Türetme (Spec Bölüm 5.2 Adım 4) ───────────────────────────
//
// ZK-ID secret, identity_secret'tan HKDF ile ayrı bir türetimdir.
// Identity_secret DID'i türetir; zk_secret ise ZK circuit'e girdi olarak kullanılır.
// İki secret birbirinden bağımsızdır: birinin sızması diğerini tehlikeye atmaz.
//
// Kullanım: client-side'da çağrılır. Sunucuya gönderilmez.

// DeriveZKIDSecret — mnemonic'ten ZK-ID secret türet.
// identity_secret'tan HKDF("obscura-zk-id-v1") ile ayrı bir 32 byte türetim.
// passphrase identity_secret için kullanılanla aynı olmalıdır.
func DeriveZKIDSecret(mnemonic, passphrase string) ([]byte, error) {
	idSecret, err := DeriveIdentitySecret(mnemonic, passphrase)
	if err != nil {
		return nil, fmt.Errorf("identity_secret türetilemedi: %w", err)
	}
	hk := hkdf.New(sha512.New, idSecret[:], nil, []byte(hkdfZKIDInfo))
	zkSecret := make([]byte, secretLen)
	if _, err := io.ReadFull(hk, zkSecret); err != nil {
		return nil, fmt.Errorf("zk_secret hkdf expand: %w", err)
	}
	return zkSecret, nil
}

// DeriveZKIDPublicParams — ZK secret'tan circuit'e girecek public commitment türet.
//
// didCommitment = hex(SHA256(zkSecret || "commitment"))
//
// Bu değer sunucuya gönderilir ve users.zk_id_commitment kolonuna kaydedilir.
// ZK kanıtı üretilirken private input olarak zkSecret, public input olarak
// didCommitment kullanılır. Sunucu yalnızca commitment'ı saklar — secret'ı asla görmez.
func DeriveZKIDPublicParams(zkSecret []byte) (didCommitment string, err error) {
	if len(zkSecret) == 0 {
		return "", fmt.Errorf("zkSecret boş olamaz")
	}
	// SHA256(zkSecret || "commitment") — deterministic bağlama etiketi
	h := sha256.New()
	h.Write(zkSecret)
	h.Write([]byte("commitment"))
	digest := h.Sum(nil)
	return hex.EncodeToString(digest), nil
}

// ─── Cihaz-Specific ZK-ID Türetme (Spec Bölüm 5.4) ──────────────────────────
//
// Her cihaz, master ZK secret'tan deterministik olarak ayrı bir zk_secret türetir.
// deviceIndex: 0 = birincil cihaz, 1 = ikincil cihaz, 2 = kurtarma cihazı
//
// Bu sayede bir cihazın secret'ı sızarsa diğer cihazlar etkilenmez.

// DeriveDeviceZKIDSecret — master ZK secret'tan cihaz-specific türetim.
// HKDF("obscura-device-zk-v1", masterZKSecret, deviceIndex bytes) → 32 byte.
func DeriveDeviceZKIDSecret(masterZKSecret []byte, deviceIndex int) ([]byte, error) {
	if len(masterZKSecret) == 0 {
		return nil, fmt.Errorf("masterZKSecret boş olamaz")
	}
	if deviceIndex < 0 || deviceIndex > 255 {
		return nil, fmt.Errorf("geçersiz deviceIndex: %d (0-255 arası olmalı)", deviceIndex)
	}
	// Salt olarak deviceIndex bytes kullan
	salt := make([]byte, 4)
	binary.BigEndian.PutUint32(salt, uint32(deviceIndex))

	hk := hkdf.New(sha512.New, masterZKSecret, salt, []byte(hkdfDeviceInfo))
	devSecret := make([]byte, secretLen)
	if _, err := io.ReadFull(hk, devSecret); err != nil {
		return nil, fmt.Errorf("device zk_secret hkdf expand: %w", err)
	}
	return devSecret, nil
}

// ─── Sosyal Kurtarma — Shamir Gizli Paylaşımı ────────────────────────────────
// Spec Bölüm 4.2: Mnemonic 3-of-5 Shamir paylaşımı ile yedeklenir.
// GF(2^8) alanında k-of-n polinomlu gizli paylaşım (AES polinomu).

// ShamirShare — tek bir Shamir payı
type ShamirShare struct {
	Index byte   `json:"index"` // 1..n
	Value []byte `json:"value"` // her byte için GF(256) polinom değeri
}

// SplitMnemonic — mnemonic'i k-of-n Shamir paylarına böl
func SplitMnemonic(phrase string, k, n int) ([]ShamirShare, error) {
	if k < 2 || n < k || n > 255 {
		return nil, fmt.Errorf("geçersiz parametreler: 2≤k≤n≤255, k=%d n=%d", k, n)
	}
	phrase = strings.TrimSpace(phrase)
	if !bip39.IsMnemonicValid(phrase) {
		return nil, fmt.Errorf("geçersiz mnemonic")
	}
	secret := []byte(phrase)
	return gf256Split(secret, k, n)
}

// CombineShares — k adet paydan mnemonic'i kurtarma
func CombineShares(shares []ShamirShare) (string, error) {
	if len(shares) < 2 {
		return "", fmt.Errorf("en az 2 pay gerekli")
	}
	secret, err := gf256Combine(shares)
	if err != nil {
		return "", err
	}
	phrase := strings.TrimSpace(string(secret))
	if !bip39.IsMnemonicValid(phrase) {
		return "", fmt.Errorf("kurtarılan mnemonic geçersiz — yanlış pay'lar kullanıldı")
	}
	return phrase, nil
}

// ─── GF(256) Shamir Aritmetiği ─────────────────────────────────────────────────
// AES GF(2^8): irreducible polinom x^8+x^4+x^3+x+1 (0x1b)

func gf256Mul(a, b byte) byte {
	var p byte
	for i := 0; i < 8; i++ {
		if b&1 != 0 {
			p ^= a
		}
		hibit := a & 0x80
		a <<= 1
		if hibit != 0 {
			a ^= 0x1b
		}
		b >>= 1
	}
	return p
}

func gf256Inv(a byte) byte {
	// a^254 = a^(-1) in GF(256) (Fermat küçük teoremi)
	result := byte(1)
	base := a
	for exp := 254; exp > 0; exp >>= 1 {
		if exp&1 != 0 {
			result = gf256Mul(result, base)
		}
		base = gf256Mul(base, base)
	}
	return result
}

func gf256Split(secret []byte, k, n int) ([]ShamirShare, error) {
	sLen := len(secret)

	// Her byte için bağımsız k-1 dereceli polinom üret
	// poly[i][0] = secret[i], poly[i][1..k-1] = rastgele
	polys := make([][]byte, sLen)
	for i, s := range secret {
		coeffs := make([]byte, k)
		coeffs[0] = s
		if _, err := rand.Read(coeffs[1:]); err != nil {
			return nil, fmt.Errorf("rand: %w", err)
		}
		polys[i] = coeffs
	}

	shares := make([]ShamirShare, n)
	for x := 1; x <= n; x++ {
		val := make([]byte, sLen)
		for i, coeffs := range polys {
			// Horner: p(x) = c[k-1]*x^(k-1) + ... + c[1]*x + c[0]
			r := coeffs[k-1]
			for j := k - 2; j >= 0; j-- {
				r = gf256Mul(r, byte(x)) ^ coeffs[j]
			}
			val[i] = r
		}
		shares[x-1] = ShamirShare{Index: byte(x), Value: val}
	}
	return shares, nil
}

func gf256Combine(shares []ShamirShare) ([]byte, error) {
	k := len(shares)
	if k == 0 {
		return nil, fmt.Errorf("pay yok")
	}
	sLen := len(shares[0].Value)
	result := make([]byte, sLen)

	for i := 0; i < sLen; i++ {
		// Lagrange interpolation at x=0
		secret := byte(0)
		for j := 0; j < k; j++ {
			xj := shares[j].Index
			num := byte(1)
			den := byte(1)
			for m := 0; m < k; m++ {
				if m == j {
					continue
				}
				xm := shares[m].Index
				num = gf256Mul(num, xm)
				den = gf256Mul(den, xj^xm)
			}
			term := gf256Mul(shares[j].Value[i], gf256Mul(num, gf256Inv(den)))
			secret ^= term
		}
		result[i] = secret
	}
	return result, nil
}
