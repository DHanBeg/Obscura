package api_test

// Madde 13 Adım 4 — Güven kişisi işaretleme testleri.
//
// contacts.is_trusted (migration 127) burada gerçek endpoint'e bağlanıyor:
// PATCH /v1/contacts/{did}. Panik butonu (Adım 5) bu flag'i taşıyan kişileri
// listeleyecek; bu testler flag'in doğru yazıldığını, sadece sahibini
// etkilediğini ve varsayılanın kapalı (güven varsayılan DEĞİL) kaldığını
// doğruluyor.

import (
	"encoding/json"
	"testing"
)

// TestGetContactsIncludesIsTrusted — GET /v1/contacts artık is_trusted alanını
// döner, varsayılan false.
func TestGetContactsIncludesIsTrusted(t *testing.T) {
	_, ownerToken := registerUserDirect(t, "+905559990201", "trust_owner_001")
	friendDID, _ := registerUserDirect(t, "+905559990202", "trust_friend_001")

	_, code := post(t, "/v1/contacts", map[string]interface{}{"did": friendDID}, ownerToken)
	if code != 201 {
		t.Fatalf("Kişi eklenemedi (code=%d)", code)
	}

	listResp, listCode := get(t, "/v1/contacts", ownerToken)
	if listCode != 200 || !listResp.Success {
		t.Fatalf("Kişi listesi alınamadı (code=%d): %s", listCode, listResp.Error)
	}
	var body struct {
		Contacts []struct {
			DID       string `json:"did"`
			IsTrusted bool   `json:"is_trusted"`
		} `json:"contacts"`
	}
	json.Unmarshal(listResp.Data, &body)

	found := false
	for _, c := range body.Contacts {
		if c.DID == friendDID {
			found = true
			if c.IsTrusted != false {
				t.Errorf("is_trusted varsayılanı = %v, beklenen false (güven varsayılan DEĞİL)", c.IsTrusted)
			}
		}
	}
	if !found {
		t.Fatal("Eklenen kişi listede bulunamadı")
	}
}

// TestUpdateContactTrustSetsAndPersists — PATCH ile is_trusted=true set edilir
// ve taze bir GET ile (sunucudan yeniden okuma) korunduğu doğrulanır.
func TestUpdateContactTrustSetsAndPersists(t *testing.T) {
	_, ownerToken := registerUserDirect(t, "+905559990203", "trust_owner_002")
	friendDID, _ := registerUserDirect(t, "+905559990204", "trust_friend_002")

	post(t, "/v1/contacts", map[string]interface{}{"did": friendDID}, ownerToken)

	patchResp, patchCode := patch(t, "/v1/contacts/"+friendDID, map[string]interface{}{"is_trusted": true}, ownerToken)
	if patchCode != 200 || !patchResp.Success {
		t.Fatalf("Güven kişisi işaretlenemedi (code=%d): %s", patchCode, patchResp.Error)
	}

	// Taze GET — bellekten değil sunucudan yeniden okuma.
	listResp, _ := get(t, "/v1/contacts", ownerToken)
	var body struct {
		Contacts []struct {
			DID       string `json:"did"`
			IsTrusted bool   `json:"is_trusted"`
		} `json:"contacts"`
	}
	json.Unmarshal(listResp.Data, &body)

	found := false
	for _, c := range body.Contacts {
		if c.DID == friendDID {
			found = true
			if !c.IsTrusted {
				t.Error("is_trusted PATCH sonrası true değil — persist edilmemiş")
			}
		}
	}
	if !found {
		t.Fatal("Kişi listede bulunamadı")
	}
}

// TestUpdateContactTrustCanUnset — toggle geri kapatılabilir.
func TestUpdateContactTrustCanUnset(t *testing.T) {
	_, ownerToken := registerUserDirect(t, "+905559990205", "trust_owner_003")
	friendDID, _ := registerUserDirect(t, "+905559990206", "trust_friend_003")

	post(t, "/v1/contacts", map[string]interface{}{"did": friendDID}, ownerToken)
	patch(t, "/v1/contacts/"+friendDID, map[string]interface{}{"is_trusted": true}, ownerToken)
	patchResp, patchCode := patch(t, "/v1/contacts/"+friendDID, map[string]interface{}{"is_trusted": false}, ownerToken)
	if patchCode != 200 || !patchResp.Success {
		t.Fatalf("Güven kişisi kapatılamadı (code=%d): %s", patchCode, patchResp.Error)
	}

	listResp, _ := get(t, "/v1/contacts", ownerToken)
	var body struct {
		Contacts []struct {
			DID       string `json:"did"`
			IsTrusted bool   `json:"is_trusted"`
		} `json:"contacts"`
	}
	json.Unmarshal(listResp.Data, &body)
	for _, c := range body.Contacts {
		if c.DID == friendDID && c.IsTrusted {
			t.Error("is_trusted kapatma sonrası hâlâ true")
		}
	}
}

// TestUpdateContactTrustRequiresExistingContact — rehberde olmayan bir DID
// için 404 dönmeli (var olmayan ilişkiyi sessizce "başarılı" saymamalı).
func TestUpdateContactTrustRequiresExistingContact(t *testing.T) {
	_, ownerToken := registerUserDirect(t, "+905559990207", "trust_owner_004")
	strangerDID, _ := registerUserDirect(t, "+905559990208", "trust_stranger_004")

	_, code := patch(t, "/v1/contacts/"+strangerDID, map[string]interface{}{"is_trusted": true}, ownerToken)
	if code != 404 {
		t.Errorf("Beklenen 404 (rehberde olmayan kişi), alınan %d", code)
	}
}

// TestUpdateContactTrustOnlyAffectsOwner — A kullanıcısının PATCH isteği B'nin
// rehberindeki aynı DID'i etkilememeli (owner_did scoping doğrulaması).
func TestUpdateContactTrustOnlyAffectsOwner(t *testing.T) {
	_, ownerAToken := registerUserDirect(t, "+905559990209", "trust_ownerA_005")
	_, ownerBToken := registerUserDirect(t, "+905559990210", "trust_ownerB_005")
	sharedFriendDID, _ := registerUserDirect(t, "+905559990211", "trust_shared_005")

	post(t, "/v1/contacts", map[string]interface{}{"did": sharedFriendDID}, ownerAToken)
	post(t, "/v1/contacts", map[string]interface{}{"did": sharedFriendDID}, ownerBToken)

	// Yalnızca A güven kişisi olarak işaretliyor.
	patchResp, patchCode := patch(t, "/v1/contacts/"+sharedFriendDID, map[string]interface{}{"is_trusted": true}, ownerAToken)
	if patchCode != 200 || !patchResp.Success {
		t.Fatalf("A'nın PATCH isteği başarısız oldu (code=%d) — pozitif senaryo doğrulanamıyor", patchCode)
	}

	listAResp, _ := get(t, "/v1/contacts", ownerAToken)
	var bodyA struct {
		Contacts []struct {
			DID       string `json:"did"`
			IsTrusted bool   `json:"is_trusted"`
		} `json:"contacts"`
	}
	json.Unmarshal(listAResp.Data, &bodyA)
	aTrusted := false
	for _, c := range bodyA.Contacts {
		if c.DID == sharedFriendDID && c.IsTrusted {
			aTrusted = true
		}
	}
	if !aTrusted {
		t.Fatal("A'nın kendi rehberinde is_trusted true değil — PATCH gerçekte etkilemedi")
	}

	listBResp, _ := get(t, "/v1/contacts", ownerBToken)
	var bodyB struct {
		Contacts []struct {
			DID       string `json:"did"`
			IsTrusted bool   `json:"is_trusted"`
		} `json:"contacts"`
	}
	json.Unmarshal(listBResp.Data, &bodyB)
	for _, c := range bodyB.Contacts {
		if c.DID == sharedFriendDID && c.IsTrusted {
			t.Error("B'nin rehberindeki aynı kişi de güven kişisi oldu — owner_did scoping kırık")
		}
	}
}
