package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/storage"
)

var globalStore *storage.Store

// InitStorage — storage paketini başlat (main.go'dan çağrılır)
func InitStorage() error {
	if err := storage.Init(db.DB); err != nil {
		return err
	}
	nodes := strings.Split(os.Getenv("NODE_PEERS"), ",")
	var cleanNodes []string
	for _, n := range nodes {
		if n = strings.TrimSpace(n); n != "" {
			cleanNodes = append(cleanNodes, n)
		}
	}
	globalStore = storage.NewStore(db.DB, cleanNodes)

	// TTL pruner — her saat
	go func() {
		for {
			time.Sleep(time.Hour)
			n, _ := globalStore.PruneExpired()
			if n > 0 {
				_ = n // log edilebilir
			}
		}
	}()
	return nil
}

// HandleShardUpload — POST /v1/storage/shard
// Veriyi shard'lara böl ve sakla.
func HandleShardUpload(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	if globalStore == nil {
		respond(w, 503, nil, "Storage başlatılmadı")
		return
	}

	// Max 10MB
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		respond(w, 400, nil, "Veri okunamadı (max 10MB)")
		return
	}
	if len(data) == 0 {
		respond(w, 400, nil, "Boş veri")
		return
	}

	messageID := r.URL.Query().Get("message_id")
	if messageID == "" {
		messageID = "unknown"
	}

	manifest, err := globalStore.Shard(user.DID, messageID, data)
	if err != nil {
		respond(w, 500, nil, "Sharding hatası: "+err.Error())
		return
	}
	respond(w, 200, manifest, "")
}

// HandleShardRetrieve — GET /v1/storage/shard/{content_id}
// Shard'lardan veriyi yeniden oluştur.
func HandleShardRetrieve(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	if globalStore == nil {
		respond(w, 503, nil, "Storage başlatılmadı")
		return
	}

	contentID := mux.Vars(r)["content_id"]
	if contentID == "" {
		respond(w, 400, nil, "content_id gerekli")
		return
	}

	data, err := globalStore.Reconstruct(contentID)
	if err != nil {
		respond(w, 404, nil, "Veri kurtarılamadı: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.WriteHeader(200)
	_, _ = w.Write(data)
}

// HandleShardDelete — DELETE /v1/storage/shard/{content_id}
func HandleShardDelete(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	if globalStore == nil {
		respond(w, 503, nil, "Storage başlatılmadı")
		return
	}

	contentID := mux.Vars(r)["content_id"]
	if contentID == "" {
		respond(w, 400, nil, "content_id gerekli")
		return
	}
	globalStore.Delete(contentID)
	respond(w, 200, map[string]bool{"deleted": true}, "")
}

// HandleShardStats — GET /v1/storage/stats
func HandleShardStats(w http.ResponseWriter, r *http.Request) {
	if globalStore == nil {
		respond(w, 503, nil, "Storage başlatılmadı")
		return
	}
	respond(w, 200, globalStore.Stats(), "")
}

// HandleFetchLocalShard — GET /v1/storage/local-shard/{shard_id}
// Node'lar arası tek shard fetch (internal, X-Internal-Secret gerekli)
func HandleFetchLocalShard(w http.ResponseWriter, r *http.Request) {
	secret := r.Header.Get("X-Internal-Secret")
	expected := os.Getenv("INTERNAL_SECRET")
	if expected != "" && secret != expected {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	shardID := mux.Vars(r)["shard_id"]
	if shardID == "" {
		respond(w, 400, nil, "shard_id gerekli")
		return
	}

	var data []byte
	if err := db.DB.QueryRow(`SELECT data FROM local_shards WHERE shard_id = ?`, shardID).Scan(&data); err != nil {
		respond(w, 404, nil, "Shard bulunamadı")
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.WriteHeader(200)
	_, _ = w.Write(data)
}

// HandleLocalShard — POST /v1/storage/local-shard (node'lar arası shard transfer)
func HandleLocalShard(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ShardID   string `json:"shard_id"`
		ContentID string `json:"content_id"`
		ChunkIdx  int    `json:"chunk_idx"`
		ShardIdx  int    `json:"shard_idx"`
		Data      []byte `json:"data"` // base64 veya raw bytes
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond(w, 400, nil, "Geçersiz JSON")
		return
	}

	// Internal endpoint — INTERNAL_SECRET ile korunmalı
	secret := r.Header.Get("X-Internal-Secret")
	expected := os.Getenv("INTERNAL_SECRET")
	if expected != "" && secret != expected {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	exp, err := time.Parse(time.RFC3339, req.ExpiresAt)
	if err != nil {
		exp = time.Now().Add(30 * 24 * time.Hour)
	}

	_, err = db.DB.Exec(`
		INSERT OR REPLACE INTO local_shards
			(shard_id, content_id, chunk_idx, shard_idx, data, stored_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		req.ShardID, req.ContentID, req.ChunkIdx, req.ShardIdx, req.Data,
		time.Now().Format(time.RFC3339), exp.Format(time.RFC3339),
	)
	if err != nil {
		respond(w, 500, nil, "Shard kaydedilemedi")
		return
	}
	respond(w, 200, map[string]bool{"stored": true}, "")
}

