package subscriber

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"os"
)

// keyLen is the required AES-256 key length in bytes.
const keyLen = 32

// aead holds the process-wide AES-256-GCM cipher used for field-level
// encryption. It is set once via InitCrypto / InitCryptoFromEnv and read by
// EncryptField / DecryptField.
var aead cipher.AEAD

// ErrCryptoNotInitialized is returned when Encrypt/DecryptField is called
// before a key has been installed.
var ErrCryptoNotInitialized = errors.New("subscriber: crypto not initialized (call InitCrypto)")

// devKeyMaterial / devPepper are INSECURE fallbacks used only outside
// production, mirroring auth.jwtKey's dev behaviour. Never rely on these in a
// real deployment — they are deterministic and public.
const devKeyMaterial = "obscura-insecure-dev-subscriber-key-change-me"
const devPepper = "obscura-insecure-dev-phone-pepper-change-me"

// InitCrypto installs a 32-byte AES-256 key for field-level encryption. It is
// the explicit-key entry point used by tests and by the env loader below.
func InitCrypto(key []byte) error {
	if len(key) != keyLen {
		return fmt.Errorf("subscriber: key must be %d bytes, got %d", keyLen, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("subscriber: aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("subscriber: gcm: %w", err)
	}
	aead = gcm
	return nil
}

// InitCryptoFromEnv loads the AES key from OBSCURA_SUBSCRIBER_KEY (32 raw bytes,
// base64 STANDARD encoded). Mirrors auth.jwtKey: hard-fail in production when
// missing/malformed, insecure deterministic fallback in dev.
func InitCryptoFromEnv() error {
	if raw := os.Getenv("OBSCURA_SUBSCRIBER_KEY"); raw != "" {
		key, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return fmt.Errorf("subscriber: OBSCURA_SUBSCRIBER_KEY must be base64: %w", err)
		}
		if len(key) != keyLen {
			return fmt.Errorf("subscriber: OBSCURA_SUBSCRIBER_KEY must decode to %d bytes, got %d", keyLen, len(key))
		}
		return InitCrypto(key)
	}

	if os.Getenv("OBSCURA_ENV") == "production" {
		log.Fatal("OBSCURA_SUBSCRIBER_KEY env required in production (32 raw bytes, base64-encoded)")
	}

	log.Println("[GÜVENLİK UYARISI] OBSCURA_SUBSCRIBER_KEY ayarlanmamış — geliştirme fallback anahtarı kullanılıyor. Üretimde ASLA kullanılmamalı.")
	sum := sha256.Sum256([]byte(devKeyMaterial))
	return InitCrypto(sum[:])
}

// PepperFromEnv loads the phone-hash pepper from OBSCURA_PHONE_PEPPER, a
// DISTINCT secret from the AES key. The raw UTF-8 bytes of the env value are
// used directly (HMAC accepts any key length; a high-entropy value >= 32 bytes
// is recommended). Hard-fail in production, insecure fallback in dev.
func PepperFromEnv() ([]byte, error) {
	if raw := os.Getenv("OBSCURA_PHONE_PEPPER"); raw != "" {
		return []byte(raw), nil
	}

	if os.Getenv("OBSCURA_ENV") == "production" {
		log.Fatal("OBSCURA_PHONE_PEPPER env required in production")
	}

	log.Println("[GÜVENLİK UYARISI] OBSCURA_PHONE_PEPPER ayarlanmamış — geliştirme fallback pepper kullanılıyor. Üretimde ASLA kullanılmamalı.")
	return []byte(devPepper), nil
}

// EncryptField encrypts plaintext with AES-256-GCM using a fresh random 12-byte
// nonce, returning nonce||ciphertext||tag. The nonce is prepended so DecryptField
// can recover it.
func EncryptField(plaintext []byte) ([]byte, error) {
	if aead == nil {
		return nil, ErrCryptoNotInitialized
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("subscriber: nonce: %w", err)
	}
	// Seal appends the ciphertext to the dst (nonce), giving nonce||ct||tag.
	return aead.Seal(nonce, nonce, plaintext, nil), nil
}

// DecryptField reverses EncryptField. It fails (non-nil error) on a wrong key or
// any tampering, because GCM verifies the authentication tag.
func DecryptField(ciphertext []byte) ([]byte, error) {
	if aead == nil {
		return nil, ErrCryptoNotInitialized
	}
	ns := aead.NonceSize()
	if len(ciphertext) < ns {
		return nil, errors.New("subscriber: ciphertext shorter than nonce")
	}
	nonce, ct := ciphertext[:ns], ciphertext[ns:]
	plaintext, err := aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("subscriber: decrypt failed (wrong key or tampered): %w", err)
	}
	return plaintext, nil
}
