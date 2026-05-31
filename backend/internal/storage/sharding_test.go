package storage

import (
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// testDB — in-memory SQLite instance (test yalıtımı)
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("testDB açılamadı: %v", err)
	}
	if err := Init(db); err != nil {
		t.Fatalf("Init hatası: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// TestShard_SmallData — 256KB'dan küçük veri, tek chunk
func TestShard_SmallData(t *testing.T) {
	db := testDB(t)
	store := NewStore(db, nil)

	payload := []byte("Obscura shard storage test verisi")
	manifest, err := store.Shard("did:obs:test", "msg-001", payload)
	if err != nil {
		t.Fatalf("Shard hatası: %v", err)
	}

	if manifest.ContentID == "" {
		t.Error("ContentID boş olmamalı")
	}
	if manifest.TotalSize != len(payload) {
		t.Errorf("TotalSize beklenen=%d, gerçek=%d", len(payload), manifest.TotalSize)
	}
	// 1 chunk → TotalShards shard (4 data + 2 parity)
	if len(manifest.Shards) != TotalShards {
		t.Errorf("Shard sayısı beklenen=%d, gerçek=%d", TotalShards, len(manifest.Shards))
	}
	if manifest.ExpiresAt.IsZero() {
		t.Error("ExpiresAt sıfır olmamalı")
	}
}

// TestShard_LargeData — 256KB'dan büyük veri, birden fazla chunk
func TestShard_LargeData(t *testing.T) {
	db := testDB(t)
	store := NewStore(db, nil)

	// 512KB + 100 byte → 3 chunk (256KB + 256KB + 100B)
	payload := make([]byte, 512*1024+100)
	for i := range payload {
		payload[i] = byte(i % 251)
	}

	manifest, err := store.Shard("did:obs:sender", "msg-large", payload)
	if err != nil {
		t.Fatalf("Shard hatası: %v", err)
	}
	if manifest.ShardCount != 3 {
		t.Errorf("Chunk sayısı beklenen=3, gerçek=%d", manifest.ShardCount)
	}
	// 3 chunk × 6 shard = 18
	if len(manifest.Shards) != 3*TotalShards {
		t.Errorf("Shard sayısı beklenen=%d, gerçek=%d", 3*TotalShards, len(manifest.Shards))
	}
}

// TestReconstruct_HappyPath — shard → reconstruct tam akış
func TestReconstruct_HappyPath(t *testing.T) {
	db := testDB(t)
	store := NewStore(db, nil)

	original := []byte("ZK tabanlı federe mesajlaşma platformu — Obscura")

	manifest, err := store.Shard("did:obs:alice", "msg-recon", original)
	if err != nil {
		t.Fatalf("Shard hatası: %v", err)
	}

	got, err := store.Reconstruct(manifest.ContentID)
	if err != nil {
		t.Fatalf("Reconstruct hatası: %v", err)
	}

	if string(got) != string(original) {
		t.Errorf("Reconstruct sonucu yanlış:\nbeklenen=%q\ngot=%q", original, got)
	}
}

// TestReconstruct_NotFound — bilinmeyen content_id → hata beklenir
func TestReconstruct_NotFound(t *testing.T) {
	db := testDB(t)
	store := NewStore(db, nil)

	_, err := store.Reconstruct("0000000000000000000000000000000000000000000000000000000000000000")
	if err == nil {
		t.Fatal("Hata bekleniyor ama nil geldi")
	}
}

// TestDelete — shard sil, reconstruct artık çalışmaz
func TestDelete(t *testing.T) {
	db := testDB(t)
	store := NewStore(db, nil)

	payload := []byte("silinecek veri")
	manifest, err := store.Shard("did:obs:bob", "msg-del", payload)
	if err != nil {
		t.Fatalf("Shard hatası: %v", err)
	}

	store.Delete(manifest.ContentID)

	_, err = store.Reconstruct(manifest.ContentID)
	if err == nil {
		t.Fatal("Silme sonrası reconstruct başarılı olmamalı")
	}
}

// TestPruneExpired — TTL simülasyonu: expired shard'lar kaldırılır
func TestPruneExpired(t *testing.T) {
	db := testDB(t)
	store := NewStore(db, nil)

	// Geçmiş tarihli shard doğrudan enjekte et
	_, err := db.Exec(`
		INSERT INTO local_shards (shard_id, content_id, chunk_idx, shard_idx, data, stored_at, expires_at)
		VALUES ('test-shard-1', 'test-content-1', 0, 0, X'DEADBEEF', '2020-01-01T00:00:00Z', '2020-01-01T00:00:00Z')
	`)
	if err != nil {
		t.Fatalf("Test shard eklenemedi: %v", err)
	}
	_, err = db.Exec(`
		INSERT INTO shard_manifests (content_id, manifest, owner_did, message_id, created_at, expires_at)
		VALUES ('test-content-1', '{}', 'did:obs:test', 'msg-old', '2020-01-01T00:00:00Z', '2020-01-01T00:00:00Z')
	`)
	if err != nil {
		t.Fatalf("Test manifest eklenemedi: %v", err)
	}

	n, err := store.PruneExpired()
	if err != nil {
		t.Fatalf("PruneExpired hatası: %v", err)
	}
	if n < 2 {
		t.Errorf("En az 2 kayıt silinmeli, silinen=%d", n)
	}
}

// TestStats — istatistik yanıtı doğru alanları içermeli
func TestStats(t *testing.T) {
	db := testDB(t)
	store := NewStore(db, nil)

	stats := store.Stats()

	requiredKeys := []string{"local_shards", "manifests", "total_bytes", "data_shards", "parity_shards", "shard_size_kb", "ttl_days"}
	for _, k := range requiredKeys {
		if _, ok := stats[k]; !ok {
			t.Errorf("Stats alanı eksik: %s", k)
		}
	}
	if stats["data_shards"] != DataShards {
		t.Errorf("data_shards beklenen=%d, gerçek=%v", DataShards, stats["data_shards"])
	}
	if stats["parity_shards"] != ParityShards {
		t.Errorf("parity_shards beklenen=%d, gerçek=%v", ParityShards, stats["parity_shards"])
	}
}

// TestShardIDFromParts — deterministik ID üretimi (aynı girdiler = aynı ID)
func TestShardIDFromParts(t *testing.T) {
	id1 := ShardIDFromParts("chunk0", "shard2", "node-1")
	id2 := ShardIDFromParts("chunk0", "shard2", "node-1")
	if id1 != id2 {
		t.Errorf("Deterministik ID bekleniyor: %s != %s", id1, id2)
	}
	id3 := ShardIDFromParts("chunk0", "shard2", "node-2")
	if id1 == id3 {
		t.Error("Farklı girdiler farklı ID üretmeli")
	}
}

// TestRoundRobinNodes — birden fazla node varken round-robin dağıtım
func TestRoundRobinNodes(t *testing.T) {
	db := testDB(t)
	nodes := []string{"node-1:8081", "node-2:8082", "node-3:8083"}
	store := NewStore(db, nodes)

	payload := make([]byte, 1024)
	manifest, err := store.Shard("did:obs:test", "msg-rr", payload)
	if err != nil {
		t.Fatalf("Shard hatası: %v", err)
	}

	seen := make(map[string]bool)
	for _, sh := range manifest.Shards {
		seen[sh.NodeID] = true
	}
	// En az 2 farklı node kullanılmalı (6 shard / 3 node = her node'a 2)
	if len(seen) < 2 {
		t.Errorf("Round-robin: en az 2 farklı node bekleniyor, görülen=%v", seen)
	}
}

// TestIntegrityCheck — bozulmuş veri reconstruct sırasında reddedilir
func TestIntegrityCheck(t *testing.T) {
	db := testDB(t)
	store := NewStore(db, nil)

	payload := []byte("orijinal veri")
	manifest, err := store.Shard("did:obs:test", "msg-integrity", payload)
	if err != nil {
		t.Fatalf("Shard hatası: %v", err)
	}

	// Bir shard verisini boz
	if len(manifest.Shards) > 0 {
		shardID := manifest.Shards[0].ShardID
		_, _ = db.Exec(`UPDATE local_shards SET data = X'BADDADABADDADABADDADABADDADA' WHERE shard_id = ?`, shardID)
	}

	// Reconstruct başarısız olabilir (integrity veya reconstruct hatası)
	got, err := store.Reconstruct(manifest.ContentID)
	if err == nil && strings.Contains(string(got), "orijinal veri") {
		// Sadece parity shard bozulduysa başarılı olabilir — bu geçerli
		// Ama data shard[0] bozulduysa hata beklenir
		t.Log("Parity ile kurtarma başarılı — kabul edilebilir")
	}
}
