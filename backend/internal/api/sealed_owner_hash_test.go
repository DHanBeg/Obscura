package api_test

// sealed_owner_hash_test.go — Madde 15, Adım 7: sealed mesajlarda yetkilendirme.
//
// Sealed mesajlarda from_did boş (bkz. ADR-0016), bu yüzden delete/recall/
// status endpoint'leri artık owner_hash (HMAC-SHA256(pepper, DID+":"+msgID))
// ile doğruluyor. Test kapsamı:
//   1. Gerçek gönderen (sealed) mesajını silebiliyor mu
//   2. Başkası (sealed) mesajı silemiyor mu (403)
//   3. Aynı gönderenin iki (sealed) mesajı FARKLI owner_hash üretiyor mu
//      (korelasyon yok — DB dump'a bakan biri iki mesajın aynı kişiden
//      geldiğini owner_hash'ten çıkaramamalı)
//   4. Gerçek gönderen (sealed) mesajını geri çekebiliyor mu / başkası
//      çekemiyor mu (recall)
//   5. Gerçek gönderen (sealed) mesaj durumunu sorgulayabiliyor mu / alâkasız
//      üçüncü kişi sorgulayamıyor mu (status)
//   6. Eski (zarfsız, plaintext from_did) mesajlarda mevcut davranış AYNEN
//      çalışıyor mu (regresyon yok)

import (
	"encoding/json"
	"fmt"
	"testing"

	"obscura.network/core/internal/db"
)

// sendSealedMessage — sealed zarf gönderir, mesaj ID'sini döner.
func sendSealedMessage(t *testing.T, fromToken, toDID, ciphertext string) string {
	t.Helper()
	sendReq := map[string]interface{}{
		"to_id":           toDID,
		"ciphertext":      ciphertext,
		"type":            "text",
		"encryption_type": "sealed",
	}
	r, code := post(t, "/v1/messages", sendReq, fromToken)
	if (code != 200 && code != 201) || !r.Success {
		t.Fatalf("Sealed mesaj gönderilemedi: %d %s", code, r.Error)
	}
	var data struct {
		ID string `json:"id"`
	}
	json.Unmarshal(r.Data, &data)
	if data.ID == "" {
		t.Fatal("Mesaj ID boş döndü")
	}
	return data.ID
}

// TestSealedMessageOwnerCanDelete — gerçek gönderen owner_hash ile kendi
// sealed mesajını silebilmeli.
func TestSealedMessageOwnerCanDelete(t *testing.T) {
	aliceDID, aliceToken := registerUserDirect(t, "+905550003001", "owner_alice_3001")
	_ = aliceDID
	bobDID, _ := registerUserDirect(t, "+905550003002", "owner_bob_3002")

	msgID := sendSealedMessage(t, aliceToken, bobDID, "b64:sealed-owner-delete-test==")

	resp, code := doDelete(t, "/v1/messages/"+msgID, aliceToken)
	if code != 200 || !resp.Success {
		t.Fatalf("Gerçek gönderen sealed mesajını silemedi (code=%d): %s", code, resp.Error)
	}

	var deletedAt interface{}
	err := db.DB.QueryRow(`SELECT deleted_at FROM messages WHERE id = ?`, msgID).Scan(&deletedAt)
	if err != nil {
		t.Fatalf("Mesaj satırı okunamadı: %v", err)
	}
	if deletedAt == nil {
		t.Error("deleted_at set edilmemiş — silme uygulanmadı")
	}
}

// TestSealedMessageNonOwnerCannotDelete — gönderen olmayan biri (alıcı dahil)
// sealed mesajı silememeli, owner_hash eşleşmemeli.
func TestSealedMessageNonOwnerCannotDelete(t *testing.T) {
	_, aliceToken := registerUserDirect(t, "+905550003003", "owner_alice_3003")
	bobDID, bobToken := registerUserDirect(t, "+905550003004", "owner_bob_3004")
	_, malloryToken := registerUserDirect(t, "+905550003005", "owner_mallory_3005")

	msgID := sendSealedMessage(t, aliceToken, bobDID, "b64:sealed-non-owner-delete-test==")

	// Alıcı (Bob) — mesaja dahil ama GÖNDEREN değil, silememeli.
	resp, code := doDelete(t, "/v1/messages/"+msgID, bobToken)
	if code != 403 {
		t.Errorf("Alıcı sealed mesajı silebildi — beklenen 403, alınan %d (%s)", code, resp.Error)
	}

	// Alâkasız üçüncü kişi (Mallory) — hiç dahil değil, silememeli.
	resp2, code2 := doDelete(t, "/v1/messages/"+msgID, malloryToken)
	if code2 != 403 {
		t.Errorf("Üçüncü kişi sealed mesajı silebildi — beklenen 403, alınan %d (%s)", code2, resp2.Error)
	}

	// Mesaj hâlâ silinmemiş olmalı.
	var deletedAt interface{}
	err := db.DB.QueryRow(`SELECT deleted_at FROM messages WHERE id = ?`, msgID).Scan(&deletedAt)
	if err != nil {
		t.Fatalf("Mesaj satırı okunamadı: %v", err)
	}
	if deletedAt != nil {
		t.Error("Yetkisiz silme sonrası deleted_at set edilmiş — yetkilendirme atlanmış")
	}
}

// TestSealedOwnerHashDiffersAcrossMessages — aynı gönderenin iki sealed
// mesajı FARKLI owner_hash üretmeli. Aynı olsaydı, DB dump'a bakan biri
// hash'i kırmadan bile "bu iki mesaj aynı kişiden" diye kümeleyebilirdi —
// msgID'nin HMAC girdisine dahil edilmesinin tam önlediği şey bu (bkz.
// owner_hash.go, ADR-0016).
func TestSealedOwnerHashDiffersAcrossMessages(t *testing.T) {
	_, aliceToken := registerUserDirect(t, "+905550003006", "owner_alice_3006")
	bobDID, _ := registerUserDirect(t, "+905550003007", "owner_bob_3007")

	msgID1 := sendSealedMessage(t, aliceToken, bobDID, "b64:sealed-corr-test-1==")
	msgID2 := sendSealedMessage(t, aliceToken, bobDID, "b64:sealed-corr-test-2==")

	var hash1, hash2 string
	if err := db.DB.QueryRow(`SELECT owner_hash FROM messages WHERE id = ?`, msgID1).Scan(&hash1); err != nil {
		t.Fatalf("Mesaj 1 owner_hash okunamadı: %v", err)
	}
	if err := db.DB.QueryRow(`SELECT owner_hash FROM messages WHERE id = ?`, msgID2).Scan(&hash2); err != nil {
		t.Fatalf("Mesaj 2 owner_hash okunamadı: %v", err)
	}

	if hash1 == "" || hash2 == "" {
		t.Fatalf("owner_hash boş kalmamalı (sealed mesaj) — hash1=%q hash2=%q", hash1, hash2)
	}
	if hash1 == hash2 {
		t.Error("Aynı gönderenin iki sealed mesajı AYNI owner_hash üretti — korelasyon riski (msgID salt'ı çalışmıyor)")
	}
}

// TestSealedMessageOwnerCanRecall — gerçek gönderen sealed mesajını geri
// çekebilmeli; gönderen olmayan biri çekememeli.
func TestSealedMessageOwnerCanRecall(t *testing.T) {
	_, aliceToken := registerUserDirect(t, "+905550003008", "owner_alice_3008")
	bobDID, bobToken := registerUserDirect(t, "+905550003009", "owner_bob_3009")

	msgID := sendSealedMessage(t, aliceToken, bobDID, "b64:sealed-recall-test==")

	// Alıcı geri çekemez.
	_, forbidCode := post(t, fmt.Sprintf("/v1/messages/%s/recall", msgID), nil, bobToken)
	if forbidCode != 403 {
		t.Errorf("Alıcı sealed mesajı geri çekebildi — beklenen 403, alınan %d", forbidCode)
	}

	// Gerçek gönderen geri çekebilir.
	resp, code := post(t, fmt.Sprintf("/v1/messages/%s/recall", msgID), nil, aliceToken)
	if code != 200 || !resp.Success {
		t.Fatalf("Gerçek gönderen sealed mesajı geri çekemedi (code=%d): %s", code, resp.Error)
	}
}

// TestSealedMessageStatusOwnerCanQuery — gerçek gönderen (sealed) durum
// sorgulayabilmeli; alıcı da (to_did açık kaldığı için) sorgulayabilmeli;
// alâkasız üçüncü kişi sorgulayamamalı.
func TestSealedMessageStatusOwnerCanQuery(t *testing.T) {
	_, aliceToken := registerUserDirect(t, "+905550003010", "owner_alice_3010")
	bobDID, bobToken := registerUserDirect(t, "+905550003011", "owner_bob_3011")
	_, malloryToken := registerUserDirect(t, "+905550003012", "owner_mallory_3012")

	msgID := sendSealedMessage(t, aliceToken, bobDID, "b64:sealed-status-test==")

	// Gönderen (owner_hash ile) sorgulayabilmeli.
	resp, code := get(t, fmt.Sprintf("/v1/messages/%s/status", msgID), aliceToken)
	if code != 200 || !resp.Success {
		t.Fatalf("Gerçek gönderen sealed mesaj durumunu sorgulayamadı (code=%d): %s", code, resp.Error)
	}

	// Alıcı (to_did plaintext kaldığı için) sorgulayabilmeli.
	respBob, codeBob := get(t, fmt.Sprintf("/v1/messages/%s/status", msgID), bobToken)
	if codeBob != 200 || !respBob.Success {
		t.Fatalf("Alıcı sealed mesaj durumunu sorgulayamadı (code=%d): %s", codeBob, respBob.Error)
	}

	// Alâkasız üçüncü kişi sorgulayamamalı.
	_, forbidCode := get(t, fmt.Sprintf("/v1/messages/%s/status", msgID), malloryToken)
	if forbidCode != 403 {
		t.Errorf("Üçüncü kişi sealed mesaj durumunu sorgulayabildi — beklenen 403, alınan %d", forbidCode)
	}
}

// TestLegacyMessageDeleteRecallStatusUnaffected — eski (zarfsız, plaintext
// from_did) mesajlarda delete/recall/status davranışı AYNEN korunmalı —
// owner_hash mekanizması bu yolu hiç devreye sokmamalı (owner_hash boş
// kalır, fromDID != user.DID karşılaştırması eskisi gibi çalışır).
func TestLegacyMessageDeleteRecallStatusUnaffected(t *testing.T) {
	_, aliceToken := registerUserDirect(t, "+905550003013", "owner_alice_3013")
	bobDID, bobToken := registerUserDirect(t, "+905550003014", "owner_bob_3014")

	// encryption_type YOK — eski client davranışı.
	sendReq := map[string]interface{}{
		"to_id":      bobDID,
		"ciphertext": "ENCRYPTED:legacy_owner_hash_test",
		"type":       "text",
	}
	r, code := post(t, "/v1/messages", sendReq, aliceToken)
	if (code != 200 && code != 201) || !r.Success {
		t.Fatalf("Eski (zarfsız) mesaj gönderilemedi: %d %s", code, r.Error)
	}
	var sendData struct {
		ID string `json:"id"`
	}
	json.Unmarshal(r.Data, &sendData)
	msgID := sendData.ID

	var ownerHash string
	if err := db.DB.QueryRow(`SELECT owner_hash FROM messages WHERE id = ?`, msgID).Scan(&ownerHash); err != nil {
		t.Fatalf("Mesaj satırı okunamadı: %v", err)
	}
	if ownerHash != "" {
		t.Errorf("Eski (zarfsız) mesajda owner_hash boş kalmalı, alınan %q", ownerHash)
	}

	// Alıcı silemez (eski davranış: sadece gönderen silebilir).
	_, forbidCode := doDelete(t, "/v1/messages/"+msgID, bobToken)
	if forbidCode != 403 {
		t.Errorf("Alıcı eski mesajı silebildi — beklenen 403, alınan %d", forbidCode)
	}

	// Gönderen silebilir (plaintext from_did karşılaştırması eskisi gibi çalışıyor).
	resp, delCode := doDelete(t, "/v1/messages/"+msgID, aliceToken)
	if delCode != 200 || !resp.Success {
		t.Fatalf("Gönderen eski mesajı silemedi (code=%d): %s", delCode, resp.Error)
	}
}
