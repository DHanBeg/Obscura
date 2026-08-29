package media

// C10 fail-open kökü kanıtı — minio.go'nun init()'i eskiden AccessKey/SecretKey
// boşsa bu repoda görünür sabit, hardcoded bir credential çiftine düşüyordu,
// prod-fatal YOKTU. Artık secrets.Require: prod'da (D1 fail-safe: OBSCURA_ENV
// açıkça development/dev değilse) eksikse boot FATAL.
//
// cfg paket-seviyesi init()'te BİR KEZ dolduğu için gerçek senaryoları test
// etmenin yolu subprocess re-exec.

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestMinioSecrets_RealCredsLoadCorrectly(t *testing.T) {
	if os.Getenv("BE_MINIO_SUBPROC") == "1" {
		if cfg.AccessKey != "real-access-key-xyz" || cfg.SecretKey != "real-secret-key-xyz" {
			t.Fatalf("expected real creds to be loaded into cfg, got AccessKey=%q SecretKey=%q", cfg.AccessKey, cfg.SecretKey)
		}
		os.Stdout.WriteString("MINIO_CREDS_LOADED_OK\n")
		return
	}

	stdout, stderr, err := runMinioSubprocess(t, "TestMinioSecrets_RealCredsLoadCorrectly", map[string]string{
		"BE_MINIO_SUBPROC": "1",
		"OBSCURA_ENV":      "production",
		"MINIO_ACCESS_KEY": "real-access-key-xyz",
		"MINIO_SECRET_KEY": "real-secret-key-xyz",
	})
	if err != nil {
		t.Fatalf("subprocess with real creds should succeed, got error: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "MINIO_CREDS_LOADED_OK") {
		t.Fatalf("expected success marker, got stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestMinioSecrets_ProdWithoutEnv_Fatal(t *testing.T) {
	if os.Getenv("BE_MINIO_SUBPROC") == "1" {
		os.Stdout.WriteString("UNEXPECTED_SUCCESS_NO_FATAL\n")
		return
	}

	stdout, stderr, err := runMinioSubprocess(t, "TestMinioSecrets_ProdWithoutEnv_Fatal", map[string]string{
		"BE_MINIO_SUBPROC": "1",
		"OBSCURA_ENV":      "production",
		"MINIO_ACCESS_KEY": "",
		"MINIO_SECRET_KEY": "",
	})
	if err == nil {
		t.Fatalf("expected subprocess to exit non-zero (boot FATAL on missing MINIO_ACCESS_KEY in production), got exit 0. stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stderr, "MINIO_ACCESS_KEY env required") {
		t.Fatalf("expected fatal message about MINIO_ACCESS_KEY, got stderr=%q", stderr)
	}
	if strings.Contains(stdout, "UNEXPECTED_SUCCESS_NO_FATAL") {
		t.Fatalf("process did not die at package-init — fell through instead of failing closed")
	}
}

func TestMinioSecrets_DevWithoutEnv_PlaceholderWorks(t *testing.T) {
	if os.Getenv("BE_MINIO_SUBPROC") == "1" {
		if cfg.AccessKey != "dev-only-placeholder-not-for-prod" || cfg.SecretKey != "dev-only-placeholder-not-for-prod" {
			t.Fatalf("expected dev placeholder creds, got AccessKey=%q SecretKey=%q", cfg.AccessKey, cfg.SecretKey)
		}
		os.Stdout.WriteString("MINIO_DEV_PLACEHOLDER_OK\n")
		return
	}

	stdout, stderr, err := runMinioSubprocess(t, "TestMinioSecrets_DevWithoutEnv_PlaceholderWorks", map[string]string{
		"BE_MINIO_SUBPROC": "1",
		"OBSCURA_ENV":      "development",
		"MINIO_ACCESS_KEY": "",
		"MINIO_SECRET_KEY": "",
	})
	if err != nil {
		t.Fatalf("dev-mode subprocess without creds must still succeed (placeholder), got error: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stderr, "MINIO_ACCESS_KEY not set — using dev placeholder") {
		t.Fatalf("expected dev-placeholder WARN log, got stderr=%q", stderr)
	}
	if !strings.Contains(stdout, "MINIO_DEV_PLACEHOLDER_OK") {
		t.Fatalf("expected placeholder success marker, got stdout=%q stderr=%q", stdout, stderr)
	}
}

func runMinioSubprocess(t *testing.T, testName string, overrides map[string]string) (stdout, stderr string, err error) {
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
