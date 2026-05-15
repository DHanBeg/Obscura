package blockchain

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAztecBridge_HealthcheckReturnsErrWhenNoNode(t *testing.T) {
	// Use a port nothing is listening on; expect connection failure.
	b, err := NewAztecBridge("http://127.0.0.1:1") // port 1 = reserved, nothing binds
	if err != nil {
		t.Fatalf("NewAztecBridge: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := b.Healthcheck(ctx); err == nil {
		t.Fatal("expected error when no node is reachable, got nil")
	}
}

func TestAztecBridge_SubmitProofReturnsNotImplemented(t *testing.T) {
	b, err := NewAztecBridge("http://localhost:8080")
	if err != nil {
		t.Fatalf("NewAztecBridge: %v", err)
	}
	_, err = b.SubmitProof(context.Background(), []byte("fake-proof"), []byte("fake-inputs"))
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("expected ErrNotImplemented, got %v", err)
	}
}

func TestNewAztecBridge_RejectsBadURL(t *testing.T) {
	for _, bad := range []string{"", "ftp://x", "localhost:8080"} {
		if _, err := NewAztecBridge(bad); err == nil {
			t.Errorf("expected error for rpcURL=%q", bad)
		}
	}
}
