package bridge

import (
	"encoding/hex"
	"math/big"
	"testing"
)

// TestEncodeCompactUint64_KnownVectors, Substrate'in standart SCALE compact
// test vektörleriyle (Polkadot.js/subxt'in ürettiğiyle birebir aynı) eşleşir.
func TestEncodeCompactUint64_KnownVectors(t *testing.T) {
	cases := []struct {
		v    uint64
		want string
	}{
		{1, "04"},
		{42, "a8"},
		{69, "1501"},
		{65535, "feff0300"},
		{0, "00"},
		{63, "fc"},          // mod 00 üst sınır (2^6 - 1)
		{64, "0101"},        // mod 01 alt sınır
		{16383, "fdff"},     // mod 01 üst sınır (2^14 - 1)
		{16384, "02000100"}, // mod 10 alt sınır
	}
	for _, tc := range cases {
		got := EncodeCompactUint64(tc.v)
		gotHex := hex.EncodeToString(got)
		if gotHex != tc.want {
			t.Errorf("EncodeCompactUint64(%d) = %s, want %s", tc.v, gotHex, tc.want)
		}
	}
}

// TestEncodeCompactBig_U128RoundTrip, u64 sınırını aşan (transfer miktarları
// için gerekli) büyük tamsayılarda encode->decode simetrisini kontrol eder.
func TestEncodeCompactBig_U128RoundTrip(t *testing.T) {
	vals := []string{
		"1073741823",         // 2^30 - 1 (mode2 üst sınır)
		"1073741824",         // 2^30 (mode3 alt sınır)
		"100000000000000000", // 0.1 PAS (10 desimal varsayımıyla) mertebesinde
		"340282366920938463463374607431768211455", // u128 max (2^128 - 1)
	}
	for _, s := range vals {
		v, ok := new(big.Int).SetString(s, 10)
		if !ok {
			t.Fatalf("bad test value %s", s)
		}
		enc := EncodeCompactBig(v)
		got, err := newScaleCursor(enc).readCompact()
		if err != nil {
			t.Fatalf("readCompact(%s): %v", s, err)
		}
		if got.Cmp(v) != 0 {
			t.Errorf("round-trip %s -> %x -> %s, want %s", s, enc, got.String(), s)
		}
	}
}

// TestEncodeCompactBig_Mode3KnownVector, mode-3 (büyük tamsayı) kodlamasını
// bilinen bir Substrate/polkadot.js vektörüyle doğrular: 100000000000
// (10^11, u64 aralığında ama 2^30'u aşıyor, mode3'ü tetikler).
func TestEncodeCompactBig_Mode3KnownVector(t *testing.T) {
	// 100_000_000_000 = 0x174876E800 -> little-endian minimal bayt: 00 e8 76 48 17 (5 bayt)
	// prefix = (5-4)<<2 | 0b11 = 0b0111 = 0x07
	v := big.NewInt(100_000_000_000)
	got := EncodeCompactBig(v)
	want := "0700e8764817"
	if hex.EncodeToString(got) != want {
		t.Errorf("EncodeCompactBig(100000000000) = %x, want %s", got, want)
	}
}

func TestReadString(t *testing.T) {
	// "Balances" -> compact-len(8)=0x20 + "Balances" ascii
	b := append(EncodeCompactUint64(8), []byte("Balances")...)
	got, err := newScaleCursor(b).readString()
	if err != nil {
		t.Fatalf("readString: %v", err)
	}
	if got != "Balances" {
		t.Errorf("readString = %q, want %q", got, "Balances")
	}
}
