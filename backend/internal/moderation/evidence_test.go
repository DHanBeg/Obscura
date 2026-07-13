package moderation

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"testing"

	_ "modernc.org/sqlite"
)

func newEvidenceFixture(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE messages (
		id TEXT PRIMARY KEY, from_did TEXT, to_did TEXT, ciphertext TEXT
	)`); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return db
}

func seedMessage(t *testing.T, db *sql.DB, id, fromDID, toDID, ciphertext string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO messages (id, from_did, to_did, ciphertext) VALUES (?, ?, ?, ?)`,
		id, fromDID, toDID, ciphertext); err != nil {
		t.Fatalf("seed message: %v", err)
	}
}

func hashHex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestVerifyEvidence_Success(t *testing.T) {
	db := newEvidenceFixture(t)
	seedMessage(t, db, "msg-1", "did:obs:alice", "did:obs:bob", "ciphertext-blob-1")

	ok, err := VerifyEvidence(context.Background(), db, "msg-1", "did:obs:alice", "did:obs:bob", hashHex("ciphertext-blob-1"))
	if err != nil {
		t.Fatalf("VerifyEvidence: %v", err)
	}
	if !ok {
		t.Fatal("expected evidence to verify")
	}
}

func TestVerifyEvidence_WrongHashRejected(t *testing.T) {
	db := newEvidenceFixture(t)
	seedMessage(t, db, "msg-1", "did:obs:alice", "did:obs:bob", "ciphertext-blob-1")

	ok, err := VerifyEvidence(context.Background(), db, "msg-1", "did:obs:alice", "did:obs:bob", hashHex("not-the-real-ciphertext"))
	if ok {
		t.Fatal("expected verification to fail on hash mismatch")
	}
	if !errors.Is(err, ErrEvidenceMismatch) {
		t.Fatalf("expected ErrEvidenceMismatch, got %v", err)
	}
}

func TestVerifyEvidence_MessageNotFoundRejected(t *testing.T) {
	db := newEvidenceFixture(t)

	ok, err := VerifyEvidence(context.Background(), db, "nonexistent", "did:obs:alice", "did:obs:bob", hashHex("x"))
	if ok {
		t.Fatal("expected verification to fail for missing message")
	}
	if !errors.Is(err, ErrEvidenceMismatch) {
		t.Fatalf("expected ErrEvidenceMismatch, got %v", err)
	}
}

func TestVerifyEvidence_WrongAccusedRejected(t *testing.T) {
	db := newEvidenceFixture(t)
	seedMessage(t, db, "msg-1", "did:obs:alice", "did:obs:bob", "ciphertext-blob-1")

	// Reporter claims "carol" sent it, but the message is actually from alice.
	ok, err := VerifyEvidence(context.Background(), db, "msg-1", "did:obs:carol", "did:obs:bob", hashHex("ciphertext-blob-1"))
	if ok {
		t.Fatal("expected verification to fail: message not from accused")
	}
	if !errors.Is(err, ErrEvidenceMismatch) {
		t.Fatalf("expected ErrEvidenceMismatch, got %v", err)
	}
}

func TestVerifyEvidence_ReporterNotRecipientRejected(t *testing.T) {
	db := newEvidenceFixture(t)
	seedMessage(t, db, "msg-1", "did:obs:alice", "did:obs:bob", "ciphertext-blob-1")

	// Eve is not the recipient of this message — she cannot be its victim.
	ok, err := VerifyEvidence(context.Background(), db, "msg-1", "did:obs:alice", "did:obs:eve", hashHex("ciphertext-blob-1"))
	if ok {
		t.Fatal("expected verification to fail: reporter is not the recipient")
	}
	if !errors.Is(err, ErrEvidenceMismatch) {
		t.Fatalf("expected ErrEvidenceMismatch, got %v", err)
	}
}

func TestVerifyEvidence_MissingFieldsRejected(t *testing.T) {
	db := newEvidenceFixture(t)
	if _, err := VerifyEvidence(context.Background(), db, "", "did:obs:alice", "did:obs:bob", "hash"); err == nil {
		t.Error("expected error for empty message_id")
	}
	if _, err := VerifyEvidence(context.Background(), db, "msg-1", "", "did:obs:bob", "hash"); err == nil {
		t.Error("expected error for empty accused")
	}
	if _, err := VerifyEvidence(context.Background(), db, "msg-1", "did:obs:alice", "", "hash"); err == nil {
		t.Error("expected error for empty reporter")
	}
	if _, err := VerifyEvidence(context.Background(), db, "msg-1", "did:obs:alice", "did:obs:bob", ""); err == nil {
		t.Error("expected error for empty hash")
	}
}
