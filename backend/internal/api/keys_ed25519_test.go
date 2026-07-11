package api_test

// Ed25519 SPK imza doğrulama testleri — POST /v1/keys/upload
//
// Mobil client X25519 identity key'in yanında ayrı bir Ed25519 signing
// keypair üretir ve SPK'yı bununla imzalar. Bu testler GERÇEK Ed25519
// keypair ve GERÇEK imza kullanır (mock yok).
//
// NOT: loginAndRegister helper'ı kullanılmıyor çünkü dev_otp response'tan
// kaldırıldı (commit b9b7fe1) ve OTP-tabanlı login akışı test ortamında
// çalışmıyor. Bunun yerine kullanıcı doğrudan DB'ye eklenip JWT
// auth.GenerateToken ile üretiliyor — AuthMiddleware bunu normal token
// gibi doğruluyor.

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"obscura.network/core/internal/auth"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/models"
)

// randomBytes — n byte rastgele veri (32 byte = X25519 public key formatı; sadece boyut önemli)
func randomBytes(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rastgele byte üretilemedi: %v", err)
	}
	return b
}

// createTestUserAndToken — kullanıcıyı doğrudan DB'ye ekler ve geçerli JWT üretir.
// OTP akışını bypass eder (dev_otp artık response'ta dönmüyor).
func createTestUserAndToken(t *testing.T, phone, username string) (token, did string) {
	t.Helper()

	now := time.Now().UTC().Format(time.RFC3339)
	user := models.User{
		ID:          uuid.New().String(),
		Phone:       phone,
		Username:    username,
		DisplayName: username,
		DID:         "did:obs:" + uuid.New().String()[:16],
		IdentityKey: base64.StdEncoding.EncodeToString(randomBytes(t, 32)),
		Tier:        1,
		CreditScore: 50,
		IsActive:    true,
	}

	_, err := db.DB.Exec(`
		INSERT INTO users (id, phone, username, display_name, did, identity_key, avatar_url,
		                   tier, credit_score, is_active, is_banned, node_id,
		                   created_at, updated_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, '', ?, ?, 1, 0, '', ?, ?, ?)`,
		user.ID, user.Phone, user.Username, user.DisplayName, user.DID,
		user.IdentityKey, user.Tier, user.CreditScore, now, now, now,
	)
	if err != nil {
		t.Fatalf("Test kullanıcısı oluşturulamadı: %v", err)
	}

	tok, err := auth.GenerateToken(&user)
	if err != nil {
		t.Fatalf("Token üretilemedi: %v", err)
	}
	return tok, user.DID
}

func TestUploadPreKeyBundleEd25519Valid(t *testing.T) {
	token, did := createTestUserAndToken(t, "+905557770001", "ed25519_user")

	// Gerçek Ed25519 keypair
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Ed25519 keypair üretilemedi: %v", err)
	}

	identityKey := randomBytes(t, 32)
	signedPrekey := randomBytes(t, 32)

	// SPK'yı gerçek Ed25519 imzasıyla imzala
	sig := ed25519.Sign(privKey, signedPrekey)

	signingKeyB64 := base64.StdEncoding.EncodeToString(pubKey)
	signedPrekeyB64 := base64.StdEncoding.EncodeToString(signedPrekey)
	sigB64 := base64.StdEncoding.EncodeToString(sig)

	bundle := map[string]interface{}{
		"identity_key":      base64.StdEncoding.EncodeToString(identityKey),
		"signing_key":       signingKeyB64,
		"signed_prekey":     signedPrekeyB64,
		"signed_prekey_sig": sigB64,
		"signed_prekey_id":  1,
		"one_time_prekeys": []map[string]interface{}{
			{"id": 0, "public_key": base64.StdEncoding.EncodeToString(randomBytes(t, 32))},
		},
	}

	r, code := post(t, "/v1/keys/upload", bundle, token)
	if code != 200 || !r.Success {
		t.Fatalf("Ed25519 imzalı bundle yükleme başarısız: %d %s", code, r.Error)
	}

	var uploaded map[string]interface{}
	json.Unmarshal(r.Data, &uploaded)
	if uploaded["uploaded"] != true {
		t.Errorf("uploaded=true beklendi, alınan: %v", uploaded["uploaded"])
	}

	// DB kaydını doğrula
	var dbSigningKey, dbSignedPrekey, dbSignedPrekeySig string
	err = db.DB.QueryRow(`
		SELECT signing_key, signed_prekey, signed_prekey_sig
		FROM prekey_bundles WHERE did = ?
	`, did).Scan(&dbSigningKey, &dbSignedPrekey, &dbSignedPrekeySig)
	if err != nil {
		t.Fatalf("DB'den bundle okunamadı: %v", err)
	}
	if dbSigningKey != signingKeyB64 {
		t.Errorf("DB signing_key uyuşmuyor: beklenen %s, alınan %s", signingKeyB64, dbSigningKey)
	}
	if dbSignedPrekey != signedPrekeyB64 {
		t.Errorf("DB signed_prekey uyuşmuyor: beklenen %s, alınan %s", signedPrekeyB64, dbSignedPrekey)
	}
	if dbSignedPrekeySig != sigB64 {
		t.Errorf("DB signed_prekey_sig uyuşmuyor: beklenen %s, alınan %s", sigB64, dbSignedPrekeySig)
	}
}

func TestUploadPreKeyBundleEd25519InvalidSig(t *testing.T) {
	token, _ := createTestUserAndToken(t, "+905557770002", "ed25519_bad_user")

	// İki AYRI gerçek keypair: bundle'daki public key ile imzalayan key farklı
	pubKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Ed25519 keypair üretilemedi: %v", err)
	}
	_, otherPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("İkinci Ed25519 keypair üretilemedi: %v", err)
	}

	signedPrekey := randomBytes(t, 32)
	wrongSig := ed25519.Sign(otherPriv, signedPrekey) // yanlış anahtarın imzası

	bundle := map[string]interface{}{
		"identity_key":      base64.StdEncoding.EncodeToString(randomBytes(t, 32)),
		"signing_key":       base64.StdEncoding.EncodeToString(pubKey),
		"signed_prekey":     base64.StdEncoding.EncodeToString(signedPrekey),
		"signed_prekey_sig": base64.StdEncoding.EncodeToString(wrongSig),
		"signed_prekey_id":  1,
	}

	r, code := post(t, "/v1/keys/upload", bundle, token)
	if code != 400 {
		t.Fatalf("Yanlış imza için 400 beklendi, alınan: %d (%s)", code, r.Error)
	}
	if !strings.Contains(r.Error, "doğrulaması başarısız") {
		t.Errorf("Hata mesajında 'doğrulaması başarısız' beklendi, alınan: %s", r.Error)
	}
}

func TestUploadPreKeyBundleInvalidSigningKeyLength(t *testing.T) {
	token, _ := createTestUserAndToken(t, "+905557770003", "ed25519_len_user")

	// 16 byte'lık geçersiz signing_key — ne 32 (Ed25519) ne 65 (P-256)
	bundle := map[string]interface{}{
		"identity_key":      base64.StdEncoding.EncodeToString(randomBytes(t, 32)),
		"signing_key":       base64.StdEncoding.EncodeToString(randomBytes(t, 16)),
		"signed_prekey":     base64.StdEncoding.EncodeToString(randomBytes(t, 32)),
		"signed_prekey_sig": base64.StdEncoding.EncodeToString(randomBytes(t, 64)),
		"signed_prekey_id":  1,
	}

	r, code := post(t, "/v1/keys/upload", bundle, token)
	if code != 400 {
		t.Fatalf("Geçersiz uzunluk için 400 beklendi, alınan: %d (%s)", code, r.Error)
	}
	if !strings.Contains(r.Error, "Geçersiz signing_key") {
		t.Errorf("Hata mesajında 'Geçersiz signing_key' beklendi, alınan: %s", r.Error)
	}
}

// TestUploadPreKeyBundleNoSigningKey — signing_key göndermeyen istemciler
// (mevcut TestPreKeyBundle senaryosu) hâlâ kabul edilmeli; imza doğrulaması
// atlanır. Regresyon koruması: OTP akışı kırık olduğu için TestPreKeyBundle
// çalışamıyor, aynı davranışı burada doğruluyoruz.
func TestUploadPreKeyBundleNoSigningKey(t *testing.T) {
	token, _ := createTestUserAndToken(t, "+905557770004", "no_sk_user")

	bundle := map[string]interface{}{
		"identity_key":      base64.StdEncoding.EncodeToString(randomBytes(t, 32)),
		"signed_prekey":     base64.StdEncoding.EncodeToString(randomBytes(t, 32)),
		"signed_prekey_sig": base64.StdEncoding.EncodeToString(randomBytes(t, 64)),
		"signed_prekey_id":  0,
	}

	r, code := post(t, "/v1/keys/upload", bundle, token)
	if code != 200 || !r.Success {
		t.Fatalf("signing_key'siz bundle yükleme başarısız: %d %s", code, r.Error)
	}
}
