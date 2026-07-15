package messaging

// ADIM 3 — Silme mekanizması testleri (Madde 5: self-destruct).
//
// runExpiryPass genelleştirildi: hem expires_at (30 gün genel TTL) hem
// self_destruct_at (kullanıcı seçimli erken silme) taranır. Sadece
// self_destruct_at tetiklerse ciphertext GERÇEKTEN silinir (deleted_at set +
// placeholder) — genel 30 gün TTL'i bilinçli olarak sadece status flag'i
// değiştirmeye devam eder (davranış değişikliği istenmedi, sadece self-destruct
// için "teşhis'teki ciphertext silinmiyor" sorunu düzeltiliyor).

import (
	"testing"
	"time"

	"obscura.network/core/internal/db"
)

func setupExpiryTestDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init hatası: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })

	_, err := db.DB.Exec(`INSERT INTO conversations (id, is_group, created_at, updated_at)
		VALUES ('conv1', 0, datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("conv insert hatası: %v", err)
	}
}

// insertTestMessage — verilen zamanları HER ZAMAN UTC ile biçimlendirir.
// runExpiryPass karşılaştırmayı time.Now().UTC() ile yapıyor; RFC3339 string
// karşılaştırması farklı offset'lerde (ör. yerel +03:00 vs "Z") sözlüksel
// olarak YANLIŞ sıralanabilir. Test makinesi local TZ'den bağımsız kalsın.
func insertTestMessage(t *testing.T, id string, expiresAt time.Time, selfDestructSeconds *int, selfDestructAt *time.Time) {
	t.Helper()
	var sdAtVal interface{}
	if selfDestructAt != nil {
		sdAtVal = selfDestructAt.UTC().Format(time.RFC3339)
	}
	_, err := db.DB.Exec(`
		INSERT INTO messages (id, conv_id, from_did, to_did, type, ciphertext, status, sent_at, expires_at,
		                      self_destruct_seconds, self_destruct_at)
		VALUES (?, 'conv1', 'did:obscura:from', 'did:obscura:to', 'text', 'gizli_icerik', 'sent',
		        datetime('now'), ?, ?, ?)`,
		id, expiresAt.UTC().Format(time.RFC3339), selfDestructSeconds, sdAtVal,
	)
	if err != nil {
		t.Fatalf("mesaj insert hatası (id=%s): %v", id, err)
	}
}

func readMessageState(t *testing.T, id string) (status, ciphertext string, deletedAt *string) {
	t.Helper()
	err := db.DB.QueryRow(`SELECT status, ciphertext, deleted_at FROM messages WHERE id = ?`, id).
		Scan(&status, &ciphertext, &deletedAt)
	if err != nil {
		t.Fatalf("mesaj okunamadı (id=%s): %v", id, err)
	}
	return
}

// TestRunExpiryPassScrubsSelfDestructCiphertext — self_destruct_at geçmişte
// olan mesajın ciphertext'i GERÇEKTEN silinmeli (deleted_at set + placeholder),
// sadece status flag'i değil (teşhiste tespit edilen eksiklik).
func TestRunExpiryPassScrubsSelfDestructCiphertext(t *testing.T) {
	setupExpiryTestDB(t)
	seconds := 10
	past := time.Now().Add(-1 * time.Minute)
	future := time.Now().Add(30 * 24 * time.Hour) // expires_at (30 gün) henüz dolmadı
	insertTestMessage(t, "m-selfdestruct", future, &seconds, &past)

	runExpiryPass(db.DB, NewHub())

	status, ciphertext, deletedAt := readMessageState(t, "m-selfdestruct")
	if status != "expired" {
		t.Errorf("status = %q, beklenen 'expired'", status)
	}
	if ciphertext == "gizli_icerik" {
		t.Error("ciphertext silinmedi — self-destruct süresi dolan mesajın içeriği hâlâ DB'de duruyor")
	}
	if deletedAt == nil {
		t.Error("deleted_at set edilmedi — self-destruct GERÇEK silme yapmıyor")
	}
}

// TestRunExpiryPassGeneralTTLDoesNotScrubCiphertext — sadece genel 30 gün
// TTL'i dolan (self_destruct_at NULL) mesaj mevcut davranışı korumalı: status
// 'expired' olur ama ciphertext silinmez (davranış değişikliği bu adımın
// kapsamı dışında, sadece self-destruct'ın eksik sildiği düzeltiliyor).
func TestRunExpiryPassGeneralTTLDoesNotScrubCiphertext(t *testing.T) {
	setupExpiryTestDB(t)
	past := time.Now().Add(-1 * time.Minute)
	insertTestMessage(t, "m-general-ttl", past, nil, nil)

	runExpiryPass(db.DB, NewHub())

	status, ciphertext, deletedAt := readMessageState(t, "m-general-ttl")
	if status != "expired" {
		t.Errorf("status = %q, beklenen 'expired'", status)
	}
	if ciphertext != "gizli_icerik" {
		t.Errorf("ciphertext = %q, genel 30 gün TTL'de değişmemeliydi", ciphertext)
	}
	if deletedAt != nil {
		t.Error("deleted_at set edildi — genel TTL'de bu adımda beklenmiyordu")
	}
}

// TestRunExpiryPassSkipsFutureSelfDestruct — self_destruct_at gelecekte olan
// mesaj (henüz dolmamış) dokunulmadan kalmalı.
func TestRunExpiryPassSkipsFutureSelfDestruct(t *testing.T) {
	setupExpiryTestDB(t)
	seconds := 3600
	future := time.Now().Add(1 * time.Hour)
	expiresFuture := time.Now().Add(30 * 24 * time.Hour)
	insertTestMessage(t, "m-future-selfdestruct", expiresFuture, &seconds, &future)

	runExpiryPass(db.DB, NewHub())

	status, ciphertext, deletedAt := readMessageState(t, "m-future-selfdestruct")
	if status == "expired" {
		t.Error("status 'expired' oldu — self_destruct_at henüz gelmedi")
	}
	if ciphertext != "gizli_icerik" {
		t.Error("ciphertext silindi — self_destruct_at henüz gelmedi")
	}
	if deletedAt != nil {
		t.Error("deleted_at set edildi — self_destruct_at henüz gelmedi")
	}
}
