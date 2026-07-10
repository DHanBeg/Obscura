package messaging

import (
	"os"
	"testing"

	"obscura.network/core/internal/signal"
)

// Confirms the messaging-package Seal/Unseal correctly delegate to
// internal/signal's obscura-crypto-cli bridge. Full crypto-correctness
// coverage (roundtrip, wrong-recipient failure, sender-hiding) lives in
// internal/signal/sealed_sender_cli_test.go; this just proves the wiring.
//
//	CRYPTO_CLI_PATH=../../../crypto/target/release/obscura-crypto-cli.exe go test ./internal/messaging/

func requireCLI(t *testing.T) {
	t.Helper()
	if os.Getenv("CRYPTO_CLI_PATH") == "" {
		t.Skip("CRYPTO_CLI_PATH not set — skipping crypto-cli subprocess tests")
	}
	if !signal.CryptoCLIAvailable() {
		t.Skip("crypto CLI not reachable")
	}
}

func TestSealUnseal_DelegatesToSignalPackage(t *testing.T) {
	requireCLI(t)

	aliceJSON, aliceDID, err := signal.NewSecureIdentity()
	if err != nil {
		t.Fatalf("alice NewSecureIdentity: %v", err)
	}
	bobJSON, _, err := signal.NewSecureIdentity()
	if err != nil {
		t.Fatalf("bob NewSecureIdentity: %v", err)
	}
	_, bobDHPubHex, _, err := signal.IdentityPubHex(bobJSON)
	if err != nil {
		t.Fatalf("bob IdentityPubHex: %v", err)
	}

	const payload = "messaging-package roundtrip"
	envelopeHex, err := Seal(aliceJSON, bobDHPubHex, payload, 0)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	got, err := Unseal(bobJSON, envelopeHex, 0)
	if err != nil {
		t.Fatalf("Unseal: %v", err)
	}
	if got.SenderDID != aliceDID {
		t.Fatalf("SenderDID = %q, want %q", got.SenderDID, aliceDID)
	}
	if got.Payload != payload {
		t.Fatalf("Payload = %q, want %q", got.Payload, payload)
	}
}
