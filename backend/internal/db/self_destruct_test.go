package db

import (
	"testing"
)

// TestMessagesSelfDestructColumns — self_destruct_seconds / self_destruct_at
// kolonlarının messages tablosunda var olduğunu doğrular. Bu kolonlar
// mevcut expires_at (30 gün genel saklama TTL) alanından AYRI: expires_at
// tüm mesajlara otomatik uygulanan sabit saklama süresidir, self_destruct_at
// ise kullanıcının bilinçli seçtiği (okununca/10sn/1dk/5dk/1saat) isteğe
// bağlı erken silme zamanıdır.
func TestMessagesSelfDestructColumns(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatalf("db.Init hatası: %v", err)
	}
	defer DB.Close()

	rows, err := DB.Query(`PRAGMA table_info(messages)`)
	if err != nil {
		t.Fatalf("table_info sorgu hatası: %v", err)
	}
	defer rows.Close()

	found := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt interface{}
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan hatası: %v", err)
		}
		found[name] = true
	}

	if !found["self_destruct_seconds"] {
		t.Error("messages.self_destruct_seconds kolonu yok")
	}
	if !found["self_destruct_at"] {
		t.Error("messages.self_destruct_at kolonu yok")
	}
	// expires_at (30 gün genel TTL) hâlâ ayrı ve mevcut olmalı — karışmadığını doğrula.
	if !found["expires_at"] {
		t.Error("messages.expires_at kolonu kayboldu (30 gün genel TTL ile self-destruct karışmamalı)")
	}
}

// TestMessagesSelfDestructRoundTrip — self_destruct alanlarına yazıp
// okuyabildiğimizi (NULL dahil) doğrular.
func TestMessagesSelfDestructRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatalf("db.Init hatası: %v", err)
	}
	defer DB.Close()

	_, err := DB.Exec(`INSERT INTO users (id, phone, did, created_at, updated_at, last_seen_at)
		VALUES ('u1', '+90', 'did:obscura:u1', datetime('now'), datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("user insert hatası: %v", err)
	}
	_, err = DB.Exec(`INSERT INTO conversations (id, is_group, created_at, updated_at)
		VALUES ('c1', 0, datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("conv insert hatası: %v", err)
	}

	// self_destruct_seconds = 0 → "okununca" sentinel; self_destruct_at NULL kalmalı (gönderimde bilinmiyor).
	_, err = DB.Exec(`INSERT INTO messages
		(id, conv_id, from_did, to_did, type, ciphertext, status, is_group, sent_at, expires_at, self_destruct_seconds, self_destruct_at)
		VALUES ('m1', 'c1', 'did:obscura:u1', 'did:obscura:u2', 'text', 'ct', 'sent', 0, datetime('now'), datetime('now','+30 days'), 0, NULL)`)
	if err != nil {
		t.Fatalf("self-destruct mesaj insert hatası: %v", err)
	}

	var seconds *int
	var selfDestructAt *string
	err = DB.QueryRow(`SELECT self_destruct_seconds, self_destruct_at FROM messages WHERE id = 'm1'`).
		Scan(&seconds, &selfDestructAt)
	if err != nil {
		t.Fatalf("select hatası: %v", err)
	}
	if seconds == nil || *seconds != 0 {
		t.Errorf("self_destruct_seconds = %v, beklenen 0", seconds)
	}
	if selfDestructAt != nil {
		t.Errorf("self_destruct_at = %v, beklenen NULL (okununca modu, henüz okunmadı)", *selfDestructAt)
	}
}
