package api_test

// COMMIT C — auth.checkOTPPhoneRateLimit (auth/auth.go) telefon-bazlı OTP
// istek limiti: api/handlers.go:59 checkOTPRateLimit'in (IP-bazlı, dakikada 5)
// emsali, aynı IP-havuzunu paylaşmayan bir saldırganın TEK bir telefon
// numarasına sınırsız OTP göndermesini (SMS-bombing) engeller. Bu dosya
// davranışı gerçek HTTP akışından (POST /v1/auth/request-otp) ve doğrudan
// auth.GenerateOTP çağrısından doğrular — derleme kanıtı değil.

import (
	"testing"
	"time"

	"obscura.network/core/internal/auth"
	"obscura.network/core/internal/db"
)

// TestRequestOTP_PhoneCooldown_SecondImmediateRequestRejected — aynı telefona
// 60sn içinde 2. istek 429 dönmeli.
func TestRequestOTP_PhoneCooldown_SecondImmediateRequestRejected(t *testing.T) {
	phone := "+905559998001"

	r1, code1 := post(t, "/v1/auth/request-otp", map[string]string{"phone": phone}, "")
	if code1 != 200 || !r1.Success {
		t.Fatalf("ilk istek başarısız olmamalıydı: %d %s", code1, r1.Error)
	}

	r2, code2 := post(t, "/v1/auth/request-otp", map[string]string{"phone": phone}, "")
	if code2 != 429 {
		t.Fatalf("cooldown içinde 2. istek 429 dönmeliydi, got %d (success=%v err=%q)", code2, r2.Success, r2.Error)
	}
}

// TestRequestOTP_PhoneCooldown_DifferentPhonesNotAffected — cooldown telefon-
// bazlı: farklı bir numara aynı anda etkilenmemeli.
func TestRequestOTP_PhoneCooldown_DifferentPhonesNotAffected(t *testing.T) {
	phoneA := "+905559998002"
	phoneB := "+905559998003"

	r1, code1 := post(t, "/v1/auth/request-otp", map[string]string{"phone": phoneA}, "")
	if code1 != 200 || !r1.Success {
		t.Fatalf("phoneA ilk istek başarısız olmamalıydı: %d %s", code1, r1.Error)
	}

	r2, code2 := post(t, "/v1/auth/request-otp", map[string]string{"phone": phoneB}, "")
	if code2 != 200 || !r2.Success {
		t.Fatalf("phoneB (farklı numara) cooldown'dan etkilenmemeliydi: %d %s", code2, r2.Error)
	}
}

// backdateLastOTPRequest — otp_request_log'daki phone için EN SON satırı
// 60sn cooldown penceresinin dışına (ama 24sn günlük tavan penceresinin
// içine) öteler. Günlük tavanı gerçek 10x60sn (10dk) beklemeden test etmenin
// yolu — satır yoksa (ilk istek) no-op, hata değil.
func backdateLastOTPRequest(t *testing.T, phone string) {
	t.Helper()
	if _, err := db.DB.Exec(
		`UPDATE otp_request_log SET requested_at = ?
		 WHERE phone = ? AND requested_at = (SELECT MAX(requested_at) FROM otp_request_log WHERE phone = ?)`,
		time.Now().Add(-2*time.Minute).Format(time.RFC3339), phone, phone,
	); err != nil {
		t.Fatalf("backdateLastOTPRequest: %v", err)
	}
}

// TestRequestOTP_DailyCap_EleventhRequestRejected — otpPhoneDailyCap=10:
// aynı telefona 11. istek reddedilmeli. Cooldown'u atlamak için her çağrı
// öncesi son kaydı geriye öteliyoruz (gerçek 10dk beklemeden) — auth.GenerateOTP
// doğrudan çağrılıyor (bu paket api_test, HTTP'ye gerek yok, davranışın
// kendisi auth katmanında).
func TestRequestOTP_DailyCap_EleventhRequestRejected(t *testing.T) {
	phone := "+905559998004"

	for i := 0; i < 10; i++ {
		backdateLastOTPRequest(t, phone)
		if _, err := auth.GenerateOTP(phone); err != nil {
			t.Fatalf("istek %d başarısız olmamalıydı: %v", i+1, err)
		}
	}

	backdateLastOTPRequest(t, phone)
	if _, err := auth.GenerateOTP(phone); err != auth.ErrOTPDailyCapExceeded {
		t.Fatalf("11. istek ErrOTPDailyCapExceeded dönmeliydi, got: %v", err)
	}
}
