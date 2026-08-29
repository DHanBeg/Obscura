package api

// C10 #7 kanıtı — multi_verify.go/callPeerVerify artık /v1/zk/verify (JWT)
// yerine bu paketin HandleVerifyZKProofInternal'ına nodeMAC ile istek atıyor.
// Bu test tam uçtan-uca gerçek round-trip: httptest.Server üzerinde
// HandleVerifyZKProofInternal'ı ayağa kaldırır, zk.MultiVerify'ı (gerçek
// callPeerVerify, gerçek nodeMAC) o sunucuya karşı çalıştırır, gerçek bir
// snarkjs Groth16 proof'u (circuits/test/smoke_proof.json) kullanır.
//
// Eski davranış (kanıtlanan bug): aynı istek /v1/zk/verify'a (JWT korumalı,
// AuthMiddleware) gidiyordu, callPeerVerify Authorization header
// göndermediği için 401 alıyordu → approvals=0 < minVerifiers → HER ZAMAN
// "Peer node'lar kanıtı reddetti".

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"obscura.network/core/internal/zk"
)

func TestZKInternalVerify_RealPeerRoundTrip_MultiVerifySucceeds(t *testing.T) {
	if err := zk.LoadVerificationKeys(""); err != nil {
		t.Fatalf("LoadVerificationKeys: %v", err)
	}

	smokeProofPath := filepath.Join("..", "..", "..", "circuits", "test", "smoke_proof.json")
	data, err := os.ReadFile(smokeProofPath)
	if err != nil {
		t.Skipf("smoke_proof.json not found — run: cd circuits && node test/smoke.js  (got: %v)", err)
	}
	var payload struct {
		ProofJSON    string   `json:"proof_json"`
		CircuitID    string   `json:"circuit_id"`
		PublicInputs []string `json:"public_inputs"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal smoke proof: %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(HandleVerifyZKProofInternal))
	defer ts.Close()
	peerAddr := strings.TrimPrefix(ts.URL, "http://")

	t.Setenv("NODE_PEERS", peerAddr)

	pd := zk.ProofData{ProofJSON: payload.ProofJSON, PublicInputs: payload.PublicInputs}
	ok, results, err := zk.MultiVerify(context.Background(), pd, zk.CircuitID(payload.CircuitID), 1)
	if err != nil {
		t.Fatalf("MultiVerify error: %v (results=%+v)", err, results)
	}
	if !ok {
		t.Fatalf("expected MultiVerify to succeed (real proof + real nodeMAC), got ok=false results=%+v", results)
	}
	if len(results) != 1 || !results[0].Valid || results[0].Error != nil {
		t.Fatalf("unexpected peer result: %+v", results)
	}
}

func TestZKInternalVerify_WrongSig_Rejected(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(HandleVerifyZKProofInternal))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL, strings.NewReader(`{"proof_json":"{}","circuit_id":"credit_threshold"}`))
	req.Header.Set("X-Node-Ts", "1")
	req.Header.Set("X-Node-Sig", "not-a-real-signature")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong nodeMAC sig, got %d", resp.StatusCode)
	}
}

func TestZKInternalVerify_MissingHeaders_Rejected(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(HandleVerifyZKProofInternal))
	defer ts.Close()

	resp, err := http.Post(ts.URL, "application/json", strings.NewReader(`{"proof_json":"{}","circuit_id":"credit_threshold"}`))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing nodeMAC headers, got %d", resp.StatusCode)
	}
}
