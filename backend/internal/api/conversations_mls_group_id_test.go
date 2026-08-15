package api_test

// L2 Tuğla 5b-1 — conv.id ↔ mls_group_id link'i (Karar 1a, conversations
// tablosunda nullable kolon). Bu dosya sadece HTTP-seviyesi sözleşmeyi test
// eder: POST /v1/conversations opsiyonel mls_group_id kabul ediyor mu,
// GET /v1/conversations onu geri veriyor mu, 1:1 akış hiç etkilenmiyor mu.
// Kolonun kendisi + nullability internal/db/conversations_mls_group_id_schema_test.go'da.

import (
	"encoding/json"
	"testing"
)

type convWithMlsGroupID struct {
	ID         string `json:"id"`
	IsGroup    bool   `json:"is_group"`
	MLSGroupID string `json:"mls_group_id"`
}

func findConv(t *testing.T, convs []convWithMlsGroupID, id string) convWithMlsGroupID {
	t.Helper()
	for _, c := range convs {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("conv_id %q GetConversations sonuçlarında bulunamadı", id)
	return convWithMlsGroupID{}
}

// TestCreateConversation_Direct_MlsGroupIdStaysEmpty — 1:1 konuşmalar MLS
// bilmiyor, mls_group_id hiç dolmamalı. Eski akış (mls_group_id request'te
// hiç yok) bozulmamalı.
func TestCreateConversation_Direct_MlsGroupIdStaysEmpty(t *testing.T) {
	aliceToken := loginAndRegister(t, "+905558880501", "mls_link_alice")
	bobToken := loginAndRegister(t, "+905558880502", "mls_link_bob")
	bobDID := currentUserDID(t, bobToken)

	r, code := post(t, "/v1/conversations", map[string]interface{}{
		"peer_did": bobDID,
	}, aliceToken)
	if code != 201 || !r.Success {
		t.Fatalf("1:1 konuşma oluşturulamadı (code=%d): %s", code, r.Error)
	}
	var created struct {
		ConvID string `json:"conv_id"`
	}
	if err := json.Unmarshal(r.Data, &created); err != nil {
		t.Fatalf("response parse: %v", err)
	}

	r, code = get(t, "/v1/conversations", aliceToken)
	if code != 200 || !r.Success {
		t.Fatalf("GetConversations başarısız (code=%d): %s", code, r.Error)
	}
	var convs []convWithMlsGroupID
	if err := json.Unmarshal(r.Data, &convs); err != nil {
		t.Fatalf("conversations parse: %v", err)
	}
	c := findConv(t, convs, created.ConvID)
	if c.MLSGroupID != "" {
		t.Errorf("1:1 konuşmada mls_group_id dolu geldi: %q, beklenen boş", c.MLSGroupID)
	}
}

// TestCreateConversation_Group_WithMlsGroupId — client createGroupWithMember'dan
// (ts-mls) elde ettiği group_id'yi createConversation'a verirse link kurulmalı.
func TestCreateConversation_Group_WithMlsGroupId(t *testing.T) {
	phone := "+905558880503"
	token := loginAndRegister(t, phone, "mls_link_gumus")
	setUserCreditScore(t, phone, 65, 2) // Gümüş — grup açabilir

	memberToken := loginAndRegister(t, "+905558880504", "mls_link_member")
	memberDID := currentUserDID(t, memberToken)

	wantGroupID := "b2JzY3VyYS1tbHMtbGluay10ZXN0LWdyb3Vw" // base64 opak grup_id, ts-mls çıktısı taklidi

	r, code := post(t, "/v1/conversations", map[string]interface{}{
		"type":         "group",
		"name":         "MLS Linkli Grup",
		"members":      []string{memberDID},
		"mls_group_id": wantGroupID,
	}, token)
	if code != 201 || !r.Success {
		t.Fatalf("grup oluşturulamadı (code=%d): %s", code, r.Error)
	}
	var created struct {
		ConvID string `json:"conv_id"`
	}
	if err := json.Unmarshal(r.Data, &created); err != nil {
		t.Fatalf("response parse: %v", err)
	}

	r, code = get(t, "/v1/conversations", token)
	if code != 200 || !r.Success {
		t.Fatalf("GetConversations başarısız (code=%d): %s", code, r.Error)
	}
	var convs []convWithMlsGroupID
	if err := json.Unmarshal(r.Data, &convs); err != nil {
		t.Fatalf("conversations parse: %v", err)
	}
	c := findConv(t, convs, created.ConvID)
	if c.MLSGroupID != wantGroupID {
		t.Errorf("mls_group_id = %q, beklenen %q", c.MLSGroupID, wantGroupID)
	}
}

// TestCreateConversation_Group_WithoutMlsGroupId_BackwardCompat — mls_group_id
// hiç gönderilmezse (bugünkü client davranışı) grup yine oluşmalı, sadece
// mls_group_id boş kalmalı — 5b-1'den ÖNCEKİ akış kırılmamalı.
func TestCreateConversation_Group_WithoutMlsGroupId_BackwardCompat(t *testing.T) {
	phone := "+905558880505"
	token := loginAndRegister(t, phone, "mls_link_nomls")
	setUserCreditScore(t, phone, 65, 2)

	memberToken := loginAndRegister(t, "+905558880506", "mls_link_member2")
	memberDID := currentUserDID(t, memberToken)

	r, code := post(t, "/v1/conversations", map[string]interface{}{
		"type":    "group",
		"name":    "MLS'siz Eski Grup",
		"members": []string{memberDID},
	}, token)
	if code != 201 || !r.Success {
		t.Fatalf("grup oluşturulamadı (code=%d): %s", code, r.Error)
	}
	var created struct {
		ConvID string `json:"conv_id"`
	}
	if err := json.Unmarshal(r.Data, &created); err != nil {
		t.Fatalf("response parse: %v", err)
	}

	r, code = get(t, "/v1/conversations", token)
	if code != 200 || !r.Success {
		t.Fatalf("GetConversations başarısız (code=%d): %s", code, r.Error)
	}
	var convs []convWithMlsGroupID
	if err := json.Unmarshal(r.Data, &convs); err != nil {
		t.Fatalf("conversations parse: %v", err)
	}
	c := findConv(t, convs, created.ConvID)
	if c.MLSGroupID != "" {
		t.Errorf("mls_group_id gönderilmediği halde dolu döndü: %q", c.MLSGroupID)
	}
}
