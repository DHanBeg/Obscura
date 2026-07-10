package subscriber

import (
	"bytes"
	"database/sql"
	"errors"
	"testing"
)

// newAccessFixture opens a fresh subscriber DB, initializes crypto, and seeds
// one subscriber row. It returns the DB handle for direct assertions.
func newAccessFixture(t *testing.T) (*sql.DB, []byte) {
	t.Helper()

	if err := InitCrypto(testKey(t, 7)); err != nil {
		t.Fatalf("InitCrypto: %v", err)
	}

	db, err := OpenSubscriberDB(t.TempDir())
	if err != nil {
		t.Fatalf("OpenSubscriberDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	phoneHash := HashPhone("+905551112233", []byte("pepper"))
	phoneEnc, err := EncryptField(phoneHash)
	if err != nil {
		t.Fatalf("encrypt phone hash: %v", err)
	}
	ipEnc, err := EncryptField([]byte("203.0.113.9"))
	if err != nil {
		t.Fatalf("encrypt ip: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO subscribers (did, phone_hash_enc, registration_ip_enc, registration_timestamp, last_seen)
		 VALUES (?, ?, ?, ?, ?)`,
		"did:obs:alice", phoneEnc, ipEnc, "2026-01-01T00:00:00Z", "2026-07-06T00:00:00Z",
	); err != nil {
		t.Fatalf("seed subscriber: %v", err)
	}
	return db, phoneHash
}

func auditCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_log`).Scan(&n); err != nil {
		t.Fatalf("count audit_log: %v", err)
	}
	return n
}

func TestGetSubscriberDataReturnsDecryptedRecord(t *testing.T) {
	db, wantHash := newAccessFixture(t)

	rec, err := getSubscriberData(db, "did:obs:alice", "warrant-2026-001", "det-jones", defaultAudit)
	if err != nil {
		t.Fatalf("getSubscriberData: %v", err)
	}
	if rec == nil {
		t.Fatal("nil record on success")
	}
	if !bytes.Equal(rec.PhoneHash, wantHash) {
		t.Fatalf("PhoneHash mismatch: got %x want %x", rec.PhoneHash, wantHash)
	}
	if rec.RegistrationIP != "203.0.113.9" {
		t.Fatalf("RegistrationIP = %q, want 203.0.113.9", rec.RegistrationIP)
	}
	if rec.RegistrationTimestamp != "2026-01-01T00:00:00Z" {
		t.Fatalf("RegistrationTimestamp = %q", rec.RegistrationTimestamp)
	}
}

func TestGetSubscriberDataWritesExactlyOneAuditPerCall(t *testing.T) {
	db, _ := newAccessFixture(t)

	if got := auditCount(t, db); got != 0 {
		t.Fatalf("precondition: audit_log should start empty, got %d", got)
	}

	for i := 1; i <= 3; i++ {
		if _, err := getSubscriberData(db, "did:obs:alice", "reason", "operator", defaultAudit); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if got := auditCount(t, db); got != i {
			t.Fatalf("after %d calls audit_log has %d rows, want %d", i, got, i)
		}
	}

	// Verify the audit row content is populated (not blank).
	var operator, did, reason, action string
	if err := db.QueryRow(
		`SELECT operator, did, legal_reason, action FROM audit_log LIMIT 1`,
	).Scan(&operator, &did, &reason, &action); err != nil {
		t.Fatalf("read audit row: %v", err)
	}
	if operator != "operator" || did != "did:obs:alice" || reason != "reason" || action != "read" {
		t.Fatalf("audit row content wrong: op=%q did=%q reason=%q action=%q", operator, did, reason, action)
	}
}

// failingAudit simulates the audit writer failing mid-transaction.
func failingAudit(_ *sql.Tx, _, _, _, _, _ string) error {
	return errors.New("injected audit failure")
}

func TestAuditFailureLeaksNoDataAndNoPartialState(t *testing.T) {
	db, _ := newAccessFixture(t)

	// Act: audit write fails.
	rec, err := getSubscriberData(db, "did:obs:alice", "reason", "operator", failingAudit)

	// Assert: no data handed back.
	if err == nil {
		t.Fatal("expected error when audit write fails, got nil")
	}
	if rec != nil {
		t.Fatalf("subscriber data leaked despite audit failure: %+v", rec)
	}

	// Assert: transaction rolled back — no audit rows landed.
	if got := auditCount(t, db); got != 0 {
		t.Fatalf("audit_log has %d rows after rolled-back call, want 0", got)
	}

	// Assert: no partial/uncommitted state — a subsequent good call still works
	// and produces exactly one audit row.
	if _, err := getSubscriberData(db, "did:obs:alice", "reason", "operator", defaultAudit); err != nil {
		t.Fatalf("follow-up call failed (stuck lock / partial state?): %v", err)
	}
	if got := auditCount(t, db); got != 1 {
		t.Fatalf("after one good call audit_log has %d rows, want 1", got)
	}
}

func TestGetSubscriberDataRequiresOperatorAndReason(t *testing.T) {
	db, _ := newAccessFixture(t)

	if _, err := getSubscriberData(db, "did:obs:alice", "", "operator", defaultAudit); !errors.Is(err, ErrInvalidAccessArgs) {
		t.Fatalf("missing legalReason: got %v, want ErrInvalidAccessArgs", err)
	}
	if _, err := getSubscriberData(db, "did:obs:alice", "reason", "", defaultAudit); !errors.Is(err, ErrInvalidAccessArgs) {
		t.Fatalf("missing operator: got %v, want ErrInvalidAccessArgs", err)
	}
	// Rejected before any DB work → no audit rows.
	if got := auditCount(t, db); got != 0 {
		t.Fatalf("audit_log has %d rows after rejected calls, want 0", got)
	}
}
