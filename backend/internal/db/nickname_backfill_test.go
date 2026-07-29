package db

import "testing"

// TestBackfillUserDisplayName — display_name boş olan satır için username
// varsayılan olarak yazılır (registration invariant'ıyla tutarlı: her zaman
// display_name=username ile başlar, bu backfill sadece güvenlik ağı).
func TestBackfillUserDisplayName(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatalf("db.Init hatası: %v", err)
	}
	defer DB.Close()

	const did = "did:obs:nickbackfilltestuser000000000000"
	_, err := DB.Exec(`INSERT INTO users (id, phone, username, display_name, did, created_at, updated_at, last_seen_at)
		VALUES ('u-nickbackfill-1', '+900000000003', 'nickbackfilluser', '', ?, datetime('now'), datetime('now'), datetime('now'))`, did)
	if err != nil {
		t.Fatalf("test kullanıcısı eklenemedi: %v", err)
	}

	if err := BackfillUserDisplayName(); err != nil {
		t.Fatalf("BackfillUserDisplayName hatası: %v", err)
	}

	var displayName string
	if err := DB.QueryRow(`SELECT display_name FROM users WHERE id = 'u-nickbackfill-1'`).Scan(&displayName); err != nil {
		t.Fatalf("display_name sorgu hatası: %v", err)
	}
	if displayName != "nickbackfilluser" {
		t.Fatalf("display_name = %q, istenen %q", displayName, "nickbackfilluser")
	}

	// İkinci çalıştırma idempotent olmalı.
	if err := BackfillUserDisplayName(); err != nil {
		t.Fatalf("ikinci BackfillUserDisplayName çağrısı hatası: %v", err)
	}
	var displayName2 string
	DB.QueryRow(`SELECT display_name FROM users WHERE id = 'u-nickbackfill-1'`).Scan(&displayName2)
	if displayName2 != displayName {
		t.Fatalf("ikinci çalıştırmada display_name değişti: %q != %q", displayName2, displayName)
	}
}

// TestBackfillUserDisplayNameFallsBackToODI — username de boşsa (register
// invariant'ı bozulmuş anormal bir satır), odi'ye düşülür.
func TestBackfillUserDisplayNameFallsBackToODI(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatalf("db.Init hatası: %v", err)
	}
	defer DB.Close()

	const did = "did:obs:nickbackfilltestuser000000000001"
	_, err := DB.Exec(`INSERT INTO users (id, phone, username, display_name, did, odi, created_at, updated_at, last_seen_at)
		VALUES ('u-nickbackfill-2', '+900000000004', '', '', ?, 'ODI-TEST-0000-0002', datetime('now'), datetime('now'), datetime('now'))`, did)
	if err != nil {
		t.Fatalf("test kullanıcısı eklenemedi: %v", err)
	}

	if err := BackfillUserDisplayName(); err != nil {
		t.Fatalf("BackfillUserDisplayName hatası: %v", err)
	}

	var displayName string
	if err := DB.QueryRow(`SELECT display_name FROM users WHERE id = 'u-nickbackfill-2'`).Scan(&displayName); err != nil {
		t.Fatalf("display_name sorgu hatası: %v", err)
	}
	if displayName != "ODI-TEST-0000-0002" {
		t.Fatalf("display_name = %q, istenen odi fallback %q", displayName, "ODI-TEST-0000-0002")
	}
}
