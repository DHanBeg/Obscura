package dbi

import "time"

// Now returns the current UTC time formatted exactly like SQLite's
// datetime('now') ("YYYY-MM-DD HH:MM:SS", verified empirically against
// modernc.org/sqlite). Bind it as a query parameter instead of embedding
// datetime('now') in SQL text: Postgres has no datetime('now') function
// (its equivalent, NOW(), returns a different type and format), so
// computing the timestamp in Go keeps the same query portable across
// both engines with zero runtime detection.
func Now() string {
	return time.Now().UTC().Format("2006-01-02 15:04:05")
}
