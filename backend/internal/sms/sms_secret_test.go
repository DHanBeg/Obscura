package sms

// C10 fail-open kökü kanıtı, "7. kardeş" — isimli listede yoktu, davranışsal
// grep (`OBSCURA_ENV.*==.*"production"`) ile bulundu. init()'teki
// SMS_PROVIDER=log prod-guard'ı eskiden yalnızca OBSCURA_ENV TAM OLARAK
// "production" ise fatal oluyordu (opt-out yönü) — env unutulur ya da
// "staging"/typo olursa SMS_PROVIDER=log sessizce prod'da aktif kalırdı, OTP
// kodları gerçek SMS yerine sunucu loglarına düşerdi. secrets.IsDev() ile D1
// fail-safe yönüne çevrildi.
//
// defaultProvider init()'te process başına bir kez set edildiği için gerçek
// senaryoları izole test etmenin yolu subprocess re-exec (repo genelindeki
// _secret_test.go deseninin kopyası).

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestSMSProvider_ProdWithRealProvider_NoFatal(t *testing.T) {
	if os.Getenv("BE_SMS_SUBPROC") == "1" {
		if _, ok := defaultProvider.(*LogProvider); ok {
			os.Stdout.WriteString("UNEXPECTED_LOG_PROVIDER\n")
			return
		}
		os.Stdout.WriteString("SMS_REAL_PROVIDER_OK\n")
		return
	}

	stdout, stderr, err := runSMSSubprocess(t, "TestSMSProvider_ProdWithRealProvider_NoFatal", map[string]string{
		"BE_SMS_SUBPROC": "1",
		"OBSCURA_ENV":    "production",
		"SMS_PROVIDER":   "custom",
		"SMS_API_URL":    "https://example.invalid/sms",
		"SMS_API_KEY":    "dummy-key",
		// sms paketi artık internal/logredact'ı import ediyor (METADATA FIX 2 —
		// SendOTP log satırları logredact.Phone kullanıyor), aynı prod-fatal desende.
		"OBSCURA_LOG_REDACT_KEY": "dummy-log-redact-key-for-isolation",
	})
	if err != nil {
		t.Fatalf("subprocess with real SMS_PROVIDER should succeed, got error: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "SMS_REAL_PROVIDER_OK") {
		t.Fatalf("expected marker, got stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestSMSProvider_ProdWithoutProvider_Fatal(t *testing.T) {
	if os.Getenv("BE_SMS_SUBPROC") == "1" {
		os.Stdout.WriteString("UNEXPECTED_SUCCESS_NO_FATAL\n")
		return
	}

	stdout, stderr, err := runSMSSubprocess(t, "TestSMSProvider_ProdWithoutProvider_Fatal", map[string]string{
		"BE_SMS_SUBPROC": "1",
		"OBSCURA_ENV":    "production",
		"SMS_PROVIDER":   "",
		// logredact.redactKey init'i SMS_PROVIDER kontrolünden ÖNCE FATAL
		// olmasın diye izole ediyoruz — bu test SADECE SMS_PROVIDER fatal
		// mesajını doğruluyor.
		"OBSCURA_LOG_REDACT_KEY": "dummy-log-redact-key-for-isolation",
	})
	if err == nil {
		t.Fatalf("expected subprocess to exit non-zero (FATAL: SMS_PROVIDER=log active in production), got exit 0. stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stderr, "SMS_PROVIDER env zorunlu") {
		t.Fatalf("expected fatal message about SMS_PROVIDER, got stderr=%q", stderr)
	}
	if strings.Contains(stdout, "UNEXPECTED_SUCCESS_NO_FATAL") {
		t.Fatalf("process did not die at package-init — fell through instead of failing closed")
	}
}

func TestSMSProvider_StagingTypo_StillFatal(t *testing.T) {
	// Kök bug'ın tam kanıtı: eski kod SADECE OBSCURA_ENV=="production" ise
	// fatal oluyordu — "staging" gibi başka herhangi bir değer sessizce
	// SMS_PROVIDER=log'u prod'da aktif bırakıyordu (OTP'ler loglara düşer).
	if os.Getenv("BE_SMS_SUBPROC") == "1" {
		os.Stdout.WriteString("UNEXPECTED_SUCCESS_NO_FATAL\n")
		return
	}

	stdout, stderr, err := runSMSSubprocess(t, "TestSMSProvider_StagingTypo_StillFatal", map[string]string{
		"BE_SMS_SUBPROC": "1",
		"OBSCURA_ENV":    "staging",
		"SMS_PROVIDER":   "",
		// logredact.redactKey init'i SMS_PROVIDER kontrolünden ÖNCE FATAL
		// olmasın diye izole ediyoruz — bu test SADECE SMS_PROVIDER fatal
		// mesajını doğruluyor.
		"OBSCURA_LOG_REDACT_KEY": "dummy-log-redact-key-for-isolation",
	})
	if err == nil {
		t.Fatalf("expected subprocess to exit non-zero (OBSCURA_ENV=staging is not a dev opt-in), got exit 0. stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stderr, "SMS_PROVIDER env zorunlu") {
		t.Fatalf("expected fatal message, got stderr=%q", stderr)
	}
	if strings.Contains(stdout, "UNEXPECTED_SUCCESS_NO_FATAL") {
		t.Fatalf("process did not die under OBSCURA_ENV=staging — fail-open regression")
	}
}

func TestSMSProvider_DevWithoutProvider_LogProviderWorks(t *testing.T) {
	if os.Getenv("BE_SMS_SUBPROC") == "1" {
		if _, ok := defaultProvider.(*LogProvider); !ok {
			os.Stdout.WriteString("UNEXPECTED_NON_LOG_PROVIDER\n")
			return
		}
		os.Stdout.WriteString("SMS_DEV_LOG_PROVIDER_OK\n")
		return
	}

	stdout, stderr, err := runSMSSubprocess(t, "TestSMSProvider_DevWithoutProvider_LogProviderWorks", map[string]string{
		"BE_SMS_SUBPROC": "1",
		"OBSCURA_ENV":    "development",
		"SMS_PROVIDER":   "",
	})
	if err != nil {
		t.Fatalf("dev-mode subprocess without SMS_PROVIDER must still succeed (log fallback), got error: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "SMS_DEV_LOG_PROVIDER_OK") {
		t.Fatalf("expected marker, got stdout=%q stderr=%q", stdout, stderr)
	}
}

func runSMSSubprocess(t *testing.T, testName string, overrides map[string]string) (stdout, stderr string, err error) {
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
