package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/models"
	"obscura.network/core/internal/secrets"
	"obscura.network/core/internal/sms"
)

// jwtKeyBytes — HS256 imzalama anahtarı (env: JWT_SECRET). Eskiden
// os.Getenv fallback'i "CHANGE_THIS_JWT_SECRET_IN_PRODUCTION" placeholder'ını
// reddedip production kontrolünü opt-out yönünde yapıyordu (OBSCURA_ENV=="production"
// DEĞİLSE literal "obscura-secret-change-in-production" repoda-açık string'ine
// düşüyordu — bu string'i bilen herkes geçerli JWT forge edebilirdi). Artık
// secrets.Require ile fail-safe yönde: OBSCURA_ENV açıkça dev opt-in değilse
// prod sayılır, secret eksikse process FATAL olur.
// (C10 fail-open kökü kapatıldı — bkz. internal/secrets.)
var jwtKeyBytes = []byte(secrets.Require("JWT_SECRET"))

// ─── OTP ─────────────────────────────────────────────────────────────────────

var ErrOTPLockedOut = fmt.Errorf("çok fazla hatalı deneme, 15 dakika bekleyiniz")

const otpLockoutDuration = 15 * time.Minute
const otpMaxAttempts = 3

// isOTPLockedOut — otp_records silinip yeni OTP istense bile devam eden,
// otp_records'tan bağımsız kilit. Yoksa 3 yanlış denemeden sonra yeni OTP
// istemek attempts sayacını sıfırlayıp kilidi bypass ederdi.
func isOTPLockedOut(phone string) bool {
	var lockedUntil string
	err := db.DB.QueryRow("SELECT locked_until FROM otp_lockouts WHERE phone = ?", phone).Scan(&lockedUntil)
	if err != nil {
		return false
	}
	t, err := time.Parse(time.RFC3339, lockedUntil)
	if err != nil {
		return false
	}
	return time.Now().Before(t)
}

func lockOTP(phone string) {
	lockedUntil := time.Now().Add(otpLockoutDuration).Format(time.RFC3339)
	db.DB.Exec(`
		INSERT INTO otp_lockouts (phone, locked_until) VALUES (?, ?)
		ON CONFLICT(phone) DO UPDATE SET locked_until = excluded.locked_until`,
		phone, lockedUntil)
}

func clearOTPLockout(phone string) {
	db.DB.Exec("DELETE FROM otp_lockouts WHERE phone = ?", phone)
}

func GenerateOTP(phone string) (string, error) {
	if isOTPLockedOut(phone) {
		return "", ErrOTPLockedOut
	}

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
	if attempts >= otpMaxAttempts {
		lockOTP(phone)
		return ErrOTPLockedOut
	}

	// Süre kontrolü
	expires, _ := time.Parse(time.RFC3339, expiresAt)
	if time.Now().After(expires) {
		return fmt.Errorf("OTP süresi dolmuş")
	}

	// Kod kontrolü
	if storedCode != code {
		db.DB.Exec("UPDATE otp_records SET attempts = attempts + 1 WHERE id = ?", id)
		if attempts+1 >= otpMaxAttempts {
			lockOTP(phone)
			return ErrOTPLockedOut
		}
		return fmt.Errorf("hatalı OTP kodu")
	}

	// Kullanıldı olarak işaretle
	db.DB.Exec("UPDATE otp_records SET used = 1 WHERE id = ?", id)
	clearOTPLockout(phone)
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
	if attempts >= otpMaxAttempts {
		lockOTP(phone)
		return ErrOTPLockedOut
	}
	expires, _ := time.Parse(time.RFC3339, expiresAt)
	if time.Now().After(expires) {
		return fmt.Errorf("OTP süresi dolmuş")
	}
	if storedCode != code {
		db.DB.Exec("UPDATE otp_records SET attempts = attempts + 1 WHERE id = ?", id)
		if attempts+1 >= otpMaxAttempts {
			lockOTP(phone)
			return ErrOTPLockedOut
		}
		return fmt.Errorf("hatalı OTP kodu")
	}
	clearOTPLockout(phone)
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
	return token.SignedString(jwtKeyBytes)
}

func ValidateToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("beklenmeyen imza metodu")
		}
		return jwtKeyBytes, nil
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

// GenerateDID, kullanıcının X25519 identity public key'inden (base64,
// mobile'ın e2e.ts:getIdentityPublicKeyBase64 ile gönderdiği aynı anahtar)
// deterministik bir DID türetir.
//
// Formül, mobile/lib/sealed-sender.ts:didFromDhPublic ile BİREBİR aynı olmak
// ZORUNDA: "did:obs:" + hex(SHA256(pubkey_ham_bayt)[:16]). Bu eşitlik olmazsa
// sealed-sender sertifikasındaki hash-DID ile sunucunun ürettiği DID hiç
// örtüşmez — bu da her mesajda gereksiz X3DH bootstrap'ına ve OPK israfına
// yol açar (bkz. docs/sessions — DID şema uyuşmazlığı bulgusu).
func GenerateDID(identityKey string) string {
	raw, err := base64.StdEncoding.DecodeString(identityKey)
	if err != nil {
		// Base64 çözülemezse (olmaması gereken bir durum — çağıran taraf
		// zaten boş olmadığını doğruladı, mobile her zaman geçerli base64
		// gönderir) panic yerine ham string'i hashle: yine deterministik,
		// sadece mobile ile eşleşmeyen ama en azından çakışmayan bir DID.
		raw = []byte(identityKey)
	}
	sum := sha256.Sum256(raw)
	return "did:obs:" + hex.EncodeToString(sum[:16])
}
