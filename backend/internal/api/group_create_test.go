package api_test

// POST /v1/conversations — grup oluşturma erişim kapısı testleri.
//
// Spec Bölüm 5.2 Katman 2 ("sağlıklı kullanıcı"): grup oluşturmak için
// models.TierToAccessLevel(tier) >= 2 gerekli. Bronz (credit tier 1,
// puan <60) grup açamaz — spec 7.2: "Katman 1: BRONZ — Grup yok".

import (
	"encoding/json"
	"testing"

	"obscura.network/core/internal/db"
)

// setUserCreditScore, verify-otp/loginAndRegister sırasında InitialScore()
// rastgele (20-100) atadığı puanı deterministik bir değere zorlar VE tier'ı
// aynı UPDATE'te tutarlı hesaplar. İkisini birlikte yazmak şart: verify-otp
// handler'ı kayıt sonrası "go credit.TrackDailyLogin(...)" ile ASENKRON bir
// +0.5 günlük-giriş olayı tetikliyor (handlers.go:295) — bu goroutine
// credit_score'u okuyup kendi tier'ını hesaplayıp yazıyor. Sadece tier
// kolonunu zorlarsak, goroutine testten sonra/önce race'e girip eski
// credit_score'dan yanlış tier'a geri döndürebilir (gözlemlendi: flaky).
// Skoru da aynı anda, hedef tier aralığının ortasına sabitleyince +0.5'lik
// olası bindirme sınırı aşmıyor — hangi sırada koşarsa koşsun sonuç tutarlı.
func setUserCreditScore(t *testing.T, phone string, score float64, tier int) {
	t.Helper()
	if _, err := db.DB.Exec("UPDATE users SET credit_score = ?, tier = ? WHERE phone = ?", score, tier, phone); err != nil {
		t.Fatalf("credit_score/tier güncellenemedi: %v", err)
	}
}

// TestCreateGroup_RequiresAccessLevel2 — Bronz (tier 1) kullanıcı grup açamaz (403).
func TestCreateGroup_RequiresAccessLevel2(t *testing.T) {
	phone := "+905559990901"
	token := loginAndRegister(t, phone, "group_bronz_001")
	setUserCreditScore(t, phone, 20, 1) // Bronz — access level 1, GroupCreateAccessLevel (2) altında

	memberToken := loginAndRegister(t, "+905559990902", "group_member_001")
	memberDID := currentUserDID(t, memberToken)

	resp, code := post(t, "/v1/conversations", map[string]interface{}{
		"type":    "group",
		"name":    "Bronz Grubu",
		"members": []string{memberDID},
	}, token)
	if code != 403 {
		t.Fatalf("Beklenen HTTP 403 (Bronz grup açamamalı), alınan %d: %s", code, resp.Error)
	}
}

// TestCreateGroup_HappyPath — Gümüş+ (tier 2) kullanıcı grup açabilir (201).
func TestCreateGroup_HappyPath(t *testing.T) {
	phone := "+905559990903"
	token := loginAndRegister(t, phone, "group_gumus_001")
	setUserCreditScore(t, phone, 65, 2) // Gümüş — access level 2, GroupCreateAccessLevel'e eşit

	memberToken := loginAndRegister(t, "+905559990904", "group_member_002")
	memberDID := currentUserDID(t, memberToken)

	resp, code := post(t, "/v1/conversations", map[string]interface{}{
		"type":    "group",
		"name":    "Gümüş Grubu",
		"members": []string{memberDID},
	}, token)
	if code != 201 || !resp.Success {
		t.Fatalf("Grup oluşturulamadı (code=%d): %s", code, resp.Error)
	}

	var data struct {
		ConvID string `json:"conv_id"`
	}
	json.Unmarshal(resp.Data, &data)
	if data.ConvID == "" {
		t.Fatal("conv_id boş döndü")
	}
}

// currentUserDID, GET /v1/users/me ile token sahibinin DID'ini döner.
func currentUserDID(t *testing.T, token string) string {
	t.Helper()
	r, code := get(t, "/v1/users/me", token)
	if code != 200 || !r.Success {
		t.Fatalf("GET /me başarısız: %d %s", code, r.Error)
	}
	var u struct {
		DID string `json:"did"`
	}
	json.Unmarshal(r.Data, &u)
	if u.DID == "" {
		t.Fatal("DID boş döndü")
	}
	return u.DID
}
