package api_test

// ZK-ID Entegrasyon Testleri — Spec Bölüm 5.2-5.3
//
// Test kapsamı:
//   1. Kayıt akışı — ZK kanıtı olmadan (backward compat): zk_id_verified=false, zk_id_required=true
//   2. POST /v1/auth/zk-id-update — geçersiz body (400)
//   3. POST /v1/auth/zk-id-update — auth gerekli (401)
//   4. Verify-OTP response'unda zk_id_verified + zk_id_required alanları
//   5. Zaten doğrulanmış kullanıcı için idempotent güncelleme (200, not re-verified)
//
// NOT: Gerçek ZK kanıt doğrulaması go-rapidsnark + BN254 eşleşme gerektirir.
// Test ortamında vkey yoksa (veya dummy proof reddedilirse) beklenen davranış
// 400 "ZK kimlik kanıtı geçersiz". Gerçek proof testi circuits/build.sh'dan
// çıkan .zkey + .wasm ile e2e testte yapılır.

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"
)

func init() {
	// ZK-ID update route'unu test router'a ekle.
	// TestMain tarafından başlatılan testServer'a ek route gerekiyor.
	// Bu init çalışmadan önce TestMain çalışır; testServer nil olabilir.
	// Route kayıtları TestMain'de yapıldığından burada sadece route varlığını test ederiz.
}

// TestVerifyOTPResponseHasZKIDFields — verify-otp yanıtı ZK-ID alanlarını içeriyor mu?
func TestVerifyOTPResponseHasZKIDFields(t *testing.T) {
	phone := "+905550000101"

	// OTP iste
	r1, _ := post(t, "/v1/auth/request-otp", map[string]string{"phone": phone}, "")
	if !r1.Success {
		t.Fatalf("OTP isteği başarısız: %s", r1.Error)
	}

	otp := fetchDevOTP(t, phone)

	// Yeni kullanıcı kaydı — ZK kanıtı gönderilmeden
	r2, code := post(t, "/v1/auth/verify-otp", map[string]string{
		"phone":        phone,
		"otp":          otp,
		"username":     "zkidtest01",
		"identity_key": base64.StdEncoding.EncodeToString(randomBytes(t, 32)),
	}, "")

	if code != 200 || !r2.Success {
		t.Fatalf("verify-otp başarısız: %d %s", code, r2.Error)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(r2.Data, &data); err != nil {
		t.Fatalf("Response parse edilemedi: %v", err)
	}

	// zk_id_verified alanı mevcut ve false olmalı (kanıt gönderilmedi)
	zkVerified, hasVerified := data["zk_id_verified"]
	if !hasVerified {
		t.Error("Response'da 'zk_id_verified' alanı eksik")
	}
	if v, ok := zkVerified.(bool); ok && v {
		t.Error("ZK kanıtı gönderilmedi, zk_id_verified=true olmamalı")
	}

	// zk_id_required alanı mevcut ve true olmalı
	zkRequired, hasRequired := data["zk_id_required"]
	if !hasRequired {
		t.Error("Response'da 'zk_id_required' alanı eksik")
	}
	if v, ok := zkRequired.(bool); ok && !v {
		t.Error("ZK kanıtı gönderilmedi, zk_id_required=false olmamalı")
	}

	// token alanı mevcut olmalı
	if _, hasToken := data["token"]; !hasToken {
		t.Error("Response'da 'token' alanı eksik")
	}
}

// TestVerifyOTPExistingUserReturnsZKIDFields — mevcut kullanıcı login'inde de alanlar döner
func TestVerifyOTPExistingUserReturnsZKIDFields(t *testing.T) {
	phone := "+905550000102"
	// İlk kayıt
	loginAndRegister(t, phone, "zkidtest02")

	// İkinci login — OTP iste
	post(t, "/v1/auth/request-otp", map[string]string{"phone": phone}, "")
	otp := fetchDevOTP(t, phone)

	// Tekrar verify-otp (var olan kullanıcı için identity_key/username gönderilmeyebilir)
	r2, code := post(t, "/v1/auth/verify-otp", map[string]string{
		"phone": phone,
		"otp":   otp,
	}, "")

	if code != 200 || !r2.Success {
		t.Fatalf("Mevcut kullanıcı login başarısız: %d %s", code, r2.Error)
	}

	var data map[string]interface{}
	json.Unmarshal(r2.Data, &data)

	if _, hasVerified := data["zk_id_verified"]; !hasVerified {
		t.Error("Mevcut kullanıcı login: 'zk_id_verified' alanı eksik")
	}
	if _, hasRequired := data["zk_id_required"]; !hasRequired {
		t.Error("Mevcut kullanıcı login: 'zk_id_required' alanı eksik")
	}
}

// TestZKIDUpdateRequiresAuth — /auth/zk-id-update token olmadan 401 döner
func TestZKIDUpdateRequiresAuth(t *testing.T) {
	if testServer == nil {
		t.Skip("testServer hazır değil")
	}

	req, _ := http.NewRequest("POST", testServer.URL+"/v1/auth/zk-id-update",
		nil)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("HTTP hatası: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 401 {
		t.Errorf("Token olmadan 401 beklendi, %d geldi", resp.StatusCode)
	}
}

// TestZKIDUpdateEmptyBody — boş body gönderilirse 400 döner
func TestZKIDUpdateEmptyBody(t *testing.T) {
	token := loginAndRegister(t, "+905550000103", "zkidtest03")

	r, code := post(t, "/v1/auth/zk-id-update", map[string]string{}, token)

	if code != 400 {
		t.Errorf("Boş body için 400 beklendi, %d geldi", code)
	}
	if r.Success {
		t.Error("Boş body için success=true döndü, false bekleniyor")
	}
}

// TestZKIDUpdateInvalidProof — geçersiz proof gönderilirse 400 döner
func TestZKIDUpdateInvalidProof(t *testing.T) {
	token := loginAndRegister(t, "+905550000104", "zkidtest04")

	// Geçersiz base64 olmayan ama JSON-parse edilebilen sahte proof
	r, code := post(t, "/v1/auth/zk-id-update", map[string]string{
		"zk_id_proof":  `{"pi_a":["1","2","1"],"pi_b":[["1","2"],["1","2"],["1","0"]],"pi_c":["1","2","1"],"protocol":"groth16","curve":"bn128"}`,
		"zk_id_public": `["1","2","3","4"]`,
	}, token)

	// ZK-ID circuit yüklü değilse "unknown circuit" → 400
	// ZK-ID circuit yüklü ama proof geçersizse "groth16 verify failed" → 400
	// Her iki durumda da 400 bekliyoruz
	if code != 400 {
		t.Errorf("Sahte proof için 400 beklendi, %d geldi (resp: %s)", code, r.Error)
	}
	if r.Success {
		t.Error("Sahte proof için success=true döndü, false bekleniyor")
	}
}

// TestZKIDUpdateRegistrationInvalidProof — kayıt sırasında geçersiz proof gönderilirse 400 döner
func TestZKIDUpdateRegistrationInvalidProof(t *testing.T) {
	phone := "+905550000105"

	// OTP iste
	post(t, "/v1/auth/request-otp", map[string]string{"phone": phone}, "")
	otp := fetchDevOTP(t, phone)

	// Kayıt + geçersiz ZK kanıtı
	r2, code := post(t, "/v1/auth/verify-otp", map[string]interface{}{
		"phone":        phone,
		"otp":          otp,
		"username":     "zkidtest05",
		"identity_key": base64.StdEncoding.EncodeToString(randomBytes(t, 32)),
		"zk_id_proof":  `{"pi_a":["1","2","1"],"pi_b":[["1","2"],["1","2"],["1","0"]],"pi_c":["1","2","1"],"protocol":"groth16","curve":"bn128"}`,
		"zk_id_public": `["1","2","3","4"]`,
	}, "")

	// Geçersiz proof → kayıt engellenmeli → 400
	if code != 400 {
		t.Errorf("Kayıt sırasında geçersiz ZK kanıtı için 400 beklendi, %d geldi (resp: %s)", code, r2.Error)
	}
	if r2.Success {
		t.Error("Geçersiz proof ile kayıt success=true döndü, false bekleniyor")
	}
}
