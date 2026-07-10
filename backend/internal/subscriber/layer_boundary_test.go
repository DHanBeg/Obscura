package subscriber_test

// Layer boundary invariant: "identity at the door, privacy in the room."
// The message content plane must never reference phone in any form, and must
// never import the subscriber identity package. These static checks encode the
// invariant so it cannot silently regress.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// subscriberImportPath is the import path the message plane must never pull in.
const subscriberImportPath = "obscura.network/core/internal/subscriber"

// messagePlaneDirs are the packages that make up the message/conversation plane.
// Paths are relative to this test's directory (internal/subscriber).
var messagePlaneDirs = []string{
	filepath.Join("..", "messaging"),
	filepath.Join("..", "api"),
}

func TestMessagePlaneDoesNotImportSubscriber(t *testing.T) {
	for _, dir := range messagePlaneDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read dir %s: %v", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			if strings.Contains(string(data), subscriberImportPath) {
				t.Errorf("%s references the subscriber package (%q); the message plane must stay isolated from the identity store",
					path, subscriberImportPath)
			}
		}
	}
}

func TestMessageAndConversationSchemasHaveNoPhoneColumn(t *testing.T) {
	dbPath := filepath.Join("..", "db", "database.go")
	data, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read %s: %v", dbPath, err)
	}
	src := string(data)

	for _, table := range []string{"messages", "conversations", "conv_members"} {
		block := extractCreateTable(t, src, table)
		if strings.Contains(strings.ToLower(block), "phone") {
			t.Errorf("CREATE TABLE %s contains a phone-related column; the message plane must never reference phone:\n%s",
				table, block)
		}
	}
}

// fromSubscribersRe matches a raw SQL read (SELECT ... FROM subscribers) so it
// can be located in source outside access.go. INSERT/UPDATE statements (writes)
// deliberately do not match this pattern — only reads of the sensitive table
// are restricted to the audited access path.
var fromSubscribersRe = regexp.MustCompile(`(?is)\bFROM\s+subscribers\b`)

// TestOnlyAccessGoReadsSubscribersTable locks the invariant documented in
// access.go: GetSubscriberData (access.go) is the sole place in the package
// that runs a SELECT/QueryRow against the subscribers table. Every other
// production source file in this directory must be free of "FROM subscribers".
func TestOnlyAccessGoReadsSubscribersTable(t *testing.T) {
	dir := "."
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		if name == "access.go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if fromSubscribersRe.MatchString(string(data)) {
			t.Errorf("%s contains a raw SELECT/QueryRow against subscribers; only access.go (GetSubscriberData) may read that table", name)
		}
	}
}

// extractCreateTable returns the CREATE TABLE ... (...) block for the given
// table name from database.go source.
func extractCreateTable(t *testing.T, src, table string) string {
	t.Helper()
	marker := "CREATE TABLE IF NOT EXISTS " + table + " ("
	start := strings.Index(src, marker)
	if start < 0 {
		t.Fatalf("could not locate %q in database.go", marker)
	}
	rest := src[start:]
	end := strings.Index(rest, ");")
	if end < 0 {
		t.Fatalf("could not find end of CREATE TABLE %s", table)
	}
	return rest[:end+2]
}
