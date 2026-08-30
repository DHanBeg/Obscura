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
//
// C12 (deploy-config placeholder-reddi, config katmanının C10'daki
// fail-open'a yedinci kardeşi — env boş değil, çöp-dolu):
//
//  6. TestIsPlaceholder_KnownLiterals — Faz 0 envanterindeki 7 CHANGE_THIS_
//     literalinin HEPSİ true dönmeli (eksik = kaçan açık).
//  7. TestIsPlaceholder_RealSecretsNotFlagged — openssl rand tarzı gerçek
//     secret'lar false dönmeli.
//  8. TestIsPlaceholder_NoFuzzyMatch — "change" geçen ama CHANGE_THIS_
//     PREFIX'i olmayan bir değer false dönmeli — substring/fuzzy'ye
//     kaymadığımızın kanıtı.
//  9. TestValidateSecret_DevAllowsPlaceholder — isDev=true + placeholder →
//     nil error (Require değeri döner, dev kırılmaz).
// 10. TestValidateSecret_ProdRejectsPlaceholder — isDev=false + placeholder →
//     non-nil error (saf, log.Fatal olmadan).
// 11. TestRequire_FatalOnPlaceholderInProd — tam entegrasyon: Require()
//     gerçekten placeholder + prod'da log.Fatal'a düşüyor mu (subprocess).

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

// TestIsPlaceholder_KnownLiterals — Faz 0 envanterindeki 7 CHANGE_THIS_
// literalinin (env dosyalarından birebir) hepsi tanınmalı.
func TestIsPlaceholder_KnownLiterals(t *testing.T) {
	literals := []string{
		"CHANGE_THIS_SECRET_IN_PRODUCTION",              // .env:10 NODE_INTERNAL_SECRET
		"CHANGE_THIS_INTERNAL_SECRET_IN_PRODUCTION",      // .env:20 INTERNAL_SECRET
		"CHANGE_THIS_JWT_SECRET_IN_PRODUCTION",           // .env:24 JWT_SECRET
		"CHANGE_THIS_PHONE_PEPPER_IN_PRODUCTION",         // .env:31 OBSCURA_PHONE_PEPPER
		"CHANGE_THIS_MESSAGE_OWNER_PEPPER_IN_PRODUCTION", // .env:34 OBSCURA_MESSAGE_OWNER_PEPPER
		"CHANGE_THIS_MINIO_SECRET",                       // .env:67 MINIO_SECRET_KEY
		"CHANGE_THIS_TURN_SECRET",                        // .env:76 TURN_SHARED_SECRET
	}
	for _, lit := range literals {
		t.Run(lit, func(t *testing.T) {
			if !isPlaceholder(lit) {
				t.Fatalf("isPlaceholder(%q) = false, want true — kaçan açık", lit)
			}
		})
	}
}

// TestIsPlaceholder_RealSecretsNotFlagged — gerçek-görünüm secret'lar
// (openssl rand -hex 32 / base64url tarzı) yanlışlıkla reddedilmemeli.
func TestIsPlaceholder_RealSecretsNotFlagged(t *testing.T) {
	real := []string{
		"9f3a1c7e2b8d4f6a0e5c9b1d7f3a8e2c4b6d0f9a1e7c3b5d8f2a0e6c4b9d1f7a", // hex32
		"kQ3rP9xL2mN7vB4wY8zT1cJ6hF0dS5uA-6nR2gK8pM4qV0xW",                // base64url
		"a3f9d1b2c8e7-secret-value-for-webrtc-turn",
	}
	for _, v := range real {
		t.Run(v, func(t *testing.T) {
			if isPlaceholder(v) {
				t.Fatalf("isPlaceholder(%q) = true, want false — gerçek secret yanlış reddedildi", v)
			}
		})
	}
}

// TestIsPlaceholder_NoFuzzyMatch — "change" kelimesi geçen ama
// CHANGE_THIS_ PREFİX'i olmayan değerler false dönmeli. Fuzzy/substring'e
// kaymadığımızın kanıtı — bu test yoksa yanlış-pozitif riski belgelenmez.
func TestIsPlaceholder_NoFuzzyMatch(t *testing.T) {
	notPlaceholders := []string{
		"my-changelog-signing-key-a3f9",
		"exchange-rate-hmac-secret-9d1f",
		"change_this_secret_in_production", // küçük harf — case-sensitive, prefix DEĞİL
		"XCHANGE_THIS_SECRET",              // prefix'te değil, ortada geçiyor
	}
	for _, v := range notPlaceholders {
		t.Run(v, func(t *testing.T) {
			if isPlaceholder(v) {
				t.Fatalf("isPlaceholder(%q) = true, want false — fuzzy/substring'e kaymış", v)
			}
		})
	}
}

// TestValidateSecret_DevAllowsPlaceholder — dev'de placeholder engellenmez,
// Require() değeri aynen döner (local development kırılmaz).
func TestValidateSecret_DevAllowsPlaceholder(t *testing.T) {
	if err := validateSecret("JWT_SECRET", "CHANGE_THIS_JWT_SECRET_IN_PRODUCTION", true); err != nil {
		t.Fatalf("dev + placeholder should be nil error, got %v", err)
	}
}

// TestValidateSecret_ProdRejectsPlaceholder — prod'da placeholder saf
// hata döner (log.Fatal olmadan, doğrudan test edilebilir).
func TestValidateSecret_ProdRejectsPlaceholder(t *testing.T) {
	err := validateSecret("JWT_SECRET", "CHANGE_THIS_JWT_SECRET_IN_PRODUCTION", false)
	if err == nil {
		t.Fatal("prod + placeholder should be non-nil error, got nil")
	}
	if !strings.Contains(err.Error(), "JWT_SECRET") || !strings.Contains(err.Error(), "CHANGE_THIS_JWT_SECRET_IN_PRODUCTION") {
		t.Fatalf("error should name the key and the literal, got %q", err.Error())
	}
	// gerçek değer prod'da asla reddedilmemeli
	if err := validateSecret("JWT_SECRET", "a3f9d1b2c8e7-real-secret", false); err != nil {
		t.Fatalf("prod + real value should be nil error, got %v", err)
	}
}

// TestRequire_FatalOnPlaceholderInProd — tam entegrasyon kanıtı: Require()
// gerçekten placeholder + prod (OBSCURA_ENV unset) kombinasyonunda
// log.Fatal'a düşüyor mu. log.Fatal os.Exit çağırdığı için subprocess
// deseni (TestRequire_FatalWhenNotExplicitDevOptIn ile aynı) kullanılıyor.
func TestRequire_FatalOnPlaceholderInProd(t *testing.T) {
	if os.Getenv("BE_SECRETS_PLACEHOLDER_SUBPROC") == "1" {
		_ = Require("SECRETS_TEST_KEY_PLACEHOLDER")
		os.Stdout.WriteString("UNEXPECTED_SUCCESS_NO_FATAL\n")
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestRequire_FatalOnPlaceholderInProd$")
	base := os.Environ()
	env := make([]string, 0, len(base)+3)
	for _, e := range base {
		if strings.HasPrefix(e, "OBSCURA_ENV=") || strings.HasPrefix(e, "SECRETS_TEST_KEY_PLACEHOLDER=") {
			continue
		}
		env = append(env, e)
	}
	env = append(env,
		"BE_SECRETS_PLACEHOLDER_SUBPROC=1",
		"OBSCURA_ENV=", // unset — D1 fail-safe: prod sayılır
		"SECRETS_TEST_KEY_PLACEHOLDER=CHANGE_THIS_SECRET_IN_PRODUCTION",
	)
	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	if err == nil {
		t.Fatalf("placeholder value in prod must be FATAL, but subprocess exited 0. stdout=%s stderr=%s",
			stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "bilinen placeholder") {
		t.Fatalf("expected fatal message about known placeholder, got stderr=%q", stderr.String())
	}
	if strings.Contains(stdout.String(), "UNEXPECTED_SUCCESS_NO_FATAL") {
		t.Fatal("Require returned the placeholder instead of dying — C12 gap not closed")
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
