package api_test

// B7 Faz 2 — conv_type/is_public yetki dallanması (onaylı semantik):
//  - grup / topluluk: tüm üyeler yazar (ek kısıt yok, mevcut davranış)
//  - kanal: sadece admin yazar (broadcast) — HTTP yazma-yetkisi, MLS'e dokunmaz
//  - davet: grup/kanal sadece admin, topluluk herhangi üye

import (
	"encoding/json"
	"testing"
)

func createConvOfType(t *testing.T, ownerToken, convType, name string, memberDIDs []string) string {
	t.Helper()
	resp, code := post(t, "/v1/conversations", map[string]interface{}{
		"type":    convType,
		"name":    name,
		"members": memberDIDs,
	}, ownerToken)
	if code != 201 {
		t.Fatalf("%s oluşturulamadı: %d %s", convType, code, resp.Error)
	}
	var data struct {
		ConvID string `json:"conv_id"`
	}
	json.Unmarshal(resp.Data, &data)
	return data.ConvID
}

// TestChannelWrite_OnlyAdminCanWrite — kanal'da sıradan üye yazamaz, admin yazabilir.
func TestChannelWrite_OnlyAdminCanWrite(t *testing.T) {
	ownerPhone := "+905550009501"
	_, ownerToken := registerUserDirect(t, ownerPhone, "chan_owner")
	setUserCreditScore(t, ownerPhone, 65, 2)

	memberDID, memberToken := registerUserDirect(t, "+905550009502", "chan_member")

	convID := createConvOfType(t, ownerToken, "channel", "Duyuru Kanalı", []string{memberDID})

	// Sıradan üye (admin değil) yazmaya çalışıyor — reddedilmeli.
	memResp, memCode := post(t, "/v1/messages", map[string]interface{}{
		"to_id":           convID,
		"ciphertext":      "ENCRYPTED:member-broadcast-attempt",
		"type":            "text",
		"is_group":        true,
		"encryption_type": "mls",
	}, memberToken)
	if memCode != 403 {
		t.Fatalf("kanal'da admin olmayan üye yazabildi: %d (success=%v)", memCode, memResp.Success)
	}

	// Admin (kurucu) yazabilmeli — regresyon yok.
	ownerResp, ownerCode := post(t, "/v1/messages", map[string]interface{}{
		"to_id":           convID,
		"ciphertext":      "ENCRYPTED:owner-broadcast",
		"type":            "text",
		"is_group":        true,
		"encryption_type": "mls",
	}, ownerToken)
	if ownerCode != 201 || !ownerResp.Success {
		t.Fatalf("kanal admin'i yazamadı (regresyon): %d %s", ownerCode, ownerResp.Error)
	}
}

// TestGroupWrite_AllMembersCanWrite — grup türünde ek kısıt yok, tüm üyeler
// yazabilir (kanal'daki admin-only kısıtı grup'a sızmamalı).
func TestGroupWrite_AllMembersCanWrite(t *testing.T) {
	ownerPhone := "+905550009503"
	_, ownerToken := registerUserDirect(t, ownerPhone, "grp_owner")
	setUserCreditScore(t, ownerPhone, 65, 2)

	memberDID, memberToken := registerUserDirect(t, "+905550009504", "grp_member")

	convID := createConvOfType(t, ownerToken, "group", "Sıradan Grup", []string{memberDID})

	memResp, memCode := post(t, "/v1/messages", map[string]interface{}{
		"to_id":           convID,
		"ciphertext":      "ENCRYPTED:member-msg",
		"type":            "text",
		"is_group":        true,
		"encryption_type": "mls",
	}, memberToken)
	if memCode != 201 || !memResp.Success {
		t.Fatalf("grup'ta sıradan üye yazamadı (regresyon): %d %s", memCode, memResp.Error)
	}
}

// TestCommunityInvite_AnyMemberCanInvite — topluluk'ta sıradan üye de davet
// linki üretebilir (grup/kanal'daki admin-only kısıtı topluluk'a sızmamalı).
func TestCommunityInvite_AnyMemberCanInvite(t *testing.T) {
	ownerPhone := "+905550009505"
	_, ownerToken := registerUserDirect(t, ownerPhone, "comm_owner")
	setUserCreditScore(t, ownerPhone, 65, 2)

	memberDID, memberToken := registerUserDirect(t, "+905550009506", "comm_member")

	convID := createConvOfType(t, ownerToken, "community", "Açık Topluluk", []string{memberDID})

	resp, code := post(t, "/v1/conversations/"+convID+"/invite/create", map[string]interface{}{}, memberToken)
	if code != 200 || !resp.Success {
		t.Fatalf("topluluk'ta sıradan üye davet oluşturamadı: %d %s", code, resp.Error)
	}
}

// TestChannelInvite_StillAdminOnly — kanal'da davet hâlâ admin-only (topluluk
// gevşetmesi kanal'a sızmamalı).
func TestChannelInvite_StillAdminOnly(t *testing.T) {
	ownerPhone := "+905550009507"
	_, ownerToken := registerUserDirect(t, ownerPhone, "chan_inv_owner")
	setUserCreditScore(t, ownerPhone, 65, 2)

	memberDID, memberToken := registerUserDirect(t, "+905550009508", "chan_inv_member")

	convID := createConvOfType(t, ownerToken, "channel", "Kanal Davet Testi", []string{memberDID})

	_, code := post(t, "/v1/conversations/"+convID+"/invite/create", map[string]interface{}{}, memberToken)
	if code != 403 {
		t.Fatalf("kanal'da admin olmayan üye davet oluşturabildi: %d", code)
	}
}
