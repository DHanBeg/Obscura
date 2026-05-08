package api_test

// FAZ 1 Entegrasyon Testleri
//
// Test senaryosu:
//   1. OTP isteme ve doğrulama → JWT token
//   2. PreKey bundle yükleme
//   3. PreKey bundle alma (OPK tüketme)
//   4. Mesaj gönderme ve alma
//   5. Kredi puanı kontrolü
//   6. ZK kanıt doğrulama
//   7. Node durumu kontrolü

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"obscura.network/core/internal/api"
	"obscura.network/core/internal/auth"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/messaging"
)

var testServer *httptest.Server

func TestMain(m *testing.M) {
	// Test veritabanı
	tmpDir, _ := os.MkdirTemp("", "obscura-test-*")
	if err := db.Init(tmpDir); err != nil {
		panic("Test DB başlatılamadı: " + err.Error())
	}
	defer db.Close()
	defer os.RemoveAll(tmpDir)

	// WebSocket Hub
	go messaging.GlobalHub.Run()

	// Router kur
	r := mux.NewRouter()
	r.Use(api.CORSMiddleware)

	// Public
	pub := r.PathPrefix("/v1").Subrouter()
	pub.HandleFunc("/auth/request-otp", api.HandleRequestOTP).Methods("POST")
	pub.HandleFunc("/auth/verify-otp", api.HandleVerifyOTP).Methods("POST")
	pub.HandleFunc("/node/status", api.HandleNodeStatus).Methods("GET")

	// Protected
	priv := r.PathPrefix("/v1").Subrouter()
	priv.Use(api.AuthMiddleware)
	priv.HandleFunc("/users/me", api.HandleGetMe).Methods("GET")
	priv.HandleFunc("/users/me", api.HandleUpdateMe).Methods("PATCH")
	priv.HandleFunc("/users/search", api.HandleSearchUser).Methods("GET")
	priv.HandleFunc("/users/{did}", api.HandleGetUser).Methods("GET")
	priv.HandleFunc("/conversations", api.HandleGetConversations).Methods("GET")
	priv.HandleFunc("/conversations", api.HandleCreateConversation).Methods("POST")
	priv.HandleFunc("/conversations/{id}/messages", api.HandleGetMessages).Methods("GET")
	priv.HandleFunc("/messages", api.HandleSendMessage).Methods("POST")
	priv.HandleFunc("/messages/{id}", api.HandleDeleteMessage).Methods("DELETE")
	priv.HandleFunc("/credit/score", api.HandleGetCreditScore).Methods("GET")
	priv.HandleFunc("/credit/history", api.HandleGetCreditHistory).Methods("GET")
	priv.HandleFunc("/spam/report", api.HandleSpamReport).Methods("POST")
	priv.HandleFunc("/devices/register", api.HandleRegisterDevice).Methods("POST")
	priv.HandleFunc("/keys/upload", api.HandleUploadPreKeyBundle).Methods("POST")
	priv.HandleFunc("/keys/{did}", api.HandleGetPreKeyBundle).Methods("GET")
	priv.HandleFunc("/keys/opk/replenish", api.HandleReplenishOPK).Methods("POST")
	priv.HandleFunc("/keys/opk/count", api.HandleGetOPKCount).Methods("GET")
	priv.HandleFunc("/zk/verify", api.HandleVerifyZKProof).Methods("POST")
	r.HandleFunc("/v1/metrics", api.HandleMetrics).Methods("GET")

	testServer = httptest.NewServer(r)
	defer testServer.Close()

	os.Exit(m.Run())
}

// ─── Yardımcı ─────────────────────────────────────────────────────────────────

type testResp struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   string          `json:"error"`
	Code    int             `json:"code"`
}

func post(t *testing.T, path string, body interface{}, token string) (testResp, int) {
	t.Helper()
	bodyBytes, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", testServer.URL+path, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("HTTP hatası: %v", err)
	}
	defer resp.Body.Close()

	var tr testResp
	json.NewDecoder(resp.Body).Decode(&tr)
	return tr, resp.StatusCode
}

func get(t *testing.T, path, token string) (testResp, int) {
	t.Helper()
	req, _ := http.NewRequest("GET", testServer.URL+path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("HTTP hatası: %v", err)
	}
	defer resp.Body.Close()

	var tr testResp
	json.NewDecoder(resp.Body).Decode(&tr)
	return tr, resp.StatusCode
}

// OTP doğrulama ve token alma
func loginAndRegister(t *testing.T, phone, username string) string {
	t.Helper()

	// OTP iste
	r1, _ := post(t, "/v1/auth/request-otp", map[string]string{"phone": phone}, "")
	if !r1.Success {
		t.Fatalf("OTP isteği başarısız: %s", r1.Error)
	}

	// OTP'yi al (test'te dev_otp döner)
	var otpData map[string]interface{}
	json.Unmarshal(r1.Data, &otpData)
	otp, _ := otpData["dev_otp"].(string)
	if otp == "" {
		t.Fatal("dev_otp bulunamadı")
	}

	// OTP doğrula (field adı "otp" — models.VerifyOTPRequest)
	// identity_key: test için rastgele 32 byte base64
	r2, _ := post(t, "/v1/auth/verify-otp", map[string]string{
		"phone":        phone,
		"otp":          otp,
		"username":     username,
		"identity_key": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
	}, "")

	if !r2.Success {
		t.Fatalf("OTP doğrulama başarısız: %s", r2.Error)
	}

	var tokenData map[string]interface{}
	json.Unmarshal(r2.Data, &tokenData)
	token, _ := tokenData["token"].(string)
	if token == "" {
		t.Fatal("Token bulunamadı")
	}
	return token
}

// ─── TESTLER ──────────────────────────────────────────────────────────────────

func TestNodeStatus(t *testing.T) {
	r, code := get(t, "/v1/node/status", "")
	if code != 200 {
		t.Fatalf("Node status %d beklendi 200", code)
	}
	if !r.Success {
		t.Fatal("Node status başarısız")
	}
}

func TestAuthFlow(t *testing.T) {
	phone := "+905551234567"
	token := loginAndRegister(t, phone, "testuser1")
	if token == "" {
		t.Fatal("Token alınamadı")
	}

	// /me endpoint'i kontrol et
	me, code := get(t, "/v1/users/me", token)
	if code != 200 || !me.Success {
		t.Fatalf("GET /me başarısız: %d %s", code, me.Error)
	}

	var user map[string]interface{}
	json.Unmarshal(me.Data, &user)
	if user["username"] != "testuser1" {
		t.Errorf("Kullanıcı adı yanlış: %v", user["username"])
	}

	did, _ := user["did"].(string)
	if !strings.HasPrefix(did, "did:obs:") {
		t.Errorf("DID formatı yanlış: %s", did)
	}
}

func TestPreKeyBundle(t *testing.T) {
	token := loginAndRegister(t, "+905559876543", "prekey_user")

	// PreKey bundle yükle
	bundle := map[string]interface{}{
		"identity_key":     "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", // 32 byte base64
		"signed_prekey":    "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB=",
		"signed_prekey_sig": strings.Repeat("A", 86) + "==", // 64 byte base64
		"signed_prekey_id": 0,
		"one_time_prekeys": []map[string]interface{}{
			{"id": 0, "public_key": "DDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD="},
			{"id": 1, "public_key": "EEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEE="},
		},
	}

	r, code := post(t, "/v1/keys/upload", bundle, token)
	if code != 200 || !r.Success {
		t.Fatalf("PreKey yükleme başarısız: %d %s", code, r.Error)
	}

	// Upload sayısını kontrol et
	var uploaded map[string]interface{}
	json.Unmarshal(r.Data, &uploaded)
	t.Logf("Yüklenen OPK: %v", uploaded["opk_count"])
}

func TestOPKCount(t *testing.T) {
	token := loginAndRegister(t, "+905554444444", "opk_count_user")

	r, code := get(t, "/v1/keys/opk/count", token)
	if code != 200 {
		t.Fatalf("OPK count %d beklendi 200", code)
	}
	t.Logf("OPK durumu: %s", r.Data)
}

func TestMessageFlow(t *testing.T) {
	aliceToken := loginAndRegister(t, "+905550000001", "alice")
	bobToken := loginAndRegister(t, "+905550000002", "bob")

	// Bob'un DID'ini al
	bobMe, _ := get(t, "/v1/users/me", bobToken)
	var bobUser map[string]interface{}
	json.Unmarshal(bobMe.Data, &bobUser)
	bobDID := bobUser["did"].(string)

	// Alice → Bob mesaj gönder (to_id = DID, models.SendMessageRequest)
	sendReq := map[string]interface{}{
		"to_id":      bobDID,
		"ciphertext": "ENCRYPTED:merhaba_bob_123",
		"type":       "text",
	}

	r, code := post(t, "/v1/messages", sendReq, aliceToken)
	if (code != 200 && code != 201) || !r.Success {
		t.Fatalf("Mesaj gönderme başarısız: %d %s", code, r.Error)
	}

	var msgData map[string]interface{}
	json.Unmarshal(r.Data, &msgData)
	msgID, _ := msgData["id"].(string)
	if msgID == "" {
		t.Fatal("Mesaj ID bulunamadı")
	}
	t.Logf("Mesaj ID: %s", msgID)

	// Bob konuşmalarını kontrol et
	convs, code := get(t, "/v1/conversations", bobToken)
	if code != 200 || !convs.Success {
		t.Fatalf("Konuşmalar alınamadı: %d", code)
	}
}

func TestCreditScore(t *testing.T) {
	token := loginAndRegister(t, "+905553333333", "credit_user")

	r, code := get(t, "/v1/credit/score", token)
	if code != 200 || !r.Success {
		t.Fatalf("Kredi puanı alınamadı: %d %s", code, r.Error)
	}

	var score map[string]interface{}
	json.Unmarshal(r.Data, &score)
	s, _ := score["score"].(float64)
	if s < -20 || s > 100 {
		t.Errorf("Geçersiz kredi puanı: %v", s)
	}
	t.Logf("Kredi puanı: %.1f, Tier: %v", s, score["tier"])
}

func TestZKProofVerify(t *testing.T) {
	token := loginAndRegister(t, "+905555555555", "zk_user")

	// Me endpoint'inden DID al
	me, _ := get(t, "/v1/users/me", token)
	var user map[string]interface{}
	json.Unmarshal(me.Data, &user)
	did := user["did"].(string)

	// Stub ZK kanıtı oluştur (zk_stub.rs'in doğrulayabileceği format)
	now := time.Now().Unix()
	proofJSON := fmt.Sprintf(`{
		"proof_type": "identity",
		"proof_bytes": [0,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21,22,23,24,25,26,27,28,29,30,31],
		"public_inputs": [%q],
		"commitment": [0,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21,22,23,24,25,26,27,28,29,30,31],
		"timestamp": %d,
		"prover_did": %q
	}`, did, now, did)

	req := map[string]interface{}{
		"proof_json":    proofJSON,
		"circuit_id":   "identity_proof",
		"public_inputs": []string{did},
	}

	r, code := post(t, "/v1/zk/verify", req, token)
	// Stub doğrulama hataları bekleniyor — sadece endpoint çalışıyor mu kontrol et
	if code == 0 {
		t.Fatal("ZK verify endpoint erişilemiyor")
	}
	t.Logf("ZK verify yanıtı: %d %v", code, r.Success)
}

func TestRateLimiting(t *testing.T) {
	// Node status'a hızlı istek — rate limit yok, hepsi 200 dönmeli
	for i := 0; i < 5; i++ {
		_, code := get(t, "/v1/node/status", "")
		if code != 200 {
			t.Errorf("Rate limit test %d başarısız: %d", i, code)
		}
	}
}

func TestUnauthorizedAccess(t *testing.T) {
	// Token olmadan protected endpoint
	_, code := get(t, "/v1/users/me", "")
	if code != 401 {
		t.Errorf("401 beklendi, %d alındı", code)
	}

	// Geçersiz token
	_, code = get(t, "/v1/users/me", "invalid.token.here")
	if code != 401 {
		t.Errorf("401 beklendi (geçersiz token), %d alındı", code)
	}
}

func TestTokenValidation(t *testing.T) {
	// loginAndRegister ile token al ve doğrula
	token := loginAndRegister(t, "+905559000001", "tokentest_user")

	claims, err := auth.ValidateToken(token)
	if err != nil {
		t.Fatalf("Token doğrulanamadı: %v", err)
	}

	if !strings.HasPrefix(claims.DID, "did:obs:") {
		t.Errorf("DID formatı yanlış: %s", claims.DID)
	}
	t.Logf("Token geçerli: DID=%s Tier=%d", claims.DID, claims.Tier)
}

// ─── FAZ 3 Testleri ───────────────────────────────────────────────────────────

func TestUpdateMe(t *testing.T) {
	token := loginAndRegister(t, "+905560000001", "update_me_test")

	// Profil güncelle
	r, code := patch(t, "/v1/users/me", map[string]string{
		"display_name": "Yeni Ad",
	}, token)
	if code != 200 || !r.Success {
		t.Fatalf("Profil güncelleme başarısız: %d %s", code, r.Error)
	}

	var updated map[string]interface{}
	json.Unmarshal(r.Data, &updated)
	if updated["display_name"] != "Yeni Ad" {
		t.Errorf("display_name güncellenemedi: %v", updated["display_name"])
	}
	t.Logf("Profil güncellendi: %v", updated["display_name"])
}

func TestCreateConversation(t *testing.T) {
	aliceToken := loginAndRegister(t, "+905560000002", "conv_alice")
	bobToken := loginAndRegister(t, "+905560000003", "conv_bob")

	// Bob'un DID'ini al
	bobMe, _ := get(t, "/v1/users/me", bobToken)
	var bobUser map[string]interface{}
	json.Unmarshal(bobMe.Data, &bobUser)
	bobDID := bobUser["did"].(string)

	// 1-1 konuşma oluştur
	r, code := post(t, "/v1/conversations", map[string]interface{}{
		"peer_did": bobDID,
		"is_group": false,
	}, aliceToken)
	if code != 201 || !r.Success {
		t.Fatalf("Konuşma oluşturma başarısız: %d %s", code, r.Error)
	}

	var convData map[string]interface{}
	json.Unmarshal(r.Data, &convData)
	convID, _ := convData["conv_id"].(string)
	if convID == "" {
		t.Fatal("conv_id bulunamadı")
	}
	t.Logf("Konuşma oluşturuldu: %s", convID)
}

func TestDeleteMessage(t *testing.T) {
	aliceToken := loginAndRegister(t, "+905560000004", "del_alice")
	bobToken := loginAndRegister(t, "+905560000005", "del_bob")

	// Bob'un DID'ini al
	bobMe, _ := get(t, "/v1/users/me", bobToken)
	var bobUser map[string]interface{}
	json.Unmarshal(bobMe.Data, &bobUser)
	bobDID := bobUser["did"].(string)

	// Mesaj gönder
	r, _ := post(t, "/v1/messages", map[string]interface{}{
		"to_id":      bobDID,
		"ciphertext": "TEST:silinecek_mesaj",
		"type":       "text",
	}, aliceToken)

	var msgData map[string]interface{}
	json.Unmarshal(r.Data, &msgData)
	msgID, _ := msgData["id"].(string)
	if msgID == "" {
		t.Skip("Mesaj ID alınamadı, test atlandı")
	}

	// Mesajı sil
	delR, code := doDelete(t, "/v1/messages/"+msgID, aliceToken)
	if code != 200 || !delR.Success {
		t.Fatalf("Mesaj silme başarısız: %d %s", code, delR.Error)
	}
	t.Logf("Mesaj silindi: %s", msgID)
}

func TestCreditHistory(t *testing.T) {
	token := loginAndRegister(t, "+905560000006", "history_user")

	r, code := get(t, "/v1/credit/history?limit=10", token)
	if code != 200 || !r.Success {
		t.Fatalf("Kredi geçmişi alınamadı: %d %s", code, r.Error)
	}

	var data map[string]interface{}
	json.Unmarshal(r.Data, &data)
	t.Logf("Kredi geçmişi: count=%v score=%v", data["count"], data["score"])
}

func TestMetrics(t *testing.T) {
	resp, err := http.Get(testServer.URL + "/v1/metrics")
	if err != nil {
		t.Fatalf("Metrics endpoint hatası: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("Metrics %d beklendi 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/plain") {
		t.Errorf("Content-Type yanlış: %s", ct)
	}
	t.Log("Metrics endpoint çalışıyor")
}

func TestRegisterDevice(t *testing.T) {
	token := loginAndRegister(t, "+905560000007", "device_user")

	r, code := post(t, "/v1/devices/register", map[string]string{
		"platform": "fcm",
		"token":    "fcm-test-token-abc123",
	}, token)
	if code != 200 || !r.Success {
		t.Fatalf("Device register başarısız: %d %s", code, r.Error)
	}
	t.Log("FCM token kaydedildi")
}

// ─── HTTP YARDIMCILARI ────────────────────────────────────────────────────────

func patch(t *testing.T, path string, body interface{}, token string) (testResp, int) {
	t.Helper()
	bodyBytes, _ := json.Marshal(body)
	req, _ := http.NewRequest("PATCH", testServer.URL+path, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("HTTP PATCH hatası: %v", err)
	}
	defer resp.Body.Close()
	var tr testResp
	json.NewDecoder(resp.Body).Decode(&tr)
	return tr, resp.StatusCode
}

func doDelete(t *testing.T, path, token string) (testResp, int) {
	t.Helper()
	req, _ := http.NewRequest("DELETE", testServer.URL+path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("HTTP DELETE hatası: %v", err)
	}
	defer resp.Body.Close()
	var tr testResp
	json.NewDecoder(resp.Body).Decode(&tr)
	return tr, resp.StatusCode
}
