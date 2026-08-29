package auth

// C10 fail-open kökü kanıtı — jwtKey() eskiden os.Getenv fallback'iyle
// "CHANGE_THIS_JWT_SECRET_IN_PRODUCTION" placeholder'ını reddedip
// OBSCURA_ENV=="production" DEĞİLSE (opt-out yönü) literal, repoda-açık
// "obscura-secret-change-in-production" string'ine düşüyordu — bu string'i
// bilen herkes geçerli JWT forge edebilirdi, prod-fatal YOKTU.
// Artık jwtKeyBytes = secrets.Require("JWT_SECRET"): fail-safe yönde
// (OBSCURA_ENV açıkça dev opt-in değilse prod sayılır), eksikse boot FATAL.
//
// jwtKeyBytes paket-seviyesi var init'te BİR KEZ dolduğu için gerçek
// senaryoları test etmenin yolu subprocess re-exec (minio_secret_test.go'daki
// desenin birebir kopyası).

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"

	"obscura.network/core/internal/models"
)

func TestJWTSecret_RealSecretSignsAndValidates(t *testing.T) {
	if os.Getenv("BE_AUTH_SUBPROC") == "1" {
		tok, err := GenerateToken(&testUserForJWTSecretTest)
		if err != nil {
			os.Stderr.WriteString("GENERATE_ERROR: " + err.Error() + "\n")
			return
		}
		claims, err := ValidateToken(tok)
		if err != nil || claims.DID != testUserForJWTSecretTest.DID {
			os.Stderr.WriteString("VALIDATE_ERROR\n")
			return
		}
		os.Stdout.WriteString("JWT_ROUNDTRIP_OK\n")
		return
	}

	stdout, stderr, err := runAuthSubprocess(t, "TestJWTSecret_RealSecretSignsAndValidates", map[string]string{
		"BE_AUTH_SUBPROC": "1",
		"OBSCURA_ENV":     "production",
		"JWT_SECRET":      "real-jwt-secret-xyz",
		"SMS_PROVIDER":    "netgsm",
		// auth paketi artık internal/zk'yi transitively import ediyor
		// (multi_verify.go, C10 #7) — zk.internalSecret de secrets.Require
		// ile prod-fatal. Bu testin SADECE JWT_SECRET'ı izole edebilmesi
		// için o kapıyı da kapatıyoruz.
		"INTERNAL_SECRET": "dummy-internal-secret-for-isolation",
	})
	if err != nil {
		t.Fatalf("subprocess with real JWT_SECRET should succeed, got error: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "JWT_ROUNDTRIP_OK") {
		t.Fatalf("expected roundtrip success marker, got stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestJWTSecret_ProdWithoutEnv_Fatal(t *testing.T) {
	if os.Getenv("BE_AUTH_SUBPROC") == "1" {
		os.Stdout.WriteString("UNEXPECTED_SUCCESS_NO_FATAL\n")
		return
	}

	stdout, stderr, err := runAuthSubprocess(t, "TestJWTSecret_ProdWithoutEnv_Fatal", map[string]string{
		"BE_AUTH_SUBPROC": "1",
		"OBSCURA_ENV":     "production",
		"JWT_SECRET":      "",
		"SMS_PROVIDER":    "netgsm",
		// auth paketi artık internal/zk'yi transitively import ediyor
		// (multi_verify.go, C10 #7) — zk.internalSecret de secrets.Require
		// ile prod-fatal. Bu testin SADECE JWT_SECRET'ı izole edebilmesi
		// için o kapıyı da kapatıyoruz.
		"INTERNAL_SECRET": "dummy-internal-secret-for-isolation",
	})
	if err == nil {
		t.Fatalf("expected subprocess to exit non-zero (boot FATAL on missing JWT_SECRET in production), got exit 0. stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stderr, "JWT_SECRET env required") {
		t.Fatalf("expected fatal message about JWT_SECRET, got stderr=%q", stderr)
	}
	if strings.Contains(stdout, "UNEXPECTED_SUCCESS_NO_FATAL") {
		t.Fatalf("process did not die at package-init — fell through instead of failing closed")
	}
}

func TestJWTSecret_ProdWithPlaceholderValue_StillLoadsAsIs(t *testing.T) {
	// secrets.Require only treats an EMPTY string as missing — it has no
	// notion of a "known placeholder value" to reject (unlike the old
	// jwtKey(), which special-cased "CHANGE_THIS_JWT_SECRET_IN_PRODUCTION").
	// Documenting this: any non-empty JWT_SECRET, including an old
	// placeholder value, loads as the real secret. Operators must not set
	// JWT_SECRET to a known/shared placeholder.
	if os.Getenv("BE_AUTH_SUBPROC") == "1" {
		if string(jwtKeyBytes) != "CHANGE_THIS_JWT_SECRET_IN_PRODUCTION" {
			os.Stdout.WriteString("UNEXPECTED_VALUE\n")
			return
		}
		os.Stdout.WriteString("LOADED_AS_IS_OK\n")
		return
	}

	stdout, stderr, err := runAuthSubprocess(t, "TestJWTSecret_ProdWithPlaceholderValue_StillLoadsAsIs", map[string]string{
		"BE_AUTH_SUBPROC": "1",
		"OBSCURA_ENV":     "production",
		"JWT_SECRET":      "CHANGE_THIS_JWT_SECRET_IN_PRODUCTION",
		"SMS_PROVIDER":    "netgsm",
		// auth paketi artık internal/zk'yi transitively import ediyor
		// (multi_verify.go, C10 #7) — zk.internalSecret de secrets.Require
		// ile prod-fatal. Bu testin SADECE JWT_SECRET'ı izole edebilmesi
		// için o kapıyı da kapatıyoruz.
		"INTERNAL_SECRET": "dummy-internal-secret-for-isolation",
	})
	if err != nil {
		t.Fatalf("non-empty JWT_SECRET must not fatal even if it's an old placeholder value, got error: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "LOADED_AS_IS_OK") {
		t.Fatalf("expected marker, got stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestJWTSecret_DevWithoutEnv_PlaceholderWorks(t *testing.T) {
	if os.Getenv("BE_AUTH_SUBPROC") == "1" {
		if string(jwtKeyBytes) != "dev-only-placeholder-not-for-prod" {
			os.Stdout.WriteString("UNEXPECTED_VALUE\n")
			return
		}
		os.Stdout.WriteString("JWT_DEV_PLACEHOLDER_OK\n")
		return
	}

	stdout, stderr, err := runAuthSubprocess(t, "TestJWTSecret_DevWithoutEnv_PlaceholderWorks", map[string]string{
		"BE_AUTH_SUBPROC": "1",
		"OBSCURA_ENV":     "development",
		"JWT_SECRET":      "",
	})
	if err != nil {
		t.Fatalf("dev-mode subprocess without JWT_SECRET must still succeed (placeholder), got error: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stderr, "JWT_SECRET not set — using dev placeholder") {
		t.Fatalf("expected dev-placeholder WARN log, got stderr=%q", stderr)
	}
	if !strings.Contains(stdout, "JWT_DEV_PLACEHOLDER_OK") {
		t.Fatalf("expected placeholder success marker, got stdout=%q stderr=%q", stdout, stderr)
	}
}

var testUserForJWTSecretTest = models.User{ID: "u-1", DID: "did:obs:test", Tier: 1}

func runAuthSubprocess(t *testing.T, testName string, overrides map[string]string) (stdout, stderr string, err error) {
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
