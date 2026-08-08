package api_test

// #37 Bug A — grup mesajında messages.to_did gerçek alıcı DEĞİL, conv_id'nin
// kendisi (bkz. handlers.go findOrCreateConversation). Bu yüzden
// HandleMarkMessageRead ve HandleGetMessageStatus'taki eski `to_did ==
// user.DID` kontrolü grupta hiçbir üye için doğru sonuç vermiyordu — üye
// olmayana bile isabet etmiyor, üyeye de erişim vermiyordu. Fix: is_group
// dalında yetki artık conv_members üyeliğine (isConvMember) dayanıyor.
//
// Senaryolar:
//  1. Üye-olmayan  → her iki endpoint'te de 403 (fix ÖNCESİ de sonrası da).
//  2. Üye (alıcı taraf) → MarkMessageRead 200, GetMessageStatus 200.
//  3. Gönderen (grup üyesi) → MarkMessageRead 403 (kendi mesajını okuyamaz),
//     GetMessageStatus 200 (gönderen HER ZAMAN sorgulayabilir — iki handler
//     gönderen kuralında KASITLI olarak ters).
//  4. 1-1 mesaj davranışı regresyon: message_status_test.go'daki mevcut
//     testler (TestMarkMessageRead, TestMarkMessageReadForbiddenForSender,
//     TestGetMessageStatusForbiddenForThirdParty) bu değişiklikten sonra da
//     yeşil kalmalı — ayrı dosya, aynı test koşusunda kanıt.

import (
	"encoding/json"
	"fmt"
	"testing"
)

func createTestGroup(t *testing.T, creatorToken string, memberDIDs []string) string {
	t.Helper()
	resp, code := post(t, "/v1/conversations", map[string]interface{}{
		"type":    "group",
		"name":    "Bug A Test Grubu",
		"members": memberDIDs,
	}, creatorToken)
	if code != 201 {
		t.Fatalf("grup oluşturulamadı: %d %s", code, resp.Error)
	}
	var convData struct {
		ConvID string `json:"conv_id"`
	}
	json.Unmarshal(resp.Data, &convData)
	if convData.ConvID == "" {
		t.Fatal("conv_id boş döndü")
	}
	return convData.ConvID
}

func sendTestGroupMessage(t *testing.T, senderToken, convID string) string {
	t.Helper()
	resp, code := post(t, "/v1/messages", map[string]interface{}{
		"to_id":      convID,
		"ciphertext": "grup_yetki_testi",
		"type":       "text",
		"is_group":   true,
	}, senderToken)
	if code != 201 || !resp.Success {
		t.Fatalf("grup mesajı gönderilemedi: %d %s", code, resp.Error)
	}
	var data struct {
		ID string `json:"id"`
	}
	json.Unmarshal(resp.Data, &data)
	if data.ID == "" {
		t.Fatal("mesaj ID boş döndü")
	}
	return data.ID
}

// TestGroupMarkRead_NonMemberForbidden — konuşmanın üyesi olmayan biri
// grup mesajını okundu işaretleyemez (fix öncesi de 403'tü, davranış korunmalı).
func TestGroupMarkRead_NonMemberForbidden(t *testing.T) {
	creatorToken := loginAndRegister(t, "+905559993001", "bugA_creator1")
	setUserCreditScore(t, "+905559993001", 65, 2)
	memberToken := loginAndRegister(t, "+905559993002", "bugA_member1")
	memberDID := currentUserDID(t, memberToken)
	outsiderToken := loginAndRegister(t, "+905559993003", "bugA_outsider1")

	convID := createTestGroup(t, creatorToken, []string{memberDID})
	msgID := sendTestGroupMessage(t, creatorToken, convID)

	_, code := post(t, fmt.Sprintf("/v1/messages/%s/read", msgID), nil, outsiderToken)
	if code != 403 {
		t.Errorf("üye olmayan MarkMessageRead: beklenen 403, alınan %d", code)
	}

	_, code = get(t, fmt.Sprintf("/v1/messages/%s/status", msgID), outsiderToken)
	if code != 403 {
		t.Errorf("üye olmayan GetMessageStatus: beklenen 403, alınan %d", code)
	}
}

// TestGroupMarkRead_MemberAllowed — grup üyesi (alıcı taraf) kendisine gelen
// grup mesajını okundu işaretleyebilir ve durumunu sorgulayabilir — Bug A'nın
// asıl fix'i.
func TestGroupMarkRead_MemberAllowed(t *testing.T) {
	creatorToken := loginAndRegister(t, "+905559993004", "bugA_creator2")
	setUserCreditScore(t, "+905559993004", 65, 2)
	memberToken := loginAndRegister(t, "+905559993005", "bugA_member2")
	memberDID := currentUserDID(t, memberToken)

	convID := createTestGroup(t, creatorToken, []string{memberDID})
	msgID := sendTestGroupMessage(t, creatorToken, convID)

	readResp, code := post(t, fmt.Sprintf("/v1/messages/%s/read", msgID), nil, memberToken)
	if code != 200 || !readResp.Success {
		t.Fatalf("üye MarkMessageRead: beklenen 200, alınan %d %s", code, readResp.Error)
	}
	var readData struct {
		Status string `json:"status"`
	}
	json.Unmarshal(readResp.Data, &readData)
	if readData.Status != "read" {
		t.Errorf("beklenen status='read', alınan %q", readData.Status)
	}

	statusResp, code := get(t, fmt.Sprintf("/v1/messages/%s/status", msgID), memberToken)
	if code != 200 || !statusResp.Success {
		t.Fatalf("üye GetMessageStatus: beklenen 200, alınan %d %s", code, statusResp.Error)
	}
}

// TestGroupMarkRead_SenderForbiddenButCanQueryStatus — gönderen kendi grup
// mesajını okundu İŞARETLEYEMEZ (spec Bölüm 6.4, grup dalında da korunmalı)
// ama durumunu HER ZAMAN sorgulayabilir — iki handler'ın gönderen kuralı
// KASITLI olarak ters, bu test ikisini birden kilitliyor.
func TestGroupMarkRead_SenderForbiddenButCanQueryStatus(t *testing.T) {
	creatorToken := loginAndRegister(t, "+905559993006", "bugA_creator3")
	setUserCreditScore(t, "+905559993006", 65, 2)
	memberToken := loginAndRegister(t, "+905559993007", "bugA_member3")
	memberDID := currentUserDID(t, memberToken)

	convID := createTestGroup(t, creatorToken, []string{memberDID})
	msgID := sendTestGroupMessage(t, creatorToken, convID)

	_, code := post(t, fmt.Sprintf("/v1/messages/%s/read", msgID), nil, creatorToken)
	if code != 403 {
		t.Errorf("gönderen MarkMessageRead: beklenen 403, alınan %d", code)
	}

	statusResp, code := get(t, fmt.Sprintf("/v1/messages/%s/status", msgID), creatorToken)
	if code != 200 || !statusResp.Success {
		t.Fatalf("gönderen GetMessageStatus: beklenen 200, alınan %d %s", code, statusResp.Error)
	}
}
