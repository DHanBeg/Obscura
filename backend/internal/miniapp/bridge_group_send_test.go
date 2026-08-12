package miniapp

// Commit 0b — mini-app bridge'in "messaging.sendGroupMessage" action'ı
// HandleSendMessage'ı (internal/api) hiç kullanmadan, doğrudan SQL INSERT
// ile is_group=1 plaintext mesaj yazıyordu (bkz. eski bridge.go satır
// 267-291) — Commit 0'daki backend MLS-etiket gate'ini tamamen bypass
// ediyordu, çünkü o gate HandleSendMessage'da, bu yol ona hiç uğramıyor.
//
// Fix: plaintext-yazan SQL INSERT yolu SİLİNDİ (gate eklenmedi — mimari
// olarak mini-app'in grup için gerçek MLS anahtarı yok, göndereceği hiçbir
// şey gerçek MLS ciphertext'i olamaz, dolayısıyla "izin ver ama etiketle"
// seçeneği yok). Action artık "henüz desteklenmiyor" hatası döndürüyor,
// createMLSGroup action'ıyla aynı desen (satır 293-302, orada da DB'ye
// hiçbir şey yazılmıyor).

import (
	"os"
	"testing"

	"obscura.network/core/internal/db"
)

func TestSendGroupMessage_DoesNotPersistPlaintext(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "obscura-miniapp-test-*")
	if err != nil {
		t.Fatalf("tmp dir oluşturulamadı: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	if err := db.Init(tmpDir); err != nil {
		t.Fatalf("test DB başlatılamadı: %v", err)
	}
	defer db.Close()

	bc := BridgeContext{
		AppID:              "test-app",
		UserDID:            "did:obscura:mls-gate-sender",
		GrantedPermissions: []string{"messaging"},
		DB:                 db.DB,
	}

	const groupID = "group-plaintext-gate-test"
	_, callErr := HandleBridgeCall(bc, "messaging.sendGroupMessage", map[string]interface{}{
		"groupId": groupID,
		"content": "plaintext_group_content",
	})
	if callErr == nil {
		t.Fatal("sendGroupMessage başarıyla dönmemeli — plaintext yol kaldırıldı, henüz desteklenmiyor olmalı")
	}

	var count int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM messages WHERE conv_id = ?`, groupID).Scan(&count); err != nil {
		t.Fatalf("messages sorgu hatası: %v", err)
	}
	if count != 0 {
		t.Fatalf("plaintext grup mesajı DB'ye YAZILMAMALIYDI, yazılan satır sayısı: %d", count)
	}
}
