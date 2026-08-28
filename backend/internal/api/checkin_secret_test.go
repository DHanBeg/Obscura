package api

// R7 fix (C10 launch-blocker) kanıt testleri — checkinSecretValue artık
// webrtc.go:31-43 (TURN_SECRET) / gossip.go:31-40 (NODE_INTERNAL_SECRET) ile
// AYNI desen: process başlangıcında INTERNAL_SECRET'tan bir kez okunur,
// prod'da eksikse boot FATAL, dev'de açık placeholder + WARN.
//
// checkinSecretValue paket-seviyesi bir var-IIFE olduğu için (process
// başlarken BİR KEZ hesaplanır), farklı env senaryolarını (prod+boş,
// dev+boş, gerçek secret) test etmenin tek yolu test binary'sini kendi
// üzerine subprocess olarak yeniden çalıştırmak (Go'nun os.Exit/log.Fatal
// içeren kod için standart test deseni) — running go test itself with
// controlled env ile.
//
// Üç kanıt, üç test:
//  1. TestCheckinSecret_WithRealEnv_RoundTripWorks — INTERNAL_SECRET set →
//     4 çağrı yerinin (544/688/788/918) dayandığı GenerateCheckinQR/
//     ValidateCheckinQR/hmacSHA256Base64 round-trip'i doğru çalışıyor.
//  2. TestCheckinSecret_ProdWithoutEnv_Fatal — prod-mode + INTERNAL_SECRET
//     boş → boot FATAL, hardcoded'a hiç düşmüyor (round-trip marker'ı hiç
//     basılmıyor çünkü package init'te ölüyor).
//  3. TestCheckinSecret_DevWithoutEnv_PlaceholderWorks — dev-mode + boş →
//     WARN log + açık placeholder ile round-trip yine çalışıyor.

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

// cleanCheckinEnv — os.Environ()'dan bu testlerin kontrol ettiği anahtarları
// çıkarır, sonra istenen değerlerle yeniden ekler — Go'da tekrarlanan
// env anahtarlarının hangisinin kazanacağı garantili olmadığı için
// (append yerine) filtrele-ve-ekle deterministik.
func cleanCheckinEnv(overrides map[string]string) []string {
	base := os.Environ()
	out := make([]string, 0, len(base)+len(overrides))
	for _, e := range base {
		skip := false
		for k := range overrides {
			if strings.HasPrefix(e, k+"=") {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, e)
		}
	}
	for k, v := range overrides {
		out = append(out, k+"="+v)
	}
	return out
}

func runCheckinSubprocess(t *testing.T, testName string, env map[string]string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^"+testName+"$", "-test.v")
	cmd.Env = cleanCheckinEnv(env)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

func TestCheckinSecret_WithRealEnv_RoundTripWorks(t *testing.T) {
	if os.Getenv("BE_CHECKIN_SUBPROC") == "1" {
		checkinRoundTripOrDie(t)
		return
	}

	stdout, stderr, err := runCheckinSubprocess(t, "TestCheckinSecret_WithRealEnv_RoundTripWorks", map[string]string{
		"BE_CHECKIN_SUBPROC":   "1",
		"INTERNAL_SECRET":      "a-real-internal-secret-for-testing-0123456789",
		"OBSCURA_ENV":          "",
		"TURN_SECRET":          "dummy-turn-secret-for-isolation",
		"NODE_INTERNAL_SECRET": "dummy-node-secret-for-isolation",
	})
	if err != nil {
		t.Fatalf("subprocess with real INTERNAL_SECRET should succeed, got error: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, checkinRoundTripMarker) {
		t.Fatalf("expected round-trip marker %q in subprocess stdout, got stdout=%q stderr=%q", checkinRoundTripMarker, stdout, stderr)
	}
}

func TestCheckinSecret_ProdWithoutEnv_Fatal(t *testing.T) {
	if os.Getenv("BE_CHECKIN_SUBPROC") == "1" {
		checkinRoundTripOrDie(t)
		return
	}

	stdout, stderr, err := runCheckinSubprocess(t, "TestCheckinSecret_ProdWithoutEnv_Fatal", map[string]string{
		"BE_CHECKIN_SUBPROC":   "1",
		"INTERNAL_SECRET":      "",
		"OBSCURA_ENV":          "production",
		"TURN_SECRET":          "dummy-turn-secret-for-isolation",
		"NODE_INTERNAL_SECRET": "dummy-node-secret-for-isolation",
		// api paketi internal/sms'i de import ediyor — sms.go'nun kendi
		// prod-fatal init()'i (SMS_PROVIDER=log ise) checkinSecretValue'dan
		// ÖNCE tetiklenip yanlış nedenle FATAL olabilir. Bu testin SADECE
		// INTERNAL_SECRET'ı izole edebilmesi için o kapıyı burada kapatıyoruz.
		"SMS_PROVIDER": "custom",
	})
	if err == nil {
		t.Fatalf("expected subprocess to exit non-zero (log.Fatal on missing INTERNAL_SECRET in production), but it exited 0. stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stderr, "INTERNAL_SECRET env required in production") {
		t.Fatalf("expected the exact fatal message, got stderr=%q", stderr)
	}
	if strings.Contains(stdout, checkinRoundTripMarker) {
		t.Fatalf("round-trip marker must NEVER appear — process must die at package-init before reaching any handler code. stdout=%q", stdout)
	}
}

func TestCheckinSecret_DevWithoutEnv_PlaceholderWorks(t *testing.T) {
	if os.Getenv("BE_CHECKIN_SUBPROC") == "1" {
		checkinRoundTripOrDie(t)
		return
	}

	stdout, stderr, err := runCheckinSubprocess(t, "TestCheckinSecret_DevWithoutEnv_PlaceholderWorks", map[string]string{
		"BE_CHECKIN_SUBPROC":   "1",
		"INTERNAL_SECRET":      "",
		"OBSCURA_ENV":          "",
		"TURN_SECRET":          "dummy-turn-secret-for-isolation",
		"NODE_INTERNAL_SECRET": "dummy-node-secret-for-isolation",
	})
	if err != nil {
		t.Fatalf("dev-mode subprocess without INTERNAL_SECRET must still succeed (placeholder), got error: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stderr, "INTERNAL_SECRET not set — using dev placeholder") {
		t.Fatalf("expected dev-placeholder WARN log, got stderr=%q", stderr)
	}
	if !strings.Contains(stdout, checkinRoundTripMarker) {
		t.Fatalf("expected round-trip to still work over the dev placeholder, got stdout=%q stderr=%q", stdout, stderr)
	}
}
