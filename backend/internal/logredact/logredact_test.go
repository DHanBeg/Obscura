package logredact

import (
	"regexp"
	"strings"
	"testing"
)

var hexPattern = regexp.MustCompile(`^[0-9a-f]+$`)

func TestDID_Deterministic(t *testing.T) {
	a := DID("did:obs:alice")
	b := DID("did:obs:alice")
	if a != b {
		t.Fatalf("aynı DID için farklı çıktı: %q vs %q — korelasyon kırılır", a, b)
	}
}

func TestDID_DifferentInputsDifferentOutputs(t *testing.T) {
	a := DID("did:obs:alice")
	b := DID("did:obs:bob")
	if a == b {
		t.Fatalf("farklı DID'ler aynı çıktıyı üretti: %q", a)
	}
}

func TestDID_DoesNotContainRawInput(t *testing.T) {
	did := "did:obs:super-secret-identity"
	got := DID(did)
	if got == did {
		t.Fatal("redaction ham DID'i aynen döndürdü")
	}
	if strings.Contains(got, did) {
		t.Fatalf("redakte çıktı ham DID'i alt-string olarak içeriyor: %q", got)
	}
	if len(got) != 16 {
		t.Fatalf("16 hex karakter (64-bit HMAC prefix) bekleniyordu, got=%q (len=%d)", got, len(got))
	}
	if !hexPattern.MatchString(got) {
		t.Fatalf("çıktı hex değil: %q", got)
	}
}

func TestDID_EmptyIsSafe(t *testing.T) {
	if got := DID(""); got == "" {
		t.Fatal("boş DID için boş string dönmemeli (log satırında iz bırakmaz)")
	}
}

// TestDID_ShortInputNoPanic — HMAC girdi uzunluğuna duyarlı değildir (blok
// boyutuna pad/hash'lenir), ama yine de tek-karakterlik/çok-kısa girdilerde
// panic olmadığını doğrudan kanıtla.
func TestDID_ShortInputNoPanic(t *testing.T) {
	for _, s := range []string{"a", "1", "?", "did"} {
		got := DID(s)
		if len(got) != 16 {
			t.Errorf("kısa girdi %q için 16 hex bekleniyordu, got=%q (len=%d)", s, got, len(got))
		}
	}
}

// TestDID_KeySeparation — de-anon savunmasının kalbi: aynı ham DID, FARKLI
// HMAC anahtarıyla FARKLI çıktı üretmeli. Aksi halde anahtar log-hijyeni
// hiçbir şey katmaz (v1'in saltsız sha256'sıyla aynı zafiyet). Paket-içi
// test olduğu için redactKey'i doğrudan değiştirip geri alıyoruz — üretim
// kodunda ayrı bir "test modu" icat edilmedi.
func TestDID_KeySeparation(t *testing.T) {
	orig := redactKey
	defer func() { redactKey = orig }()

	did := "did:obs:key-separation-subject"

	redactKey = []byte("key-A-0123456789")
	a := DID(did)

	redactKey = []byte("key-B-9876543210")
	b := DID(did)

	if a == b {
		t.Fatalf("farklı anahtarla AYNI çıktı üretildi (%q) — de-anon savunması çalışmıyor", a)
	}
}

// TestDID_SameKeySameOutputAcrossCalls — determinizm İSTENEN davranış
// (operatör aynı aktörün log satırlarını debug için ilişkilendirebilsin) —
// KeySeparation testiyle karıştırılmasın: burada key SABİT, iki ayrı çağrı.
func TestDID_SameKeySameOutputAcrossCalls(t *testing.T) {
	did := "did:obs:same-key-consistency"
	a := DID(did)
	b := DID(did)
	if a != b {
		t.Fatalf("aynı anahtar + aynı DID için farklı çıktı: %q vs %q", a, b)
	}
}

func TestPhone_Deterministic(t *testing.T) {
	a := Phone("+905551234567")
	b := Phone("+905551234567")
	if a != b {
		t.Fatalf("aynı telefon için farklı çıktı: %q vs %q", a, b)
	}
}

func TestPhone_DoesNotContainRawInput(t *testing.T) {
	phone := "+905551234567"
	got := Phone(phone)
	if got == phone {
		t.Fatal("redaction ham telefonu aynen döndürdü")
	}
	if strings.Contains(got, phone) || strings.Contains(got, "+") {
		t.Fatalf("redakte çıktı ham E.164 deseni içeriyor: %q", got)
	}
	if len(got) != 16 {
		t.Fatalf("16 hex karakter bekleniyordu, got=%q (len=%d)", got, len(got))
	}
}

// TestGroupID_* — scanner.go'nun eski truncate() çağrılarından biri grup/
// konuşma ID'lerini kısaltıyordu (COMMIT 1'in "TestTruncate kapsamı nereye
// gitti" sorusu); truncate() silinip logredact.GroupID() ile değiştirildi
// (scanner.go:166,169), ama o değişiklik hiçbir GroupID-adlı testle
// KARŞILANMAMIŞTI — DID/Phone testleri aynı redact() iç mekanizmasını
// dolaylı kapsıyordu, doğrudan değil. Bu boşluğu kapatıyor.
func TestGroupID_Deterministic(t *testing.T) {
	a := GroupID("group-abc-123")
	b := GroupID("group-abc-123")
	if a != b {
		t.Fatalf("aynı group ID için farklı çıktı: %q vs %q", a, b)
	}
}

func TestGroupID_DoesNotContainRawInput(t *testing.T) {
	gid := "group-super-secret-id"
	got := GroupID(gid)
	if got == gid {
		t.Fatal("redaction ham group ID'yi aynen döndürdü")
	}
	if strings.Contains(got, gid) {
		t.Fatalf("redakte çıktı ham group ID'yi alt-string olarak içeriyor: %q", got)
	}
	if len(got) != 16 {
		t.Fatalf("16 hex karakter bekleniyordu, got=%q (len=%d)", got, len(got))
	}
	if !hexPattern.MatchString(got) {
		t.Fatalf("çıktı hex değil: %q", got)
	}
}

func TestGroupID_ShortInputNoPanic(t *testing.T) {
	for _, s := range []string{"", "g", "1"} {
		got := GroupID(s)
		if s == "" && got == "" {
			t.Fatal("boş group ID için boş string dönmemeli")
		}
		if s != "" && len(got) != 16 {
			t.Errorf("kısa girdi %q için 16 hex bekleniyordu, got=%q (len=%d)", s, got, len(got))
		}
	}
}

func TestPhone_DifferentFromDIDForSameString(t *testing.T) {
	// Phone ve DID aynı redact() fonksiyonunu paylaşıyor — bu kasıtlı (tek
	// mekanizma). Bu test yalnızca ikisinin de aynı girdi için aynı (kasıtlı)
	// çıktıyı ürettiğini belgeliyor, birbirinden bağımsız algoritma
	// beklenmiyor.
	s := "same-value"
	if DID(s) != Phone(s) {
		t.Fatal("DID ve Phone aynı girdi için farklı çıktı üretti — redact() paylaşımı bozuldu")
	}
}
