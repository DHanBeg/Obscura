// Package bridge — SCALE encoding temeli (Bridge madde 4, PARÇA 2).
//
// Substrate'in SCALE (Simple Concatenated Aggregate Little-Endian) kodlaması
// elle implement edildi — go-substrate-rpc-client / subxt YOK, mevcut
// bridge.go/relayer.go'nun "ham JSON-RPC + elle encode" felsefesiyle tutarlı.
//
// Compact integer kodlaması dört moda ayrılır (bkz. parity-scale-codec):
//
//	mod 00: değer < 2^6      -> tek bayt,  değer<<2 | 0b00
//	mod 01: değer < 2^14     -> iki bayt,  (değer<<2 | 0b01) little-endian u16
//	mod 10: değer < 2^30     -> dört bayt, (değer<<2 | 0b10) little-endian u32
//	mod 11: değer >= 2^30    -> 1 öncü bayt ((bayt_sayısı-4)<<2 | 0b11) +
//	        little-endian minimal bayt dizisi (u128'e kadar burada gerekli)
package bridge

import (
	"fmt"
	"math/big"
)

// EncodeCompactUint64, bir uint64 değeri SCALE compact formatında kodlar.
// Kanıt: encodeCompact_test.go'daki bilinen vektörlerle (1->0x04, 42->0xa8,
// 69->0x1501, 65535->0xfeff0300) eşleşiyor.
func EncodeCompactUint64(v uint64) []byte {
	return EncodeCompactBig(new(big.Int).SetUint64(v))
}

// EncodeCompactBig, keyfi büyüklükte (u128'e kadar, transfer miktarları için
// gerekli) bir tamsayıyı SCALE compact formatında kodlar.
func EncodeCompactBig(v *big.Int) []byte {
	if v.Sign() < 0 {
		panic("EncodeCompactBig: negatif değer SCALE compact'ta desteklenmez")
	}

	const maxMode0 = 1 << 6  // 64
	const maxMode1 = 1 << 14 // 16384
	const maxMode2 = 1 << 30 // 1073741824

	if v.IsUint64() {
		u := v.Uint64()
		switch {
		case u < maxMode0:
			return []byte{byte(u << 2)}
		case u < maxMode1:
			x := uint16(u<<2) | 0b01
			return []byte{byte(x), byte(x >> 8)}
		case u < maxMode2:
			x := uint32(u<<2) | 0b10
			return []byte{byte(x), byte(x >> 8), byte(x >> 16), byte(x >> 24)}
		}
	}

	// Mod 11: büyük tamsayı modu — little-endian minimal bayt dizisi.
	le := littleEndianBytes(v)
	for len(le) < 4 {
		le = append(le, 0) // minimum 4 bayt (2^30 eşiğinin altı zaten mode2'de ele alındı)
	}
	if len(le) > 67 {
		panic("EncodeCompactBig: değer SCALE compact'ın desteklediği azami boyutu aşıyor")
	}
	prefix := byte((len(le)-4)<<2) | 0b11
	return append([]byte{prefix}, le...)
}

// littleEndianBytes, big.Int'in little-endian bayt dizisini döndürür (big.Int.Bytes
// big-endian döndürür, burada ters çevriliyor).
func littleEndianBytes(v *big.Int) []byte {
	be := v.Bytes()
	le := make([]byte, len(be))
	for i, b := range be {
		le[len(be)-1-i] = b
	}
	return le
}

// scaleCursor, bir SCALE bayt dizisini sırayla okuyan basit bir imleçtir.
// Yalnızca bu paketin ihtiyaç duyduğu decode işlemleri için (metadata +
// dry-run gösterimi) kullanılır.
type scaleCursor struct {
	b []byte
	i int
}

func newScaleCursor(b []byte) *scaleCursor { return &scaleCursor{b: b} }

func (c *scaleCursor) remaining() int { return len(c.b) - c.i }

func (c *scaleCursor) readByte() (byte, error) {
	if c.remaining() < 1 {
		return 0, fmt.Errorf("scale: beklenmeyen dosya sonu (byte, offset %d)", c.i)
	}
	v := c.b[c.i]
	c.i++
	return v, nil
}

func (c *scaleCursor) readBytes(n int) ([]byte, error) {
	if n < 0 || c.remaining() < n {
		return nil, fmt.Errorf("scale: beklenmeyen dosya sonu (%d bayt istendi, offset %d)", n, c.i)
	}
	v := c.b[c.i : c.i+n]
	c.i += n
	return v, nil
}

func (c *scaleCursor) readU32LE() (uint32, error) {
	b, err := c.readBytes(4)
	if err != nil {
		return 0, err
	}
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24, nil
}

// readCompact, imleçteki SCALE compact tamsayıyı *big.Int olarak okur (u128'e
// kadar herhangi bir büyüklüğü destekler — pallet/call index çözümü ve nonce
// gösterimi için yeterli hassasiyet).
func (c *scaleCursor) readCompact() (*big.Int, error) {
	b0, err := c.readByte()
	if err != nil {
		return nil, err
	}
	mode := b0 & 0b11
	switch mode {
	case 0b00:
		return big.NewInt(int64(b0 >> 2)), nil
	case 0b01:
		b1, err := c.readByte()
		if err != nil {
			return nil, err
		}
		v := (uint16(b0) | uint16(b1)<<8) >> 2
		return big.NewInt(int64(v)), nil
	case 0b10:
		rest, err := c.readBytes(3)
		if err != nil {
			return nil, err
		}
		v := uint32(b0) | uint32(rest[0])<<8 | uint32(rest[1])<<16 | uint32(rest[2])<<24
		return big.NewInt(int64(v >> 2)), nil
	default: // 0b11
		n := int(b0>>2) + 4
		le, err := c.readBytes(n)
		if err != nil {
			return nil, err
		}
		be := make([]byte, n)
		for i, bb := range le {
			be[n-1-i] = bb
		}
		return new(big.Int).SetBytes(be), nil
	}
}

// readCompactUint32, readCompact'ın u32 sınırları içindeki kullanım için
// kısaltmasıdır (ör. tip registry uzunlukları, pallet/call sayaçları).
func (c *scaleCursor) readCompactUint32() (uint32, error) {
	v, err := c.readCompact()
	if err != nil {
		return 0, err
	}
	if !v.IsUint64() || v.Uint64() > 1<<32-1 {
		return 0, fmt.Errorf("scale: compact değer u32 sınırını aşıyor: %s", v.String())
	}
	return uint32(v.Uint64()), nil
}

// readString, SCALE compact-length-prefixed UTF-8 string okur.
func (c *scaleCursor) readString() (string, error) {
	n, err := c.readCompactUint32()
	if err != nil {
		return "", fmt.Errorf("string uzunluk: %w", err)
	}
	b, err := c.readBytes(int(n))
	if err != nil {
		return "", fmt.Errorf("string bayt: %w", err)
	}
	return string(b), nil
}

// readOptionTag, bir Option<T>'nin öncü bayrağını okur: 0=None, 1=Some.
// Diğer değerler hata (bozuk/beklenmeyen veri).
func (c *scaleCursor) readOptionTag() (bool, error) {
	b, err := c.readByte()
	if err != nil {
		return false, err
	}
	switch b {
	case 0:
		return false, nil
	case 1:
		return true, nil
	default:
		return false, fmt.Errorf("scale: geçersiz Option etiketi: %d (offset %d)", b, c.i-1)
	}
}
