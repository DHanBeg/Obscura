---
name: sqlite-pure-go-migrations
description: SQLite via modernc.org/sqlite (CGO_ENABLED=0) + idempotent migration system for Obscura. Use for any database/ work or schema change.
---

# SQLite Pure-Go + Migrations

## Why pure Go (no CGO)

Spec rule: `CGO_ENABLED=0`. Reasons:
- Cross-compile from Linux to Windows/macOS without C toolchain
- Smaller, statically linked binaries
- Easier Docker images (no gcc/sqlite-dev in Alpine)

`modernc.org/sqlite` is a C-to-Go transpilation of SQLite, fully compatible API.

## Setup

```go
// backend/internal/db/database.go
package db

import (
    "database/sql"
    "fmt"
    "log"
    _ "modernc.org/sqlite"
)

var DB *sql.DB

func Init(path string) error {
    var err error
    DB, err = sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)")
    if err != nil {
        return fmt.Errorf("open db: %w", err)
    }
    DB.SetMaxOpenConns(25)
    DB.SetMaxIdleConns(10)

    if err := DB.Ping(); err != nil {
        return fmt.Errorf("ping: %w", err)
    }

    if err := createInitialSchema(); err != nil {
        return err
    }
    if err := runMigrations(); err != nil {
        return err
    }
    return nil
}
```

## Initial schema (idempotent)

```go
func createInitialSchema() error {
    schemas := []string{
        `CREATE TABLE IF NOT EXISTS users (
            did TEXT PRIMARY KEY,
            phone TEXT UNIQUE,
            username TEXT UNIQUE,
            display_name TEXT DEFAULT '',
            avatar_url TEXT DEFAULT '',
            tier INTEGER NOT NULL DEFAULT 1,
            created_at INTEGER NOT NULL DEFAULT (unixepoch()),
            updated_at INTEGER NOT NULL DEFAULT (unixepoch())
        )`,
        `CREATE INDEX IF NOT EXISTS idx_users_phone ON users(phone)`,
        `CREATE INDEX IF NOT EXISTS idx_users_username ON users(username)`,

        // ... rest of tables
    }
    for _, sch := range schemas {
        if _, err := DB.Exec(sch); err != nil {
            return fmt.Errorf("schema: %w", err)
        }
    }
    return nil
}
```

## Migration tracking

```go
func runMigrations() error {
    _, err := DB.Exec(`CREATE TABLE IF NOT EXISTS _migrations (
        id TEXT PRIMARY KEY,
        applied_at INTEGER NOT NULL DEFAULT (unixepoch())
    )`)
    if err != nil { return err }

    migrations := []struct{ id, sql string }{
        {"001_add_fcm_token", "ALTER TABLE users ADD COLUMN fcm_token TEXT DEFAULT ''"},
        {"002_add_apns_token", "ALTER TABLE users ADD COLUMN apns_token TEXT DEFAULT ''"},
        // append-only, never modify existing
    }

    for _, m := range migrations {
        var count int
        DB.QueryRow("SELECT COUNT(*) FROM _migrations WHERE id = ?", m.id).Scan(&count)
        if count > 0 { continue }

        if _, err := DB.Exec(m.sql); err != nil {
            // Idempotency for "duplicate column" from previous partial run
            if strings.Contains(err.Error(), "duplicate column") {
                log.Printf("migration %s skipped: column exists", m.id)
            } else {
                return fmt.Errorf("migration %s: %w", m.id, err)
            }
        }

        _, _ = DB.Exec("INSERT INTO _migrations(id) VALUES (?)", m.id)
        log.Printf("migration applied: %s", m.id)
    }
    return nil
}
```

## Adding a new migration

1. Append to `migrations` slice (never modify existing)
2. Use sequential numeric prefix: `003_`, `004_`, ...
3. Description short, snake_case
4. SQL idempotent where possible (`IF NOT EXISTS`, `IF EXISTS`)
5. Test on copy of prod DB before deploying

## SQLite limitations to know

- No native `ALTER TABLE DROP COLUMN` before SQLite 3.35 — need rebuild table
- No native `ALTER TABLE ALTER COLUMN` — need rebuild
- `PRAGMA foreign_keys` must be set on every connection
- WAL mode requires write access to dir for `.db-wal` and `.db-shm` files
- Concurrent writers serialized at SQLite layer (use connection pool)

## Rebuild table for breaking changes

```sql
BEGIN;
CREATE TABLE users_new (...new schema...);
INSERT INTO users_new SELECT ... FROM users;
DROP TABLE users;
ALTER TABLE users_new RENAME TO users;
CREATE INDEX ...;
COMMIT;
```

## Backup strategy

```bash
# Hot backup (no downtime)
sqlite3 obscura.db ".backup '/backups/obscura-$(date +%Y%m%d-%H%M%S).db'"

# Compressed
sqlite3 obscura.db ".backup '/tmp/b.db'" && gzip /tmp/b.db && mv /tmp/b.db.gz /backups/

# Restore
sqlite3 new.db ".restore /backups/obscura-XXXX.db"
```

## Performance tuning

```go
// On connection
?_pragma=journal_mode(WAL)
?_pragma=synchronous(NORMAL)        // FULL is slow, OFF unsafe
?_pragma=foreign_keys(ON)
?_pragma=busy_timeout(5000)         // wait 5s on lock
?_pragma=cache_size(-64000)         // 64MB cache
?_pragma=temp_store(MEMORY)
?_pragma=mmap_size(268435456)       // 256MB mmap
```

## Hard rules

- All queries parameterized (`?`)
- No `db.Exec(fmt.Sprintf(...))`
- Migrations append-only, never edit shipped ones
- Backup hourly, verify weekly
- Indexes on every WHERE / JOIN / ORDER BY column
