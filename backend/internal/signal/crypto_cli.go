package signal

// crypto_cli.go — subprocess bridge to the Rust obscura-crypto-cli binary.
//
// CGO_ENABLED=0 forbids linking the Rust crypto crate directly, so the backend
// shells out to a one-shot CLI that reads hex/JSON arguments and writes a single
// JSON object to stdout. No C dependency — only os/exec.
//
// The CLI prints {"error": "..."} to stdout and exits 1 on failure; we parse
// stdout regardless of exit status so the structured error surfaces.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

// cryptoCLIPath returns the binary path from CRYPTO_CLI_PATH env,
// falling back to "obscura-crypto-cli" in PATH.
func cryptoCLIPath() string {
	if p := os.Getenv("CRYPTO_CLI_PATH"); p != "" {
		return p
	}
	return "obscura-crypto-cli"
}

// runCryptoCLI runs the CLI with args and returns the parsed JSON stdout.
// It parses stdout even when the process exits non-zero, because the CLI
// reports failures as {"error": "..."} on stdout with exit code 1.
func runCryptoCLI(args ...string) (map[string]interface{}, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("crypto-cli: no command given")
	}
	cmd := exec.Command(cryptoCLIPath(), args...)
	out, runErr := cmd.Output()

	// Even on a non-zero exit the CLI emits its error JSON on stdout, so try to
	// parse first and only fall back to the exec error when stdout is unusable.
	var result map[string]interface{}
	if len(out) > 0 {
		if jsonErr := json.Unmarshal(out, &result); jsonErr == nil {
			if errMsg, ok := result["error"].(string); ok {
				return nil, fmt.Errorf("crypto-cli error: %s", errMsg)
			}
			return result, nil
		}
	}
	if runErr != nil {
		return nil, fmt.Errorf("crypto-cli %s: %w", args[0], runErr)
	}
	return nil, fmt.Errorf("crypto-cli %s: empty or invalid output", args[0])
}

// strField extracts a required string field from a CLI result map.
func strField(r map[string]interface{}, key string) (string, error) {
	v, ok := r[key].(string)
	if !ok {
		return "", fmt.Errorf("crypto-cli: missing field %q in output", key)
	}
	return v, nil
}

// GenerateKeypair generates a new Ed25519/X25519 identity keypair.
// Both values are hex: 64 bytes = Ed25519 seed/verifying (32) || X25519 priv/pub (32).
func GenerateKeypair() (privkeyHex, pubkeyHex string, err error) {
	r, err := runCryptoCLI("keygen")
	if err != nil {
		return "", "", err
	}
	if privkeyHex, err = strField(r, "privkey"); err != nil {
		return "", "", err
	}
	if pubkeyHex, err = strField(r, "pubkey"); err != nil {
		return "", "", err
	}
	return privkeyHex, pubkeyHex, nil
}

// PrekeyBundleJSON produces a public PreKey bundle JSON string from an identity
// private key (hex, from GenerateKeypair). The returned JSON is suitable to pass
// straight back to X3DHInitiate as the remote bundle.
func PrekeyBundleJSON(privkeyHex string) (bundleJSON string, err error) {
	r, err := runCryptoCLI("prekey-bundle", "--privkey", privkeyHex)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("crypto-cli: re-marshal bundle: %w", err)
	}
	return string(b), nil
}

// X3DHInitiate runs the Alice-side X3DH handshake against a remote bundle JSON.
// Returns the derived shared secret (hex) and Alice's ephemeral public key (hex).
func X3DHInitiate(privkeyHex, remoteBundleJSON string) (sharedSecretHex, ephemeralPubkeyHex string, err error) {
	r, err := runCryptoCLI("x3dh-initiate",
		"--privkey", privkeyHex,
		"--remote-bundle", remoteBundleJSON)
	if err != nil {
		return "", "", err
	}
	if sharedSecretHex, err = strField(r, "shared_secret"); err != nil {
		return "", "", err
	}
	if ephemeralPubkeyHex, err = strField(r, "ephemeral_pubkey"); err != nil {
		return "", "", err
	}
	return sharedSecretHex, ephemeralPubkeyHex, nil
}

// X3DHRespond completes the Bob-side X3DH handshake from Alice's ephemeral and
// identity public keys. Returns the derived shared secret (hex), which must
// match the value X3DHInitiate produced.
func X3DHRespond(privkeyHex, ephemeralPubkeyHex, senderIdentityPubkeyHex string) (sharedSecretHex string, err error) {
	r, err := runCryptoCLI("x3dh-respond",
		"--privkey", privkeyHex,
		"--ephemeral-pubkey", ephemeralPubkeyHex,
		"--sender-identity-pubkey", senderIdentityPubkeyHex)
	if err != nil {
		return "", err
	}
	return strField(r, "shared_secret")
}

// InitRatchetAsSender initializes a Double Ratchet session (Alice side).
// sharedSecret and remotePubkey are hex strings from X3DH.
// Returns opaque state JSON to be stored encrypted by caller.
func InitRatchetAsSender(sharedSecretHex, remotePubkeyHex string) (stateJSON string, err error) {
	r, err := runCryptoCLI("ratchet-init-sender",
		"--shared-secret", sharedSecretHex,
		"--remote-pubkey", remotePubkeyHex)
	if err != nil {
		return "", err
	}
	return strField(r, "state")
}

// InitRatchetAsReceiver initializes a Double Ratchet session (Bob side).
func InitRatchetAsReceiver(sharedSecretHex, ourPrivkeyHex string) (stateJSON string, err error) {
	r, err := runCryptoCLI("ratchet-init-receiver",
		"--shared-secret", sharedSecretHex,
		"--our-privkey", ourPrivkeyHex)
	if err != nil {
		return "", err
	}
	return strField(r, "state")
}

// RatchetEncrypt encrypts plaintext using Double Ratchet.
// stateJSON is the current ratchet state (must be replaced after call).
// plaintextHex is hex-encoded plaintext bytes.
// Returns new stateJSON, ciphertextHex, headerHex.
func RatchetEncrypt(stateJSON, plaintextHex string) (newState, ciphertextHex, headerHex string, err error) {
	r, err := runCryptoCLI("ratchet-encrypt",
		"--state", stateJSON,
		"--plaintext", plaintextHex)
	if err != nil {
		return "", "", "", err
	}
	if newState, err = strField(r, "state"); err != nil {
		return "", "", "", err
	}
	if ciphertextHex, err = strField(r, "ciphertext"); err != nil {
		return "", "", "", err
	}
	if headerHex, err = strField(r, "header"); err != nil {
		return "", "", "", err
	}
	return newState, ciphertextHex, headerHex, nil
}

// RatchetDecrypt decrypts a Double Ratchet message.
// Returns new stateJSON and plaintextHex.
func RatchetDecrypt(stateJSON, ciphertextHex, headerHex string) (newState, plaintextHex string, err error) {
	r, err := runCryptoCLI("ratchet-decrypt",
		"--state", stateJSON,
		"--ciphertext", ciphertextHex,
		"--header", headerHex)
	if err != nil {
		return "", "", err
	}
	if newState, err = strField(r, "state"); err != nil {
		return "", "", err
	}
	if plaintextHex, err = strField(r, "plaintext"); err != nil {
		return "", "", err
	}
	return newState, plaintextHex, nil
}

// CryptoCLIAvailable returns true if the crypto CLI binary is reachable.
func CryptoCLIAvailable() bool {
	if _, err := exec.LookPath(cryptoCLIPath()); err == nil {
		return true
	}
	// Also check CRYPTO_CLI_PATH directly (may be an absolute path).
	if p := os.Getenv("CRYPTO_CLI_PATH"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}
