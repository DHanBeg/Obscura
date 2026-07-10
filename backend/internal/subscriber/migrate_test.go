package subscriber

import (
	"bytes"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// newMainDBFixture builds a temp main DB whose users table mirrors production:
// phone is `TEXT UNIQUE NOT NULL` and a phone_migrated column exists.
func newMainDBFixture(t *testing.T) *sql.DB {
	t.Helper()

	path := filepath.Join(t.TempDir(), "main.db")
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=10000")
	if err != nil {
		t.Fatalf("open main db: %v", err)
	}
	db.SetMaxOpenConns(1) // mimic production single-writer
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id             TEXT PRIMARY KEY,
			phone          TEXT UNIQUE NOT NULL,
			username       TEXT UNIQUE,
			did            TEXT UNIQUE NOT NULL,
			created_at     TEXT NOT NULL,
			phone_migrated INTEGER DEFAULT 0
		);`); err != nil {
		t.Fatalf("create users: %v", err)
	}

	seed := []struct{ id, phone, username, did, created string }{
		{"u1", "+905551112233", "alice", "did:obs:alice", "2026-01-01T00:00:00Z"},
		{"u2", "+905559998877", "bob", "did:obs:bob", "2026-02-02T00:00:00Z"},
	}
	for _, s := range seed {
		if _, err := db.Exec(
			`INSERT INTO users (id, phone, username, did, created_at) VALUES (?, ?, ?, ?, ?)`,
			s.id, s.phone, s.username, s.did, s.created,
		); err != nil {
			t.Fatalf("seed user %s: %v", s.id, err)
		}
	}
	return db
}

func TestMigratePhoneToSubscriberStore(t *testing.T) {
	if err := InitCrypto(testKey(t, 5)); err != nil {
		t.Fatalf("InitCrypto: %v", err)
	}
	pepper := []byte("migration-test-pepper")

	mainDB := newMainDBFixture(t)
	subDB, err := OpenSubscriberDB(t.TempDir())
	if err != nil {
		t.Fatalf("OpenSubscriberDB: %v", err)
	}
	t.Cleanup(func() { _ = subDB.Close() })

	// Act
	if err := MigratePhoneToSubscriberStore(mainDB, subDB, pepper); err != nil {
		t.Fatalf("migration: %v", err)
	}

	// Assert: both phones removed from users, marked migrated.
	assertUsersMigrated(t, mainDB, 2)

	// Assert: subscriber rows carry the correct encrypted hash.
	assertSubscriberHash(t, subDB, "did:obs:alice", HashPhone("+905551112233", pepper))
	assertSubscriberHash(t, subDB, "did:obs:bob", HashPhone("+905559998877", pepper))

	// registration_ip_enc is NULL for backfilled rows.
	var ipEnc []byte
	if err := subDB.QueryRow(
		`SELECT registration_ip_enc FROM subscribers WHERE did = 'did:obs:alice'`,
	).Scan(&ipEnc); err != nil {
		t.Fatalf("read ip: %v", err)
	}
	if ipEnc != nil {
		t.Fatalf("expected NULL registration_ip_enc for backfilled row, got %d bytes", len(ipEnc))
	}
	// registration_timestamp carries over users.created_at.
	var regTs string
	if err := subDB.QueryRow(
		`SELECT registration_timestamp FROM subscribers WHERE did = 'did:obs:alice'`,
	).Scan(&regTs); err != nil {
		t.Fatalf("read reg ts: %v", err)
	}
	if regTs != "2026-01-01T00:00:00Z" {
		t.Fatalf("registration_timestamp = %q, want users.created_at", regTs)
	}

	// Act again: second run must be a no-op (idempotent).
	if err := MigratePhoneToSubscriberStore(mainDB, subDB, pepper); err != nil {
		t.Fatalf("second migration: %v", err)
	}
	assertUsersMigrated(t, mainDB, 2)
	if got := countRows(t, subDB, "SELECT COUNT(*) FROM subscribers"); got != 2 {
		t.Fatalf("subscribers row count after second run = %d, want 2 (no duplicates)", got)
	}
}

func TestMigrateNoUsersIsNoop(t *testing.T) {
	if err := InitCrypto(testKey(t, 6)); err != nil {
		t.Fatalf("InitCrypto: %v", err)
	}
	// users table exists but empty.
	path := filepath.Join(t.TempDir(), "empty.db")
	mainDB, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer mainDB.Close()
	if _, err := mainDB.Exec(`CREATE TABLE users (
		id TEXT PRIMARY KEY, phone TEXT UNIQUE NOT NULL, did TEXT UNIQUE NOT NULL,
		created_at TEXT NOT NULL, phone_migrated INTEGER DEFAULT 0);`); err != nil {
		t.Fatalf("create users: %v", err)
	}

	subDB, err := OpenSubscriberDB(t.TempDir())
	if err != nil {
		t.Fatalf("OpenSubscriberDB: %v", err)
	}
	defer subDB.Close()

	if err := MigratePhoneToSubscriberStore(mainDB, subDB, []byte("p")); err != nil {
		t.Fatalf("migration on empty users: %v", err)
	}
	if got := countRows(t, subDB, "SELECT COUNT(*) FROM subscribers"); got != 0 {
		t.Fatalf("subscribers = %d, want 0", got)
	}
}

// TestMigrateRecoversFromCrashBetweenInsertAndUpdate simulates the exact
// window documented in migrate.go: the subscribers INSERT lands durably, then
// the process dies before the users UPDATE (phone=NULL, phone_migrated=1)
// runs. A subsequent run must catch users up without erroring or duplicating
// the subscriber row (relies on INSERT OR IGNORE keyed on did).
func TestMigrateRecoversFromCrashBetweenInsertAndUpdate(t *testing.T) {
	if err := InitCrypto(testKey(t, 8)); err != nil {
		t.Fatalf("InitCrypto: %v", err)
	}
	pepper := []byte("crash-test-pepper")

	mainDB := newMainDBFixture(t)
	subDB, err := OpenSubscriberDB(t.TempDir())
	if err != nil {
		t.Fatalf("OpenSubscriberDB: %v", err)
	}
	t.Cleanup(func() { _ = subDB.Close() })

	// Pre-seed the subscribers row as if the first run's INSERT had already
	// committed for alice, but the crash happened before the users UPDATE.
	// mainDB still shows alice's phone set and phone_migrated = 0.
	enc, err := EncryptField(HashPhone("+905551112233", pepper))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := subDB.Exec(
		`INSERT INTO subscribers (did, phone_hash_enc, registration_ip_enc, registration_timestamp, last_seen)
		 VALUES (?, ?, NULL, ?, ?)`,
		"did:obs:alice", enc, "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z",
	); err != nil {
		t.Fatalf("seed pre-crash subscriber row: %v", err)
	}

	// Act: the retry run must finish alice's dangling update and process bob
	// normally, without erroring on the already-present subscriber row.
	if err := MigratePhoneToSubscriberStore(mainDB, subDB, pepper); err != nil {
		t.Fatalf("retry migration: %v", err)
	}

	// Assert: users side caught up for both.
	assertUsersMigrated(t, mainDB, 2)
	// Assert: INSERT OR IGNORE prevented a duplicate row for alice.
	if got := countRows(t, subDB, "SELECT COUNT(*) FROM subscribers WHERE did = 'did:obs:alice'"); got != 1 {
		t.Fatalf("subscribers rows for alice = %d, want 1 (retry must not duplicate)", got)
	}
	if got := countRows(t, subDB, "SELECT COUNT(*) FROM subscribers"); got != 2 {
		t.Fatalf("total subscribers = %d, want 2", got)
	}
}

func assertUsersMigrated(t *testing.T, db *sql.DB, wantMigrated int) {
	t.Helper()
	// No plaintext phones remain.
	if got := countRows(t, db, "SELECT COUNT(*) FROM users WHERE phone IS NOT NULL"); got != 0 {
		t.Fatalf("%d users still have a non-NULL phone, want 0", got)
	}
	if got := countRows(t, db, "SELECT COUNT(*) FROM users WHERE phone_migrated = 1"); got != wantMigrated {
		t.Fatalf("%d users marked phone_migrated=1, want %d", got, wantMigrated)
	}
}

func assertSubscriberHash(t *testing.T, subDB *sql.DB, did string, wantHash []byte) {
	t.Helper()
	var enc []byte
	if err := subDB.QueryRow(
		`SELECT phone_hash_enc FROM subscribers WHERE did = ?`, did,
	).Scan(&enc); err != nil {
		t.Fatalf("read subscriber %s: %v", did, err)
	}
	got, err := DecryptField(enc)
	if err != nil {
		t.Fatalf("decrypt %s: %v", did, err)
	}
	if !bytes.Equal(got, wantHash) {
		t.Fatalf("%s hash mismatch: got %x want %x", did, got, wantHash)
	}
}

func countRows(t *testing.T, db *sql.DB, query string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query).Scan(&n); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return n
}
