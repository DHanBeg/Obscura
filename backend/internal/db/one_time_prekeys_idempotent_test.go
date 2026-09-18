package db

import "testing"

// TestOneTimePrekeysDidOpkUniqueIndexExists — migration 176. id (PK) her zaman
// taze uuid olduğu için eskiden "ON CONFLICT DO NOTHING" hiç tetiklenmiyordu;
// aynı (did, opk_id) çifti her yeniden yüklemede yeni satır olarak birikiyordu
// (web login/page.tsx eski akışı: her login taze 100 OPK üretip yüklerdi).
func TestOneTimePrekeysDidOpkUniqueIndexExists(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatalf("db.Init hatası: %v", err)
	}
	defer DB.Close()

	rows, err := DB.Query(`PRAGMA index_list(one_time_prekeys)`)
	if err != nil {
		t.Fatalf("index_list sorgu hatası: %v", err)
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		var seq int
		var name, origin string
		var unique, partial int
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			t.Fatalf("scan hatası: %v", err)
		}
		if name == "idx_one_time_prekeys_did_opk" {
			if unique != 1 {
				t.Fatalf("idx_one_time_prekeys_did_opk unique değil")
			}
			found = true
		}
	}
	if !found {
		t.Fatal("idx_one_time_prekeys_did_opk index'i yok — migration 176 çalışmadı")
	}
}

// TestOneTimePrekeysUploadIdempotent — keys.go HandleUploadPreKeyBundle /
// HandleReplenishOPK'nın kullandığı gerçek INSERT şeklinin regresyon kanıtı.
// Fix öncesi ("ON CONFLICT DO NOTHING", hedefsiz) bu test kırmızıydı: ikinci
// insert de RowsAffected=1 dönerdi ve tabloda 2 satır birikirdi.
func TestOneTimePrekeysUploadIdempotent(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatalf("db.Init hatası: %v", err)
	}
	defer DB.Close()

	const insertSQL = `
		INSERT INTO one_time_prekeys (id, did, opk_id, public_key, used, created_at)
		VALUES (?, ?, ?, ?, 0, ?)
		ON CONFLICT(did, opk_id) DO NOTHING
	`
	did := "did:obs:idempotent-test-user"

	res1, err := DB.Exec(insertSQL, "row-uuid-1", did, 0, "pubkeyA", "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("ilk insert hatası: %v", err)
	}
	n1, _ := res1.RowsAffected()
	if n1 != 1 {
		t.Fatalf("ilk insert RowsAffected=1 beklendi, geldi: %d", n1)
	}

	// Aynı (did, opk_id) — farklı bir uuid ile (keys.go her seferinde id
	// için uuid.New() üretiyor, bu yüzden PK asla çakışmıyordu).
	res2, err := DB.Exec(insertSQL, "row-uuid-2", did, 0, "pubkeyA", "2026-01-01T00:00:05Z")
	if err != nil {
		t.Fatalf("ikinci insert hatası: %v", err)
	}
	n2, _ := res2.RowsAffected()
	if n2 != 0 {
		t.Fatalf("ikinci insert RowsAffected=0 (çakışmalı, atlanmalı) beklendi, geldi: %d — fix tetiklenmedi", n2)
	}

	var count int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM one_time_prekeys WHERE did = ? AND opk_id = 0`, did).Scan(&count); err != nil {
		t.Fatalf("count sorgu hatası: %v", err)
	}
	if count != 1 {
		t.Fatalf("(did, opk_id=0) için 1 satır beklendi, geldi: %d — satır sınırsız birikiyor", count)
	}
}

// TestOneTimePrekeysDedupeMigrationSQL — migration 175'in DELETE mantığının
// doğrudan kanıtı: index'i geçici kaldırıp production'daki gibi (aynı
// did+opk_id, farklı id) çoğaltmalar seed edilir, migration 175'teki AYNI SQL
// çalıştırılır, sadece kullanılmamış (used=0) olan hayatta kalmalı.
func TestOneTimePrekeysDedupeMigrationSQL(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatalf("db.Init hatası: %v", err)
	}
	defer DB.Close()

	if _, err := DB.Exec(`DROP INDEX idx_one_time_prekeys_did_opk`); err != nil {
		t.Fatalf("index drop hatası: %v", err)
	}

	did := "did:obs:dedupe-test-user"
	seedRows := []struct {
		id, publicKey, createdAt string
		used                     int
	}{
		{"dupe-1", "pubkeyOLD", "2026-01-01T00:00:00Z", 1}, // kullanılmış — dedupe sonrası ölmeli
		{"dupe-2", "pubkeyFRESH", "2026-01-01T00:00:05Z", 0}, // kullanılmamış — hayatta kalmalı
		{"dupe-3", "pubkeyNEWER", "2026-01-01T00:00:10Z", 0},
	}
	for _, r := range seedRows {
		if _, err := DB.Exec(`
			INSERT INTO one_time_prekeys (id, did, opk_id, public_key, used, created_at)
			VALUES (?, ?, 0, ?, ?, ?)
		`, r.id, did, r.publicKey, r.used, r.createdAt); err != nil {
			t.Fatalf("seed insert hatası (%s): %v", r.id, err)
		}
	}

	dedupeSQL := `DELETE FROM one_time_prekeys
		WHERE id NOT IN (
			SELECT id FROM (
				SELECT id,
				       ROW_NUMBER() OVER (PARTITION BY did, opk_id ORDER BY used ASC, created_at ASC, id ASC) AS rn
				FROM one_time_prekeys
			) ranked
			WHERE rn = 1
		)`
	if _, err := DB.Exec(dedupeSQL); err != nil {
		t.Fatalf("dedupe SQL hatası: %v", err)
	}

	var count int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM one_time_prekeys WHERE did = ? AND opk_id = 0`, did).Scan(&count); err != nil {
		t.Fatalf("count sorgu hatası: %v", err)
	}
	if count != 1 {
		t.Fatalf("dedupe sonrası 1 satır beklendi, geldi: %d", count)
	}

	var survivorID string
	if err := DB.QueryRow(`SELECT id FROM one_time_prekeys WHERE did = ? AND opk_id = 0`, did).Scan(&survivorID); err != nil {
		t.Fatalf("survivor sorgu hatası: %v", err)
	}
	if survivorID != "dupe-2" {
		t.Fatalf("hayatta kalan satır kullanılmamış+en eski (dupe-2) olmalıydı, geldi: %s", survivorID)
	}
}
