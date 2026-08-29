package api

// C10 fail-open kökü kanıtı, #9 — messageOwnerPepper eskiden
// InitMessageOwnerPepperFromEnv() ile main.go'da elle çağrılıyordu,
// `os.Getenv("OBSCURA_ENV") == "production"` (opt-out yönü) kontrolü vardı —
// env unutulur/yanlış yazılırsa sessizce repoda-açık devMessageOwnerPepper'a
// düşüyordu. Artık paket-seviyesi var (messageOwnerPepper = secrets.Require(...)),
// D1 fail-safe: OBSCURA_ENV açık dev opt-in değilse prod sayılır, eksikse
// boot FATAL.
//
// Paket-seviyesi var process başına bir kez set edildiği için gerçek
// senaryoları izole test etmenin yolu subprocess re-exec (checkin_secret_test.go
// deseninin kopyası — aynı izolasyon env'leri gerekli: api paketi auth/media
// üzerinden JWT_SECRET/MINIO_*'yi de, kendi içinde INTERNAL_SECRET'ı da
// package-init'te gerektiriyor).

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func ownerHashSubprocIsolationEnv(overrides map[string]string) map[string]string {
	base := map[string]string{
		"INTERNAL_SECRET":      "dummy-internal-secret-for-isolation",
		"TURN_SECRET":          "dummy-turn-secret-for-isolation",
		"NODE_INTERNAL_SECRET": "dummy-node-secret-for-isolation",
		"SMS_PROVIDER":         "custom",
		"MINIO_ACCESS_KEY":     "dummy-minio-access-key-for-isolation",
		"MINIO_SECRET_KEY":     "dummy-minio-secret-key-for-isolation",
		"JWT_SECRET":           "dummy-jwt-secret-for-isolation",
	}
	for k, v := range overrides {
		base[k] = v
	}
	return base
}

func TestOwnerHashPepper_RealEnv_HashRoundtripWorks(t *testing.T) {
	if os.Getenv("BE_OWNERHASH_SUBPROC") == "1" {
		h1 := computeOwnerHash("did:obs:abc", "msg-1")
		h2 := computeOwnerHash("did:obs:abc", "msg-1")
		if h1 != h2 {
			os.Stderr.WriteString("HASH_NOT_DETERMINISTIC\n")
			return
		}
		if !ownerHashMatches("did:obs:abc", "msg-1", h1) {
			os.Stderr.WriteString("HASH_DOES_NOT_MATCH_ITSELF\n")
			return
		}
		if ownerHashMatches("did:obs:other", "msg-1", h1) {
			os.Stderr.WriteString("WRONG_DID_MATCHED\n")
			return
		}
		os.Stdout.WriteString("OWNER_HASH_ROUNDTRIP_OK\n")
		return
	}

	stdout, stderr, err := runOwnerHashSubprocess(t, "TestOwnerHashPepper_RealEnv_HashRoundtripWorks", ownerHashSubprocIsolationEnv(map[string]string{
		"BE_OWNERHASH_SUBPROC":         "1",
		"OBSCURA_ENV":                  "production",
		"OBSCURA_MESSAGE_OWNER_PEPPER": "real-owner-pepper-xyz",
	}))
	if err != nil {
		t.Fatalf("subprocess with real pepper should succeed, got error: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "OWNER_HASH_ROUNDTRIP_OK") {
		t.Fatalf("expected roundtrip success marker, got stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestOwnerHashPepper_ProdWithoutEnv_Fatal(t *testing.T) {
	if os.Getenv("BE_OWNERHASH_SUBPROC") == "1" {
		os.Stdout.WriteString("UNEXPECTED_SUCCESS_NO_FATAL\n")
		return
	}

	stdout, stderr, err := runOwnerHashSubprocess(t, "TestOwnerHashPepper_ProdWithoutEnv_Fatal", ownerHashSubprocIsolationEnv(map[string]string{
		"BE_OWNERHASH_SUBPROC":         "1",
		"OBSCURA_ENV":                  "production",
		"OBSCURA_MESSAGE_OWNER_PEPPER": "",
	}))
	if err == nil {
		t.Fatalf("expected subprocess to exit non-zero (FATAL on missing OBSCURA_MESSAGE_OWNER_PEPPER in production), got exit 0. stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stderr, "OBSCURA_MESSAGE_OWNER_PEPPER env required") {
		t.Fatalf("expected fatal message about OBSCURA_MESSAGE_OWNER_PEPPER, got stderr=%q", stderr)
	}
	if strings.Contains(stdout, "UNEXPECTED_SUCCESS_NO_FATAL") {
		t.Fatalf("process did not die at package-init — fell through instead of failing closed")
	}
}

func TestOwnerHashPepper_DevWithoutEnv_PlaceholderWorks(t *testing.T) {
	if os.Getenv("BE_OWNERHASH_SUBPROC") == "1" {
		if !ownerHashMatches("did:obs:abc", "msg-1", computeOwnerHash("did:obs:abc", "msg-1")) {
			os.Stderr.WriteString("HASH_DOES_NOT_MATCH_ITSELF\n")
			return
		}
		os.Stdout.WriteString("OWNER_HASH_DEV_PLACEHOLDER_OK\n")
		return
	}

	stdout, stderr, err := runOwnerHashSubprocess(t, "TestOwnerHashPepper_DevWithoutEnv_PlaceholderWorks", ownerHashSubprocIsolationEnv(map[string]string{
		"BE_OWNERHASH_SUBPROC":         "1",
		"OBSCURA_ENV":                  "development",
		"OBSCURA_MESSAGE_OWNER_PEPPER": "",
	}))
	if err != nil {
		t.Fatalf("dev-mode subprocess without pepper must still succeed (placeholder), got error: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stderr, "OBSCURA_MESSAGE_OWNER_PEPPER not set — using dev placeholder") {
		t.Fatalf("expected dev-placeholder WARN log, got stderr=%q", stderr)
	}
	if !strings.Contains(stdout, "OWNER_HASH_DEV_PLACEHOLDER_OK") {
		t.Fatalf("expected placeholder success marker, got stdout=%q stderr=%q", stdout, stderr)
	}
}

func runOwnerHashSubprocess(t *testing.T, testName string, overrides map[string]string) (stdout, stderr string, err error) {
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
