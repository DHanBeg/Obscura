// Package mls provides a Go client for the Rust mls-cli subprocess.
//
// The Rust binary (crypto/target/release/mls-cli) speaks JSON-RPC over stdio.
// This client owns the subprocess lifecycle, sends commands, parses responses.
//
// CGO_ENABLED=0 friendly: subprocess only, no FFI.
//
// Spec: docs/spec/obscura_spec_v3.txt Bölüm 6.3
// ADR:  docs/adr/0007-openmls-for-groups.md
package mls

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// Client wraps the Rust mls-cli subprocess.
type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner
	mu     sync.Mutex
	closed atomic.Bool
}

// Request matches the wire format expected by mls-cli.
type Request struct {
	ID     string `json:"id"`
	Op     string `json:"op"`
	Params any    `json:"params,omitempty"`
}

// Response matches mls-cli output.
type Response struct {
	ID     string          `json:"id"`
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// NewClient spawns mls-cli at binPath and reads its ready greeting.
func NewClient(binPath string) (*Client, error) {
	if _, err := os.Stat(binPath); err != nil {
		return nil, fmt.Errorf("mls-cli not found at %s: %w", binPath, err)
	}

	cmd := exec.Command(binPath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1<<20), 16<<20) // up to 16MB lines (large MLS messages)

	// Read ready greeting
	if !scanner.Scan() {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("no greeting from mls-cli: %v", scanner.Err())
	}
	var greet struct {
		Ready   bool   `json:"ready"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(scanner.Bytes(), &greet); err != nil || !greet.Ready {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("bad greeting: %s", scanner.Bytes())
	}

	c := &Client{
		cmd:    cmd,
		stdin:  stdin,
		stdout: scanner,
	}
	return c, nil
}

// Close terminates the subprocess.
func (c *Client) Close() error {
	if c.closed.Swap(true) {
		return nil
	}
	_ = c.stdin.Close()
	if c.cmd.Process != nil {
		// Give it a moment to exit cleanly
		done := make(chan error, 1)
		go func() { done <- c.cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			_ = c.cmd.Process.Kill()
		}
	}
	return nil
}

// Call sends one request and waits for the response.
func (c *Client) Call(ctx context.Context, op string, params any) (json.RawMessage, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("mls client closed")
	}

	req := Request{
		ID:     uuid.New().String(),
		Op:     op,
		Params: params,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, err := c.stdin.Write(append(body, '\n')); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}

	// Read line — context only matters for cancellation; bufio.Scanner is blocking.
	type readResult struct {
		line []byte
		err  error
	}
	ch := make(chan readResult, 1)
	go func() {
		if c.stdout.Scan() {
			line := append([]byte(nil), c.stdout.Bytes()...)
			ch <- readResult{line: line}
		} else {
			ch <- readResult{err: c.stdout.Err()}
		}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		if r.err != nil {
			return nil, fmt.Errorf("read: %w", r.err)
		}
		var resp Response
		if err := json.Unmarshal(r.line, &resp); err != nil {
			return nil, fmt.Errorf("decode: %w (line: %s)", err, r.line)
		}
		if !resp.OK {
			return nil, fmt.Errorf("mls error: %s", resp.Error)
		}
		return resp.Result, nil
	}
}

// ─── High-level wrappers ─────────────────────────────────────────────────────

func (c *Client) NewIdentity(ctx context.Context, did string) (string, error) {
	r, err := c.Call(ctx, "new_identity", map[string]string{"did": did})
	if err != nil {
		return "", err
	}
	var out struct {
		IdentityID string `json:"identity_id"`
	}
	if err := json.Unmarshal(r, &out); err != nil {
		return "", err
	}
	return out.IdentityID, nil
}

func (c *Client) GenerateKeyPackage(ctx context.Context, identityID string) (string, error) {
	r, err := c.Call(ctx, "generate_key_package", map[string]string{"identity_id": identityID})
	if err != nil {
		return "", err
	}
	var out struct {
		KeyPackageB64 string `json:"key_package_b64"`
	}
	if err := json.Unmarshal(r, &out); err != nil {
		return "", err
	}
	return out.KeyPackageB64, nil
}

func (c *Client) CreateGroup(ctx context.Context, identityID string) (string, error) {
	r, err := c.Call(ctx, "create_group", map[string]string{"identity_id": identityID})
	if err != nil {
		return "", err
	}
	var out struct {
		GroupID string `json:"group_id"`
	}
	if err := json.Unmarshal(r, &out); err != nil {
		return "", err
	}
	return out.GroupID, nil
}

type AddMemberResult struct {
	CommitB64  string `json:"commit_b64"`
	WelcomeB64 string `json:"welcome_b64"`
	Epoch      uint64 `json:"epoch"`
}

func (c *Client) AddMember(ctx context.Context, groupID, identityID, keyPackageB64 string) (*AddMemberResult, error) {
	r, err := c.Call(ctx, "add_member", map[string]string{
		"group_id":         groupID,
		"identity_id":      identityID,
		"key_package_b64":  keyPackageB64,
	})
	if err != nil {
		return nil, err
	}
	var out AddMemberResult
	if err := json.Unmarshal(r, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

type ProcessWelcomeResult struct {
	GroupID string `json:"group_id"`
	Epoch   uint64 `json:"epoch"`
}

func (c *Client) ProcessWelcome(ctx context.Context, identityID, welcomeB64 string) (*ProcessWelcomeResult, error) {
	r, err := c.Call(ctx, "process_welcome", map[string]string{
		"identity_id": identityID,
		"welcome_b64": welcomeB64,
	})
	if err != nil {
		return nil, err
	}
	var out ProcessWelcomeResult
	if err := json.Unmarshal(r, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Encrypt(ctx context.Context, groupID, identityID string, plaintext []byte) (string, error) {
	r, err := c.Call(ctx, "encrypt", map[string]string{
		"group_id":      groupID,
		"identity_id":   identityID,
		"plaintext_b64": base64.StdEncoding.EncodeToString(plaintext),
	})
	if err != nil {
		return "", err
	}
	var out struct {
		CiphertextB64 string `json:"ciphertext_b64"`
	}
	if err := json.Unmarshal(r, &out); err != nil {
		return "", err
	}
	return out.CiphertextB64, nil
}

type ProcessMessageResult struct {
	PlaintextB64 *string `json:"plaintext_b64"`
	Epoch        uint64  `json:"epoch"`
}

// ProcessMessage handles either application messages or commits.
// For application messages, Plaintext returns the decoded bytes.
// For commits, Plaintext is nil but state is updated.
func (c *Client) ProcessMessage(ctx context.Context, groupID string, messageB64 string) (plaintext []byte, epoch uint64, err error) {
	r, err := c.Call(ctx, "process_message", map[string]string{
		"group_id":    groupID,
		"message_b64": messageB64,
	})
	if err != nil {
		return nil, 0, err
	}
	var out ProcessMessageResult
	if err := json.Unmarshal(r, &out); err != nil {
		return nil, 0, err
	}
	if out.PlaintextB64 != nil {
		pt, err := base64.StdEncoding.DecodeString(*out.PlaintextB64)
		if err != nil {
			return nil, 0, fmt.Errorf("decode plaintext: %w", err)
		}
		return pt, out.Epoch, nil
	}
	return nil, out.Epoch, nil
}

// Ping verifies the subprocess is responsive.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.Call(ctx, "ping", nil)
	return err
}

// ─── BIP39 Mnemonic ──────────────────────────────────────────────────────────

// GenerateMnemonic returns a fresh 12-word BIP39 phrase.
// Spec Bölüm 4.2 — store ONLY in OS secure enclave (Keychain/Keystore).
func (c *Client) GenerateMnemonic(ctx context.Context) (string, error) {
	r, err := c.Call(ctx, "mnemonic_generate", nil)
	if err != nil {
		return "", err
	}
	var out struct {
		Mnemonic string `json:"mnemonic"`
	}
	if err := json.Unmarshal(r, &out); err != nil {
		return "", err
	}
	return out.Mnemonic, nil
}

// ValidateMnemonic checks if the phrase is valid BIP39 English 12 words.
func (c *Client) ValidateMnemonic(ctx context.Context, phrase string) error {
	_, err := c.Call(ctx, "mnemonic_validate", map[string]string{"phrase": phrase})
	return err
}

// MnemonicIdentity is the result of deriving an identity from a mnemonic.
type MnemonicIdentity struct {
	DID               string `json:"did"`
	IdentitySecretB64 string `json:"identity_secret_b64"`
	Ed25519PublicB64  string `json:"ed25519_public_b64"`
	Ed25519PrivateB64 string `json:"ed25519_private_b64"`
}

// DeriveIdentityFromMnemonic returns DID + ed25519 keypair derived from a mnemonic.
// Deterministic: same mnemonic always → same DID. Use for account recovery.
func (c *Client) DeriveIdentityFromMnemonic(ctx context.Context, phrase string, passphrase string) (*MnemonicIdentity, error) {
	params := map[string]string{"phrase": phrase}
	if passphrase != "" {
		params["passphrase"] = passphrase
	}
	r, err := c.Call(ctx, "mnemonic_derive_did", params)
	if err != nil {
		return nil, err
	}
	var out MnemonicIdentity
	if err := json.Unmarshal(r, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
