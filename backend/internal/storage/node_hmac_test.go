package storage

// N11-storage kanıtı, gönderici tarafı — sharding.go'nun 4 node-to-node
// call-site'ı (distributeShards, fetchRemoteShard, PushShardToNode,
// FetchShardFromNode) eskiden ham INTERNAL_SECRET'ı X-Internal-Secret
// header'ında gönderiyordu. Artık nodeMAC (internal/gossip'in birebir
// kopyası): X-Node-Ts/X-Node-Sig, HMAC-SHA256(secret, ts+body).
//
// Kanıt 1 (wire'da ham secret yok): httptest.Server ile gerçek isteği
// yakalayıp header'ları incele — X-Internal-Secret YOK, gönderilen imza
// internalSecret'a eşit DEĞİL.

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPushShardToNode_WireHasNoRawSecret(t *testing.T) {
	var captured http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Header.Clone()
		_, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := PushShardToNode(srv.URL, "shard-1", []byte("payload")); err != nil {
		t.Fatalf("PushShardToNode failed: %v", err)
	}

	if got := captured.Get("X-Internal-Secret"); got != "" {
		t.Fatalf("raw secret still on the wire via X-Internal-Secret: %q", got)
	}
	ts := captured.Get("X-Node-Ts")
	sig := captured.Get("X-Node-Sig")
	if ts == "" || sig == "" {
		t.Fatalf("expected X-Node-Ts/X-Node-Sig headers, got ts=%q sig=%q", ts, sig)
	}
	if sig == internalSecret {
		t.Fatalf("X-Node-Sig equals the raw secret — not actually HMAC'd")
	}
}

func TestFetchShardFromNode_WireHasNoRawSecret(t *testing.T) {
	var captured http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data"))
	}))
	defer srv.Close()

	if _, err := FetchShardFromNode(srv.URL, "shard-1"); err != nil {
		t.Fatalf("FetchShardFromNode failed: %v", err)
	}

	if got := captured.Get("X-Internal-Secret"); got != "" {
		t.Fatalf("raw secret still on the wire via X-Internal-Secret: %q", got)
	}
	if captured.Get("X-Node-Ts") == "" || captured.Get("X-Node-Sig") == "" {
		t.Fatalf("expected X-Node-Ts/X-Node-Sig headers to be set")
	}
}

func TestNodeMAC_DifferentBodiesDifferentSigs(t *testing.T) {
	_, sig1 := nodeMAC([]byte("body-a"))
	_, sig2 := nodeMAC([]byte("body-b"))
	if sig1 == sig2 {
		t.Fatalf("different bodies produced the same signature")
	}
}
