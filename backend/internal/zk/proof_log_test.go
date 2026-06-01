package zk

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// setupProofLogDB — in-memory SQLite veritabanı kurar ve zk_proof_log tablosunu oluşturur.
func setupProofLogDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("in-memory DB açılamadı: %v", err)
	}
	if _, err := db.Exec(ProofLogChainMigrationSQL); err != nil {
		t.Fatalf("zk_proof_log tablosu oluşturulamadı: %v", err)
	}
	return db
}

// ─── AppendProofLog testleri ──────────────────────────────────────────────────

func TestAppendProofLog_HappyPath(t *testing.T) {
	db := setupProofLogDB(t)
	defer db.Close()

	chainHash, err := AppendProofLog(db, "credit_threshold", "did:obs:abc123",
		`{"pi_a":["1","2"],"pi_b":[["3","4"],["5","6"]],"pi_c":["7","8"]}`,
		[]string{"100", "200"},
	)
	if err != nil {
		t.Fatalf("AppendProofLog hatası: %v", err)
	}
	if chainHash == "" {
		t.Fatal("chain_hash boş döndü")
	}

	// Satır DB'ye yazıldı mı?
	var count int
	db.QueryRow("SELECT COUNT(*) FROM zk_proof_log").Scan(&count)
	if count != 1 {
		t.Fatalf("beklenen 1 satır, bulundu %d", count)
	}
}

func TestAppendProofLog_ChainLinksBetweenEntries(t *testing.T) {
	db := setupProofLogDB(t)
	defer db.Close()

	// İlk kayıt — prev_hash boş olmalı
	hash1, err := AppendProofLog(db, "identity_proof", "did:obs:user1",
		`{"pi_a":["10"]}`, []string{"1"},
	)
	if err != nil {
		t.Fatalf("ilk AppendProofLog hatası: %v", err)
	}

	// İkinci kayıt — prev_hash = hash1 olmalı
	hash2, err := AppendProofLog(db, "identity_proof", "did:obs:user2",
		`{"pi_a":["20"]}`, []string{"2"},
	)
	if err != nil {
		t.Fatalf("ikinci AppendProofLog hatası: %v", err)
	}

	if hash1 == hash2 {
		t.Fatal("iki farklı proof'un chain_hash'i aynı olamaz")
	}

	// İkinci satırın prev_hash'i birincinin chain_hash'ine eşit mi?
	var storedPrevHash string
	db.QueryRow("SELECT prev_hash FROM zk_proof_log WHERE id = 2").Scan(&storedPrevHash)
	if storedPrevHash != hash1 {
		t.Fatalf("ikinci satırın prev_hash=%q beklenen=%q", storedPrevHash, hash1)
	}

	// Üçüncü kayıt
	_, err = AppendProofLog(db, "vote_proof", "did:obs:user3",
		`{"pi_a":["30"]}`, []string{"3"},
	)
	if err != nil {
		t.Fatalf("üçüncü AppendProofLog hatası: %v", err)
	}

	var count int
	db.QueryRow("SELECT COUNT(*) FROM zk_proof_log").Scan(&count)
	if count != 3 {
		t.Fatalf("beklenen 3 satır, bulundu %d", count)
	}
}

func TestAppendProofLog_NilDB(t *testing.T) {
	_, err := AppendProofLog(nil, "identity_proof", "did:obs:x",
		`{}`, nil,
	)
	if err == nil {
		t.Fatal("nil DB ile hata bekleniyor")
	}
}

// ─── VerifyLogIntegrity testleri ─────────────────────────────────────────────

func TestVerifyLogIntegrity_EmptyTable(t *testing.T) {
	db := setupProofLogDB(t)
	defer db.Close()

	valid, brokenID, err := VerifyLogIntegrity(db)
	if err != nil {
		t.Fatalf("VerifyLogIntegrity hatası: %v", err)
	}
	if !valid {
		t.Fatal("boş tablo geçerli kabul edilmeli")
	}
	if brokenID != 0 {
		t.Fatalf("beklenen brokenID=0, bulundu %d", brokenID)
	}
}

func TestVerifyLogIntegrity_ValidChain(t *testing.T) {
	db := setupProofLogDB(t)
	defer db.Close()

	// 3 kayıt ekle — hepsi geçerli zincir oluşturmali
	for i := 0; i < 3; i++ {
		if _, err := AppendProofLog(db,
			"credit_threshold",
			"did:obs:testuser",
			`{"pi_a":["1","2"],"pi_b":[["3","4"],["5","6"]],"pi_c":["7","8"]}`,
			[]string{"100", "200", "300"},
		); err != nil {
			t.Fatalf("AppendProofLog[%d] hatası: %v", i, err)
		}
	}

	valid, brokenID, err := VerifyLogIntegrity(db)
	if err != nil {
		t.Fatalf("VerifyLogIntegrity hatası: %v", err)
	}
	if !valid {
		t.Fatalf("geçerli zincir geçersiz olarak işaretlendi (brokenID=%d)", brokenID)
	}
	if brokenID != 0 {
		t.Fatalf("beklenen brokenID=0, bulundu %d", brokenID)
	}
}

func TestVerifyLogIntegrity_TamperedChainHash(t *testing.T) {
	db := setupProofLogDB(t)
	defer db.Close()

	// 3 kayıt ekle
	for i := 0; i < 3; i++ {
		if _, err := AppendProofLog(db,
			"identity_proof",
			"did:obs:tampertest",
			`{"pi_a":["9"]}`,
			[]string{"500"},
		); err != nil {
			t.Fatalf("AppendProofLog[%d] hatası: %v", i, err)
		}
	}

	// Ortadaki satırın chain_hash'ini boz (id=2)
	_, err := db.Exec(
		`UPDATE zk_proof_log SET chain_hash = 'deadbeefdeadbeefdeadbeefdeadbeef' WHERE id = 2`,
	)
	if err != nil {
		t.Fatalf("UPDATE beklenen şekilde çalışmalı (test kurulum): %v", err)
	}

	valid, brokenID, verifyErr := VerifyLogIntegrity(db)
	if verifyErr != nil {
		t.Fatalf("VerifyLogIntegrity hatası: %v", verifyErr)
	}
	if valid {
		t.Fatal("bozulmuş zincir geçerli olarak işaretlendi")
	}
	// Bozulan satır id=2 veya id=3 olabilir (3. satırın prev_hash'i artık tutarsız)
	if brokenID < 2 {
		t.Fatalf("beklenen brokenID>=2, bulundu %d", brokenID)
	}
}

func TestVerifyLogIntegrity_TamperedPrevHash(t *testing.T) {
	db := setupProofLogDB(t)
	defer db.Close()

	// 2 kayıt
	for i := 0; i < 2; i++ {
		if _, err := AppendProofLog(db,
			"vote_proof",
			"did:obs:prevhashtest",
			`{"pi_a":["42"]}`,
			[]string{"999"},
		); err != nil {
			t.Fatalf("AppendProofLog[%d] hatası: %v", i, err)
		}
	}

	// İkinci satırın prev_hash'ini boz
	_, err := db.Exec(
		`UPDATE zk_proof_log SET prev_hash = 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' WHERE id = 2`,
	)
	if err != nil {
		t.Fatalf("UPDATE setup hatası: %v", err)
	}

	valid, brokenID, verifyErr := VerifyLogIntegrity(db)
	if verifyErr != nil {
		t.Fatalf("VerifyLogIntegrity hatası: %v", verifyErr)
	}
	if valid {
		t.Fatal("bozulmuş prev_hash geçerli olarak işaretlendi")
	}
	if brokenID != 2 {
		t.Fatalf("beklenen brokenID=2, bulundu %d", brokenID)
	}
}

func TestVerifyLogIntegrity_NilDB(t *testing.T) {
	_, _, err := VerifyLogIntegrity(nil)
	if err == nil {
		t.Fatal("nil DB ile hata bekleniyor")
	}
}
