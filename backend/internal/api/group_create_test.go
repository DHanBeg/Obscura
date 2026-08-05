package api_test

// POST /v1/conversations — grup oluşturma erişim kapısı testleri.
//
// Spec Bölüm 5.2 Katman 2 ("sağlıklı kullanıcı"): grup oluşturmak için
// models.TierToAccessLevel(tier) >= 2 gerekli. Bronz (credit tier 1,
// puan <60) grup açamaz — spec 7.2: "Katman 1: BRONZ — Grup yok".

import (
	"encoding/json"
	"testing"
	"time"

	"obscura.network/core/internal/db"
)

// setUserCreditScore, verify-otp/loginAndRegister sırasında InitialScore()
// rastgele (20-100) atadığı puanı deterministik bir değere zorlar VE tier'ı
// aynı UPDATE'te tutarlı hesaplar. İkisini birlikte yazmak GEREKLİ AMA
// YETERLİ DEĞİL: verify-otp handler'ı kayıt sonrası "go credit.TrackDailyLogin(...)"
// ile ASENKRON bir +0.5 günlük-giriş olayı tetikliyor (handlers.go:295) —
// bu goroutine SELECT credit_score → hesapla → UPDATE şeklinde, ATOMIK
// DEĞİL. Eğer goroutine'in SELECT'i bizim UPDATE'imizden ÖNCE, ama kendi
// UPDATE'i bizimkinden SONRA çalışırsa, goroutine ESKİ (bizim yazmadan
// önceki rastgele) skordan türettiği değeri yazıp bizim değerimizi
// TAMAMEN İLGİSİZ bir sonuçla ezer — "aynı anda iki alanı yaz" tek başına
// bunu çözmüyor, çünkü sorun alan tutarlılığı değil, stale-read'e dayalı
// geç yazma (gözlemlendi: admin_resolve_test.go'da tier 5/score 90 yazılıp
// birkaç ms sonra tier 1/score ~55 okunuyordu — goroutine'in kendi eski
// okumasından üretilmiş). TrackDailyLogin kullanıcı başına GÜNDE BİR KEZ
// ateşleniyor (daily_activity.login_count kapısı) — bu yüzden "yaz, kısa
// süre bekle, doğrula, tutmadıysa tekrar yaz" döngüsü nihayetinde
// KESİNLİKLE kararlı bir sonuca yakınsıyor (goroutine bir kere ateşlenip
// bitince bir daha dokunmuyor).
func setUserCreditScore(t *testing.T, phone string, score float64, tier int) {
	t.Helper()
	const maxAttempts = 5
	for i := 0; i < maxAttempts; i++ {
		if _, err := db.DB.Exec("UPDATE users SET credit_score = ?, tier = ? WHERE phone = ?", score, tier, phone); err != nil {
			t.Fatalf("credit_score/tier güncellenemedi: %v", err)
		}
		time.Sleep(15 * time.Millisecond)
		var gotScore float64
		var gotTier int
		if err := db.DB.QueryRow("SELECT credit_score, tier FROM users WHERE phone = ?", phone).Scan(&gotScore, &gotTier); err != nil {
			t.Fatalf("credit_score/tier doğrulanamadı: %v", err)
		}
		if gotScore == score && gotTier == tier {
			return
		}
	}
	t.Fatalf("setUserCreditScore: %d denemeden sonra değer kararlı olmadı (async TrackDailyLogin ile sürekli yarış)", maxAttempts)
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
