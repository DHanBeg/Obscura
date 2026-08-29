package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/dbi"
	"obscura.network/core/internal/storage"
)

// verifyNodeHMAC — internal/gossip'in verifyRelayHMAC'inin storage için
// birebir kopyası (N11-storage): HMAC-SHA256(secret, ts+body), ±30s replay
// penceresi, sabit-zamanlı karşılaştırma. Ortak pakete çıkarılmadı — bkz.
// internal/storage/sharding.go'daki nodeMAC yorumu (gossip'in kendi test
// dosyası yok, ayrı env/trust-domain).
func verifyNodeHMAC(tsStr, sigHex string, body []byte) bool {
	if tsStr == "" || sigHex == "" {
		return false
	}
	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return false
	}
	diff := time.Now().UnixMilli() - ts
	if diff < -30_000 || diff > 30_000 {
		return false
	}
	mac := hmac.New(sha256.New, []byte(internalSecretValue))
	mac.Write([]byte(tsStr))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(sigHex), []byte(expected))
}

// initNodeShards — node_shards tablosunu oluşturur (idempotent).
// HandleStoreShard ve HandleFetchShardInternal tarafından lazy olarak çağrılır;
// ancak en güvenli kullanım InitStorage() içinde çağırmaktır.
func initNodeShards() error {
	schema := `
		CREATE TABLE IF NOT EXISTS node_shards (
			shard_id    TEXT PRIMARY KEY,
			message_id  TEXT,
			chunk_index INTEGER,
			total_chunks INTEGER,
			is_parity   INTEGER DEFAULT 0,
			data        BLOB NOT NULL,
			stored_at   INTEGER NOT NULL,
			expires_at  INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_node_shards_expires ON node_shards(expires_at);
	`
	if dbi.DriverFromEnv() == dbi.DriverPostgres {
		// BLOB Postgres'te yok — data []byte olarak bind/scan ediliyor
		// (HandleStoreShard/HandleFetchShardInternal), BYTEA doğrudan karşılığı.
		schema = `
			CREATE TABLE IF NOT EXISTS node_shards (
				shard_id    TEXT PRIMARY KEY,
				message_id  TEXT,
				chunk_index INTEGER,
				total_chunks INTEGER,
				is_parity   INTEGER DEFAULT 0,
				data        BYTEA NOT NULL,
				stored_at   INTEGER NOT NULL,
				expires_at  INTEGER NOT NULL
			);
			CREATE INDEX IF NOT EXISTS idx_node_shards_expires ON node_shards(expires_at);
		`
	}
	_, err := db.DB.Exec(schema)
	return err
}

var globalStore *storage.Store

// InitStorage — storage paketini başlat (main.go'dan çağrılır)
func InitStorage() error {
	if err := storage.Init(db.DB); err != nil {
		return err
	}
	// node_shards tablosu — diğer node'lardan alınan shard'lar burada saklanır.
	// TODO-WIRE: /v1/internal/store-shard ve /v1/internal/fetch-shard route'larını main.go'ya ekle.
	if err := initNodeShards(); err != nil {
		return fmt.Errorf("node_shards init: %w", err)
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
	if !verifyNodeHMAC(r.Header.Get("X-Node-Ts"), r.Header.Get("X-Node-Sig"), nil) {
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
	body, err := io.ReadAll(r.Body)
	if err != nil {
		respond(w, 400, nil, "Body okunamadı")
		return
	}

	// Internal endpoint — nodeMAC (HMAC, N11) ile korunmalı; JSON parse'dan
	// önce doğrula (auth önce).
	if !verifyNodeHMAC(r.Header.Get("X-Node-Ts"), r.Header.Get("X-Node-Sig"), body) {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	var req struct {
		ShardID   string `json:"shard_id"`
		ContentID string `json:"content_id"`
		ChunkIdx  int    `json:"chunk_idx"`
		ShardIdx  int    `json:"shard_idx"`
		Data      []byte `json:"data"` // base64 veya raw bytes
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		respond(w, 400, nil, "Geçersiz JSON")
		return
	}

	exp, err := time.Parse(time.RFC3339, req.ExpiresAt)
	if err != nil {
		exp = time.Now().Add(30 * 24 * time.Hour)
	}

	_, err = db.DB.Exec(`
		INSERT INTO local_shards
			(shard_id, content_id, chunk_idx, shard_idx, data, stored_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(shard_id) DO UPDATE SET
			content_id = excluded.content_id,
			chunk_idx  = excluded.chunk_idx,
			shard_idx  = excluded.shard_idx,
			data       = excluded.data,
			stored_at  = excluded.stored_at,
			expires_at = excluded.expires_at`,
		req.ShardID, req.ContentID, req.ChunkIdx, req.ShardIdx, req.Data,
		time.Now().Format(time.RFC3339), exp.Format(time.RFC3339),
	)
	if err != nil {
		respond(w, 500, nil, "Shard kaydedilemedi")
		return
	}
	respond(w, 200, map[string]bool{"stored": true}, "")
}

// ─── Internal Shard Endpoints (P2P DHT routing) ───────────────────────────────
//
// Bu endpoint'ler diğer Obscura node'larından gelen shard push/fetch isteklerini
// karşılar. node_shards tablosunu kullanır (local_shards'tan ayrı).
// Auth: X-Internal-Secret (INTERNAL_SECRET env).
//
// TODO-WIRE: main.go'ya şu route'ları ekle:
//   r.HandleFunc("/v1/internal/store-shard", api.HandleStoreShard).Methods("POST")
//   r.HandleFunc("/v1/internal/fetch-shard", api.HandleFetchShardInternal).Methods("GET")

// HandleStoreShard — POST /v1/internal/store-shard
// Başka bir node'dan gelen shard'ı node_shards tablosuna kaydeder.
// Body: {"shard_id": "...", "data": "<base64>", "message_id": "...", "chunk_index": N, "total_chunks": N, "is_parity": false}
func HandleStoreShard(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		respond(w, 400, nil, "Body okunamadı")
		return
	}

	// nodeMAC doğrula (HMAC, N11) — JSON parse'dan önce
	if !verifyNodeHMAC(r.Header.Get("X-Node-Ts"), r.Header.Get("X-Node-Sig"), body) {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	var req struct {
		ShardID     string `json:"shard_id"`
		Data        string `json:"data"`         // base64-encoded shard bytes
		MessageID   string `json:"message_id"`
		ChunkIndex  int    `json:"chunk_index"`
		TotalChunks int    `json:"total_chunks"`
		IsParity    bool   `json:"is_parity"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		respond(w, 400, nil, "Geçersiz JSON")
		return
	}
	if req.ShardID == "" || req.Data == "" {
		respond(w, 400, nil, "shard_id ve data zorunlu")
		return
	}

	// base64 → raw bytes
	raw, err := base64.StdEncoding.DecodeString(req.Data)
	if err != nil {
		// URL-safe base64 fallback
		raw, err = base64.URLEncoding.DecodeString(req.Data)
		if err != nil {
			respond(w, 400, nil, "data base64 decode hatası")
			return
		}
	}

	now := time.Now().Unix()
	expiresAt := now + int64(30*24*3600) // 30 gün TTL

	isParity := 0
	if req.IsParity {
		isParity = 1
	}

	_, err = db.DB.Exec(`
		INSERT INTO node_shards
			(shard_id, message_id, chunk_index, total_chunks, is_parity, data, stored_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(shard_id) DO UPDATE SET
			message_id   = excluded.message_id,
			chunk_index  = excluded.chunk_index,
			total_chunks = excluded.total_chunks,
			is_parity    = excluded.is_parity,
			data         = excluded.data,
			stored_at    = excluded.stored_at,
			expires_at   = excluded.expires_at`,
		req.ShardID, req.MessageID, req.ChunkIndex, req.TotalChunks, isParity, raw, now, expiresAt,
	)
	if err != nil {
		respond(w, 500, nil, "Shard kaydedilemedi: "+err.Error())
		return
	}
	respond(w, 200, map[string]interface{}{"stored": true, "shard_id": req.ShardID}, "")
}

// HandleFetchShardInternal — GET /v1/internal/fetch-shard?id={shardID}
// node_shards tablosundan tek bir shard okur ve base64 encode ederek döndürür.
func HandleFetchShardInternal(w http.ResponseWriter, r *http.Request) {
	if !verifyNodeHMAC(r.Header.Get("X-Node-Ts"), r.Header.Get("X-Node-Sig"), nil) {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	shardID := r.URL.Query().Get("id")
	if shardID == "" {
		respond(w, 400, nil, "id parametresi zorunlu")
		return
	}

	var data []byte
	err := db.DB.QueryRow(`SELECT data FROM node_shards WHERE shard_id = ?`, shardID).Scan(&data)
	if err != nil {
		// local_shards tablosunda da ara (bu node'un kendi shard'ları)
		err2 := db.DB.QueryRow(`SELECT data FROM local_shards WHERE shard_id = ?`, shardID).Scan(&data)
		if err2 != nil {
			respond(w, 404, nil, "Shard bulunamadı")
			return
		}
	}

	// base64 encode edip JSON içinde döndür (binary-safe transport)
	encoded := base64.StdEncoding.EncodeToString(data)
	respond(w, 200, map[string]interface{}{
		"shard_id": shardID,
		"data":     encoded,
		"size":     len(data),
	}, "")

	// Aynı zamanda ham binary olarak da sunulabilmesi için Content-Type header'ını
	// uygulayan alternatif client'lar için: raw data isteyen client'lar
	// Accept: application/octet-stream header'ı gönderebilir.
	// Bu implementasyon JSON döner (mevcut respond() helper uyumluluğu için).
	_ = io.Discard // kullanılmıyor — raw binary path şimdilik yok
}

