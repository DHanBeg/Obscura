package subscriber

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// store is the process-wide subscriber DB handle used by GetSubscriberData.
// Set once at startup via InitStore.
var store *sql.DB

// ErrStoreNotInitialized is returned when GetSubscriberData is called before
// InitStore.
var ErrStoreNotInitialized = errors.New("subscriber: store not initialized (call InitStore)")

// ErrInvalidAccessArgs is returned when an audited access is attempted without
// the mandatory operator / legal reason.
var ErrInvalidAccessArgs = errors.New("subscriber: operator and legalReason are required")

// SubscriberRecord is the decrypted view of one identity row returned to an
// authorized, audited caller.
type SubscriberRecord struct {
	DID                   string
	PhoneHash             []byte // decrypted HMAC-SHA256 hash (see phone_hash.go)
	RegistrationIP        string // decrypted; empty for backfilled rows
	RegistrationTimestamp string
	LastSeen              string
}

// InitStore installs the subscriber DB handle used by the exported
// GetSubscriberData entry point.
func InitStore(db *sql.DB) {
	store = db
}

// auditFunc inserts the mandatory audit_log row for an access. It is a seam so
// that failure of the audit write can be exercised in tests, proving the
// no-audit-no-data invariant.
type auditFunc func(tx *sql.Tx, id, operator, did, legalReason, action string) error

// defaultAudit is the production audit writer.
func defaultAudit(tx *sql.Tx, id, operator, did, legalReason, action string) error {
	_, err := tx.Exec(
		`INSERT INTO audit_log (id, operator, accessed_at, did, legal_reason, action)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, operator, time.Now().UTC().Format(time.RFC3339), did, legalReason, action,
	)
	return err
}

// GetSubscriberData is the single, sole entry point for reading sensitive
// subscriber columns. Every successful read is accompanied by exactly one
// audit_log row committed in the SAME transaction. If the audit write fails,
// the transaction rolls back and NO data is returned — there is no code path
// that yields subscriber data without a corresponding audit row landing.
//
// No other exported function in this package reads the subscribers table's
// sensitive columns.
func GetSubscriberData(did, legalReason, operator string) (*SubscriberRecord, error) {
	return getSubscriberData(store, did, legalReason, operator, defaultAudit)
}

// getSubscriberData is the testable core. audit is injected so tests can force
// an audit-write failure and assert the invariant.
func getSubscriberData(db *sql.DB, did, legalReason, operator string, audit auditFunc) (*SubscriberRecord, error) {
	if db == nil {
		return nil, ErrStoreNotInitialized
	}
	if operator == "" || legalReason == "" {
		return nil, ErrInvalidAccessArgs
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("subscriber: begin tx: %w", err)
	}
	// Safe no-op after a successful Commit; guarantees rollback on every early
	// return below.
	defer func() { _ = tx.Rollback() }()

	var (
		phoneEnc []byte
		ipEnc    []byte
		regTs    string
		lastSeen string
	)
	err = tx.QueryRow(
		`SELECT phone_hash_enc, registration_ip_enc, registration_timestamp, last_seen
		 FROM subscribers WHERE did = ?`, did,
	).Scan(&phoneEnc, &ipEnc, &regTs, &lastSeen)
	if err != nil {
		return nil, fmt.Errorf("subscriber: read row: %w", err)
	}

	phoneHash, err := DecryptField(phoneEnc)
	if err != nil {
		return nil, err
	}
	ip := ""
	if len(ipEnc) > 0 {
		p, err := DecryptField(ipEnc)
		if err != nil {
			return nil, err
		}
		ip = string(p)
	}

	// Audit write is part of the same transaction. On failure we return before
	// handing back any record; the deferred Rollback discards everything.
	if err := audit(tx, uuid.New().String(), operator, did, legalReason, "read"); err != nil {
		return nil, fmt.Errorf("subscriber: audit write failed, access denied: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("subscriber: commit failed, access denied: %w", err)
	}

	return &SubscriberRecord{
		DID:                   did,
		PhoneHash:             phoneHash,
		RegistrationIP:        ip,
		RegistrationTimestamp: regTs,
		LastSeen:              lastSeen,
	}, nil
}
