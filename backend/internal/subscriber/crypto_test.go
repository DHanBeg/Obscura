package subscriber

import (
	"bytes"
	"crypto/rand"
	"testing"
)

// testKey returns a deterministic 32-byte key for a test.
func testKey(t *testing.T, seed byte) []byte {
	t.Helper()
	k := make([]byte, keyLen)
	for i := range k {
		k[i] = seed + byte(i)
	}
	return k
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	cases := []struct {
		name      string
		plaintext []byte
	}{
		{"empty", []byte{}},
		{"short", []byte("+905551112233")},
		{"hash-sized", func() []byte { b := make([]byte, 32); _, _ = rand.Read(b); return b }()},
		{"binary", []byte{0x00, 0xff, 0x10, 0x7f, 0x80}},
	}

	if err := InitCrypto(testKey(t, 1)); err != nil {
		t.Fatalf("InitCrypto: %v", err)
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange / Act
			ct, err := EncryptField(tc.plaintext)
			if err != nil {
				t.Fatalf("EncryptField: %v", err)
			}
			pt, err := DecryptField(ct)
			if err != nil {
				t.Fatalf("DecryptField: %v", err)
			}

			// Assert
			if !bytes.Equal(pt, tc.plaintext) {
				t.Fatalf("round-trip mismatch: got %x want %x", pt, tc.plaintext)
			}
			// nonce (12) must be prepended → ciphertext strictly longer than plaintext.
			if len(ct) <= len(tc.plaintext) {
				t.Fatalf("ciphertext not longer than plaintext: %d <= %d", len(ct), len(tc.plaintext))
			}
		})
	}
}

func TestEncryptFieldFreshNoncePerCall(t *testing.T) {
	if err := InitCrypto(testKey(t, 2)); err != nil {
		t.Fatalf("InitCrypto: %v", err)
	}
	msg := []byte("same message")
	a, _ := EncryptField(msg)
	b, _ := EncryptField(msg)
	if bytes.Equal(a, b) {
		t.Fatal("two encryptions of the same plaintext produced identical ciphertext (nonce reuse)")
	}
}

func TestDecryptWrongKeyFails(t *testing.T) {
	// Arrange: encrypt under key A.
	if err := InitCrypto(testKey(t, 3)); err != nil {
		t.Fatalf("InitCrypto A: %v", err)
	}
	ct, err := EncryptField([]byte("secret phone hash"))
	if err != nil {
		t.Fatalf("EncryptField: %v", err)
	}

	// Act: swap to key B and attempt decrypt.
	if err := InitCrypto(testKey(t, 99)); err != nil {
		t.Fatalf("InitCrypto B: %v", err)
	}
	if _, err := DecryptField(ct); err == nil {
		t.Fatal("expected decrypt with wrong key to fail, got nil error")
	}
}

func TestTamperedCiphertextFails(t *testing.T) {
	if err := InitCrypto(testKey(t, 4)); err != nil {
		t.Fatalf("InitCrypto: %v", err)
	}
	ct, err := EncryptField([]byte("integrity matters"))
	if err != nil {
		t.Fatalf("EncryptField: %v", err)
	}

	// Flip the last byte (inside the GCM auth tag) — Open must reject it.
	tampered := append([]byte(nil), ct...)
	tampered[len(tampered)-1] ^= 0x01
	if _, err := DecryptField(tampered); err == nil {
		t.Fatal("expected tampered ciphertext to fail GCM auth, got nil error")
	}
}

func TestInitCryptoRejectsWrongKeyLength(t *testing.T) {
	for _, n := range []int{0, 16, 31, 33, 64} {
		if err := InitCrypto(make([]byte, n)); err == nil {
			t.Fatalf("InitCrypto accepted %d-byte key, want error", n)
		}
	}
}
