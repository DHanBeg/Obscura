package auth

import "testing"

// GenerateDID formülü mobile/lib/sealed-sender.ts:didFromDhPublic ile birebir
// aynı olmak zorunda: "did:obs:" + hex(SHA256(pubkey_ham_bayt)[:16]). Bu
// testler hem determinizmi hem de bilinen vektörlerle (Python'da bağımsız
// hesaplanmış) formülün doğruluğunu doğrular.

func TestGenerateDID_Deterministic(t *testing.T) {
	const key = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="
	first := GenerateDID(key)
	second := GenerateDID(key)
	if first != second {
		t.Fatalf("aynı identityKey için farklı DID üretildi: %q != %q", first, second)
	}
}

func TestGenerateDID_DifferentKeysProduceDifferentDID(t *testing.T) {
	a := GenerateDID("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	b := GenerateDID("AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=")
	if a == b {
		t.Fatalf("farklı identityKey'ler aynı DID'i üretti: %q", a)
	}
}

func TestGenerateDID_KnownVectorZeroKey(t *testing.T) {
	// 32 sıfır bayt — base64.StdEncoding.EncodeToString(make([]byte, 32))
	const key = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	const want = "did:obs:66687aadf862bd776c8fc18b8e9f8e20"
	got := GenerateDID(key)
	if got != want {
		t.Fatalf("bilinen vektör uyuşmuyor: got %q, want %q", got, want)
	}
}

func TestGenerateDID_KnownVectorSequentialBytes(t *testing.T) {
	// bytes(range(32)) yani 0x00..0x1f — base64.StdEncoding.EncodeToString
	const key = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="
	const want = "did:obs:630dcd2966c4336691125448bbb25b4f"
	got := GenerateDID(key)
	if got != want {
		t.Fatalf("bilinen vektör uyuşmuyor: got %q, want %q", got, want)
	}
}

func TestGenerateDID_FormatIsDidObsPlus32HexChars(t *testing.T) {
	did := GenerateDID("AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=")
	const prefix = "did:obs:"
	if len(did) != len(prefix)+32 {
		t.Fatalf("DID uzunluğu beklenmiyor: %q (len=%d)", did, len(did))
	}
	if did[:len(prefix)] != prefix {
		t.Fatalf("DID prefix'i yanlış: %q", did)
	}
	for _, c := range did[len(prefix):] {
		isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')
		if !isHex {
			t.Fatalf("DID'de hex olmayan karakter: %q içinde %q", did, c)
		}
	}
}

func TestGenerateDID_MalformedBase64FallsBackWithoutPanicking(t *testing.T) {
	// Geçerli base64 olmayan bir identityKey panic ETMEMELİ — deterministik
	// (ama mobile ile eşleşmeyen) bir DID'e düşmeli.
	did := GenerateDID("bu-gecerli-base64-degil-!!!")
	if did == "" {
		t.Fatal("malformed base64 için boş DID döndü")
	}
	again := GenerateDID("bu-gecerli-base64-degil-!!!")
	if did != again {
		t.Fatalf("malformed base64 fallback deterministik değil: %q != %q", did, again)
	}
}
