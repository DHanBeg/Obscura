// Package signal — Signal Protocol state management (backend side).
//
// The backend is E2EE-blind: it never sees plaintext, never derives keys,
// never touches the Double Ratchet internals. Its only job is to:
//   - Store opaque, client-encrypted session-state blobs (Double Ratchet state).
//   - Distribute X3DH prekey bundles from the prekey_bundles / one_time_prekeys
//     tables that already exist in the DB.
//
// Security contract: session_state_b64 is encrypted by the client with its own
// device key before upload. The server stores and returns it as an opaque blob.
package signal

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"obscura.network/core/internal/dbi"
)

// SessionStore holds a reference to the shared DB connection.
type SessionStore struct {
	db dbi.Querier
}

// NewSessionStore creates a SessionStore wrapping the given dbi.Querier.
func NewSessionStore(db dbi.Querier) *SessionStore {
	return &SessionStore{db: db}
}

// SessionInfo is the public view of a stored session (no state blob).
type SessionInfo struct {
	ID        string    `json:"id"`
	OwnerDID  string    `json:"owner_did"`
	PeerDID   string    `json:"peer_did"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SaveSession upserts an opaque Double Ratchet state blob for an
// (owner, peer) pair.  stateB64 must be base64-encoded and already encrypted
// by the client — the server treats it as an opaque octet string.
func (s *SessionStore) SaveSession(ownerDID, peerDID, stateB64 string) error {
	if ownerDID == "" || peerDID == "" {
		return fmt.Errorf("signal.SaveSession: owner_did and peer_did are required")
	}
	if stateB64 == "" {
		return fmt.Errorf("signal.SaveSession: state blob is empty")
	}
	// Hard cap: 64 KB is more than enough for any Double Ratchet state blob.
	if len(stateB64) > 65536 {
		return fmt.Errorf("signal.SaveSession: state blob exceeds 64 KB limit")
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// UPSERT: SQLite ≥ 3.24 (bundled via modernc.org/sqlite, always OK).
	_, err := s.db.Exec(`
		INSERT INTO signal_sessions (id, owner_did, peer_did, session_state_b64, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(owner_did, peer_did)
		DO UPDATE SET session_state_b64 = excluded.session_state_b64,
		              updated_at        = excluded.updated_at`,
		uuid.New().String(), ownerDID, peerDID, stateB64, now, now,
	)
	if err != nil {
		return fmt.Errorf("signal.SaveSession: %w", err)
	}
	return nil
}

// GetSession retrieves the opaque state blob for an (owner, peer) pair.
// Returns ("", sql.ErrNoRows) when no session exists yet.
func (s *SessionStore) GetSession(ownerDID, peerDID string) (string, error) {
	var stateB64 string
	err := s.db.QueryRow(`
		SELECT session_state_b64
		FROM signal_sessions
		WHERE owner_did = ? AND peer_did = ?`,
		ownerDID, peerDID,
	).Scan(&stateB64)
	if err != nil {
		return "", fmt.Errorf("signal.GetSession: %w", err)
	}
	return stateB64, nil
}

// DeleteSession removes a session entirely. Idempotent — returns nil when
// the session did not exist.
func (s *SessionStore) DeleteSession(ownerDID, peerDID string) error {
	_, err := s.db.Exec(`
		DELETE FROM signal_sessions WHERE owner_did = ? AND peer_did = ?`,
		ownerDID, peerDID,
	)
	if err != nil {
		return fmt.Errorf("signal.DeleteSession: %w", err)
	}
	return nil
}

// ListSessions returns the metadata for all sessions owned by ownerDID.
// The state blob is not included in the result.
func (s *SessionStore) ListSessions(ownerDID string) ([]SessionInfo, error) {
	rows, err := s.db.Query(`
		SELECT id, owner_did, peer_did, created_at, updated_at
		FROM signal_sessions
		WHERE owner_did = ?
		ORDER BY updated_at DESC`,
		ownerDID,
	)
	if err != nil {
		return nil, fmt.Errorf("signal.ListSessions: %w", err)
	}
	defer rows.Close()

	var sessions []SessionInfo
	for rows.Next() {
		var si SessionInfo
		var createdAt, updatedAt string
		if err := rows.Scan(&si.ID, &si.OwnerDID, &si.PeerDID, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("signal.ListSessions scan: %w", err)
		}
		si.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		si.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		sessions = append(sessions, si)
	}
	if sessions == nil {
		sessions = []SessionInfo{}
	}
	return sessions, rows.Err()
}
