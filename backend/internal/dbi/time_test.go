package dbi

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// TestNow_MatchesSQLiteDatetimeNowFormat pins dbi.Now() to the exact string
// shape SQLite's datetime('now') produces, empirically, against the real
// driver — not just a hardcoded layout string that could drift silently.
func TestNow_MatchesSQLiteDatetimeNowFormat(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	var sqliteNow string
	if err := db.QueryRow("SELECT datetime('now')").Scan(&sqliteNow); err != nil {
		t.Fatalf("query datetime('now'): %v", err)
	}

	got := Now()

	if len(got) != len(sqliteNow) {
		t.Fatalf("dbi.Now() length %d (%q) does not match datetime('now') length %d (%q)",
			len(got), got, len(sqliteNow), sqliteNow)
	}
	// Compare shape (digits/separators), not the exact value — both were
	// captured microseconds apart so the seconds field may legitimately
	// differ.
	for i := range got {
		gotIsDigit := got[i] >= '0' && got[i] <= '9'
		refIsDigit := sqliteNow[i] >= '0' && sqliteNow[i] <= '9'
		if gotIsDigit != refIsDigit {
			t.Fatalf("dbi.Now() shape mismatch at byte %d: got %q, sqlite %q", i, got, sqliteNow)
		}
		if !gotIsDigit && got[i] != sqliteNow[i] {
			t.Fatalf("dbi.Now() separator mismatch at byte %d: got %q, sqlite %q", i, got, sqliteNow)
		}
	}
}
