package db

import (
	"testing"

	"obscura.network/core/internal/identity"
)

// TestUsersODIColumn — users.odi kolonunun var olduğunu doğrular.
func TestUsersODIColumn(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatalf("db.Init hatası: %v", err)
	}
	defer DB.Close()

	rows, err := DB.Query(`PRAGMA table_info(users)`)
	if err != nil {
		t.Fatalf("table_info sorgu hatası: %v", err)
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt interface{}
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan hatası: %v", err)
		}
		if name == "odi" {
			found = true
		}
	}
	if !found {
		t.Error("users.odi kolonu yok")
	}
}

// TestBackfillUserODI — DID'i olan ama odi'si boş satırlar için ODI hesaplanıp
// yazıldığını, ve DeriveODI ile aynı sonucu ürettiğini doğrular.
func TestBackfillUserODI(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatalf("db.Init hatası: %v", err)
	}
	defer DB.Close()

	const did = "did:obs:backfilltestuser0000000000000000"
	_, err := DB.Exec(`INSERT INTO users (id, phone, username, display_name, did, created_at, updated_at, last_seen_at)
		VALUES ('u-backfill-1', '+900000000001', 'backfilluser', 'Backfill User', ?, datetime('now'), datetime('now'), datetime('now'))`, did)
	if err != nil {
		t.Fatalf("test kullanıcısı eklenemedi: %v", err)
	}

	if err := BackfillUserODI(); err != nil {
		t.Fatalf("BackfillUserODI hatası: %v", err)
	}

	var odi string
	if err := DB.QueryRow(`SELECT odi FROM users WHERE id = 'u-backfill-1'`).Scan(&odi); err != nil {
		t.Fatalf("odi sorgu hatası: %v", err)
	}

	want := identity.DeriveODI(did)
	if odi != want {
		t.Fatalf("odi = %q, istenen %q", odi, want)
	}

	// İkinci çalıştırma idempotent olmalı — mevcut odi değişmemeli.
	if err := BackfillUserODI(); err != nil {
		t.Fatalf("ikinci BackfillUserODI çağrısı hatası: %v", err)
	}
	var odi2 string
	DB.QueryRow(`SELECT odi FROM users WHERE id = 'u-backfill-1'`).Scan(&odi2)
	if odi2 != odi {
		t.Fatalf("ikinci çalıştırmada odi değişti: %q != %q", odi2, odi)
	}
}
