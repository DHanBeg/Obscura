package api_test

// group_created kredi kablolaması testleri (Adım A, madde 1) — HandleMLSCreateGroup
// tx.Commit() sonrası credit.AddEvent(EventGroupCreated) tetikliyor. Aynı
// integration_test.go TestMain'in httptest sunucusu + gerçek db.DB'sini
// kullanır (bu dosya değil, integration_test.go route'u ekledi).

import (
	"testing"

	"obscura.network/core/internal/credit"
	"obscura.network/core/internal/db"
)

func TestMLSCreateGroup_AwardsGroupCreatedCredit(t *testing.T) {
	phone := "+905559990905"
	token := loginAndRegister(t, phone, "mls_group_credit_001")
	creatorDID := currentUserDID(t, token)
	setUserCreditScore(t, phone, 10, 1)

	resp, code := post(t, "/v1/mls/group", map[string]interface{}{
		"group_id": "credit-test-group-" + creatorDID,
		"name":     "Kredi Test Grubu",
	}, token)
	if code != 200 || !resp.Success {
		t.Fatalf("grup oluşturulamadı (code=%d): %s", code, resp.Error)
	}

	var delta float64
	err := db.DB.QueryRow(
		`SELECT delta FROM credit_events WHERE user_did = ? AND event_type = ?`,
		creatorDID, credit.EventGroupCreated,
	).Scan(&delta)
	if err != nil {
		t.Fatalf("group_created credit_events satırı bulunamadı: %v", err)
	}
	if delta != credit.EventDeltas[credit.EventGroupCreated] {
		t.Errorf("delta = %v, beklenen %v", delta, credit.EventDeltas[credit.EventGroupCreated])
	}

	var score float64
	if err := db.DB.QueryRow("SELECT credit_score FROM users WHERE did = ?", creatorDID).Scan(&score); err != nil {
		t.Fatalf("credit_score okunamadı: %v", err)
	}
	if score != 10.0+delta {
		t.Errorf("credit_score = %v, beklenen %v", score, 10.0+delta)
	}
}

// TestMLSCreateGroup_DuplicateGroupID_NoDoubleCredit — ON CONFLICT DO NOTHING
// no-op olduğunda (aynı group_id ikinci kez POST edilince) kredi TEKRAR
// verilmemeli (RowsAffected==0 koruması, mls_handlers.go).
func TestMLSCreateGroup_DuplicateGroupID_NoDoubleCredit(t *testing.T) {
	phone := "+905559990906"
	token := loginAndRegister(t, phone, "mls_group_credit_002")
	creatorDID := currentUserDID(t, token)
	setUserCreditScore(t, phone, 10, 1)

	groupID := "dup-credit-test-group-" + creatorDID
	resp1, code1 := post(t, "/v1/mls/group", map[string]interface{}{
		"group_id": groupID,
		"name":     "İlk",
	}, token)
	if code1 != 200 || !resp1.Success {
		t.Fatalf("ilk grup oluşturma başarısız (code=%d): %s", code1, resp1.Error)
	}

	resp2, code2 := post(t, "/v1/mls/group", map[string]interface{}{
		"group_id": groupID,
		"name":     "Tekrar (aynı id)",
	}, token)
	if code2 != 200 || !resp2.Success {
		t.Fatalf("ikinci (duplicate) POST başarısız (code=%d): %s", code2, resp2.Error)
	}

	var n int
	if err := db.DB.QueryRow(
		`SELECT COUNT(*) FROM credit_events WHERE user_did = ? AND event_type = ?`,
		creatorDID, credit.EventGroupCreated,
	).Scan(&n); err != nil {
		t.Fatalf("credit_events sayımı: %v", err)
	}
	if n != 1 {
		t.Errorf("credit_events satır sayısı = %d, beklenen 1 (duplicate group_id kredi çiftlememeli)", n)
	}
}
