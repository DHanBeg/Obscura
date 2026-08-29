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

	"obscura.network/core/internal/secrets"
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

// devKeyMaterial is an INSECURE fallback used only outside production,
// mirroring auth.jwtKey's dev behaviour. Never rely on this in a real
// deployment — it is deterministic and public.
const devKeyMaterial = "obscura-insecure-dev-subscriber-key-change-me"

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
// base64 STANDARD encoded). Hard-fail in production when missing/malformed,
// insecure deterministic fallback in dev.
//
// (C10 fail-open kökü kapatıldı, #10) Eskiden burada
// `os.Getenv("OBSCURA_ENV") == "production"` kontrolü vardı — opt-out yönü:
// yalnızca değer TAM OLARAK "production" ise fatal oluyordu, env unutulur ya
// da "staging"/yanlış yazılırsa sessizce dev fallback'e (deterministik,
// public devKeyMaterial) düşüyordu. secrets.IsDev() ile D1 fail-safe yönüne
// çevrildi: OBSCURA_ENV açıkça development/dev DEĞİLSE prod sayılır. Anahtar
// base64+32-byte format doğrulaması gerektirdiği için secrets.Require'ın
// generic placeholder string'i buraya doğrudan uymuyor (base64 alfabesinde
// değil) — bu yüzden secrets.IsDev() ile aynı fail-safe yönü, yerel
// deterministik dev-fallback korunarak uygulandı.
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

	if !secrets.IsDev() {
		log.Fatal("OBSCURA_SUBSCRIBER_KEY env required (OBSCURA_ENV is not an explicit dev opt-in)")
	}

	log.Println("[GÜVENLİK UYARISI] OBSCURA_SUBSCRIBER_KEY ayarlanmamış — geliştirme fallback anahtarı kullanılıyor. Üretimde ASLA kullanılmamalı.")
	sum := sha256.Sum256([]byte(devKeyMaterial))
	return InitCrypto(sum[:])
}

// PepperFromEnv loads the phone-hash pepper from OBSCURA_PHONE_PEPPER, a
// DISTINCT secret from the AES key. The raw UTF-8 bytes of the env value are
// used directly (HMAC accepts any key length; a high-entropy value >= 32 bytes
// is recommended).
//
// (C10 fail-open kökü kapatıldı, #11) Pepper'ın format kısıtı yok (herhangi
// bir uzunluktaki string HMAC anahtarı olarak geçerli) — secrets.Require
// doğrudan kullanılabiliyor: D1 fail-safe (OBSCURA_ENV açık dev opt-in
// değilse prod, eksikse FATAL), dev'de placeholder + uyarı.
func PepperFromEnv() ([]byte, error) {
	return []byte(secrets.Require("OBSCURA_PHONE_PEPPER")), nil
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
