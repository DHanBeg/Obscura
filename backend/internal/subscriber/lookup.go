package subscriber

// FindDIDByPhone reverse-looks-up a subscriber's DID from their phone number
// via the unencrypted, indexed phone_hash column (HMAC(phone, pepper) -- a
// one-way keyed hash, same protection model as a password verifier: it
// cannot be reversed to the phone number without the pepper, so storing it
// unencrypted for O(1) lookup is the intended design, not a shortcut).
//
// This never decrypts any subscriber PII (contrast with the audited
// GetSubscriberData legal-access path in access.go, which does) -- it's a
// routine, unaudited hash-equality check, the same class of operation as
// comparing a bcrypt hash, so it's the right primitive for every
// login/registration attempt.
//
// This is the path auth must use now that the main users table's phone
// column is nulled out post phone_migrated (see migrate.go) -- a plain
// SELECT ... FROM users WHERE phone = ? can never match again once a row
// has been migrated.
func FindDIDByPhone(phone string) (string, error) {
	if store == nil {
		return "", ErrStoreNotInitialized
	}
	pepper, err := PepperFromEnv()
	if err != nil {
		return "", err
	}
	wanted := HashPhone(phone, pepper)

	return queryDIDByPhoneHash(store, wanted)
}
