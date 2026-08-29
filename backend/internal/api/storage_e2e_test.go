package api

// N11-storage kanıt 6 — gerçek node-to-node push/fetch/store, uçtan uca,
// imzalı: internal/storage'ın gönderici fonksiyonları (PushShardToNode,
// FetchShardFromNode) internal/api'nin gerçek handler'larına (HandleStoreShard,
// HandleFetchShardInternal) karşı, gerçek HTTP üzerinden çalıştırılır — mock
// yok, sadece iki paket birbirine httptest.Server ile bağlanıyor.

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"obscura.network/core/internal/storage"
)

func TestStorageE2E_PushThenFetch_NodeToNode(t *testing.T) {
	if err := initNodeShards(); err != nil {
		t.Fatalf("initNodeShards: %v", err)
	}

	r := mux.NewRouter()
	r.HandleFunc("/v1/internal/store-shard", HandleStoreShard).Methods("POST")
	r.HandleFunc("/v1/internal/fetch-shard", HandleFetchShardInternal).Methods("GET")
	srv := httptest.NewServer(r)
	defer srv.Close()

	shardID := "e2e-shard-" + t.Name()
	payload := []byte("hello from node A, signed not raw")

	// Gönderici: gerçek storage.PushShardToNode, gerçek HMAC ile.
	if err := storage.PushShardToNode(srv.URL, shardID, payload); err != nil {
		t.Fatalf("PushShardToNode: %v", err)
	}

	// Alıcı: gerçek storage.FetchShardFromNode, aynı node-to-node kanaldan.
	got, err := storage.FetchShardFromNode(srv.URL, shardID)
	if err != nil {
		t.Fatalf("FetchShardFromNode: %v", err)
	}

	// FetchShardFromNode ham body döner; handler JSON içinde base64 data taşıyor
	// (HandleFetchShardInternal), o yüzden burada JSON'dan çöz.
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Data string `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(got, &resp); err != nil {
		t.Fatalf("response JSON parse: %v (raw=%s)", err, got)
	}
	decoded, err := base64.StdEncoding.DecodeString(resp.Data.Data)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}
	if string(decoded) != string(payload) {
		t.Fatalf("round-trip mismatch: pushed %q, fetched %q", payload, decoded)
	}
}

func TestStorageE2E_WrongSigNodeToNode_Rejected(t *testing.T) {
	if err := initNodeShards(); err != nil {
		t.Fatalf("initNodeShards: %v", err)
	}

	r := mux.NewRouter()
	r.HandleFunc("/v1/internal/store-shard", HandleStoreShard).Methods("POST")
	srv := httptest.NewServer(r)
	defer srv.Close()

	req, _ := http.NewRequest("POST", srv.URL+"/v1/internal/store-shard",
		strings.NewReader(`{"shard_id":"x","data":"AAAA"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Node-Ts", "1700000000000")
	req.Header.Set("X-Node-Sig", "deadbeef")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for forged signature over real HTTP, got %d", resp.StatusCode)
	}
}
