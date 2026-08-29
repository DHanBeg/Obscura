package subscriber

// C10 fail-open kökü kanıtı, #10/#11 — InitCryptoFromEnv (OBSCURA_SUBSCRIBER_KEY)
// ve PepperFromEnv (OBSCURA_PHONE_PEPPER) eskiden `os.Getenv("OBSCURA_ENV") ==
// "production"` (opt-out yönü) kontrolüyle prod-fatal yapıyordu — env unutulur
// ya da yanlış yazılırsa (ör. "staging") sessizce deterministik, repoda-açık
// dev fallback'e düşüyordu. Artık D1 fail-safe (secrets.IsDev() / secrets.Require):
// OBSCURA_ENV açıkça development/dev opt-in DEĞİLSE prod sayılır, eksikse FATAL.
//
// Paket-seviyesi durum (aead) process başına bir kez set edildiği ve
// InitCryptoFromEnv/PepperFromEnv os.Getenv'i her çağrıda taze okuduğu için
// (package-init-once var değil, fonksiyon) gerçek prod-fatal senaryosunu
// izole test etmenin yolu yine subprocess re-exec (minio_secret_test.go
// deseninin kopyası).

import (
	"bytes"
	"encoding/base64"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestSubscriberKey_RealEnvLoadsCorrectly(t *testing.T) {
	if os.Getenv("BE_SUB_SUBPROC") == "1" {
		if err := InitCryptoFromEnv(); err != nil {
			os.Stderr.WriteString("INIT_ERROR: " + err.Error() + "\n")
			return
		}
		pt := []byte("roundtrip-check")
		ct, err := EncryptField(pt)
		if err != nil {
			os.Stderr.WriteString("ENCRYPT_ERROR: " + err.Error() + "\n")
			return
		}
		got, err := DecryptField(ct)
		if err != nil || string(got) != string(pt) {
			os.Stderr.WriteString("DECRYPT_MISMATCH\n")
			return
		}
		os.Stdout.WriteString("SUB_KEY_ROUNDTRIP_OK\n")
		return
	}

	realKey := base64Std32()
	stdout, stderr, err := runSubscriberSubprocess(t, "TestSubscriberKey_RealEnvLoadsCorrectly", map[string]string{
		"BE_SUB_SUBPROC":         "1",
		"OBSCURA_ENV":            "production",
		"OBSCURA_SUBSCRIBER_KEY": realKey,
	})
	if err != nil {
		t.Fatalf("subprocess with real key should succeed, got error: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "SUB_KEY_ROUNDTRIP_OK") {
		t.Fatalf("expected roundtrip success marker, got stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestSubscriberKey_ProdWithoutEnv_Fatal(t *testing.T) {
	if os.Getenv("BE_SUB_SUBPROC") == "1" {
		if err := InitCryptoFromEnv(); err != nil {
			os.Stderr.WriteString("INIT_ERROR: " + err.Error() + "\n")
			return
		}
		os.Stdout.WriteString("UNEXPECTED_SUCCESS_NO_FATAL\n")
		return
	}

	stdout, stderr, err := runSubscriberSubprocess(t, "TestSubscriberKey_ProdWithoutEnv_Fatal", map[string]string{
		"BE_SUB_SUBPROC":         "1",
		"OBSCURA_ENV":            "production",
		"OBSCURA_SUBSCRIBER_KEY": "",
	})
	if err == nil {
		t.Fatalf("expected subprocess to exit non-zero (FATAL on missing OBSCURA_SUBSCRIBER_KEY in production), got exit 0. stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stderr, "OBSCURA_SUBSCRIBER_KEY env required") {
		t.Fatalf("expected fatal message about OBSCURA_SUBSCRIBER_KEY, got stderr=%q", stderr)
	}
	if strings.Contains(stdout, "UNEXPECTED_SUCCESS_NO_FATAL") {
		t.Fatalf("process did not die — fell through instead of failing closed")
	}
}

func TestSubscriberKey_StagingTypo_StillFatal(t *testing.T) {
	// Kök bug'ın tam kanıtı: eski kod SADECE OBSCURA_ENV=="production" ise
	// fatal oluyordu — "staging" gibi başka herhangi bir değer (ya da typo)
	// sessizce dev fallback'e düşüyordu. D1 fail-safe altında "staging" da
	// prod sayılmalı (yalnızca development/dev açık opt-in).
	if os.Getenv("BE_SUB_SUBPROC") == "1" {
		if err := InitCryptoFromEnv(); err != nil {
			os.Stderr.WriteString("INIT_ERROR: " + err.Error() + "\n")
			return
		}
		os.Stdout.WriteString("UNEXPECTED_SUCCESS_NO_FATAL\n")
		return
	}

	stdout, stderr, err := runSubscriberSubprocess(t, "TestSubscriberKey_StagingTypo_StillFatal", map[string]string{
		"BE_SUB_SUBPROC":         "1",
		"OBSCURA_ENV":            "staging",
		"OBSCURA_SUBSCRIBER_KEY": "",
	})
	if err == nil {
		t.Fatalf("expected subprocess to exit non-zero (OBSCURA_ENV=staging is not a dev opt-in), got exit 0. stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stderr, "OBSCURA_SUBSCRIBER_KEY env required") {
		t.Fatalf("expected fatal message, got stderr=%q", stderr)
	}
	if strings.Contains(stdout, "UNEXPECTED_SUCCESS_NO_FATAL") {
		t.Fatalf("process did not die under OBSCURA_ENV=staging — fail-open regression")
	}
}

func TestSubscriberKey_DevWithoutEnv_PlaceholderWorks(t *testing.T) {
	if os.Getenv("BE_SUB_SUBPROC") == "1" {
		if err := InitCryptoFromEnv(); err != nil {
			os.Stderr.WriteString("INIT_ERROR: " + err.Error() + "\n")
			return
		}
		pt := []byte("dev-roundtrip")
		ct, err := EncryptField(pt)
		if err != nil {
			os.Stderr.WriteString("ENCRYPT_ERROR: " + err.Error() + "\n")
			return
		}
		got, err := DecryptField(ct)
		if err != nil || string(got) != string(pt) {
			os.Stderr.WriteString("DECRYPT_MISMATCH\n")
			return
		}
		os.Stdout.WriteString("SUB_KEY_DEV_PLACEHOLDER_OK\n")
		return
	}

	stdout, stderr, err := runSubscriberSubprocess(t, "TestSubscriberKey_DevWithoutEnv_PlaceholderWorks", map[string]string{
		"BE_SUB_SUBPROC":         "1",
		"OBSCURA_ENV":            "development",
		"OBSCURA_SUBSCRIBER_KEY": "",
	})
	if err != nil {
		t.Fatalf("dev-mode subprocess without key must still succeed (placeholder), got error: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stderr, "OBSCURA_SUBSCRIBER_KEY ayarlanmamış") {
		t.Fatalf("expected dev-placeholder WARN log, got stderr=%q", stderr)
	}
	if !strings.Contains(stdout, "SUB_KEY_DEV_PLACEHOLDER_OK") {
		t.Fatalf("expected placeholder success marker, got stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestPhonePepper_RealEnvLoadsCorrectly(t *testing.T) {
	if os.Getenv("BE_SUB_SUBPROC") == "1" {
		p, err := PepperFromEnv()
		if err != nil || string(p) != "real-phone-pepper-xyz" {
			os.Stderr.WriteString("PEPPER_MISMATCH\n")
			return
		}
		os.Stdout.WriteString("PEPPER_REAL_OK\n")
		return
	}

	stdout, stderr, err := runSubscriberSubprocess(t, "TestPhonePepper_RealEnvLoadsCorrectly", map[string]string{
		"BE_SUB_SUBPROC":       "1",
		"OBSCURA_ENV":          "production",
		"OBSCURA_PHONE_PEPPER": "real-phone-pepper-xyz",
	})
	if err != nil {
		t.Fatalf("subprocess with real pepper should succeed, got error: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "PEPPER_REAL_OK") {
		t.Fatalf("expected marker, got stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestPhonePepper_ProdWithoutEnv_Fatal(t *testing.T) {
	if os.Getenv("BE_SUB_SUBPROC") == "1" {
		_, _ = PepperFromEnv()
		os.Stdout.WriteString("UNEXPECTED_SUCCESS_NO_FATAL\n")
		return
	}

	stdout, stderr, err := runSubscriberSubprocess(t, "TestPhonePepper_ProdWithoutEnv_Fatal", map[string]string{
		"BE_SUB_SUBPROC":       "1",
		"OBSCURA_ENV":          "production",
		"OBSCURA_PHONE_PEPPER": "",
	})
	if err == nil {
		t.Fatalf("expected subprocess to exit non-zero (FATAL on missing OBSCURA_PHONE_PEPPER in production), got exit 0. stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stderr, "OBSCURA_PHONE_PEPPER env required") {
		t.Fatalf("expected fatal message about OBSCURA_PHONE_PEPPER, got stderr=%q", stderr)
	}
	if strings.Contains(stdout, "UNEXPECTED_SUCCESS_NO_FATAL") {
		t.Fatalf("process did not die — fell through instead of failing closed")
	}
}

func TestPhonePepper_DevWithoutEnv_PlaceholderWorks(t *testing.T) {
	if os.Getenv("BE_SUB_SUBPROC") == "1" {
		p, err := PepperFromEnv()
		if err != nil || string(p) != "dev-only-placeholder-not-for-prod" {
			os.Stderr.WriteString("PEPPER_MISMATCH\n")
			return
		}
		os.Stdout.WriteString("PEPPER_DEV_PLACEHOLDER_OK\n")
		return
	}

	stdout, stderr, err := runSubscriberSubprocess(t, "TestPhonePepper_DevWithoutEnv_PlaceholderWorks", map[string]string{
		"BE_SUB_SUBPROC":       "1",
		"OBSCURA_ENV":          "development",
		"OBSCURA_PHONE_PEPPER": "",
	})
	if err != nil {
		t.Fatalf("dev-mode subprocess without pepper must still succeed (placeholder), got error: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stderr, "OBSCURA_PHONE_PEPPER not set — using dev placeholder") {
		t.Fatalf("expected dev-placeholder WARN log, got stderr=%q", stderr)
	}
	if !strings.Contains(stdout, "PEPPER_DEV_PLACEHOLDER_OK") {
		t.Fatalf("expected placeholder success marker, got stdout=%q stderr=%q", stdout, stderr)
	}
}

func base64Std32() string {
	// 32 raw bytes, base64 STANDARD encoded (InitCryptoFromEnv's expected format).
	raw := make([]byte, keyLen)
	for i := range raw {
		raw[i] = byte(i + 1)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func runSubscriberSubprocess(t *testing.T, testName string, overrides map[string]string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^"+testName+"$")
	base := os.Environ()
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

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return outBuf.String(), errBuf.String(), err
}
