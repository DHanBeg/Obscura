package subscriber

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"time"
)

// phoneNotNullRe matches the phone column's NOT NULL constraint in the users
// CREATE TABLE statement, capturing everything up to (but not including)
// "NOT NULL" so it can be dropped while preserving type/UNIQUE.
var phoneNotNullRe = regexp.MustCompile(`(?i)(phone\s+TEXT[^,\n]*?)\s+NOT NULL`)

// createUsersTableRe locates the "CREATE TABLE [IF NOT EXISTS] users" prefix so
// the rebuild can retarget it to a temporary table.
var createUsersTableRe = regexp.MustCompile(`(?i)CREATE TABLE (IF NOT EXISTS )?["']?users["']?`)

// MigratePhoneToSubscriberStore performs the one-time, idempotent backfill that
// moves plaintext phone numbers out of the main DB's users table into the
// encrypted subscriber identity store.
//
// For each users row not yet migrated it: HMAC-hashes the phone (pepper),
// AES-256-GCM encrypts the hash, inserts a subscribers row (registration_ip is
// NULL — IP was not historically tracked, a known backfill limitation), then
// NULLs users.phone and sets users.phone_migrated = 1.
//
// Idempotent: already-migrated rows (phone IS NULL / phone_migrated = 1) are
// skipped, and the subscribers INSERT is OR IGNORE keyed on did, so a second
// run is a no-op. Safe to call on every boot.
//
// Crypto must be initialized (InitCrypto/InitCryptoFromEnv) before calling.
func MigratePhoneToSubscriberStore(mainDB *sql.DB, subDB *sql.DB, pepper []byte) error {
	if mainDB == nil || subDB == nil {
		return errors.New("subscriber: migrate requires non-nil mainDB and subDB")
	}
	if aead == nil {
		return ErrCryptoNotInitialized
	}

	// users.phone is `TEXT UNIQUE NOT NULL` in production; NULLing it requires
	// relaxing NOT NULL first. Done here (not in database.go) to keep the main
	// schema diff to the single additive phone_migrated column.
	if err := ensurePhoneNullable(mainDB); err != nil {
		return err
	}

	// Collect first, then write: mainDB runs with MaxOpenConns(1), so holding a
	// read cursor open while issuing UPDATEs on the same handle would deadlock.
	type pending struct {
		did       string
		phone     string
		createdAt string
	}
	rows, err := mainDB.Query(
		`SELECT did, phone, created_at FROM users
		 WHERE phone IS NOT NULL AND phone_migrated = 0`)
	if err != nil {
		return fmt.Errorf("subscriber: migrate select: %w", err)
	}
	var todo []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.did, &p.phone, &p.createdAt); err != nil {
			_ = rows.Close()
			return fmt.Errorf("subscriber: migrate scan: %w", err)
		}
		todo = append(todo, p)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("subscriber: migrate rows: %w", err)
	}
	_ = rows.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	for _, p := range todo {
		enc, err := EncryptField(HashPhone(p.phone, pepper))
		if err != nil {
			return fmt.Errorf("subscriber: migrate encrypt (%s): %w", p.did, err)
		}

		// registration_ip_enc left NULL for backfilled rows (see doc above).
		if _, err := subDB.Exec(
			`INSERT OR IGNORE INTO subscribers
			 (did, phone_hash_enc, registration_ip_enc, registration_timestamp, last_seen)
			 VALUES (?, ?, NULL, ?, ?)`,
			p.did, enc, p.createdAt, now,
		); err != nil {
			return fmt.Errorf("subscriber: migrate insert (%s): %w", p.did, err)
		}

		// Only after the subscriber row is durably stored do we drop the
		// plaintext from users. A crash between the two leaves phone_migrated=0,
		// so the next run reprocesses (INSERT OR IGNORE makes it a no-op).
		if _, err := mainDB.Exec(
			`UPDATE users SET phone = NULL, phone_migrated = 1 WHERE did = ?`, p.did,
		); err != nil {
			return fmt.Errorf("subscriber: migrate update users (%s): %w", p.did, err)
		}
	}
	return nil
}

// ensurePhoneNullable relaxes the NOT NULL constraint on users.phone via the
// standard SQLite table-rebuild, so the backfill can set phone = NULL. It is a
// no-op once phone is already nullable (idempotent). The users table has no
// indexes, foreign keys, or triggers referencing it (verified), so the rebuild
// only needs to recreate the table itself.
func ensurePhoneNullable(db *sql.DB) error {
	notNull, err := phoneIsNotNull(db)
	if err != nil {
		return err
	}
	if !notNull {
		return nil // already nullable — nothing to do
	}

	var createSQL string
	if err := db.QueryRow(
		`SELECT sql FROM sqlite_master WHERE type='table' AND name='users'`,
	).Scan(&createSQL); err != nil {
		return fmt.Errorf("subscriber: read users schema: %w", err)
	}

	if !phoneNotNullRe.MatchString(createSQL) {
		return fmt.Errorf("subscriber: users.phone is NOT NULL but its definition did not match the expected pattern; refusing to guess the rebuild SQL")
	}

	// Retarget CREATE to a temp table and drop NOT NULL from phone only. All
	// other columns (including UNIQUE constraints) are preserved verbatim, so
	// `INSERT INTO users_new SELECT * FROM users` matches column-for-column.
	newSQL := createUsersTableRe.ReplaceAllString(createSQL, "CREATE TABLE users_new")
	newSQL = phoneNotNullRe.ReplaceAllString(newSQL, "$1")

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("subscriber: rebuild begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, stmt := range []string{
		newSQL,
		`INSERT INTO users_new SELECT * FROM users`,
		`DROP TABLE users`,
		`ALTER TABLE users_new RENAME TO users`,
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("subscriber: rebuild step failed (%.40q): %w", stmt, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("subscriber: rebuild commit: %w", err)
	}
	return nil
}

// phoneIsNotNull reports whether users.phone currently carries a NOT NULL
// constraint, via PRAGMA table_info.
func phoneIsNotNull(db *sql.DB) (bool, error) {
	rows, err := db.Query(`PRAGMA table_info(users)`)
	if err != nil {
		return false, fmt.Errorf("subscriber: table_info: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid     int
			name    string
			ctype   string
			notnull int
			dflt    sql.NullString
			pk      int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, fmt.Errorf("subscriber: table_info scan: %w", err)
		}
		if name == "phone" {
			return notnull == 1, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, errors.New("subscriber: users table has no phone column")
}
