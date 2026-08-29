package secrets

// C10 fail-open kökü kapatıldı — bu paket 9 call-site'ın (checkin, webrtc,
// gossip, minio x2, storage x4) tek doğruluk kaynağı. Kanıt testleri:
//
//  1. TestRequire_ReturnsRealValueWhenSet — env set → onu döner (fallback'e
//     hiç düşmez).
//  2. TestRequire_DevPlaceholderWhenExplicitDevOptIn — OBSCURA_ENV=development
//     + env boş → placeholder + WARN, FATAL olmaz.
//  3. TestRequire_FatalWhenNotExplicitDevOptIn — D1 fail-safe: OBSCURA_ENV
//     UNSET (boş string, en yaygın "unutuldu" durumu) VE OBSCURA_ENV=production
//     VE yanlış yazılmış bir değer (ör. "produciton") — HEPSİ FATAL. Sadece
//     tam olarak "development"/"dev" placeholder'a düşürür, başka hiçbir şey
//     değil.
//  4. TestRequire_AliasChain — webrtc.go'nun TURN_SECRET→TURN_SHARED_SECRET
//     zincirinin dayandığı çoklu-anahtar davranışı.
//  5. TestRequireEqual — sabit-zamanlı karşılaştırma doğruluğu.

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestRequire_ReturnsRealValueWhenSet(t *testing.T) {
	t.Setenv("SECRETS_TEST_KEY_A", "real-value-123")
	got := Require("SECRETS_TEST_KEY_A")
	if got != "real-value-123" {
		t.Fatalf("expected real env value, got %q", got)
	}
}

func TestRequire_DevPlaceholderWhenExplicitDevOptIn(t *testing.T) {
	for _, dev := range []string{"development", "dev"} {
		t.Run(dev, func(t *testing.T) {
			t.Setenv("OBSCURA_ENV", dev)
			t.Setenv("SECRETS_TEST_KEY_B", "")
			got := Require("SECRETS_TEST_KEY_B")
			if got != "dev-only-placeholder-not-for-prod" {
				t.Fatalf("expected dev placeholder, got %q", got)
			}
		})
	}
}

func TestRequire_AliasChain(t *testing.T) {
	// birincil boş, ikinci alias'ta değer var → alias kazanmalı
	t.Setenv("SECRETS_TEST_PRIMARY", "")
	t.Setenv("SECRETS_TEST_ALIAS", "alias-value")
	got := Require("SECRETS_TEST_PRIMARY", "SECRETS_TEST_ALIAS")
	if got != "alias-value" {
		t.Fatalf("expected alias value, got %q", got)
	}
}

func TestRequireEqual(t *testing.T) {
	if !RequireEqual("same-secret", "same-secret") {
		t.Fatal("expected match to return true")
	}
	if RequireEqual("secret-a", "secret-b") {
		t.Fatal("expected mismatch to return false")
	}
	if RequireEqual("real-secret", "") {
		t.Fatal("empty incoming value must never match a real secret")
	}
}

// TestRequire_FatalWhenNotExplicitDevOptIn — D1 kanıtı. log.Fatal os.Exit
// çağırdığı için test binary'yi kendi üzerine subprocess olarak
// yeniden çalıştırıyoruz (Go'nun os.Exit içeren kod için standart deseni).
func TestRequire_FatalWhenNotExplicitDevOptIn(t *testing.T) {
	if os.Getenv("BE_SECRETS_SUBPROC") == "1" {
		// Bu satırlara ulaşmak BAŞARISIZLIK demek — Require paket-init'te
		// değil ama BURADA (test body) çağrılıyor, o yüzden gerçekten
		// log.Fatal'a düşüp düşmediğini görebiliriz.
		_ = Require("SECRETS_TEST_KEY_FATAL")
		os.Stdout.WriteString("UNEXPECTED_SUCCESS_NO_FATAL\n")
		return
	}

	cases := []struct {
		name      string
		obscuraEnv string
	}{
		{"unset", ""},              // en yaygın durum: OBSCURA_ENV hiç ayarlanmamış
		{"production", "production"},
		{"typo", "produciton"},     // yanlış yazım fail-safe tarafa düşmeli
		{"Development_wrong_case", "Development"}, // case-sensitive — tam eşleşme değilse dev sayılmaz
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestRequire_FatalWhenNotExplicitDevOptIn$")
			base := os.Environ()
			env := make([]string, 0, len(base)+3)
			for _, e := range base {
				if strings.HasPrefix(e, "OBSCURA_ENV=") || strings.HasPrefix(e, "SECRETS_TEST_KEY_FATAL=") {
					continue
				}
				env = append(env, e)
			}
			env = append(env,
				"BE_SECRETS_SUBPROC=1",
				"OBSCURA_ENV="+tc.obscuraEnv,
				"SECRETS_TEST_KEY_FATAL=",
			)
			cmd.Env = env

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			err := cmd.Run()

			if err == nil {
				t.Fatalf("OBSCURA_ENV=%q must be FATAL (fail-safe default), but subprocess exited 0. stdout=%s stderr=%s",
					tc.obscuraEnv, stdout.String(), stderr.String())
			}
			if !strings.Contains(stderr.String(), "SECRETS_TEST_KEY_FATAL env required") {
				t.Fatalf("expected fatal message mentioning the key, got stderr=%q", stderr.String())
			}
			if strings.Contains(stdout.String(), "UNEXPECTED_SUCCESS_NO_FATAL") {
				t.Fatalf("Require returned instead of dying — fail-open regression. OBSCURA_ENV=%q", tc.obscuraEnv)
			}
		})
	}
}
