package api

import (
	"net/http"
	"os"

	"obscura.network/core/internal/db"
)

// HandleDevOTP returns the latest unused OTP for a phone number.
// Only available when OBSCURA_ENV=development. Never expose in production.
func HandleDevOTP(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("OBSCURA_ENV") != "development" {
		respond(w, http.StatusNotFound, nil, "not found")
		return
	}

	phone := r.URL.Query().Get("phone")
	if phone == "" {
		respond(w, http.StatusBadRequest, nil, "phone parametresi gerekli")
		return
	}

	var code string
	var expiresAt string
	err := db.DB.QueryRow(`
		SELECT code, expires_at FROM otp_records
		WHERE phone = ? AND used = 0
		ORDER BY created_at DESC LIMIT 1
	`, phone).Scan(&code, &expiresAt)

	if err != nil {
		respond(w, http.StatusNotFound, nil, "OTP bulunamadı — önce /auth/request-otp çağırın")
		return
	}

	respond(w, http.StatusOK, map[string]interface{}{
		"otp":        code,
		"phone":      phone,
		"expires_at": expiresAt,
		"warning":    "DEV ONLY — production'da bu endpoint kapalıdır",
	}, "")
}
