package dbi

import "os"

// Supported drivers. Kept here (not in internal/db) so any package can
// read the active driver without risking an import cycle back into
// internal/db (which imports internal/zk, internal/identity, ...).
const (
	DriverSQLite   = "sqlite"
	DriverPostgres = "postgres"
)

// DriverFromEnv reads OBSCURA_DB_DRIVER; empty/unrecognized values default
// to sqlite. This is the single source of truth for "which driver is this
// process configured for" — internal/db.Init() reads it to pick the actual
// connection, and any other package (federation, dao, storage, ...) that
// needs to emit driver-specific DDL reads the same value here rather than
// inspecting a specific dbi.Querier handle's concrete type.
func DriverFromEnv() string {
	if os.Getenv("OBSCURA_DB_DRIVER") == DriverPostgres {
		return DriverPostgres
	}
	return DriverSQLite
}
