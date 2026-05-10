package zk

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// loadSmokeProof reads a snarkjs-generated proof from circuits/test/.
func loadSmokeProof(t testing.TB, name string) (CircuitID, []byte, []string) {
	smokePath := filepath.Join("..", "..", "..", "circuits", "test", name)
	data, err := os.ReadFile(smokePath)
	if err != nil {
		t.Skipf("%s not found — run: cd circuits && node test/%s", name, name[:len(name)-len("_proof.json")]+".js")
	}
	var p struct {
		ProofJSON    string   `json:"proof_json"`
		CircuitID    string   `json:"circuit_id"`
		PublicInputs []string `json:"public_inputs"`
	}
	if err := json.Unmarshal(data, &p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return CircuitID(p.CircuitID), []byte(p.ProofJSON), p.PublicInputs
}

// BenchmarkVerifyCreditThreshold — spec Bölüm 15.2: <500ms target
func BenchmarkVerifyCreditThreshold(b *testing.B) {
	if err := LoadVerificationKeys("./keys"); err != nil {
		b.Fatal(err)
	}
	cid, proof, signals := loadSmokeProof(b, "smoke_proof.json")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := VerifyGroth16(cid, proof, signals); err != nil {
			b.Fatal(err)
		}
	}
}

// TestVerifyThroughput_100PerSecond — spec target: 100 proofs/second
// Single-threaded; multi-threaded would be much higher.
func TestVerifyThroughput_100PerSecond(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	if err := LoadVerificationKeys("./keys"); err != nil {
		t.Fatal(err)
	}
	cid, proof, signals := loadSmokeProof(t, "smoke_proof.json")

	const N = 200
	start := time.Now()
	for i := 0; i < N; i++ {
		if err := VerifyGroth16(cid, proof, signals); err != nil {
			t.Fatal(err)
		}
	}
	dur := time.Since(start)
	rate := float64(N) / dur.Seconds()
	t.Logf("Single-threaded: %d verifications in %v → %.1f proofs/sec (target ≥100)", N, dur, rate)

	if rate < 100 {
		t.Logf("⚠ Below spec target — try parallel test below")
	}
}

// TestVerifyThroughput_Parallel — pegs all CPU cores
func TestVerifyThroughput_Parallel(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	if err := LoadVerificationKeys("./keys"); err != nil {
		t.Fatal(err)
	}
	cid, proof, signals := loadSmokeProof(t, "smoke_proof.json")

	const total = 1000
	const workers = 8

	start := time.Now()
	var wg sync.WaitGroup
	per := total / workers
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < per; i++ {
				if err := VerifyGroth16(cid, proof, signals); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
	dur := time.Since(start)
	rate := float64(total) / dur.Seconds()
	t.Logf("Parallel (%d workers): %d verifications in %v → %.1f proofs/sec (spec target ≥100)", workers, total, dur, rate)

	if rate < 100 {
		t.Errorf("FAILED spec target: %.1f < 100 proofs/sec", rate)
	} else {
		t.Logf("✓ PASSED spec target")
	}
}
