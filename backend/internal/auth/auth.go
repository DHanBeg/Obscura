package auth

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"log"
	"math/big"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/models"
	"obscura.network/core/internal/sms"
)

func jwtKey() []byte {
	if s := os.Getenv("JWT_SECRET"); s != "" {
		return []byte(s)
	}
	log.Println("[GÜVENLİK UYARISI] JWT_SECRET env değişkeni ayarlanmamış! Üretimde mutlaka güçlü bir secret belirleyin.")
	return []byte("obscura-secret-change-in-production")
}

// ─── OTP ─────────────────────────────────────────────────────────────────────

func GenerateOTP(phone string) (string, error) {
	// 6 haneli OTP üret
	n, err := rand.Int(rand.Reader, big.NewInt(900000))
	if err != nil {
		return "", err
	}
	code := fmt.Sprintf("%06d", n.Int64()+100000)

	// Eski OTP'leri temizle
	db.DB.Exec("DELETE FROM otp_records WHERE phone = ?", phone)

	// Yeni OTP kaydet (5 dakika geçerli)
	id := uuid.New().String()
	expires := time.Now().Add(5 * time.Minute)
	now := time.Now()

	_, err = db.DB.Exec(`
		INSERT INTO otp_records (id, phone, code, attempts, expires_at, used, created_at)
		VALUES (?, ?, ?, 0, ?, 0, ?)`,
		id, phone, code, expires.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		return "", fmt.Errorf("OTP kaydedilemedi: %w", err)
	}

	// SMS gönder — pluggable provider (log / netgsm / vodafone / custom)
	if err := sms.SendOTP(phone, code); err != nil {
		log.Printf("⚠️ SMS gönderilemedi (%s): %v", phone, err)
		// SMS hatası OTP oluşturmayı engellememeli — devam et
	}

	return code, nil
}

func VerifyOTP(phone, code string) error {
	var (
		id        string
		storedCode string
		attempts  int
		expiresAt string
		used      bool
	)

	err := db.DB.QueryRow(`
		SELECT id, code, attempts, expires_at, used
		FROM otp_records
		WHERE phone = ? AND used = 0
		ORDER BY created_at DESC LIMIT 1`,
		phone,
	).Scan(&id, &storedCode, &attempts, &expiresAt, &used)

	if err == sql.ErrNoRows {
		return fmt.Errorf("OTP bulunamadı, lütfen tekrar gönderiniz")
	}
	if err != nil {
		log.Printf("VerifyOTP DB hatası (phone=%s): %v", phone, err)
		return fmt.Errorf("Doğrulama sırasında hata oluştu")
	}

	// Deneme limiti
	if attempts >= 3 {
		return fmt.Errorf("çok fazla hatalı deneme, 15 dakika bekleyiniz")
	}

	// Süre kontrolü
	expires, _ := time.Parse(time.RFC3339, expiresAt)
	if time.Now().After(expires) {
		return fmt.Errorf("OTP süresi dolmuş")
	}

	// Kod kontrolü
	if storedCode != code {
		db.DB.Exec("UPDATE otp_records SET attempts = attempts + 1 WHERE id = ?", id)
		return fmt.Errorf("hatalı OTP kodu")
	}

		// Kullanıldı olarak işaretle
	db.DB.Exec("UPDATE otp_records SET used = 1 WHERE id = ?", id)
	return nil
}

// ValidateOTP — OTP'yi doğrular ama kullanıldı olarak işaretlemez.
// Yeni kullanıcı kayıt akışında, username adımından önce çağrılır.
func ValidateOTP(phone, code string) error {
	var (
		id         string
		storedCode string
		attempts   int
		expiresAt  string
	)

	err := db.DB.QueryRow(`
		SELECT id, code, attempts, expires_at
		FROM otp_records
		WHERE phone = ? AND used = 0
		ORDER BY created_at DESC LIMIT 1`,
		phone,
	).Scan(&id, &storedCode, &attempts, &expiresAt)

	if err == sql.ErrNoRows {
		return fmt.Errorf("OTP bulunamadı, lütfen tekrar gönderiniz")
	}
	if err != nil {
		log.Printf("ValidateOTP DB hatası (phone=%s): %v", phone, err)
		return fmt.Errorf("Doğrulama sırasında hata oluştu")
	}
	if attempts >= 3 {
		return fmt.Errorf("çok fazla hatalı deneme, 15 dakika bekleyiniz")
	}
	expires, _ := time.Parse(time.RFC3339, expiresAt)
	if time.Now().After(expires) {
		return fmt.Errorf("OTP süresi dolmuş")
	}
	if storedCode != code {
		db.DB.Exec("UPDATE otp_records SET attempts = attempts + 1 WHERE id = ?", id)
		return fmt.Errorf("hatalı OTP kodu")
	}
	return nil // geçerli — tüketilmedi
}

// ConsumeOTP — telefona ait tüm geçerli OTP'leri kullanıldı olarak işaretler.
func ConsumeOTP(phone string) {
	db.DB.Exec("UPDATE otp_records SET used = 1 WHERE phone = ? AND used = 0", phone)
}

// ─── JWT ──────────────────────────────────────────────────────────────────────

type Claims struct {
	UserID string `json:"user_id"`
	DID    string `json:"did"`
	Tier   int    `json:"tier"`
	jwt.RegisteredClaims
}

func GenerateToken(user *models.User) (string, error) {
	claims := &Claims{
		UserID: user.ID,
		DID:    user.DID,
		Tier:   user.Tier,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(30 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   user.ID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtKey())
}

func ValidateToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("beklenmeyen imza metodu")
		}
		return jwtKey(), nil
	})

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("geçersiz token")
	}

	return claims, nil
}

// ─── DID ──────────────────────────────────────────────────────────────────────

func GenerateDID(identityKey string) string {
	// Gerçek uygulamada SHA-256(pubkey)
	// Şimdilik UUID bazlı
	id := uuid.New().String()
	return fmt.Sprintf("did:obs:%s", id[:16])
}
