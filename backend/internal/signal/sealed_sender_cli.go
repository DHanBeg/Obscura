package signal

// sealed_sender_cli.go — subprocess bridge to the Rust obscura-crypto-cli
// `seal` / `unseal` / `identity-new-secure` subcommands. Same bridge as
// crypto_cli.go (CGO_ENABLED=0 stays intact — no C dependency, os/exec only).

import (
	"encoding/hex"
	"fmt"
)

// UnsealResult is the decrypted view of a sealed-sender envelope: the
// recovered sender identity plus the original payload.
type UnsealResult struct {
	SenderDID            string
	SenderIdentityPubHex string // sender's X25519 identity public key (hex)
	SenderSigningPubHex  string // sender's Ed25519 signing public key (hex) — a DIFFERENT key from the one above
	Payload              string
}

// NewSecureIdentity generates a fresh IdentityKeyPair in "secure JSON" form —
// the format Seal/Unseal require for their identity arguments. This is a
// distinct representation from GenerateKeypair's raw hex blob (used by
// x3dh/ratchet); the two are not interchangeable.
func NewSecureIdentity() (identityJSON, did string, err error) {
	r, err := runCryptoCLI("identity-new-secure")
	if err != nil {
		return "", "", err
	}
	if identityJSON, err = strField(r, "identity_json"); err != nil {
		return "", "", err
	}
	if did, err = strField(r, "did"); err != nil {
		return "", "", err
	}
	return identityJSON, did, nil
}

// IdentityPubHex extracts the hex-encoded DH (X25519) and signing (Ed25519)
// public keys plus the DID from a secure-JSON identity (see NewSecureIdentity).
// dhPubHex is the value Seal expects as recipientPubHex.
func IdentityPubHex(identityJSON string) (did, dhPubHex, signingPubHex string, err error) {
	r, err := runCryptoCLI("identity-pub-hex", "--identity-json", identityJSON)
	if err != nil {
		return "", "", "", err
	}
	if did, err = strField(r, "did"); err != nil {
		return "", "", "", err
	}
	if dhPubHex, err = strField(r, "dh_public_hex"); err != nil {
		return "", "", "", err
	}
	if signingPubHex, err = strField(r, "signing_public_hex"); err != nil {
		return "", "", "", err
	}
	return did, dhPubHex, signingPubHex, nil
}

// Seal builds a sealed-sender envelope: payload encrypted so only the holder
// of recipientPubHex's matching private key can open it, with the sender's
// identity never appearing in the clear. senderIdentityJSON is the sender's
// own secure-JSON identity (see NewSecureIdentity); recipientPubHex is the
// recipient's 32-byte X25519 identity public key (64 hex chars); payload is
// the plaintext to seal (typically an already ratchet-encrypted message);
// expiresAt is a unix-seconds certificate expiry (0 = no expiry).
func Seal(senderIdentityJSON, recipientPubHex, payload string, expiresAt uint64) (envelopeHex string, err error) {
	r, err := runCryptoCLI("seal",
		"--sender-identity-json", senderIdentityJSON,
		"--recipient-identity-pub-hex", recipientPubHex,
		"--payload-hex", hex.EncodeToString([]byte(payload)),
		"--expires-at", fmt.Sprintf("%d", expiresAt))
	if err != nil {
		return "", err
	}
	return strField(r, "envelope_hex")
}

// Unseal opens an envelope produced by Seal, recovering the sender's identity
// and the original payload.
//
// recipientIdentityJSON MUST be the actual message recipient's own secure-JSON
// identity (private key material). Sealed sender's entire purpose is hiding
// the sender from the server, so this must never be called server-side with
// a server-held identity — only the true recipient, client-side, should ever
// unseal. now is unix seconds, used for certificate expiry checks.
func Unseal(recipientIdentityJSON, envelopeHex string, now uint64) (*UnsealResult, error) {
	r, err := runCryptoCLI("unseal",
		"--recipient-identity-json", recipientIdentityJSON,
		"--envelope-hex", envelopeHex,
		"--now", fmt.Sprintf("%d", now))
	if err != nil {
		return nil, err
	}

	var out UnsealResult
	if out.SenderDID, err = strField(r, "sender_did"); err != nil {
		return nil, err
	}
	if out.SenderIdentityPubHex, err = strField(r, "sender_identity_pub_hex"); err != nil {
		return nil, err
	}
	if out.SenderSigningPubHex, err = strField(r, "sender_signing_pub_hex"); err != nil {
		return nil, err
	}
	payloadHex, err := strField(r, "payload_hex")
	if err != nil {
		return nil, err
	}
	payloadBytes, err := hex.DecodeString(payloadHex)
	if err != nil {
		return nil, fmt.Errorf("crypto-cli: payload_hex not valid hex: %w", err)
	}
	out.Payload = string(payloadBytes)
	return &out, nil
}
