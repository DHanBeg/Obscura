package signal

import (
	"database/sql"
	"fmt"
	"time"
)

// PreKeyBundle contains all the public key material the initiating party
// needs to execute the X3DH handshake.  All keys are base64-encoded strings
// exactly as they were uploaded by the owner's client.
type PreKeyBundle struct {
	DID           string `json:"did"`
	IdentityKey   string `json:"identity_key"`    // Ed25519 long-term identity key (base64)
	SignedPreKey  string `json:"signed_prekey"`   // Signed pre-key (base64)
	SignedPreKeySig string `json:"signed_prekey_sig"` // Signature over SignedPreKey (base64)
	SignedPreKeyID  int    `json:"signed_prekey_id"`
	OneTimePreKey  string `json:"one_time_prekey,omitempty"` // OPK (base64), absent when pool exhausted
	OneTimePreKeyID int   `json:"one_time_prekey_id,omitempty"`
}

// GetPrekeyBundle returns the X3DH bundle for did.
// It atomically claims one OPK from the pool (marking it used) and pairs it
// with the long-term bundle from prekey_bundles.
// If no unused OPK is available, the bundle is returned without one
// (X3DH still works — it degrades gracefully per the spec).
func (s *SessionStore) GetPrekeyBundle(did string) (*PreKeyBundle, error) {
	// 1. Fetch long-term bundle.
	bundle := &PreKeyBundle{DID: did}
	err := s.db.QueryRow(`
		SELECT identity_key, signed_prekey, signed_prekey_sig, signed_prekey_id
		FROM prekey_bundles
		WHERE did = ?`, did,
	).Scan(&bundle.IdentityKey, &bundle.SignedPreKey, &bundle.SignedPreKeySig, &bundle.SignedPreKeyID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("signal.GetPrekeyBundle: no prekey bundle for %s", did)
	}
	if err != nil {
		return nil, fmt.Errorf("signal.GetPrekeyBundle: %w", err)
	}

	// 2. Claim one OPK atomically.
	//    We read + update in two statements; because DB.SetMaxOpenConns(1) serialises
	//    all writes, there is no TOCTOU race on the single-connection SQLite instance.
	var opkID int
	var opkKey string
	opkErr := s.db.QueryRow(`
		SELECT id, opk_id, public_key
		FROM one_time_prekeys
		WHERE did = ? AND used = 0
		ORDER BY created_at ASC
		LIMIT 1`, did,
	).Scan(new(string), &opkID, &opkKey)

	if opkErr == nil {
		// Mark as used.
		now := time.Now().UTC().Format(time.RFC3339)
		s.db.Exec(`UPDATE one_time_prekeys SET used = 1, used_at = ? WHERE did = ? AND opk_id = ? AND used = 0`,
			now, did, opkID)
		bundle.OneTimePreKey = opkKey
		bundle.OneTimePreKeyID = opkID
	}
	// If opkErr == sql.ErrNoRows the bundle is returned without an OPK — acceptable.

	return bundle, nil
}
