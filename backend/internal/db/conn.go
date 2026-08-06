package db

import (
	"context"
	"database/sql"
	"os"

	"github.com/jmoiron/sqlx"

	"obscura.network/core/internal/dbi"
)

// Desteklenen sürücüler. Postgres şu an yalnızca placeholder rebind
// altyapısı olarak var — createTables()/runMigrations() hâlâ SQLite'a
// özgü söz dizimi (PRAGMA, INSERT OR IGNORE, datetime('now')) kullanıyor,
// o yüzden Init() postgres seçilirse şimdilik açıkça hata döner (bkz. altta).
const (
	DriverSQLite   = "sqlite"
	DriverPostgres = "postgres"
)

// driverFromEnv OBSCURA_DB_DRIVER'ı okur; boş/tanınmayan değerde sqlite
// varsayılan olur.
func driverFromEnv() string {
	if os.Getenv("OBSCURA_DB_DRIVER") == DriverPostgres {
		return DriverPostgres
	}
	return DriverSQLite
}

// rebind sqlite için no-op (aynı query string döner), postgres için
// ?, ?, ... yer tutucularını $1, $2, ...'e çevirir.
func rebind(driver, query string) string {
	if driver != DriverPostgres {
		return query
	}
	return sqlx.Rebind(sqlx.DOLLAR, query)
}

// Conn *sql.DB'yi sarar. Exec/Query/QueryRow ailesi çağrılmadan önce query
// string aktif driver'a göre rebind edilir; sqlite'ta davranış değişmez.
// Diğer tüm *sql.DB metodları (Close, Ping, SetMaxOpenConns, ...) embed
// üzerinden olduğu gibi promote edilir.
type Conn struct {
	*sql.DB
	driver string
}

func newConn(sqlDB *sql.DB, driver string) *Conn {
	return &Conn{DB: sqlDB, driver: driver}
}

func (c *Conn) Exec(query string, args ...any) (sql.Result, error) {
	return c.DB.Exec(rebind(c.driver, query), args...)
}

func (c *Conn) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return c.DB.ExecContext(ctx, rebind(c.driver, query), args...)
}

func (c *Conn) Query(query string, args ...any) (*sql.Rows, error) {
	return c.DB.Query(rebind(c.driver, query), args...)
}

func (c *Conn) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return c.DB.QueryContext(ctx, rebind(c.driver, query), args...)
}

func (c *Conn) QueryRow(query string, args ...any) *sql.Row {
	return c.DB.QueryRow(rebind(c.driver, query), args...)
}

func (c *Conn) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return c.DB.QueryRowContext(ctx, rebind(c.driver, query), args...)
}

// Begin/BeginTx kendi Tx sarmalayıcımızı döner ki transaction içindeki
// Exec/Query çağrıları da aynı rebind'den geçsin.
func (c *Conn) Begin() (*Tx, error) {
	tx, err := c.DB.Begin()
	if err != nil {
		return nil, err
	}
	return &Tx{Tx: tx, driver: c.driver}, nil
}

func (c *Conn) BeginTx(ctx context.Context, opts *sql.TxOptions) (*Tx, error) {
	tx, err := c.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &Tx{Tx: tx, driver: c.driver}, nil
}

// Tx *sql.Tx sarmalayıcısı, Conn ile aynı rebind davranışı. Commit/Rollback
// embed üzerinden olduğu gibi promote edilir.
type Tx struct {
	*sql.Tx
	driver string
}

func (t *Tx) Exec(query string, args ...any) (sql.Result, error) {
	return t.Tx.Exec(rebind(t.driver, query), args...)
}

func (t *Tx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return t.Tx.ExecContext(ctx, rebind(t.driver, query), args...)
}

func (t *Tx) Query(query string, args ...any) (*sql.Rows, error) {
	return t.Tx.Query(rebind(t.driver, query), args...)
}

func (t *Tx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return t.Tx.QueryContext(ctx, rebind(t.driver, query), args...)
}

func (t *Tx) QueryRow(query string, args ...any) *sql.Row {
	return t.Tx.QueryRow(rebind(t.driver, query), args...)
}

func (t *Tx) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return t.Tx.QueryRowContext(ctx, rebind(t.driver, query), args...)
}

// Derleme zamanı kontrolü: Conn ve Tx, dbi.Querier'i sağlamalı.
var (
	_ dbi.Querier = (*Conn)(nil)
	_ dbi.Querier = (*Tx)(nil)
)
