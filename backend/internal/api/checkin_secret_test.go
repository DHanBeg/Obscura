package api

// R7 fix (C10 launch-blocker) kanıt testi — checkinSecret()/internalSecretValue
// artık internal/secrets.Require üzerinden yükleniyor (C10 fail-open kökü
// kapatıldı — tek doğruluk kaynağı). Require'ın kendi prod-fatal/dev-
// placeholder/D1-fail-safe davranışı internal/secrets/secrets_test.go'da
// kapsamlı test ediliyor (bu dosyada tekrar edilmiyor); burada SADECE
// checkin'e özgü değer — 4 gerçek kullanım yerinin (event_handlers.go:544,
// 688, 788, 918) dayandığı GenerateCheckinQR/ValidateCheckinQR/
// hmacSHA256Base64 round-trip'inin gerçek bir secret'la uçtan uca çalıştığı
// — doğrulanıyor.
//
// checkinSecretValue paket-seviyesi bir var (process başlarken bir kez
// hesaplanır) olduğu için gerçek bir INTERNAL_SECRET ile test etmenin yolu
// test binary'sini kendi üzerine subprocess olarak yeniden çalıştırmak.

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

const checkinRoundTripMarker = "CHECKIN_ROUNDTRIP_OK"

// checkinRoundTripOrDie — 4 gerçek kullanım yerinin (event_handlers.go:544,
// 688, 788, 918) dayandığı iki mekanizmayı da uçtan uca çalıştırır.
func checkinRoundTripOrDie(t *testing.T) {
	t.Helper()

	// call site'lar 544 (ValidateCheckinQR) + 918 (GenerateCheckinQR)
	tok := GenerateCheckinQR("evt-r7-test", checkinSecret())
	gotEventID, valid := ValidateCheckinQR(tok, checkinSecret())
	if !valid || gotEventID != "evt-r7-test" {
		t.Fatalf("QR round-trip failed: valid=%v eventID=%q token=%q", valid, gotEventID, tok)
	}

	// call site'lar 688 (commitment) + 788 (expected) — ZK-QR şeması
	commitment := hmacSHA256Base64(checkinSecret(), "evt-r7-test|1234567890")
	expected := hmacSHA256Base64(checkinSecret(), "evt-r7-test|1234567890")
	if commitment != expected {
		t.Fatalf("commitment/expected mismatch: %q != %q", commitment, expected)
	}

	fmt.Println(checkinRoundTripMarker)
}

func TestCheckinSecret_WithRealEnv_RoundTripWorks(t *testing.T) {
	if os.Getenv("BE_CHECKIN_SUBPROC") == "1" {
		checkinRoundTripOrDie(t)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestCheckinSecret_WithRealEnv_RoundTripWorks$")
	base := os.Environ()
	overrides := map[string]string{
		"BE_CHECKIN_SUBPROC":   "1",
		"INTERNAL_SECRET":      "a-real-internal-secret-for-testing-0123456789",
		"OBSCURA_ENV":          "production", // gerçek secret varken prod'da da sorunsuz çalışmalı
		"TURN_SECRET":          "dummy-turn-secret-for-isolation",
		"NODE_INTERNAL_SECRET": "dummy-node-secret-for-isolation",
		"SMS_PROVIDER":         "custom", // sms.go'nun kendi prod-fatal init()'ini izole et
		// api paketi internal/media'yı da (dolaylı) import ediyor — minio.go'nun
		// init()'i artık secrets.Require ile aynı prod-fatal desende. Bu testin
		// SADECE INTERNAL_SECRET'ı izole edebilmesi için o kapıları da kapatıyoruz.
		"MINIO_ACCESS_KEY": "dummy-minio-access-key-for-isolation",
		"MINIO_SECRET_KEY": "dummy-minio-secret-key-for-isolation",
		// api paketi internal/auth'u (dolaylı) import ediyor — auth.jwtKeyBytes
		// artık secrets.Require ile aynı prod-fatal desende (C10 #8). owner_hash.go
		// da (paketin kendi içinde) messageOwnerPepper için aynı desende (C10 #9).
		// İkisi de bu testin izolasyon kapsamında.
		"JWT_SECRET":                   "dummy-jwt-secret-for-isolation",
		"OBSCURA_MESSAGE_OWNER_PEPPER": "dummy-owner-pepper-for-isolation",
		// api paketi internal/logredact'ı da (dolaylı, auth+kendi içinde) import
		// ediyor (METADATA FIX 2) — logredact.redactKey aynı prod-fatal desende.
		"OBSCURA_LOG_REDACT_KEY": "dummy-log-redact-key-for-isolation",
	}
	env := make([]string, 0, len(base)+len(overrides))
	for _, e := range base {
		skip := false
		for k := range overrides {
			if strings.HasPrefix(e, k+"=") {
				skip = true
				break
			}
		}
		if !skip {
			env = append(env, e)
		}
	}
	for k, v := range overrides {
		env = append(env, k+"="+v)
	}
	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("subprocess with real INTERNAL_SECRET should succeed, got error: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), checkinRoundTripMarker) {
		t.Fatalf("expected round-trip marker %q in subprocess stdout, got stdout=%q stderr=%q", checkinRoundTripMarker, stdout.String(), stderr.String())
	}
}
