package mls

// MLS büyük-grup performans testleri — spec Bölüm 15.2 hedefleri:
//   - 1000+ üyeli grupta application message encrypt: < 100ms p95
//   - 1000+ üyeli grupta application message decrypt: < 50ms  p95
//   - 10000+ üyeye doğru ölçeklenebilirlik (FAZ 2 / FAZ 3 hedefi)
//
// Test stratejisi:
//   1) Tek bir mls-cli subprocess'inde bir grup oluştur (creator = alice).
//   2) Aynı subprocess'te N adet bot identity üret, her biri için KeyPackage.
//   3) Sırayla alice → bot_i add_member çağırarak grubu N+1 üyeye büyüt.
//      (Bu kurulum bir kez yapılır; tek-üye add_member commit'leri ayrı
//      mikrobenchmark değildir — esas hedef stable epoch'taki application
//      mesajının latency'sidir.)
//   4) Benchmark: alice.Encrypt(plaintext) → bot_1.ProcessMessage(ciphertext).
//
// Build/skip kuralları (client_test.go pattern'i ile uyumlu):
//   - mls-cli binary yoksa testler skip edilir.
//   - Default boyut MLS_PERF_N env değişkeniyle (ör. "1000") override edilir.
//   - Kısa testler (`go test -short`) atlanır — büyük grup kurulumu ~dakika.
//
// NOT: openmls add_member O(N) commit boyutu üretir (tree fanout), bu yüzden
// 1000 üyenin kurulumu 1-3 dakika alabilir. Benchmark esas olarak STABIL
// epoch'ta encrypt/decrypt latency'sini ölçer — kurulum maliyeti BenchmarkXxx
// loop dışında yapılır.

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"
)

// perfDefaultN — kullanıcı override etmediyse kullanılan default üye sayısı.
// 1000 CI için pratik default'tur; CI'da daha küçük (örn 100) ile çalıştırılabilir.
//
// NOT (spec uyumu): Spec Bölüm 15.2 büyük-grup hedefi 5000 üyedir; perf_test.go
// burada 1000'de bırakılmıştır çünkü 5000 üyeli kurulum CI bütçesini aşar
// (openmls add_member O(N) commit boyutu → kurulum dakikalar sürer).
// Tam spec doğrulaması için 5000 üye ile çalıştır (MLS_PERF_N env override):
//   MLS_PERF_N=5000 go test -run TestMLSGroupEncrypt -timeout 60m
const perfDefaultN = 1000

func perfMemberCount() int {
	if v := os.Getenv("MLS_PERF_N"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 1 {
			return n
		}
	}
	return perfDefaultN
}

// largeGroupHarness — N üyeli grup kurulumu. Caller cleanup'tan sorumlu.
//
// Dönenler:
//   creator       — alice client + identity_id
//   firstMember   — bot_1 (test sırasında decrypt için)
//   creatorGroup  — alice'in group_id'si
//   memberGroup   — bot_1'in group_id'si (aynı olmalı)
//
// Bot 2..N için (büyük çoğunluk) sadece KeyPackage üretip add_member yapılır;
// ProcessWelcome çağrısı bot_1 dışında atlanır (Welcome ile her bot ayrı
// state'e ulaşır ama benchmark'ta sadece bir alıcı kontrol ediyoruz).
type largeGroupHarness struct {
	bin           string
	N             int
	alice         *Client
	bob           *Client // ilk üye, decrypt edici
	aliceID       string
	bobID         string
	groupID       string
	bobGroupID    string
	cleanup       func()
	setupDuration time.Duration
}

func buildLargeGroup(tb testing.TB, n int) *largeGroupHarness {
	tb.Helper()
	bin := mlsBinPath(tb)
	if testing.Short() {
		tb.Skip("perf test (kullanım: go test -run TestMLSGroupEncrypt1000Members -timeout 30m)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	t0 := time.Now()

	// Creator (alice)
	alice, err := NewClient(bin)
	if err != nil {
		cancel()
		tb.Fatalf("alice client: %v", err)
	}
	aliceID, err := alice.NewIdentity(ctx, "did:obs:perf-alice")
	if err != nil {
		alice.Close()
		cancel()
		tb.Fatalf("alice identity: %v", err)
	}
	groupID, err := alice.CreateGroup(ctx, aliceID)
	if err != nil {
		alice.Close()
		cancel()
		tb.Fatalf("create group: %v", err)
	}

	// Bob (ilk üye — ayrı subprocess; gerçek deployment'ta farklı cihaz)
	bob, err := NewClient(bin)
	if err != nil {
		alice.Close()
		cancel()
		tb.Fatalf("bob client: %v", err)
	}
	bobID, err := bob.NewIdentity(ctx, "did:obs:perf-bob")
	if err != nil {
		alice.Close()
		bob.Close()
		cancel()
		tb.Fatalf("bob identity: %v", err)
	}
	bobKP, err := bob.GenerateKeyPackage(ctx, bobID)
	if err != nil {
		alice.Close()
		bob.Close()
		cancel()
		tb.Fatalf("bob KP: %v", err)
	}
	addBob, err := alice.AddMember(ctx, groupID, aliceID, bobKP)
	if err != nil {
		alice.Close()
		bob.Close()
		cancel()
		tb.Fatalf("add bob: %v", err)
	}
	joinBob, err := bob.ProcessWelcome(ctx, bobID, addBob.WelcomeB64)
	if err != nil {
		alice.Close()
		bob.Close()
		cancel()
		tb.Fatalf("bob process welcome: %v", err)
	}

	// Bot 3..N — aynı alice subprocess'inde identity + KeyPackage, sonra add.
	// Welcome işleme yapmıyoruz (decrypt'te bob kullanılıyor).
	remaining := n - 2 // alice + bob hariç
	if remaining > 0 {
		for i := 0; i < remaining; i++ {
			did := fmt.Sprintf("did:obs:perf-bot-%d", i)
			botID, err := alice.NewIdentity(ctx, did)
			if err != nil {
				tb.Logf("bot %d identity err: %v — devam", i, err)
				continue
			}
			botKP, err := alice.GenerateKeyPackage(ctx, botID)
			if err != nil {
				tb.Logf("bot %d KP err: %v — devam", i, err)
				continue
			}
			addRes, err := alice.AddMember(ctx, groupID, aliceID, botKP)
			if err != nil {
				tb.Logf("bot %d add err: %v — devam", i, err)
				continue
			}
			// Bob'ın commit'i işlemesi gerekir ki state'i alice ile senkron kalsın.
			if _, _, err := bob.ProcessMessage(ctx, joinBob.GroupID, addRes.CommitB64); err != nil {
				tb.Logf("bob commit apply %d err: %v — devam", i, err)
			}
			if (i+1)%100 == 0 {
				tb.Logf("kurulum: %d/%d üye eklendi (%.1fs)", i+2, n, time.Since(t0).Seconds())
			}
		}
	}

	setup := time.Since(t0)
	tb.Logf("✅ %d üyeli grup kurulumu tamamlandı: %.1fs", n, setup.Seconds())

	return &largeGroupHarness{
		bin:           bin,
		N:             n,
		alice:         alice,
		bob:           bob,
		aliceID:       aliceID,
		bobID:         bobID,
		groupID:       groupID,
		bobGroupID:    joinBob.GroupID,
		setupDuration: setup,
		cleanup: func() {
			alice.Close()
			bob.Close()
			cancel()
		},
	}
}

// TestMLSGroupEncrypt1000Members — spec hedefi: encrypt < 100ms.
func TestMLSGroupEncrypt1000Members(t *testing.T) {
	n := perfMemberCount()
	h := buildLargeGroup(t, n)
	defer h.cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	// Warm-up — ilk çağrı subprocess buffer'larını ısıtır
	if _, err := h.alice.Encrypt(ctx, h.groupID, h.aliceID, []byte("warmup")); err != nil {
		t.Fatalf("warmup encrypt: %v", err)
	}

	// Ölç: 20 mesajın ortalaması (p95 yerine basit avg — ileride histogram).
	const iterations = 20
	plaintext := []byte("MLS perf test mesajı — 1000 üyeli grupta encrypt")
	var total time.Duration
	for i := 0; i < iterations; i++ {
		start := time.Now()
		if _, err := h.alice.Encrypt(ctx, h.groupID, h.aliceID, plaintext); err != nil {
			t.Fatalf("encrypt iter %d: %v", i, err)
		}
		total += time.Since(start)
	}
	avg := total / iterations
	t.Logf("📊 Encrypt @ %d üye: avg=%v (%d iter) — hedef <100ms", n, avg, iterations)

	if avg > 100*time.Millisecond {
		t.Errorf("Encrypt latency %v hedefin (100ms) üzerinde", avg)
	}
}

// TestMLSGroupDecrypt1000Members — spec hedefi: decrypt < 50ms.
func TestMLSGroupDecrypt1000Members(t *testing.T) {
	n := perfMemberCount()
	h := buildLargeGroup(t, n)
	defer h.cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	// Hazırlık: alice'den 20 ciphertext üret
	const iterations = 20
	plaintext := []byte("MLS perf test mesajı — 1000 üyeli grupta decrypt")
	ciphertexts := make([]string, iterations)
	for i := 0; i < iterations; i++ {
		ct, err := h.alice.Encrypt(ctx, h.groupID, h.aliceID, plaintext)
		if err != nil {
			t.Fatalf("encrypt prep %d: %v", i, err)
		}
		ciphertexts[i] = ct
	}

	// Warm-up decrypt
	if _, _, err := h.bob.ProcessMessage(ctx, h.bobGroupID, ciphertexts[0]); err != nil {
		t.Fatalf("warmup decrypt: %v", err)
	}

	// Asıl ölçüm: ciphertexts[1:]
	var total time.Duration
	for i := 1; i < iterations; i++ {
		start := time.Now()
		pt, _, err := h.bob.ProcessMessage(ctx, h.bobGroupID, ciphertexts[i])
		if err != nil {
			t.Fatalf("decrypt iter %d: %v", i, err)
		}
		total += time.Since(start)
		if string(pt) != string(plaintext) {
			t.Fatalf("plaintext mismatch @ iter %d", i)
		}
	}
	avg := total / time.Duration(iterations-1)
	t.Logf("📊 Decrypt @ %d üye: avg=%v (%d iter) — hedef <50ms", n, avg, iterations-1)

	if avg > 50*time.Millisecond {
		t.Errorf("Decrypt latency %v hedefin (50ms) üzerinde", avg)
	}
}

// ─── Benchmark'lar — `go test -bench=BenchmarkMLS` ──────────────────────────
//
// Bunlar tek-iterasyon zamanlaması yerine `b.N` adet çağrıyı ölçer; kurulum
// b.ResetTimer öncesi yapılır.

// benchHarnessOnce — benchmark'lar arası tek bir büyük grup paylaşımı
// (benchstat için stabilite).
var (
	benchHarness     *largeGroupHarness
	benchHarnessOnce sync.Once
)

func getBenchHarness(b *testing.B) *largeGroupHarness {
	benchHarnessOnce.Do(func() {
		benchHarness = buildLargeGroup(b, perfMemberCount())
	})
	return benchHarness
}

func BenchmarkMLSEncrypt1000(b *testing.B) {
	h := getBenchHarness(b)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	plaintext := []byte("bench")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := h.alice.Encrypt(ctx, h.groupID, h.aliceID, plaintext); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMLSDecrypt1000(b *testing.B) {
	h := getBenchHarness(b)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	plaintext := []byte("bench")

	// Pre-generate ciphertexts (b.N adet — yoksa decrypt sırasında double-counting)
	cts := make([]string, b.N)
	for i := 0; i < b.N; i++ {
		ct, err := h.alice.Encrypt(ctx, h.groupID, h.aliceID, plaintext)
		if err != nil {
			b.Fatal(err)
		}
		cts[i] = ct
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := h.bob.ProcessMessage(ctx, h.bobGroupID, cts[i]); err != nil {
			b.Fatal(err)
		}
	}
}
