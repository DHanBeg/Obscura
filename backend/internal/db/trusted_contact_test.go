package db

import (
	"testing"
)

// TestUserLocationsTableDropped — Madde 13 kararı: user_locations tablosu
// (grid_id bazlı da olsa) operatörde konum toplama riski taşıyan ölü kod
// olduğu için tamamen kaldırıldı. Bu test tablonun artık VAR OLMADIĞINI
// doğrular — ileride biri yanlışlıkla geri eklerse (veya bir handler ona
// yazmaya başlarsa) bu test kırılıp uyarır.
func TestUserLocationsTableDropped(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatalf("db.Init hatası: %v", err)
	}
	defer DB.Close()

	var name string
	err := DB.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='user_locations'`).Scan(&name)
	if err == nil {
		t.Fatalf("user_locations tablosu hâlâ var (silinmesi gerekiyordu, İlke 6: operatör konum tutmaz)")
	}
}

// TestContactsIsTrustedColumn — contacts tablosunda güven kişisi flag'i
// (is_trusted) var olmalı ve varsayılan olarak kapalı (0) olmalı.
func TestContactsIsTrustedColumn(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatalf("db.Init hatası: %v", err)
	}
	defer DB.Close()

	rows, err := DB.Query(`PRAGMA table_info(contacts)`)
	if err != nil {
		t.Fatalf("table_info sorgu hatası: %v", err)
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		var cid int
		var cname, ctype string
		var notnull, pk int
		var dflt interface{}
		if err := rows.Scan(&cid, &cname, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan hatası: %v", err)
		}
		if cname == "is_trusted" {
			found = true
		}
	}
	if !found {
		t.Fatal("contacts.is_trusted kolonu yok")
	}
}

// TestContactsIsTrustedRoundTrip — is_trusted alanına yazıp okuyabildiğimizi
// ve varsayılanın 0 (güvenilmeyen) olduğunu doğrular. Güven varsayılan
// DEĞİLDİR — kullanıcı elle işaretlemeden hiçbir kişi güven kişisi olamaz.
func TestContactsIsTrustedRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatalf("db.Init hatası: %v", err)
	}
	defer DB.Close()

	_, err := DB.Exec(`INSERT INTO contacts (id, owner_did, contact_did, nickname, created_at)
		VALUES ('c1', 'did:obscura:owner', 'did:obscura:friend', 'Arkadaş', datetime('now'))`)
	if err != nil {
		t.Fatalf("contact insert hatası: %v", err)
	}

	var isTrusted int
	err = DB.QueryRow(`SELECT is_trusted FROM contacts WHERE id = 'c1'`).Scan(&isTrusted)
	if err != nil {
		t.Fatalf("select hatası: %v", err)
	}
	if isTrusted != 0 {
		t.Errorf("is_trusted varsayılanı = %d, beklenen 0 (güven varsayılan DEĞİL)", isTrusted)
	}

	_, err = DB.Exec(`UPDATE contacts SET is_trusted = 1 WHERE id = 'c1'`)
	if err != nil {
		t.Fatalf("update hatası: %v", err)
	}
	err = DB.QueryRow(`SELECT is_trusted FROM contacts WHERE id = 'c1'`).Scan(&isTrusted)
	if err != nil {
		t.Fatalf("select hatası (update sonrası): %v", err)
	}
	if isTrusted != 1 {
		t.Errorf("is_trusted update sonrası = %d, beklenen 1", isTrusted)
	}
}
