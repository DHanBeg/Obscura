package signal_test

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"

	"obscura.network/core/internal/signal"
)

// openTestDB creates an in-memory SQLite DB with the minimal schema required
// by signal.SessionStore and signal.GetPrekeyBundle.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS signal_sessions (
			id                TEXT PRIMARY KEY,
			owner_did         TEXT NOT NULL,
			peer_did          TEXT NOT NULL,
			session_state_b64 TEXT NOT NULL,
			created_at        TEXT NOT NULL,
			updated_at        TEXT NOT NULL,
			UNIQUE(owner_did, peer_did)
		);
		CREATE TABLE IF NOT EXISTS prekey_bundles (
			did               TEXT PRIMARY KEY,
			identity_key      TEXT NOT NULL,
			signed_prekey     TEXT NOT NULL,
			signed_prekey_sig TEXT NOT NULL,
			signed_prekey_id  INTEGER DEFAULT 0,
			updated_at        TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS one_time_prekeys (
			id         TEXT PRIMARY KEY,
			did        TEXT NOT NULL,
			opk_id     INTEGER NOT NULL,
			public_key TEXT NOT NULL,
			used       INTEGER DEFAULT 0,
			created_at TEXT NOT NULL,
			used_at    TEXT
		);
	`)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

// ─── SessionStore tests ───────────────────────────────────────────────────────

func TestSaveAndGetSession(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	store := signal.NewSessionStore(db)

	const owner = "did:obs:alice"
	const peer = "did:obs:bob"
	const blob = "aGVsbG8gd29ybGQ=" // base64("hello world")

	if err := store.SaveSession(owner, peer, blob); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	got, err := store.GetSession(owner, peer)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got != blob {
		t.Errorf("GetSession: got %q, want %q", got, blob)
	}
}

func TestSaveSession_Upsert(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	store := signal.NewSessionStore(db)

	const owner = "did:obs:alice"
	const peer = "did:obs:bob"

	if err := store.SaveSession(owner, peer, "blob_v1"); err != nil {
		t.Fatalf("first SaveSession: %v", err)
	}
	if err := store.SaveSession(owner, peer, "blob_v2"); err != nil {
		t.Fatalf("upsert SaveSession: %v", err)
	}

	got, _ := store.GetSession(owner, peer)
	if got != "blob_v2" {
		t.Errorf("expected blob_v2 after upsert, got %q", got)
	}
}

func TestSaveSession_EmptyBlobRejected(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	store := signal.NewSessionStore(db)

	err := store.SaveSession("did:obs:a", "did:obs:b", "")
	if err == nil {
		t.Fatal("expected error for empty blob, got nil")
	}
}

func TestSaveSession_OversizeBlobRejected(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	store := signal.NewSessionStore(db)

	big := make([]byte, 65537)
	for i := range big {
		big[i] = 'A'
	}
	err := store.SaveSession("did:obs:a", "did:obs:b", string(big))
	if err == nil {
		t.Fatal("expected error for oversized blob, got nil")
	}
}

func TestGetSession_NotFound(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	store := signal.NewSessionStore(db)

	_, err := store.GetSession("did:obs:nobody", "did:obs:nobody2")
	if err == nil {
		t.Fatal("expected error for non-existent session, got nil")
	}
}

func TestDeleteSession(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	store := signal.NewSessionStore(db)

	const owner = "did:obs:alice"
	const peer = "did:obs:bob"

	store.SaveSession(owner, peer, "someblob")

	if err := store.DeleteSession(owner, peer); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	_, err := store.GetSession(owner, peer)
	if err == nil {
		t.Fatal("expected error after delete, got nil")
	}
}

func TestDeleteSession_Idempotent(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	store := signal.NewSessionStore(db)

	// Delete a session that does not exist — must not error.
	if err := store.DeleteSession("did:obs:x", "did:obs:y"); err != nil {
		t.Fatalf("DeleteSession on non-existent: %v", err)
	}
}

func TestListSessions(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	store := signal.NewSessionStore(db)

	const owner = "did:obs:alice"
	store.SaveSession(owner, "did:obs:bob", "blob1")
	store.SaveSession(owner, "did:obs:carol", "blob2")

	sessions, err := store.ListSessions(owner)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestListSessions_EmptyForUnknownOwner(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	store := signal.NewSessionStore(db)

	sessions, err := store.ListSessions("did:obs:nobody")
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

// ─── PreKeyBundle tests ───────────────────────────────────────────────────────

func seedPrekeyBundle(t *testing.T, db *sql.DB, did string) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO prekey_bundles (did, identity_key, signed_prekey, signed_prekey_sig, signed_prekey_id, updated_at)
		VALUES (?, 'ik_base64', 'spk_base64', 'sig_base64', 1, datetime('now'))`, did)
	if err != nil {
		t.Fatalf("seed prekey_bundles: %v", err)
	}
}

func seedOPK(t *testing.T, db *sql.DB, did string, opkID int) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO one_time_prekeys (id, did, opk_id, public_key, used, created_at)
		VALUES (?, ?, ?, 'opk_pub_base64', 0, datetime('now'))`,
		"opk-"+did+"-1", did, opkID)
	if err != nil {
		t.Fatalf("seed one_time_prekeys: %v", err)
	}
}

func TestGetPrekeyBundle_WithOPK(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	store := signal.NewSessionStore(db)

	const did = "did:obs:bob"
	seedPrekeyBundle(t, db, did)
	seedOPK(t, db, did, 42)

	bundle, err := store.GetPrekeyBundle(did)
	if err != nil {
		t.Fatalf("GetPrekeyBundle: %v", err)
	}
	if bundle.DID != did {
		t.Errorf("bundle.DID: got %q, want %q", bundle.DID, did)
	}
	if bundle.IdentityKey != "ik_base64" {
		t.Errorf("bundle.IdentityKey: got %q", bundle.IdentityKey)
	}
	if bundle.OneTimePreKey != "opk_pub_base64" {
		t.Errorf("expected OPK in bundle, got %q", bundle.OneTimePreKey)
	}
	if bundle.OneTimePreKeyID != 42 {
		t.Errorf("expected OPK ID 42, got %d", bundle.OneTimePreKeyID)
	}

	// OPK must now be marked used.
	var used int
	db.QueryRow("SELECT used FROM one_time_prekeys WHERE did = ? AND opk_id = ?", did, 42).Scan(&used)
	if used != 1 {
		t.Errorf("OPK should be marked used after bundle fetch, got used=%d", used)
	}
}

func TestGetPrekeyBundle_WithoutOPK(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	store := signal.NewSessionStore(db)

	const did = "did:obs:carol"
	seedPrekeyBundle(t, db, did)
	// No OPK seeded — bundle should still be returned (graceful degradation).

	bundle, err := store.GetPrekeyBundle(did)
	if err != nil {
		t.Fatalf("GetPrekeyBundle without OPK: %v", err)
	}
	if bundle.OneTimePreKey != "" {
		t.Errorf("expected empty OPK when pool is empty, got %q", bundle.OneTimePreKey)
	}
}

func TestGetPrekeyBundle_NotFound(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	store := signal.NewSessionStore(db)

	_, err := store.GetPrekeyBundle("did:obs:nobody")
	if err == nil {
		t.Fatal("expected error for missing bundle, got nil")
	}
}

func TestGetPrekeyBundle_OPKConsumedOnce(t *testing.T) {
	// Two concurrent fetchers must each get a different OPK (or one gets none).
	// With DB.SetMaxOpenConns(1) there is no actual race, but we verify the
	// used-flag prevents a second consumer from getting the same OPK.
	db := openTestDB(t)
	defer db.Close()
	store := signal.NewSessionStore(db)

	const did = "did:obs:dave"
	seedPrekeyBundle(t, db, did)
	seedOPK(t, db, did, 1)

	b1, _ := store.GetPrekeyBundle(did)
	b2, _ := store.GetPrekeyBundle(did)

	if b1.OneTimePreKey != "" && b2.OneTimePreKey != "" {
		t.Error("both fetches got an OPK — the same OPK was handed out twice")
	}
}
