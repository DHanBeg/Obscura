package api_test

// B7 Faz 1 — is_public self-join, SADECE HTTP üyelik katmanı (guardrail:
// MLS grup üyeliği ayrı, senkron değil — yanıt her zaman mls_synced:false
// döner).

import (
	"encoding/json"
	"testing"
)

// TestSelfJoin_PublicChannel_Succeeds — is_public=1 kanal'a invite'sız katılma.
func TestSelfJoin_PublicChannel_Succeeds(t *testing.T) {
	ownerPhone := "+905550009601"
	_, ownerToken := registerUserDirect(t, ownerPhone, "sj_owner")
	setUserCreditScore(t, ownerPhone, 65, 2)

	resp, code := post(t, "/v1/conversations", map[string]interface{}{
		"type":      "channel",
		"name":      "Herkese Açık Kanal",
		"is_public": true,
	}, ownerToken)
	if code != 201 {
		t.Fatalf("kanal oluşturulamadı: %d %s", code, resp.Error)
	}
	var convData struct {
		ConvID string `json:"conv_id"`
	}
	json.Unmarshal(resp.Data, &convData)

	_, joinerToken := registerUserDirect(t, "+905550009602", "sj_joiner")

	joinResp, joinCode := post(t, "/v1/conversations/"+convData.ConvID+"/join", map[string]interface{}{}, joinerToken)
	if joinCode != 200 || !joinResp.Success {
		t.Fatalf("public kanal'a self-join başarısız: %d %s", joinCode, joinResp.Error)
	}
	var joinData struct {
		Status    string `json:"status"`
		MLSSynced bool   `json:"mls_synced"`
	}
	json.Unmarshal(joinResp.Data, &joinData)
	if joinData.Status != "joined" {
		t.Errorf("status=joined bekleniyordu, alınan: %s", joinData.Status)
	}
	if joinData.MLSSynced {
		t.Error("mls_synced=false olmalıydı (guardrail: MLS senkron değil), true döndü")
	}

	// Kontrol: gerçekten conv_members'a yazıldı — members listesinde görünmeli.
	membersResp, membersCode := get(t, "/v1/conversations/"+convData.ConvID+"/members", joinerToken)
	if membersCode != 200 {
		t.Fatalf("üye listesi alınamadı: %d", membersCode)
	}
	var members []map[string]interface{}
	json.Unmarshal(membersResp.Data, &members)
	if len(members) != 2 {
		t.Errorf("2 üye bekleniyordu (owner+joiner), alınan: %d", len(members))
	}
}

// TestSelfJoin_PrivateConv_Rejected — is_public=0 konuşmaya self-join reddedilmeli.
func TestSelfJoin_PrivateConv_Rejected(t *testing.T) {
	ownerPhone := "+905550009603"
	_, ownerToken := registerUserDirect(t, ownerPhone, "sj_priv_owner")
	setUserCreditScore(t, ownerPhone, 65, 2)

	resp, code := post(t, "/v1/conversations", map[string]interface{}{
		"type":      "channel",
		"name":      "Kapalı Kanal",
		"is_public": false,
	}, ownerToken)
	if code != 201 {
		t.Fatalf("kanal oluşturulamadı: %d %s", code, resp.Error)
	}
	var convData struct {
		ConvID string `json:"conv_id"`
	}
	json.Unmarshal(resp.Data, &convData)

	_, joinerToken := registerUserDirect(t, "+905550009604", "sj_priv_joiner")

	_, joinCode := post(t, "/v1/conversations/"+convData.ConvID+"/join", map[string]interface{}{}, joinerToken)
	if joinCode != 403 {
		t.Fatalf("kapalı konuşmaya self-join başarılı oldu (delik): %d", joinCode)
	}
}

// TestSelfJoin_AlreadyMember_Idempotent — zaten üye olan tekrar join'lerse
// hata almamalı, "already_member" dönmeli.
func TestSelfJoin_AlreadyMember_Idempotent(t *testing.T) {
	ownerPhone := "+905550009605"
	_, ownerToken := registerUserDirect(t, ownerPhone, "sj_idem_owner")
	setUserCreditScore(t, ownerPhone, 65, 2)

	resp, code := post(t, "/v1/conversations", map[string]interface{}{
		"type":      "community",
		"name":      "Idempotent Topluluk",
		"is_public": true,
	}, ownerToken)
	if code != 201 {
		t.Fatalf("topluluk oluşturulamadı: %d %s", code, resp.Error)
	}
	var convData struct {
		ConvID string `json:"conv_id"`
	}
	json.Unmarshal(resp.Data, &convData)

	// Kurucu (zaten admin/üye) kendi konuşmasına join dener.
	joinResp, joinCode := post(t, "/v1/conversations/"+convData.ConvID+"/join", map[string]interface{}{}, ownerToken)
	if joinCode != 200 || !joinResp.Success {
		t.Fatalf("zaten üye olan join denemesi hata verdi: %d %s", joinCode, joinResp.Error)
	}
	var joinData struct {
		Status string `json:"status"`
	}
	json.Unmarshal(joinResp.Data, &joinData)
	if joinData.Status != "already_member" {
		t.Errorf("status=already_member bekleniyordu, alınan: %s", joinData.Status)
	}
}
