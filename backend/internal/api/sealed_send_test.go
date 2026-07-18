package api_test

// sealed_send_test.go — Madde 15, Adım 5: HandleSendMessage sealed-sender
// zarflarını opak taşımalı (from_did DB'ye plaintext yazılmamalı), ama
// to_did AÇIK kalmalı (sealed-sender göndereni gizler, alıcıyı değil —
// yönlendirme bozulmamalı). Eski (encryption_type boş/"signal") client'lar
// HÂLÂ eskisi gibi plaintext from_did ile çalışmalı — kademeli geçiş, sunucu
// "sealed zorunlu" değil.
//
// loginAndRegister() (integration_test.go) yerine registerUserDirect()
// (self_destruct_send_test.go) kullanılıyor — OTP akışı bu depoda önceden
// bozuk (dev_otp alanı handler'da artık yok, sealed-sender'dan bağımsız
// pre-existing durum), registerUserDirect bunu atlayıp gerçek JWT üretir.

import (
	"encoding/json"
	"testing"

	"obscura.network/core/internal/db"
)

func TestSealedMessageStoresOpaqueFromDID(t *testing.T) {
	_, aliceToken := registerUserDirect(t, "+905550000101", "sealed_alice")
	bobDID, bobToken := registerUserDirect(t, "+905550000102", "sealed_bob")

	sendReq := map[string]interface{}{
		"to_id":           bobDID,
		"ciphertext":      "b64:sealed-envelope-opaque-bytes==",
		"type":            "text",
		"encryption_type": "sealed",
	}
	r, code := post(t, "/v1/messages", sendReq, aliceToken)
	if (code != 200 && code != 201) || !r.Success {
		t.Fatalf("Sealed mesaj gönderme başarısız: %d %s", code, r.Error)
	}

	var msgData struct {
		ID string `json:"id"`
	}
	json.Unmarshal(r.Data, &msgData)
	if msgData.ID == "" {
		t.Fatal("Mesaj ID bulunamadı")
	}

	var fromDID, toDID, encType string
	err := db.DB.QueryRow(
		`SELECT from_did, to_did, encryption_type FROM messages WHERE id = ?`, msgData.ID,
	).Scan(&fromDID, &toDID, &encType)
	if err != nil {
		t.Fatalf("Mesaj satırı okunamadı: %v", err)
	}

	if fromDID != "" {
		t.Errorf("sealed mesajda from_did opak (boş) OLMALI, sunucu plaintext yazmış: %q", fromDID)
	}
	if toDID != bobDID {
		t.Errorf("to_did AÇIK kalmalı (yönlendirme) — beklenen %q, alınan %q", bobDID, toDID)
	}
	if encType != "sealed" {
		t.Errorf("encryption_type 'sealed' olarak saklanmalı, alınan %q", encType)
	}

	// Yönlendirme gerçekten bozulmamış mı — Bob konuşmalarını normal şekilde görebilmeli.
	convs, code := get(t, "/v1/conversations", bobToken)
	if code != 200 || !convs.Success {
		t.Fatalf("Bob konuşmalarını göremiyor — yönlendirme bozulmuş olabilir: %d", code)
	}
}

func TestLegacyMessageStillStoresPlaintextFromDID(t *testing.T) {
	aliceDID, aliceToken := registerUserDirect(t, "+905550000103", "legacy_alice")
	bobDID, _ := registerUserDirect(t, "+905550000104", "legacy_bob")

	// encryption_type YOK — eski client davranışı (kademeli geçiş: sunucu
	// hâlâ kabul etmeli, "sealed zorunlu" anahtarı Adım 10'da açılacak).
	sendReq := map[string]interface{}{
		"to_id":      bobDID,
		"ciphertext": "ENCRYPTED:eski_client_mesaji",
		"type":       "text",
	}
	r, code := post(t, "/v1/messages", sendReq, aliceToken)
	if (code != 200 && code != 201) || !r.Success {
		t.Fatalf("Eski (zarfsız) mesaj gönderme başarısız: %d %s", code, r.Error)
	}

	var msgData struct {
		ID string `json:"id"`
	}
	json.Unmarshal(r.Data, &msgData)

	var fromDID, toDID string
	err := db.DB.QueryRow(
		`SELECT from_did, to_did FROM messages WHERE id = ?`, msgData.ID,
	).Scan(&fromDID, &toDID)
	if err != nil {
		t.Fatalf("Mesaj satırı okunamadı: %v", err)
	}

	if fromDID != aliceDID {
		t.Errorf("eski client davranışı KIRILMIŞ — from_did hâlâ plaintext olmalı, beklenen %q, alınan %q", aliceDID, fromDID)
	}
	if toDID != bobDID {
		t.Errorf("to_did beklenen %q, alınan %q", bobDID, toDID)
	}
}
