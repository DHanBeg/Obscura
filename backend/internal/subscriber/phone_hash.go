package subscriber

import (
	"crypto/hmac"
	"crypto/sha256"
)

// HashPhone derives a deterministic, keyed hash of a phone number using
// HMAC-SHA256(key=pepper, message=phone).
//
// Deliberately NOT bcrypt/argon2: this is an EXACT-MATCH lookup key, not a
// password verifier. We need the same phone to always map to the same value so
// a subscriber can be found by number. Phone numbers are low-entropy (a bounded
// numeric space that is cheaply brute-forced), so a slow salted KDF would give
// no meaningful protection on its own. Brute-force resistance here comes from
// the SECRECY of the pepper (a separate server-side secret, never stored beside
// the hashes), which an attacker with only DB access does not possess.
func HashPhone(phone string, pepper []byte) []byte {
	mac := hmac.New(sha256.New, pepper)
	mac.Write([]byte(phone))
	return mac.Sum(nil)
}
