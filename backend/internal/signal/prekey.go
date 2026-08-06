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
	//    Read + conditional update, retried against the next candidate on a
	//    lost race. On SQLite (DB.SetMaxOpenConns(1)) there is only ever one
	//    connection, so no concurrent claimer can exist and this loop always
	//    succeeds (or exhausts) on its first iteration — same observable
	//    behavior as before. Under a real connection pool (Postgres) two
	//    callers can both SELECT the same unused row before either UPDATEs;
	//    the WHERE used = 0 guard means only one UPDATE actually takes
	//    effect, and checking RowsAffected is what stops the loser from
	//    handing out an OPK it didn't actually claim (the DB row was already
	//    consistent either way — it's the in-memory bundle that was wrong).
	for {
		var rowID string
		var opkID int
		var opkKey string
		opkErr := s.db.QueryRow(`
			SELECT id, opk_id, public_key
			FROM one_time_prekeys
			WHERE did = ? AND used = 0
			ORDER BY created_at ASC
			LIMIT 1`, did,
		).Scan(&rowID, &opkID, &opkKey)
		if opkErr == sql.ErrNoRows {
			// Pool exhausted (or every remaining candidate was just lost to
			// a concurrent claimer) — bundle is returned without an OPK,
			// acceptable per the X3DH graceful-degradation contract.
			break
		}
		if opkErr != nil {
			return nil, fmt.Errorf("signal.GetPrekeyBundle: opk lookup: %w", opkErr)
		}

		now := time.Now().UTC().Format(time.RFC3339)
		res, err := s.db.Exec(`UPDATE one_time_prekeys SET used = 1, used_at = ? WHERE id = ? AND used = 0`,
			now, rowID)
		if err != nil {
			return nil, fmt.Errorf("signal.GetPrekeyBundle: opk claim: %w", err)
		}
		if affected, _ := res.RowsAffected(); affected == 1 {
			bundle.OneTimePreKey = opkKey
			bundle.OneTimePreKeyID = opkID
			break
		}
		// Lost the race for this row to another concurrent claimer — retry
		// with the next-oldest candidate (this one is now used=1 and will
		// no longer be selected).
	}

	return bundle, nil
}
