// Package secrets is the single source of truth for loading required
// runtime secrets (C10 audit — R7/N10 kardeşi kopyalar, "fail-open kökü").
//
// Every copy of the old pattern (getEnv("KEY", "hardcoded-default"), or a
// bare os.Getenv comparison with no prod guard) either fell back to a secret
// literally visible in this public repo, or silently disabled auth entirely
// when the env var was forgotten. Both fail OPEN. This package fails CLOSED
// by default: anything that isn't an explicit, spelled-out dev opt-in is
// treated as production.
package secrets

import (
	"crypto/hmac"
	"log"
	"os"
)

// isDevEnv reports whether OBSCURA_ENV explicitly opts into development
// behavior. Fail-safe direction (C10 D1): the default — OBSCURA_ENV unset,
// empty, misspelled, or anything else — is treated as production. Forgetting
// to set OBSCURA_ENV must lock things down, never quietly relax them.
func isDevEnv() bool {
	switch os.Getenv("OBSCURA_ENV") {
	case "development", "dev":
		return true
	default:
		return false
	}
}

// IsDev reports whether OBSCURA_ENV explicitly opts into development
// behavior (see isDevEnv). Exported so callers that can't route a secret
// through Require/RequireEqual directly — e.g. binary key material that
// needs its own decode/format validation, where Require's generic
// dev-placeholder string would fail that validation — can still apply the
// same D1 fail-safe direction (default = production) themselves.
func IsDev() bool {
	return isDevEnv()
}

// Require reads envKey (or, in order, any alias) at call time. Outside an
// explicit dev opt-in (see isDevEnv), a missing value is FATAL — call this
// from a package-level var initializer (var x = secrets.Require(...)) so it
// runs once, at process start, and a misconfigured deploy never boots. In
// dev it logs a warning and returns a fixed, clearly-marked placeholder so
// local development still works without any real secret configured.
func Require(envKey string, aliases ...string) string {
	for _, k := range append([]string{envKey}, aliases...) {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	if !isDevEnv() {
		log.Fatalf("%s env required (OBSCURA_ENV is not an explicit dev opt-in)", envKey)
	}
	log.Printf("⚠ %s not set — using dev placeholder", envKey)
	return "dev-only-placeholder-not-for-prod"
}

// RequireEqual does a constant-time comparison between a secret loaded via
// Require and a value received over the wire (e.g. an X-Internal-Secret
// request header). Unlike a bare `expected != "" && got != expected` check,
// this can never fail open: `loaded` came from Require, which is fatal in
// production when unset — it is never the empty string here in a running
// production process, so there is no "unset → skip the check" branch left
// to accidentally take.
func RequireEqual(loaded, got string) bool {
	return hmac.Equal([]byte(loaded), []byte(got))
}
