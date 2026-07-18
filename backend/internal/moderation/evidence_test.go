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
		id TEXT PRIMARY KEY, from_did TEXT, to_did TEXT, ciphertext TEXT,
		encryption_type TEXT NOT NULL DEFAULT 'signal'
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

// seedSealedMessage — gerçek üretim satırını taklit eder: from_did BOŞ
// (HandleSendMessage sealed mesajlarda plaintext from_did yazmaz, bkz.
// ADR-0016), encryption_type = 'sealed'.
func seedSealedMessage(t *testing.T, db *sql.DB, id, toDID, ciphertext string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO messages (id, from_did, to_did, ciphertext, encryption_type) VALUES (?, '', ?, ?, 'sealed')`,
		id, toDID, ciphertext); err != nil {
		t.Fatalf("seed sealed message: %v", err)
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

// ─── SEALED-SENDER KANITI (Adım 8) ─────────────────────────────────────────
//
// Gerçek crypto/test-vectors/sealed_sender_vectors.json çıktısı (obscura-
// crypto-cli tarafından üretildi) — Go tarafının certSigningBytes/
// verifySealedCertificate'ının Rust (sealed_sender.rs) ve TS
// (sealed-sender.ts) ile BİREBİR aynı byte dizilimini ürettiğini kanıtlar;
// kendi kendine tutarlı bir test (Go'nun kendi ürettiği sertifikayı yine
// Go'nun doğrulaması) bunu KANITLAYAMAZ — üç taraf da aynı hatayı yapabilir.
const (
	sealedVecDID           = "did:obs:d8b48e57c873c76f0e97924d621baa33"
	sealedVecIdentityDHPub = "3662bd99971a3cf55890e0adfa34d8da0b07623bfe4922b3b8aa6493d7501702"
	sealedVecSigningPub    = "9c9a96e314fbc34584e38bb3625c17ee4bd5f17e665447b15f760b05e839c8f2"
	sealedVecSignature     = "4ea949b8971b21249d436c37634d4e6fb836aad6fe4ef10029d85471604a5e2469317e6b3fed4865bfca602074b7af04d31611873386aca5e8d4de36d27cb00f"
	sealedVecExpiresAt     = uint64(2000000000)
	sealedVecNow           = uint64(1900000000) // vektördeki "now" — süre dolmadan önce
)

func sealedVecEvidence() SealedCertEvidence {
	return SealedCertEvidence{
		IdentityDHPubHex: sealedVecIdentityDHPub,
		SigningPubHex:    sealedVecSigningPub,
		ExpiresAt:        sealedVecExpiresAt,
		SignatureHex:     sealedVecSignature,
	}
}

func TestVerifySealedEvidence_CrossImplementationVectorVerifies(t *testing.T) {
	db := newEvidenceFixture(t)
	seedSealedMessage(t, db, "msg-1", "did:obs:bob", "sealed-envelope-blob")

	ok, err := VerifySealedEvidence(context.Background(), db, "msg-1", sealedVecDID, "did:obs:bob",
		hashHex("sealed-envelope-blob"), sealedVecEvidence(), sealedVecNow)
	if err != nil {
		t.Fatalf("VerifySealedEvidence: %v", err)
	}
	if !ok {
		t.Fatal("expected Rust-generated sealed certificate to verify in Go")
	}
}

func TestVerifySealedEvidence_ForgedSignatureRejected(t *testing.T) {
	db := newEvidenceFixture(t)
	seedSealedMessage(t, db, "msg-1", "did:obs:bob", "sealed-envelope-blob")

	forged := sealedVecEvidence()
	sigBytes, err := hex.DecodeString(forged.SignatureHex)
	if err != nil {
		t.Fatalf("decode sig: %v", err)
	}
	sigBytes[0] ^= 0xFF
	forged.SignatureHex = hex.EncodeToString(sigBytes)

	ok, err := VerifySealedEvidence(context.Background(), db, "msg-1", sealedVecDID, "did:obs:bob",
		hashHex("sealed-envelope-blob"), forged, sealedVecNow)
	if ok {
		t.Fatal("expected forged signature to be rejected")
	}
	if !errors.Is(err, ErrEvidenceMismatch) {
		t.Fatalf("expected ErrEvidenceMismatch, got %v", err)
	}
}

func TestVerifySealedEvidence_ExpiredCertRejected(t *testing.T) {
	db := newEvidenceFixture(t)
	seedSealedMessage(t, db, "msg-1", "did:obs:bob", "sealed-envelope-blob")

	afterExpiry := sealedVecExpiresAt + 1
	ok, err := VerifySealedEvidence(context.Background(), db, "msg-1", sealedVecDID, "did:obs:bob",
		hashHex("sealed-envelope-blob"), sealedVecEvidence(), afterExpiry)
	if ok {
		t.Fatal("expected expired certificate to be rejected")
	}
	if !errors.Is(err, ErrEvidenceMismatch) {
		t.Fatalf("expected ErrEvidenceMismatch, got %v", err)
	}
}

func TestVerifySealedEvidence_WrongAccusedDIDRejected(t *testing.T) {
	db := newEvidenceFixture(t)
	seedSealedMessage(t, db, "msg-1", "did:obs:bob", "sealed-envelope-blob")

	// Reporter, mesajı vektördeki gerçek gönderen yerine başka bir DID'e
	// (carol) mal ediyor — signing_bytes'a giren did değişince imza artık
	// hiç uyuşmaz (certSigningBytes içine accusedDID gömülüyor).
	ok, err := VerifySealedEvidence(context.Background(), db, "msg-1", "did:obs:carol", "did:obs:bob",
		hashHex("sealed-envelope-blob"), sealedVecEvidence(), sealedVecNow)
	if ok {
		t.Fatal("expected mismatched accused DID to be rejected")
	}
	if !errors.Is(err, ErrEvidenceMismatch) {
		t.Fatalf("expected ErrEvidenceMismatch, got %v", err)
	}
}

func TestVerifySealedEvidence_ReporterNotRecipientRejected(t *testing.T) {
	db := newEvidenceFixture(t)
	seedSealedMessage(t, db, "msg-1", "did:obs:bob", "sealed-envelope-blob")

	ok, err := VerifySealedEvidence(context.Background(), db, "msg-1", sealedVecDID, "did:obs:eve",
		hashHex("sealed-envelope-blob"), sealedVecEvidence(), sealedVecNow)
	if ok {
		t.Fatal("expected verification to fail: reporter is not the recipient")
	}
	if !errors.Is(err, ErrEvidenceMismatch) {
		t.Fatalf("expected ErrEvidenceMismatch, got %v", err)
	}
}

func TestVerifySealedEvidence_HashMismatchRejected(t *testing.T) {
	db := newEvidenceFixture(t)
	seedSealedMessage(t, db, "msg-1", "did:obs:bob", "sealed-envelope-blob")

	ok, err := VerifySealedEvidence(context.Background(), db, "msg-1", sealedVecDID, "did:obs:bob",
		hashHex("not-the-real-envelope"), sealedVecEvidence(), sealedVecNow)
	if ok {
		t.Fatal("expected hash mismatch to be rejected")
	}
	if !errors.Is(err, ErrEvidenceMismatch) {
		t.Fatalf("expected ErrEvidenceMismatch, got %v", err)
	}
}

func TestVerifySealedEvidence_NonSealedMessageRejected(t *testing.T) {
	db := newEvidenceFixture(t)
	// Eski (zarfsız) mesaj — from_did dolu, encryption_type default 'signal'.
	// Sealed-only yol bunu kabul ETMEMELİ; çağıran (HandleSpamReport)
	// bunun yerine VerifyEvidence'a (legacy) düşmeli.
	seedMessage(t, db, "msg-1", "did:obs:alice", "did:obs:bob", "plain-ciphertext")

	ok, err := VerifySealedEvidence(context.Background(), db, "msg-1", "did:obs:alice", "did:obs:bob",
		hashHex("plain-ciphertext"), sealedVecEvidence(), sealedVecNow)
	if ok {
		t.Fatal("expected non-sealed message to be rejected by sealed-only path")
	}
	if !errors.Is(err, ErrEvidenceMismatch) {
		t.Fatalf("expected ErrEvidenceMismatch, got %v", err)
	}
}

// Eski (zarfsız) mesajlarda VerifyEvidence hâlâ AYNEN çalışır — kademeli
// geçiş: sealed evidence eklenmesi legacy yolu bozmamalı.
func TestVerifyEvidence_StillWorksAfterSealedPathAdded(t *testing.T) {
	db := newEvidenceFixture(t)
	seedMessage(t, db, "msg-1", "did:obs:alice", "did:obs:bob", "ciphertext-blob-1")

	ok, err := VerifyEvidence(context.Background(), db, "msg-1", "did:obs:alice", "did:obs:bob", hashHex("ciphertext-blob-1"))
	if err != nil {
		t.Fatalf("VerifyEvidence: %v", err)
	}
	if !ok {
		t.Fatal("expected legacy evidence path to keep working")
	}
}
