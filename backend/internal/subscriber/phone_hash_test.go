package subscriber

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

func TestHashPhoneDeterministic(t *testing.T) {
	pepper := []byte("server-side-secret-pepper")
	phone := "+905551112233"

	a := HashPhone(phone, pepper)
	b := HashPhone(phone, pepper)

	if !bytes.Equal(a, b) {
		t.Fatalf("same phone+pepper produced different hashes: %x vs %x", a, b)
	}
	if len(a) != sha256.Size {
		t.Fatalf("hash length = %d, want %d (SHA-256)", len(a), sha256.Size)
	}
}

func TestHashPhoneDifferentPepperDiffers(t *testing.T) {
	phone := "+905551112233"

	a := HashPhone(phone, []byte("pepper-one"))
	b := HashPhone(phone, []byte("pepper-two"))

	if bytes.Equal(a, b) {
		t.Fatal("different peppers produced identical hashes")
	}
}

func TestHashPhoneDifferentPhoneDiffers(t *testing.T) {
	pepper := []byte("same-pepper")

	a := HashPhone("+905551112233", pepper)
	b := HashPhone("+905559998877", pepper)

	if bytes.Equal(a, b) {
		t.Fatal("different phones produced identical hashes")
	}
}
