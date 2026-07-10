// Package subscriber implements the legally-accessible subscriber identity
// store for Obscura — the "identity at the door" layer. It is deliberately
// kept in a SEPARATE SQLite database file (subscriber.db) from the message
// content plane (obscura.db). The message/conversation tables never reference
// phone in any form; this package is the only place a phone (hashed, then
// encrypted) is retained, gated behind an audited access path (access.go).
//
// Threat model summary:
//   - phone is HMAC-hashed (phone_hash.go) so plaintext numbers never rest here.
//   - the hash is then AES-256-GCM encrypted at the field level (crypto.go).
//   - every read of sensitive columns is forced through GetSubscriberData,
//     which writes an audit_log row in the same transaction (access.go).
package subscriber

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// subscriberDBFile is the on-disk name for the identity store, kept separate
// from the main obscura.db message plane.
const subscriberDBFile = "subscriber.db"

// connMaxLifetime mirrors the main DB's connection recycling window.
const connMaxLifetime = 30 * time.Minute

// OpenSubscriberDB opens (creating if absent) the subscriber identity database
// in dataDir, applying the same connection pragmas the main DB uses: WAL,
// foreign_keys on, busy_timeout, single-writer via MaxOpenConns(1).
func OpenSubscriberDB(dataDir string) (*sql.DB, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("subscriber veri dizini oluşturulamadı: %w", err)
	}

	dbPath := filepath.Join(dataDir, subscriberDBFile)
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=10000&_synchronous=NORMAL")
	if err != nil {
		return nil, fmt.Errorf("subscriber veritabanı açılamadı: %w", err)
	}

	// SQLite single-writer semantics — identical to the main DB layer.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(connMaxLifetime)

	if _, err := db.Exec("PRAGMA busy_timeout = 15000"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("subscriber busy_timeout ayarlanamadı: %w", err)
	}

	if err := createSubscriberTables(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// createSubscriberTables is idempotent (IF NOT EXISTS) DDL for the two tables
// that make up the identity store.
//
//   - subscribers: the identity rows. The _enc suffixed columns are
//     AES-256-GCM ciphertext blobs (12-byte nonce prepended), NOT plaintext.
//     registration_ip_enc is nullable: it is empty for rows backfilled by the
//     phone->subscriber migration, since IP was not historically tracked.
//   - audit_log: append-only access log. Exactly one row is written per
//     GetSubscriberData call, in the same transaction as the read.
func createSubscriberTables(db *sql.DB) error {
	const schema = `
	CREATE TABLE IF NOT EXISTS subscribers (
		did                    TEXT PRIMARY KEY,
		phone_hash_enc         BLOB NOT NULL,
		registration_ip_enc    BLOB,
		registration_timestamp TEXT NOT NULL,
		last_seen              TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS audit_log (
		id           TEXT PRIMARY KEY,
		operator     TEXT NOT NULL,
		accessed_at  TEXT NOT NULL,
		did          TEXT NOT NULL,
		legal_reason TEXT NOT NULL,
		action       TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_audit_did ON audit_log(did, accessed_at DESC);
	`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("subscriber tabloları oluşturulamadı: %w", err)
	}
	return nil
}
