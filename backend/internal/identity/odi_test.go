package identity_test

import (
	"regexp"
	"strings"
	"testing"

	"obscura.network/core/internal/identity"
)

var odiFormat = regexp.MustCompile(`^ODI-[0-9A-Z]{4}-[0-9A-Z]{4}-[0-9A-Z]{4}$`)

func TestDeriveODI_Format(t *testing.T) {
	odi := identity.DeriveODI("did:obs:aabbccddeeff00112233445566778899")
	if !odiFormat.MatchString(odi) {
		t.Fatalf("ODI format geçersiz: %q", odi)
	}
}

func TestDeriveODI_Deterministic(t *testing.T) {
	did := "did:obs:1234567890abcdef1234567890abcdef"
	a := identity.DeriveODI(did)
	b := identity.DeriveODI(did)
	if a != b {
		t.Fatalf("aynı DID farklı ODI üretti: %q != %q", a, b)
	}
}

func TestDeriveODI_DifferentInputsDifferentOutput(t *testing.T) {
	a := identity.DeriveODI("did:obs:1111111111111111111111111111111111")
	b := identity.DeriveODI("did:obs:2222222222222222222222222222222222")
	if a == b {
		t.Fatalf("farklı DID'ler aynı ODI üretti: %q", a)
	}
}

func TestDeriveODI_NoAmbiguousChars(t *testing.T) {
	// I, L, O, U hiç çıkmamalı (Crockford Base32 alfabesi bu harfleri içermez)
	for i := 0; i < 200; i++ {
		did := "did:obs:" + strings.Repeat("0", i%16) + "test-vector"
		odi := identity.DeriveODI(did)
		code := strings.TrimPrefix(odi, "ODI-") // sabit prefix hariç, sadece türetilen kod kısmı
		for _, c := range []byte{'I', 'L', 'O', 'U'} {
			if strings.IndexByte(code, c) != -1 {
				t.Fatalf("ODI kod kısmı belirsiz karakter içeriyor (%q): %q", string(c), odi)
			}
		}
	}
}

func TestDeriveODI_EmptyInputStillProducesValidFormat(t *testing.T) {
	odi := identity.DeriveODI("")
	if !odiFormat.MatchString(odi) {
		t.Fatalf("boş girdi için ODI format geçersiz: %q", odi)
	}
}
