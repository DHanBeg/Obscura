package api_test

// sealed_policy_test.go — Adım 10: kademeli geçiş anahtarı testleri.
//
// Kapsanan senaryolar (kullanıcının test-önce listesi):
//  1. eski (zarfsız) mesaj hâlâ gidiyor mu → TestLegacyMessageStillStoresPlaintextFromDID
//     (sealed_send_test.go, önceden var)
//  2. sealed mesaj çalışıyor mu → TestSealedMessageStoresOpaqueFromDID
//     (sealed_send_test.go, önceden var)
//  3. zorunlu anahtar KAPALIYKEN ikisi bir arada mı → TestMixedFormats_BothAcceptedWhenPolicyOff
//  4. zorunlu anahtar AÇIKKEN eski reddediliyor mu (grup HARİÇ) →
//     TestSealedSenderMandatory_RejectsLegacyDirectMessage,
//     TestSealedSenderMandatory_AllowsSealedMessage,
//     TestSealedSenderMandatory_GroupMessageExempt

import (
	"encoding/json"
	"os"
	"testing"

	"obscura.network/core/internal/api"
)

// withSealedSenderRequired — testin süresince OBSCURA_SEALED_SENDER_REQUIRED
// politikasını açar, test bitince (t.Cleanup) varsayılan (kapalı) duruma
// geri döndürür. TestMain zaten api.InitSealedSenderPolicyFromEnv()'i env
// boşken bir kez çağırıyor — main.go'daki gerçek boot sırasını yansıtır.
func withSealedSenderRequired(t *testing.T, enabled bool) {
	t.Helper()
	if enabled {
		os.Setenv("OBSCURA_SEALED_SENDER_REQUIRED", "true")
	} else {
		os.Unsetenv("OBSCURA_SEALED_SENDER_REQUIRED")
	}
	api.InitSealedSenderPolicyFromEnv()
	t.Cleanup(func() {
		os.Unsetenv("OBSCURA_SEALED_SENDER_REQUIRED")
		api.InitSealedSenderPolicyFromEnv()
	})
}

// TestMixedFormats_BothAcceptedWhenPolicyOff — varsayılan (kapalı) politikada
// AYNI konuşmaya hem eski hem sealed mesaj art arda gönderilebilmeli —
// biri diğerini bozmamalı/reddetmemeli.
func TestMixedFormats_BothAcceptedWhenPolicyOff(t *testing.T) {
	withSealedSenderRequired(t, false)

	_, aliceToken := registerUserDirect(t, "+905550009301", "mixed_alice")
	bobDID, _ := registerUserDirect(t, "+905550009302", "mixed_bob")

	legacyReq := map[string]interface{}{
		"to_id":      bobDID,
		"ciphertext": "ENCRYPTED:legacy",
		"type":       "text",
	}
	r1, code1 := post(t, "/v1/messages", legacyReq, aliceToken)
	if (code1 != 200 && code1 != 201) || !r1.Success {
		t.Fatalf("eski mesaj reddedildi (politika kapalıyken kabul edilmeliydi): %d %s", code1, r1.Error)
	}

	sealedReq := map[string]interface{}{
		"to_id":           bobDID,
		"ciphertext":      "b64:sealed-envelope==",
		"type":            "text",
		"encryption_type": "sealed",
	}
	r2, code2 := post(t, "/v1/messages", sealedReq, aliceToken)
	if (code2 != 200 && code2 != 201) || !r2.Success {
		t.Fatalf("sealed mesaj reddedildi (politika kapalıyken kabul edilmeliydi): %d %s", code2, r2.Error)
	}

}

// TestSealedSenderMandatory_RejectsLegacyDirectMessage — politika AÇIKKEN
// zarfsız 1:1 mesaj reddedilmeli.
func TestSealedSenderMandatory_RejectsLegacyDirectMessage(t *testing.T) {
	withSealedSenderRequired(t, true)

	_, aliceToken := registerUserDirect(t, "+905550009303", "mand_alice")
	bobDID, _ := registerUserDirect(t, "+905550009304", "mand_bob")

	legacyReq := map[string]interface{}{
		"to_id":      bobDID,
		"ciphertext": "ENCRYPTED:should-be-rejected",
		"type":       "text",
	}
	r, code := post(t, "/v1/messages", legacyReq, aliceToken)
	if code == 200 || code == 201 || r.Success {
		t.Fatalf("zorunlu mod açıkken eski (zarfsız) mesaj kabul edildi: %d %s", code, r.Error)
	}
}

// TestSealedSenderMandatory_AllowsSealedMessage — politika AÇIKKEN sealed
// mesaj hâlâ kabul edilmeli (zorunlu mod = eskiyi reddet, yeniyi engelleme).
func TestSealedSenderMandatory_AllowsSealedMessage(t *testing.T) {
	withSealedSenderRequired(t, true)

	_, aliceToken := registerUserDirect(t, "+905550009305", "mand_sealed_alice")
	bobDID, _ := registerUserDirect(t, "+905550009306", "mand_sealed_bob")

	sealedReq := map[string]interface{}{
		"to_id":           bobDID,
		"ciphertext":      "b64:sealed-envelope-mandatory==",
		"type":            "text",
		"encryption_type": "sealed",
	}
	r, code := post(t, "/v1/messages", sealedReq, aliceToken)
	if (code != 200 && code != 201) || !r.Success {
		t.Fatalf("zorunlu mod açıkken sealed mesaj reddedildi: %d %s", code, r.Error)
	}
}

// TestSealedSenderMandatory_GroupMessageExempt — politika AÇIKKEN bile grup
// mesajları muaf kalmalı: sealed-sender zarfı tek-alıcı X25519 DH'ına
// dayanır, grup fanout'una mimari olarak uygulanamaz (bkz. sealed_policy.go
// yorum). Muafiyet olmasaydı zorunlu mod TÜM grup mesajlaşmasını kırardı.
func TestSealedSenderMandatory_GroupMessageExempt(t *testing.T) {
	withSealedSenderRequired(t, true)

	alicePhone := "+905550009307"
	_, aliceToken := registerUserDirect(t, alicePhone, "mand_group_alice")
	setUserCreditScore(t, alicePhone, 65, 2) // Gümüş — grup açabilir

	bobDID, _ := registerUserDirect(t, "+905550009308", "mand_group_bob")

	// conv_id gerçek bir grup olmalı, alice üyesi olmalı — HandleSendMessage
	// artık isConvMember gate'i kullanıyor (bkz. B7 Faz 1 güvenlik düzeltmesi),
	// fabrikasyon bir conv_id ile üye-olmayan yazma denemesi 403 döner.
	resp, code := post(t, "/v1/conversations", map[string]interface{}{
		"type":    "group",
		"name":    "Mandatory Sealed Test Grubu",
		"members": []string{bobDID},
	}, aliceToken)
	if code != 201 {
		t.Fatalf("grup oluşturulamadı: %d %s", code, resp.Error)
	}
	var convData struct {
		ConvID string `json:"conv_id"`
	}
	json.Unmarshal(resp.Data, &convData)

	groupReq := map[string]interface{}{
		"to_id":           convData.ConvID,
		"ciphertext":      "ENCRYPTED:group-message",
		"type":            "text",
		"is_group":        true,
		"encryption_type": "mls",
	}
	r, code := post(t, "/v1/messages", groupReq, aliceToken)
	if (code != 200 && code != 201) || !r.Success {
		t.Fatalf("zorunlu mod açıkken grup mesajı MUAF olmalıydı ama reddedildi: %d %s", code, r.Error)
	}
}
