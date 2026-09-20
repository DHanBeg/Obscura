package api

import "sync"

// DIDResolver resolves a user's DID from their phone number. It is defined
// here, on the consumer side, so the message plane (this package) never has
// to import the subscriber identity store -- see
// subscriber/layer_boundary_test.go. The composition root (cmd/node) injects
// the concrete implementation via SetDIDResolver.
type DIDResolver interface {
	FindDIDByPhone(phone string) (string, error)
}

var (
	didResolverMu sync.RWMutex
	didResolver   DIDResolver
)

// SetDIDResolver installs the resolver used by HandleVerifyOTP's fallback
// lookup. Passing nil disables the fallback.
func SetDIDResolver(r DIDResolver) {
	didResolverMu.Lock()
	defer didResolverMu.Unlock()
	didResolver = r
}

// findDIDByPhone returns "" when no resolver is installed or the lookup
// fails; callers treat that as "not found".
func findDIDByPhone(phone string) string {
	didResolverMu.RLock()
	r := didResolver
	didResolverMu.RUnlock()
	if r == nil {
		return ""
	}
	did, err := r.FindDIDByPhone(phone)
	if err != nil {
		return ""
	}
	return did
}
