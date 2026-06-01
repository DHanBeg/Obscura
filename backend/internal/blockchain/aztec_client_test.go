package blockchain

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAztecClient_TransferStubFallback(t *testing.T) {
	// Sandbox unreachable + stub mode → synthetic tx hash, no error.
	c := &AztecClient{
		SandboxURL:   "http://127.0.0.1:1", // nothing binds port 1
		ContractAddr: "0xabc",
		StubMode:     true,
		http:         &http.Client{},
	}
	hash, err := c.Transfer("did:obs:a", "did:obs:b", 100)
	if err != nil {
		t.Fatalf("expected stub fallback, got error: %v", err)
	}
	if !strings.HasPrefix(hash, "0x") || len(hash) != 66 {
		t.Fatalf("expected 0x + 64 hex chars, got %q", hash)
	}
}

func TestAztecClient_TransferStrictFails(t *testing.T) {
	c := &AztecClient{
		SandboxURL:   "http://127.0.0.1:1",
		ContractAddr: "0xabc",
		StubMode:     false,
		http:         &http.Client{},
	}
	if _, err := c.Transfer("did:obs:a", "did:obs:b", 100); err == nil {
		t.Fatal("expected error in strict mode when sandbox unreachable")
	}
}

func TestAztecClient_TransferValidation(t *testing.T) {
	c := &AztecClient{SandboxURL: "http://x", ContractAddr: "0xabc", http: &http.Client{}}
	if _, err := c.Transfer("", "b", 1); err == nil {
		t.Error("expected error on empty from")
	}
	if _, err := c.Transfer("a", "b", 0); err == nil {
		t.Error("expected error on zero amount")
	}
	missing := &AztecClient{SandboxURL: "http://x", http: &http.Client{}}
	if _, err := missing.Transfer("a", "b", 1); err == nil {
		t.Error("expected error when contract address missing")
	}
}

func TestAztecClient_TransferAgainstFakePXE(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Echo a valid JSON-RPC result for both simulate and send.
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","result":"0xdeadbeef","id":1}`))
	}))
	defer srv.Close()

	c := &AztecClient{
		SandboxURL:   srv.URL,
		ContractAddr: "0xabc",
		StubMode:     false,
		http:         srv.Client(),
	}
	hash, err := c.Transfer("did:obs:a", "did:obs:b", 100)
	if err != nil {
		t.Fatalf("Transfer: %v", err)
	}
	if hash != "0xdeadbeef" {
		t.Fatalf("expected 0xdeadbeef, got %q", hash)
	}
}

func TestAztecClient_RPCErrorPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","error":{"code":-32000,"message":"insufficient balance"},"id":1}`))
	}))
	defer srv.Close()

	c := &AztecClient{
		SandboxURL:   srv.URL,
		ContractAddr: "0xabc",
		StubMode:     true, // RPC error is not "unreachable", so no stub fallback
		http:         srv.Client(),
	}
	if _, err := c.Transfer("did:obs:a", "did:obs:b", 100); err == nil {
		t.Fatal("expected rpc error to propagate")
	}
}
