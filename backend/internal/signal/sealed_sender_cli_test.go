package signal

import (
	"strings"
	"testing"
)

// These tests exercise the real Rust obscura-crypto-cli binary via subprocess,
// same convention as crypto_cli_test.go:
//
//	CRYPTO_CLI_PATH=../../../crypto/target/release/obscura-crypto-cli.exe go test ./internal/signal/

func TestSealedSenderCLI_Roundtrip(t *testing.T) {
	requireCLI(t)

	aliceJSON, aliceDID, err := NewSecureIdentity()
	if err != nil {
		t.Fatalf("alice NewSecureIdentity: %v", err)
	}
	bobJSON, _, err := NewSecureIdentity()
	if err != nil {
		t.Fatalf("bob NewSecureIdentity: %v", err)
	}
	_, bobDHPubHex, _, err := IdentityPubHex(bobJSON)
	if err != nil {
		t.Fatalf("bob IdentityPubHex: %v", err)
	}

	const payload = "merhaba bob — gizli sealed-sender mesaji"
	envelopeHex, err := Seal(aliceJSON, bobDHPubHex, payload, 0)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if envelopeHex == "" {
		t.Fatal("Seal returned empty envelope")
	}

	got, err := Unseal(bobJSON, envelopeHex, 0)
	if err != nil {
		t.Fatalf("Unseal (correct recipient): %v", err)
	}
	if got.SenderDID != aliceDID {
		t.Fatalf("SenderDID = %q, want %q", got.SenderDID, aliceDID)
	}
	if got.Payload != payload {
		t.Fatalf("Payload = %q, want %q", got.Payload, payload)
	}
	if got.SenderIdentityPubHex == "" || got.SenderSigningPubHex == "" {
		t.Fatal("expected non-empty sender identity/signing pub hex")
	}
	if got.SenderIdentityPubHex == got.SenderSigningPubHex {
		t.Fatal("sender identity pub and signing pub must be distinct keys, got identical hex")
	}
}

func TestSealedSenderCLI_WrongRecipientFails(t *testing.T) {
	requireCLI(t)

	aliceJSON, _, err := NewSecureIdentity()
	if err != nil {
		t.Fatalf("alice NewSecureIdentity: %v", err)
	}
	bobJSON, _, err := NewSecureIdentity()
	if err != nil {
		t.Fatalf("bob NewSecureIdentity: %v", err)
	}
	eveJSON, _, err := NewSecureIdentity()
	if err != nil {
		t.Fatalf("eve NewSecureIdentity: %v", err)
	}
	_, bobDHPubHex, _, err := IdentityPubHex(bobJSON)
	if err != nil {
		t.Fatalf("bob IdentityPubHex: %v", err)
	}

	envelopeHex, err := Seal(aliceJSON, bobDHPubHex, "yalnizca bob icin", 0)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	// Eve is not the intended recipient — she does not hold the private key
	// matching bobDHPubHex, so unsealing must fail. This is the property that
	// makes sealed sender safe: the server, holding no recipient's private
	// identity, can never unseal any envelope either.
	if _, err := Unseal(eveJSON, envelopeHex, 0); err == nil {
		t.Fatal("Unseal with wrong recipient identity succeeded, want error")
	}

	// Sanity: the correct recipient still works on the same envelope.
	if _, err := Unseal(bobJSON, envelopeHex, 0); err != nil {
		t.Fatalf("Unseal (correct recipient, sanity check): %v", err)
	}
}

func TestSealedSenderCLI_EnvelopeHidesSenderPubHex(t *testing.T) {
	requireCLI(t)

	aliceJSON, _, err := NewSecureIdentity()
	if err != nil {
		t.Fatalf("alice NewSecureIdentity: %v", err)
	}
	bobJSON, _, err := NewSecureIdentity()
	if err != nil {
		t.Fatalf("bob NewSecureIdentity: %v", err)
	}
	_, aliceDHPubHex, _, err := IdentityPubHex(aliceJSON)
	if err != nil {
		t.Fatalf("alice IdentityPubHex: %v", err)
	}
	_, bobDHPubHex, _, err := IdentityPubHex(bobJSON)
	if err != nil {
		t.Fatalf("bob IdentityPubHex: %v", err)
	}

	envelopeHex, err := Seal(aliceJSON, bobDHPubHex, "gizli", 0)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	if strings.Contains(envelopeHex, aliceDHPubHex) {
		t.Fatal("envelope_hex contains alice's identity pub key in the clear — sealed sender is broken")
	}
}
