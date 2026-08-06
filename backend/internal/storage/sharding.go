// Package storage — Spec Bölüm 3.2: Distributed shard storage
//
// Mesajlar 256KB chunk'lara bölünür, Reed-Solomon 4-of-6 erasure coding ile
// korunur ve farklı node'lara dağıtılır. Herhangi 4 shard ile tam veri
// kurtarılabilir.
//
// Spec: 30 gün TTL, silinmiş mesajlar tüm shard'lardan kaldırılır.
package storage

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/klauspost/reedsolomon"
	"obscura.network/core/internal/dbi"
)

const (
	DataShards   = 4
	ParityShards = 2
	TotalShards  = DataShards + ParityShards
	ShardSize    = 256 * 1024 // 256KB
	TTL          = 30 * 24 * time.Hour
)

// ShardManifest — bir mesajın shard haritası (DB'de JSON olarak saklanır)
type ShardManifest struct {
	ContentID  string    `json:"content_id"`  // SHA-256 of original data (hex)
	TotalSize  int       `json:"total_size"`
	ShardCount int       `json:"shard_count"` // her chunk için TotalShards shard
	Shards     []ShardMeta `json:"shards"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// ShardMeta — tek bir shard'ın metadata'sı
type ShardMeta struct {
	Index    int    `json:"index"`    // 0..TotalShards-1 (0-3 data, 4-5 parity)
	Chunk    int    `json:"chunk"`    // hangi 256KB chunk'tan
	NodeID   string `json:"node_id"` // hangi node'da saklanıyor
	ShardID  string `json:"shard_id"` // SHA-256(chunk+index) hex
	Size     int    `json:"size"`
}

// Store — shard storage işlemlerinin odak noktası
type Store struct {
	db             dbi.Querier
	nodes          []string // bilinen node adresleri "host:port" (round-robin dağıtım için)
	internalSecret string   // INTERNAL_SECRET — node'lar arası auth
}

func NewStore(db dbi.Querier, nodes []string) *Store {
	return &Store{
		db:             db,
		nodes:          nodes,
		internalSecret: os.Getenv("INTERNAL_SECRET"),
	}
}

// nodeURL — "host:port" formatındaki node adresini HTTP URL'ye çevirir
func (s *Store) nodeURL(nodeID string) string {
	if strings.HasPrefix(nodeID, "http") {
		return nodeID
	}
	return "http://" + nodeID
}

// Init — storage tablosunu oluştur
func Init(db dbi.Querier) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS shard_manifests (
			content_id  TEXT PRIMARY KEY,
			manifest    TEXT NOT NULL,
			owner_did   TEXT NOT NULL,
			message_id  TEXT NOT NULL,
			created_at  DATETIME NOT NULL,
			expires_at  DATETIME NOT NULL
		);
		CREATE TABLE IF NOT EXISTS local_shards (
			shard_id   TEXT PRIMARY KEY,
			content_id TEXT NOT NULL,
			chunk_idx  INTEGER NOT NULL,
			shard_idx  INTEGER NOT NULL,
			data       BLOB NOT NULL,
			stored_at  DATETIME NOT NULL,
			expires_at DATETIME NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_local_shards_content ON local_shards(content_id);
		CREATE INDEX IF NOT EXISTS idx_shards_expires ON local_shards(expires_at);
	`)
	return err
}

// Shard — veriyi shard'lara böl, DB'e kaydet, manifest döndür
func (s *Store) Shard(ownerDID, messageID string, data []byte) (*ShardManifest, error) {
	contentHash := sha256.Sum256(data)
	contentID := hex.EncodeToString(contentHash[:])

	now := time.Now()
	manifest := &ShardManifest{
		ContentID:  contentID,
		TotalSize:  len(data),
		CreatedAt:  now,
		ExpiresAt:  now.Add(TTL),
	}

	// Veriyi 256KB chunk'lara böl
	chunks := splitChunks(data, ShardSize)
	manifest.ShardCount = len(chunks)

	enc, err := reedsolomon.New(DataShards, ParityShards)
	if err != nil {
		return nil, fmt.Errorf("reedsolomon.New: %w", err)
	}

	nodeIdx := 0
	for chunkIdx, chunk := range chunks {
		// Chunk'ı DataShards eşit parçaya böl (padding ile)
		shardLen := (len(chunk) + DataShards - 1) / DataShards
		shards := make([][]byte, TotalShards)
		for i := 0; i < DataShards; i++ {
			start := i * shardLen
			end := start + shardLen
			if end > len(chunk) {
				end = len(chunk)
			}
			shard := make([]byte, shardLen)
			if start < len(chunk) {
				copy(shard, chunk[start:end])
			}
			shards[i] = shard
		}
		for i := DataShards; i < TotalShards; i++ {
			shards[i] = make([]byte, shardLen)
		}

		// Parity shard'larını hesapla
		if err := enc.Encode(shards); err != nil {
			return nil, fmt.Errorf("encode chunk %d: %w", chunkIdx, err)
		}

		// Her shard'ı local DB'e yaz + manifest'e ekle
		for shardIdx, shardData := range shards {
			shardHash := sha256.Sum256(append([]byte{byte(chunkIdx), byte(shardIdx)}, shardData...))
			shardID := hex.EncodeToString(shardHash[:])

			nodeID := "local"
			if len(s.nodes) > 0 {
				nodeID = s.nodes[nodeIdx%len(s.nodes)]
				nodeIdx++
			}

			manifest.Shards = append(manifest.Shards, ShardMeta{
				Index:   shardIdx,
				Chunk:   chunkIdx,
				NodeID:  nodeID,
				ShardID: shardID,
				Size:    len(shardData),
			})

			_, err := s.db.Exec(`
				INSERT OR REPLACE INTO local_shards
					(shard_id, content_id, chunk_idx, shard_idx, data, stored_at, expires_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)`,
				shardID, contentID, chunkIdx, shardIdx, shardData,
				now.Format(time.RFC3339), manifest.ExpiresAt.Format(time.RFC3339),
			)
			if err != nil {
				return nil, fmt.Errorf("store shard %d/%d: %w", chunkIdx, shardIdx, err)
			}
		}
	}

	// Manifest'i kaydet
	manifestJSON, _ := json.Marshal(manifest)
	_, err = s.db.Exec(`
		INSERT OR REPLACE INTO shard_manifests
			(content_id, manifest, owner_did, message_id, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		contentID, string(manifestJSON), ownerDID, messageID,
		now.Format(time.RFC3339), manifest.ExpiresAt.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("store manifest: %w", err)
	}

	// Remote shard dağıtımı — uzak node'lara atanan shard'ları gönder (async)
	if len(s.nodes) > 0 {
		go s.distributeShards(manifest)
	}

	return manifest, nil
}

// Reconstruct — contentID ile veriyi yeniden oluştur (herhangi 4 shard yeterli)
func (s *Store) Reconstruct(contentID string) ([]byte, error) {
	// Manifest'i yükle
	var manifestJSON string
	err := s.db.QueryRow(`SELECT manifest FROM shard_manifests WHERE content_id = ?`, contentID).Scan(&manifestJSON)
	if err != nil {
		return nil, fmt.Errorf("manifest not found: %w", err)
	}

	var manifest ShardManifest
	if err := json.Unmarshal([]byte(manifestJSON), &manifest); err != nil {
		return nil, fmt.Errorf("manifest parse: %w", err)
	}

	enc, err := reedsolomon.New(DataShards, ParityShards)
	if err != nil {
		return nil, fmt.Errorf("reedsolomon.New: %w", err)
	}

	// chunk sayısını bul
	numChunks := manifest.ShardCount
	var result []byte

	for chunkIdx := 0; chunkIdx < numChunks; chunkIdx++ {
		shards := make([][]byte, TotalShards)

		for _, meta := range manifest.Shards {
			if meta.Chunk != chunkIdx {
				continue
			}

			var shardData []byte
			err := s.db.QueryRow(`SELECT data FROM local_shards WHERE shard_id = ?`, meta.ShardID).Scan(&shardData)
			if err == nil {
				shards[meta.Index] = shardData
			} else if meta.NodeID != "local" {
				// Uzak node'dan shard çek
				if data, ferr := s.fetchRemoteShard(meta.NodeID, meta.ShardID); ferr == nil {
					shards[meta.Index] = data
					// Yerel cache'e yaz
					s.db.Exec(`INSERT OR IGNORE INTO local_shards
						(shard_id, content_id, chunk_idx, shard_idx, data, stored_at, expires_at)
						VALUES (?, ?, ?, ?, ?, ?, ?)`,
						meta.ShardID, contentID, meta.Chunk, meta.Index, data,
						time.Now().Format(time.RFC3339), manifest.ExpiresAt.Format(time.RFC3339))
				}
			}
			// nil = missing shard (Reed-Solomon reconstruct eder)
		}

		// Yeterli shard var mı kontrol et (en az DataShards adet)
		have := 0
		for _, sh := range shards {
			if sh != nil {
				have++
			}
		}
		if have < DataShards {
			return nil, fmt.Errorf("chunk %d: only %d/%d shards available", chunkIdx, have, DataShards)
		}

		if err := enc.Reconstruct(shards); err != nil {
			return nil, fmt.Errorf("reconstruct chunk %d: %w", chunkIdx, err)
		}

		// Veri shard'larını birleştir
		for i := 0; i < DataShards; i++ {
			result = append(result, shards[i]...)
		}
	}

	// Boyutu orijinal değere kırp (padding temizle)
	if len(result) > manifest.TotalSize {
		result = result[:manifest.TotalSize]
	}

	// Bütünlük doğrula
	hash := sha256.Sum256(result)
	if hex.EncodeToString(hash[:]) != contentID {
		return nil, fmt.Errorf("content integrity check failed — data corrupted")
	}

	return result, nil
}

// distributeShards — uzak node'lara atanan shard'ları HTTP POST ile gönderir.
// Fire-and-forget; hata durumunda yerel kopya hâlâ geçerli.
func (s *Store) distributeShards(manifest *ShardManifest) {
	type remoteReq struct {
		ShardID   string `json:"shard_id"`
		ContentID string `json:"content_id"`
		ChunkIdx  int    `json:"chunk_idx"`
		ShardIdx  int    `json:"shard_idx"`
		Data      []byte `json:"data"`
		ExpiresAt string `json:"expires_at"`
	}

	selfID := os.Getenv("NODE_ID")
	expStr := manifest.ExpiresAt.Format(time.RFC3339)

	for _, meta := range manifest.Shards {
		if meta.NodeID == "local" || meta.NodeID == selfID {
			continue // yerel shard — dağıtma
		}

		// Shard verisini DB'den oku
		var data []byte
		if err := s.db.QueryRow(`SELECT data FROM local_shards WHERE shard_id = ?`, meta.ShardID).Scan(&data); err != nil {
			continue
		}

		req := remoteReq{
			ShardID:   meta.ShardID,
			ContentID: manifest.ContentID,
			ChunkIdx:  meta.Chunk,
			ShardIdx:  meta.Index,
			Data:      data,
			ExpiresAt: expStr,
		}
		body, _ := json.Marshal(req)
		url := s.nodeURL(meta.NodeID) + "/v1/storage/local-shard"

		httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
		if err != nil {
			continue
		}
		httpReq.Header.Set("Content-Type", "application/json")
		if s.internalSecret != "" {
			httpReq.Header.Set("X-Internal-Secret", s.internalSecret)
		}

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(httpReq)
		if err != nil {
			log.Printf("Shard dağıtım hatası [%s → %s]: %v", meta.ShardID[:8], meta.NodeID, err)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == 200 {
			log.Printf("Shard dağıtıldı: %s → %s", meta.ShardID[:8], meta.NodeID)
		}
	}
}

// fetchRemoteShard — uzak node'dan tek bir shard çeker.
// GET /v1/storage/local-shard/{shard_id}
func (s *Store) fetchRemoteShard(nodeID, shardID string) ([]byte, error) {
	url := s.nodeURL(nodeID) + "/v1/storage/local-shard/" + shardID
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if s.internalSecret != "" {
		req.Header.Set("X-Internal-Secret", s.internalSecret)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("remote shard fetch: HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// Delete — bir içeriğin tüm shard'larını sil (mesaj silme)
func (s *Store) Delete(contentID string) error {
	s.db.Exec(`DELETE FROM local_shards WHERE content_id = ?`, contentID)
	s.db.Exec(`DELETE FROM shard_manifests WHERE content_id = ?`, contentID)
	return nil
}

// PruneExpired — süresi dolmuş shard'ları temizle (TTL 30 gün)
func (s *Store) PruneExpired() (int64, error) {
	now := time.Now().Format(time.RFC3339)
	r1, _ := s.db.Exec(`DELETE FROM local_shards WHERE expires_at < ?`, now)
	r2, _ := s.db.Exec(`DELETE FROM shard_manifests WHERE expires_at < ?`, now)
	n1, _ := r1.RowsAffected()
	n2, _ := r2.RowsAffected()
	return n1 + n2, nil
}

// Stats — storage istatistikleri
func (s *Store) Stats() map[string]interface{} {
	var shardCount, manifestCount int
	var totalBytes int64
	s.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(LENGTH(data)),0) FROM local_shards`).Scan(&shardCount, &totalBytes)
	s.db.QueryRow(`SELECT COUNT(*) FROM shard_manifests`).Scan(&manifestCount)
	return map[string]interface{}{
		"local_shards":   shardCount,
		"manifests":      manifestCount,
		"total_bytes":    totalBytes,
		"data_shards":    DataShards,
		"parity_shards":  ParityShards,
		"shard_size_kb":  ShardSize / 1024,
		"ttl_days":       30,
	}
}

// ─── Standalone P2P DHT routing helpers ──────────────────────────────────────
//
// Bu fonksiyonlar paket düzeyinde (Store'dan bağımsız) çalışır ve
// sharding.go içindeki mevcut Store metodlarından farklı olarak
// deterministik DHT-benzeri bir hash-mod atama stratejisi kullanır.
// Kısa vadede HTTP ile uyumludur; ileride libp2p stream protocol ile
// değiştirilebilir.

// DistributeShards — shard listesini nodeList'e deterministik olarak dağıtır.
// Atama kuralı: nodeIndex = sha256(shardID)[0:8] mod len(nodes)
// Aynı giriş her zaman aynı sonucu üretir (DHT tutarlılığı).
// Dönen map: nodeAddr → []ShardMeta
func DistributeShards(shards []ShardMeta, nodeList []string) map[string][]ShardMeta {
	result := make(map[string][]ShardMeta, len(nodeList))
	if len(nodeList) == 0 {
		return result
	}
	for _, s := range shards {
		// Deterministik node seçimi: shardID'nin SHA-256'sının ilk 8 byte'ı
		h := sha256.Sum256([]byte(s.ShardID))
		// 8 byte → uint64
		var idx uint64
		for i := 0; i < 8; i++ {
			idx = (idx << 8) | uint64(h[i])
		}
		node := nodeList[idx%uint64(len(nodeList))]
		result[node] = append(result[node], s)
	}
	return result
}

// PushShardToNode — tek bir shard'ı uzak node'a HTTP POST ile gönderir.
// Endpoint: POST http://{nodeAddr}/v1/internal/store-shard
// Body: {"shard_id": "...", "data": "<base64>"}
// Auth: X-Internal-Secret header (INTERNAL_SECRET env)
// Timeout: 10s, Retry: 2x
func PushShardToNode(nodeAddr, shardID string, data []byte) error {
	secret := os.Getenv("INTERNAL_SECRET")
	url := nodeHTTPBase(nodeAddr) + "/v1/internal/store-shard"

	type pushReq struct {
		ShardID string `json:"shard_id"`
		Data    []byte `json:"data"` // encoding/json base64 olarak encode eder
	}
	body, err := json.Marshal(pushReq{ShardID: shardID, Data: data})
	if err != nil {
		return fmt.Errorf("PushShardToNode marshal: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 500ms, 1s
			time.Sleep(time.Duration(attempt*500) * time.Millisecond)
		}
		req, err := http.NewRequest("POST", url, bytes.NewReader(body))
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		if secret != "" {
			req.Header.Set("X-Internal-Secret", secret)
		}
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("attempt %d: %w", attempt+1, err)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return nil
		}
		lastErr = fmt.Errorf("attempt %d: HTTP %d", attempt+1, resp.StatusCode)
	}
	return fmt.Errorf("PushShardToNode(%s, %s): %w", nodeAddr, shardID[:8], lastErr)
}

// FetchShardFromNode — uzak node'dan tek bir shard'ı çeker.
// Endpoint: GET http://{nodeAddr}/v1/internal/fetch-shard?id={shardID}
// Auth: X-Internal-Secret header
// Timeout: 10s
func FetchShardFromNode(nodeAddr, shardID string) ([]byte, error) {
	secret := os.Getenv("INTERNAL_SECRET")
	url := nodeHTTPBase(nodeAddr) + "/v1/internal/fetch-shard?id=" + shardID

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if secret != "" {
		req.Header.Set("X-Internal-Secret", secret)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("FetchShardFromNode(%s): %w", shardID[:8], err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("FetchShardFromNode: HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// nodeHTTPBase — "host:port" veya tam URL girişini http://host:port'a normalize eder.
func nodeHTTPBase(addr string) string {
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return strings.TrimRight(addr, "/")
	}
	return "http://" + strings.TrimRight(addr, "/")
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func splitChunks(data []byte, size int) [][]byte {
	var chunks [][]byte
	for len(data) > 0 {
		end := size
		if end > len(data) {
			end = len(data)
		}
		chunks = append(chunks, data[:end])
		data = data[end:]
	}
	return chunks
}

// randHex — kriptografik rastgele hex string
func randHex(n int) string {
	b := make([]byte, n)
	_, _ = io.ReadFull(rand.Reader, b)
	return hex.EncodeToString(b)
}

// ShardIDFromParts — deterministik shard ID üretimi
func ShardIDFromParts(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, ":")))
	return hex.EncodeToString(h[:])
}
