package messaging

// sealed_sender.go — message-envelope-facing entry point for sealed sender.
// Delegates to internal/signal, which owns the actual obscura-crypto-cli
// subprocess bridge (CGO_ENABLED=0: no cgo, os/exec only — see
// internal/signal/crypto_cli.go). Kept here, rather than duplicating the
// subprocess plumbing, so message envelope code has a Seal/Unseal it can call
// without importing internal/signal directly.

import "obscura.network/core/internal/signal"

// SealedSenderResult mirrors signal.UnsealResult at the messaging boundary.
type SealedSenderResult = signal.UnsealResult

// Seal builds a sealed-sender envelope hiding the sender's identity from the
// server: only the holder of recipientPubHex's matching private key can open
// it. senderIdentity is the sender's own secure-JSON identity (see
// signal.NewSecureIdentity); recipientPubHex is the recipient's 32-byte X25519
// identity public key (64 hex chars); payload is the plaintext to seal
// (typically an already ratchet-encrypted message); expiresAt is a
// unix-seconds certificate expiry (0 = no expiry).
func Seal(senderIdentity, recipientPubHex, payload string, expiresAt uint64) (envelopeHex string, err error) {
	return signal.Seal(senderIdentity, recipientPubHex, payload, expiresAt)
}

// Unseal opens an envelope built by Seal, recovering the sender's identity
// and the original payload.
//
// recipientIdentity MUST be the actual message recipient's own secure-JSON
// identity (private key material) — never a server-held identity. Sealed
// sender's entire purpose is that the server cannot learn the sender; calling
// Unseal server-side with a real recipient key would defeat that guarantee.
// This function exists for client-side use (or client-simulating tests), not
// for the live message hub to call on inbound traffic.
func Unseal(recipientIdentity, envelopeHex string, now uint64) (*SealedSenderResult, error) {
	return signal.Unseal(recipientIdentity, envelopeHex, now)
}
