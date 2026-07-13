package moderation

// boundary_test.go — İlke 1 (özel alan asla taranmaz) kilidi. Sunucu zaten
// E2EE nedeniyle plaintext'i çözemez, ama bu test o garantiyi statik olarak
// da kilitler: moderation paketindeki hiçbir üretim dosyası bir decrypt/
// plaintext-okuma çağrısı içermemeli. Desen: internal/subscriber/layer_boundary_test.go.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// forbiddenDecryptRe matches any call that would decrypt message content.
// If this package ever needs real crypto, it must go through the audited
// obscura-crypto-cli subprocess bridge (internal/signal), never call these
// directly — and even then, moderation's job is metadata/hash comparison,
// never plaintext.
var forbiddenDecryptRe = regexp.MustCompile(`(?i)\b(DecryptField|\.Decrypt\(|sealed_sender\.[Uu]nseal|ratchet\.RatchetState)\b`)

func TestModerationPackageNeverDecryptsContent(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if forbiddenDecryptRe.MatchString(string(data)) {
			t.Errorf("%s calls a decrypt/plaintext-reading function; moderation must stay metadata/hash-only (İlke 1)", name)
		}
	}
}
