package api_test

// B7 Faz 1 güvenlik düzeltmesi kanıtı: Faz 0'da bulunan iki delik.
//  1. HandleSendMessage isConvMember gate KULLANMIYORDU — üye olmayan
//     bir kullanıcı bildiği herhangi bir grup conv_id'sine yazabiliyordu.
//  2. HandleCreateConvInvite requester'ın üye/admin olduğunu HİÇ
//     kontrol etmiyordu — herkes herhangi bir conv için davet üretebiliyordu.
//
// Bu testler düzeltmeden ÖNCE (gate eklenmeden) FAIL, düzeltmeden SONRA PASS
// olacak şekilde yazıldı — regresyonu kanıtlıyor.

import (
	"encoding/json"
	"testing"
)

// TestSendMessage_RejectsNonMemberGroupWrite — üyesi olmadığı bir grup
// conv_id'sine mesaj yazmaya çalışan kullanıcı 403 almalı.
func TestSendMessage_RejectsNonMemberGroupWrite(t *testing.T) {
	ownerPhone := "+905550009401"
	_, ownerToken := registerUserDirect(t, ownerPhone, "gate_owner")
	setUserCreditScore(t, ownerPhone, 65, 2) // Gümüş — grup açabilir

	memberDID, _ := registerUserDirect(t, "+905550009402", "gate_member")

	resp, code := post(t, "/v1/conversations", map[string]interface{}{
		"type":    "group",
		"name":    "Gate Test Grubu",
		"members": []string{memberDID},
	}, ownerToken)
	if code != 201 {
		t.Fatalf("grup oluşturulamadı: %d %s", code, resp.Error)
	}
	var convData struct {
		ConvID string `json:"conv_id"`
	}
	json.Unmarshal(resp.Data, &convData)

	// Gruba HİÇ eklenmemiş üçüncü bir kullanıcı — sadece conv_id'yi biliyor.
	_, outsiderToken := registerUserDirect(t, "+905550009403", "gate_outsider")

	sendResp, sendCode := post(t, "/v1/messages", map[string]interface{}{
		"to_id":           convData.ConvID,
		"ciphertext":      "ENCRYPTED:should-not-land",
		"type":            "text",
		"is_group":        true,
		"encryption_type": "mls",
	}, outsiderToken)
	if sendCode != 403 {
		t.Fatalf("üye olmayan yazma 403 almalıydı, alınan: %d (success=%v, err=%s)", sendCode, sendResp.Success, sendResp.Error)
	}

	// Kontrol: gerçek üye (owner) hâlâ yazabiliyor — regresyon yok.
	okResp, okCode := post(t, "/v1/messages", map[string]interface{}{
		"to_id":           convData.ConvID,
		"ciphertext":      "ENCRYPTED:legit",
		"type":            "text",
		"is_group":        true,
		"encryption_type": "mls",
	}, ownerToken)
	if okCode != 201 || !okResp.Success {
		t.Fatalf("gerçek üye yazamadı (regresyon): %d %s", okCode, okResp.Error)
	}
}

// TestCreateConvInvite_RejectsNonMember — bir konuşmanın üyesi olmayan
// kullanıcı o konuşma için davet linki üretemez; üye ama admin olmayan
// kullanıcı da üretemez (sadece admin).
func TestCreateConvInvite_RejectsNonMember(t *testing.T) {
	ownerPhone := "+905550009404"
	_, ownerToken := registerUserDirect(t, ownerPhone, "gate_inv_owner")
	setUserCreditScore(t, ownerPhone, 65, 2)

	memberDID, memberToken := registerUserDirect(t, "+905550009405", "gate_inv_member")

	resp, code := post(t, "/v1/conversations", map[string]interface{}{
		"type":    "group",
		"name":    "Invite Gate Test Grubu",
		"members": []string{memberDID},
	}, ownerToken)
	if code != 201 {
		t.Fatalf("grup oluşturulamadı: %d %s", code, resp.Error)
	}
	var convData struct {
		ConvID string `json:"conv_id"`
	}
	json.Unmarshal(resp.Data, &convData)

	_, outsiderToken := registerUserDirect(t, "+905550009406", "gate_inv_outsider")

	// Üye olmayan: reddedilmeli.
	_, outsiderCode := post(t, "/v1/conversations/"+convData.ConvID+"/invite/create", map[string]interface{}{}, outsiderToken)
	if outsiderCode != 403 {
		t.Fatalf("üye olmayan davet oluşturabildi (delik açık): %d", outsiderCode)
	}

	// Üye ama admin değil (sıradan member): reddedilmeli.
	_, memberCode := post(t, "/v1/conversations/"+convData.ConvID+"/invite/create", map[string]interface{}{}, memberToken)
	if memberCode != 403 {
		t.Fatalf("admin olmayan üye davet oluşturabildi: %d", memberCode)
	}

	// Admin (kurucu): başarılı olmalı.
	ownerResp, ownerCode := post(t, "/v1/conversations/"+convData.ConvID+"/invite/create", map[string]interface{}{}, ownerToken)
	if ownerCode != 200 || !ownerResp.Success {
		t.Fatalf("admin davet oluşturamadı (regresyon): %d %s", ownerCode, ownerResp.Error)
	}
}
