package consensus

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE consensus_blocks (
		height       INTEGER PRIMARY KEY,
		round        INTEGER NOT NULL,
		parent_hash  TEXT NOT NULL,
		tx_root      TEXT NOT NULL,
		proposer     TEXT NOT NULL,
		block_hash   TEXT NOT NULL,
		block_ts     INTEGER NOT NULL,
		committed_at TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE consensus_block_ops (
		op_id       TEXT PRIMARY KEY,
		height      INTEGER NOT NULL,
		recorded_at TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestLatestBlockHash_EmptyReturnsGenesis(t *testing.T) {
	db := newTestDB(t)
	height, hash, err := LatestBlockHash(db)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if height != 0 {
		t.Fatalf("boş tabloda height=0 beklenirdi, got=%d", height)
	}
	if hash != GenesisParentHash {
		t.Fatalf("boş tabloda genesis hash beklenirdi, got=%q", hash)
	}
}

func TestSaveBlock_ThenLatestBlockHashReturnsIt(t *testing.T) {
	db := newTestDB(t)
	b := Block{Height: 1, Round: 0, ParentHash: GenesisParentHash, TxRoot: "root1", Proposer: "node-1", Hash: "hash1", Timestamp: 100}
	if err := SaveBlock(db, b, "2026-08-02T00:00:00Z"); err != nil {
		t.Fatalf("SaveBlock: %v", err)
	}
	height, hash, err := LatestBlockHash(db)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if height != 1 || hash != "hash1" {
		t.Fatalf("got height=%d hash=%q, want height=1 hash=hash1", height, hash)
	}
}

func TestLatestBlockHash_ReturnsHighestHeight(t *testing.T) {
	db := newTestDB(t)
	blocks := []Block{
		{Height: 1, ParentHash: GenesisParentHash, TxRoot: "r1", Proposer: "node-1", Hash: "hash1"},
		{Height: 2, ParentHash: "hash1", TxRoot: "r2", Proposer: "node-1", Hash: "hash2"},
		{Height: 3, ParentHash: "hash2", TxRoot: "r3", Proposer: "node-1", Hash: "hash3"},
	}
	for _, b := range blocks {
		if err := SaveBlock(db, b, "2026-08-02T00:00:00Z"); err != nil {
			t.Fatalf("SaveBlock height=%d: %v", b.Height, err)
		}
	}
	height, hash, err := LatestBlockHash(db)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if height != 3 || hash != "hash3" {
		t.Fatalf("got height=%d hash=%q, want height=3 hash=hash3", height, hash)
	}
}

func TestSaveBlock_DuplicateHeightIsIdempotent(t *testing.T) {
	db := newTestDB(t)
	b := Block{Height: 1, ParentHash: GenesisParentHash, TxRoot: "root1", Proposer: "node-1", Hash: "hash1"}
	if err := SaveBlock(db, b, "2026-08-02T00:00:00Z"); err != nil {
		t.Fatalf("ilk SaveBlock: %v", err)
	}
	// Aynı height'i FARKLI içerikle tekrar kaydetmeye çalış (çift-commit
	// senaryosu) — INSERT OR IGNORE PK çakışmasında sessizce yok saymalı,
	// hata dönmemeli, VE ilk kayıt korunmalı (ikincisiyle üzerine yazılmamalı).
	b2 := Block{Height: 1, ParentHash: GenesisParentHash, TxRoot: "farkli-root", Proposer: "node-2", Hash: "farkli-hash"}
	if err := SaveBlock(db, b2, "2026-08-02T00:00:01Z"); err != nil {
		t.Fatalf("ikinci SaveBlock (aynı height) hata vermemeliydi: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM consensus_blocks WHERE height = 1`).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 1 {
		t.Fatalf("height=1 için tek satır olmalıydı, got=%d", count)
	}
	_, hash, _ := LatestBlockHash(db)
	if hash != "hash1" {
		t.Fatalf("ilk kayıt korunmalıydı (hash1), got=%q", hash)
	}
}

// ─── consensus_block_ops audit-log (ADIM 7) ────────────────────────────────

func TestSaveBlockOps_ThenOpsForBlockReturnsThem(t *testing.T) {
	db := newTestDB(t)
	ops := []string{"op-a", "op-b", "op-c"}
	if err := SaveBlockOps(db, 1, ops, "2026-08-02T00:00:00Z"); err != nil {
		t.Fatalf("SaveBlockOps: %v", err)
	}
	got, err := OpsForBlock(db, 1)
	if err != nil {
		t.Fatalf("OpsForBlock: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("3 op beklenirdi, got=%d (%v)", len(got), got)
	}
}

func TestHasOp_FalseBeforeSaveTrueAfter(t *testing.T) {
	db := newTestDB(t)
	exists, err := HasOp(db, "op-x")
	if err != nil {
		t.Fatalf("HasOp: %v", err)
	}
	if exists {
		t.Fatal("henüz kaydedilmemiş op için HasOp true döndü")
	}
	if err := SaveBlockOps(db, 5, []string{"op-x"}, "2026-08-02T00:00:00Z"); err != nil {
		t.Fatalf("SaveBlockOps: %v", err)
	}
	exists, err = HasOp(db, "op-x")
	if err != nil {
		t.Fatalf("HasOp: %v", err)
	}
	if !exists {
		t.Fatal("kaydedilmiş op için HasOp false döndü")
	}
}

// TestSaveBlockOps_ReplayAtDifferentHeightIsIgnored — REPLAY-GUARD: aynı
// op-ID'nin FARKLI bir height'te tekrar tasdik edilmeye çalışılması (ör.
// kötü niyetli/hatalı bir proposer'ın eski bir op'u yeniden bir bloğa
// dahil etmesi) veritabanı seviyesinde (op_id PRIMARY KEY) sessizce
// engellenmeli — orijinal height/kayıt korunmalı.
func TestSaveBlockOps_ReplayAtDifferentHeightIsIgnored(t *testing.T) {
	db := newTestDB(t)
	if err := SaveBlockOps(db, 1, []string{"op-replay"}, "2026-08-02T00:00:00Z"); err != nil {
		t.Fatalf("ilk SaveBlockOps: %v", err)
	}
	// Aynı op'u FARKLI bir height'te (10) tekrar tasdik etmeye çalış.
	if err := SaveBlockOps(db, 10, []string{"op-replay"}, "2026-08-02T00:00:01Z"); err != nil {
		t.Fatalf("replay denemesi hata döndürmemeliydi (sessizce yok sayılmalı): %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM consensus_block_ops WHERE op_id = 'op-replay'`).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 1 {
		t.Fatalf("op-replay için TEK satır olmalıydı (replay engellendi), got=%d", count)
	}

	opsAtHeight1, _ := OpsForBlock(db, 1)
	opsAtHeight10, _ := OpsForBlock(db, 10)
	if len(opsAtHeight1) != 1 {
		t.Fatalf("orijinal height=1 kaydı korunmalıydı, got=%v", opsAtHeight1)
	}
	if len(opsAtHeight10) != 0 {
		t.Fatalf("height=10'da replay edilen op görünmemeliydi, got=%v", opsAtHeight10)
	}
}

func TestSaveBlockOps_DuplicateWithinSameBlockIsIdempotent(t *testing.T) {
	db := newTestDB(t)
	// Aynı op'un aynı çağrıda/tekrar denemede iki kez geçmesi de idempotent
	// olmalı (ör. retry senaryosu — commit sonrası kayıt başarısız olup
	// tekrar denenirse).
	if err := SaveBlockOps(db, 1, []string{"op-1", "op-1"}, "2026-08-02T00:00:00Z"); err != nil {
		t.Fatalf("SaveBlockOps: %v", err)
	}
	got, _ := OpsForBlock(db, 1)
	if len(got) != 1 {
		t.Fatalf("tekrarlanan op-id tek satıra düşmeliydi, got=%v", got)
	}
}
