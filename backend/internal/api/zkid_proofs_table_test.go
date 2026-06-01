package api_test

// zk_proofs tablosu insert testleri — ZK-ID kayıt akışı.
//
// Spec Ek B: ZK proof'lar zk_proofs tablosunda indexlenir.
// Kayıt (verify-otp) ve güncelleme (zk-id-update) sırasında
// kanıt gönderilmişse zk_proofs satırı oluşturulmalı.
//
// Bu testler DB doğrudan okur — integration_test.go'daki testServer'ı kullanır.

import (
	"encoding/json"
	"testing"

	"obscura.network/core/internal/db"
)

// TestVerifyOTPWithoutZKIDDoesNotInsertZKProofs — ZK kanıtı olmadan kayıt:
// zk_proofs tablosuna identity_proof satırı eklenmemeli.
func TestVerifyOTPWithoutZKIDDoesNotInsertZKProofs(t *testing.T) {
	phone := "+905559000201"

	// Kayıt — ZK kanıtı yok
	r1, _ := post(t, "/v1/auth/request-otp", map[string]string{"phone": phone}, "")
	if !r1.Success {
		t.Fatalf("OTP isteği başarısız: %s", r1.Error)
	}
	var otpData map[string]interface{}
	json.Unmarshal(r1.Data, &otpData)
	otp, _ := otpData["dev_otp"].(string)

	r2, code := post(t, "/v1/auth/verify-otp", map[string]string{
		"phone":        phone,
		"otp":          otp,
		"username":     "nozk_user01",
		"identity_key": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
	}, "")
	if code != 200 || !r2.Success {
		t.Fatalf("verify-otp başarısız: %d %s", code, r2.Error)
	}

	// Kullanıcının DID'ini al
	var data map[string]interface{}
	json.Unmarshal(r2.Data, &data)
	userMap, _ := data["user"].(map[string]interface{})
	did, _ := userMap["did"].(string)
	if did == "" {
		t.Fatal("DID alınamadı")
	}

	// zk_proofs tablosunda bu kullanıcıya ait identity_proof satırı OLMAMALI
	var count int
	err := db.DB.QueryRow(
		`SELECT COUNT(*) FROM zk_proofs WHERE user_did = ? AND circuit_id = 'identity_proof'`,
		did,
	).Scan(&count)
	if err != nil {
		t.Fatalf("DB sorgusu başarısız: %v", err)
	}
	if count != 0 {
		t.Errorf("ZK kanıtı gönderilmedi ama zk_proofs satırı oluştu (count=%d)", count)
	}
}

// TestZKIDUpdateInvalidProofDoesNotInsertZKProofs — geçersiz proof:
// zk_proofs tablosuna satır eklenmemeli (400 döner ve işlem rollback olur).
func TestZKIDUpdateInvalidProofDoesNotInsertZKProofs(t *testing.T) {
	phone := "+905559000202"
	token := loginAndRegister(t, phone, "nozk_user02")

	// Kullanıcı DID'ini al
	r, code := get(t, "/v1/users/me", token)
	if code != 200 {
		t.Fatalf("GET /users/me başarısız: %d", code)
	}
	var user map[string]interface{}
	json.Unmarshal(r.Data, &user)
	did, _ := user["did"].(string)

	// Geçersiz proof ile zk-id-update
	before := countZKProofs(t, did)

	resp, code2 := post(t, "/v1/auth/zk-id-update", map[string]string{
		"zk_id_proof":  `{"pi_a":["1","2","1"],"pi_b":[["1","2"],["1","2"],["1","0"]],"pi_c":["1","2","1"],"protocol":"groth16","curve":"bn128"}`,
		"zk_id_public": `["1","2","3","4"]`,
	}, token)

	if code2 != 400 {
		t.Errorf("Geçersiz proof için 400 beklendi, %d geldi (resp=%s)", code2, resp.Error)
	}

	after := countZKProofs(t, did)
	if after != before {
		t.Errorf("Geçersiz proof sonrası zk_proofs sayısı değişti: önceki=%d sonraki=%d", before, after)
	}
}

// countZKProofs — yardımcı: verilen DID için identity_proof satır sayısı.
func countZKProofs(t *testing.T, did string) int {
	t.Helper()
	var count int
	if err := db.DB.QueryRow(
		`SELECT COUNT(*) FROM zk_proofs WHERE user_did = ? AND circuit_id = 'identity_proof'`,
		did,
	).Scan(&count); err != nil {
		t.Fatalf("zk_proofs count sorgusu başarısız (did=%s): %v", did, err)
	}
	return count
}
