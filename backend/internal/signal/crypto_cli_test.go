package signal

import (
	"encoding/hex"
	"os"
	"testing"
)

// These tests exercise the real Rust obscura-crypto-cli binary via subprocess.
// They are skipped unless CRYPTO_CLI_PATH points at a built binary, so CI
// without the Rust toolchain still passes.
//
//	CRYPTO_CLI_PATH=../../../crypto/target/release/obscura-crypto-cli.exe go test ./internal/signal/

func requireCLI(t *testing.T) {
	t.Helper()
	if os.Getenv("CRYPTO_CLI_PATH") == "" {
		t.Skip("CRYPTO_CLI_PATH not set — skipping crypto-cli subprocess tests")
	}
	if !CryptoCLIAvailable() {
		t.Skipf("crypto CLI not reachable at %q", cryptoCLIPath())
	}
}

func TestCryptoCLI_Keygen(t *testing.T) {
	requireCLI(t)

	priv, pub, err := GenerateKeypair()
	if err != nil {
		t.Fatalf("GenerateKeypair: %v", err)
	}
	// 64 bytes = 128 hex chars (Ed25519 32 || X25519 32).
	if len(priv) != 128 {
		t.Errorf("privkey hex len = %d, want 128", len(priv))
	}
	if len(pub) != 128 {
		t.Errorf("pubkey hex len = %d, want 128", len(pub))
	}
	if _, err := hex.DecodeString(priv); err != nil {
		t.Errorf("privkey not valid hex: %v", err)
	}
}

func TestCryptoCLI_X3DHRoundtrip(t *testing.T) {
	requireCLI(t)

	alicePriv, alicePub, err := GenerateKeypair()
	if err != nil {
		t.Fatalf("alice keygen: %v", err)
	}
	bobPriv, _, err := GenerateKeypair()
	if err != nil {
		t.Fatalf("bob keygen: %v", err)
	}

	// Alice's X25519 identity public = last 32 bytes (64 hex chars) of pubkey.
	aliceDHPub := alicePub[64:]

	bundle, err := PrekeyBundleJSON(bobPriv)
	if err != nil {
		t.Fatalf("PrekeyBundleJSON: %v", err)
	}

	aliceSS, eph, err := X3DHInitiate(alicePriv, bundle)
	if err != nil {
		t.Fatalf("X3DHInitiate: %v", err)
	}
	bobSS, err := X3DHRespond(bobPriv, eph, aliceDHPub)
	if err != nil {
		t.Fatalf("X3DHRespond: %v", err)
	}

	if aliceSS != bobSS {
		t.Fatalf("shared secret mismatch:\n alice=%s\n bob  =%s", aliceSS, bobSS)
	}
}

func TestCryptoCLI_RatchetRoundtrip(t *testing.T) {
	requireCLI(t)

	// Derive a real shared secret via X3DH first.
	alicePriv, alicePub, _ := GenerateKeypair()
	bobPriv, _, _ := GenerateKeypair()
	bundle, err := PrekeyBundleJSON(bobPriv)
	if err != nil {
		t.Fatalf("PrekeyBundleJSON: %v", err)
	}
	sharedSecret, _, err := X3DHInitiate(alicePriv, bundle)
	if err != nil {
		t.Fatalf("X3DHInitiate: %v", err)
	}
	// X3DHRespond proves both sides agree; not needed for the ratchet keys.
	if _, err := X3DHRespond(bobPriv, mustEphemeral(t, alicePriv, bundle), alicePub[64:]); err != nil {
		t.Fatalf("X3DHRespond: %v", err)
	}

	// Bob's ratchet keypair (fresh DH keypair, like the prekey published to Alice).
	bobRatchetPriv, bobRatchetPub, err := GenerateKeypair()
	if err != nil {
		t.Fatalf("bob ratchet keygen: %v", err)
	}
	bobRatchetDHPub := bobRatchetPub[64:] // X25519 pub portion

	aliceState, err := InitRatchetAsSender(sharedSecret, bobRatchetDHPub)
	if err != nil {
		t.Fatalf("InitRatchetAsSender: %v", err)
	}
	bobState, err := InitRatchetAsReceiver(sharedSecret, bobRatchetPriv)
	if err != nil {
		t.Fatalf("InitRatchetAsReceiver: %v", err)
	}

	msg := []byte("Merhaba Bob — gizli ratchet mesaji")
	ptHex := hex.EncodeToString(msg)

	aliceState, ctHex, hdrHex, err := RatchetEncrypt(aliceState, ptHex)
	if err != nil {
		t.Fatalf("RatchetEncrypt: %v", err)
	}
	if ctHex == "" || hdrHex == "" {
		t.Fatal("RatchetEncrypt returned empty ciphertext/header")
	}

	_, decHex, err := RatchetDecrypt(bobState, ctHex, hdrHex)
	if err != nil {
		t.Fatalf("RatchetDecrypt: %v", err)
	}

	dec, err := hex.DecodeString(decHex)
	if err != nil {
		t.Fatalf("decode plaintext hex: %v", err)
	}
	if string(dec) != string(msg) {
		t.Fatalf("plaintext mismatch: got %q want %q", dec, msg)
	}

	// Sending a second message must keep working with the advanced state.
	_, ct2, hdr2, err := RatchetEncrypt(aliceState, hex.EncodeToString([]byte("ikinci")))
	if err != nil {
		t.Fatalf("RatchetEncrypt #2: %v", err)
	}
	_, dec2Hex, err := RatchetDecrypt(bobState, ct2, hdr2)
	if err != nil {
		t.Fatalf("RatchetDecrypt #2: %v", err)
	}
	dec2, _ := hex.DecodeString(dec2Hex)
	if string(dec2) != "ikinci" {
		t.Fatalf("second message mismatch: got %q", dec2)
	}
}

func TestCryptoCLI_ErrorSurfaces(t *testing.T) {
	requireCLI(t)
	if _, err := PrekeyBundleJSON("not-hex"); err == nil {
		t.Fatal("expected error for invalid hex privkey")
	}
}

// mustEphemeral re-runs X3DHInitiate just to capture the ephemeral pubkey
// (the shared secret it returns is deterministic for the same inputs only up to
// Alice's random ephemeral, so we use the value paired with the shared secret in
// the caller). It keeps the ratchet test self-contained.
func mustEphemeral(t *testing.T, alicePriv, bundle string) string {
	t.Helper()
	_, eph, err := X3DHInitiate(alicePriv, bundle)
	if err != nil {
		t.Fatalf("mustEphemeral X3DHInitiate: %v", err)
	}
	return eph
}
