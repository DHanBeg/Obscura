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
//
// If ctx has no deadline, a default 30s timeout is applied. On context
// cancellation or timeout the client is marked closed and any subsequent Call
// returns "mls client closed" immediately. This makes the client effectively
// single-shot after the first timeout/cancel: we deliberately do NOT kill the
// subprocess (process kill on Windows is fragile and the read goroutine would
// still block on the now-orphaned pipe). Operators must construct a fresh
// Client to recover.
func (c *Client) Call(ctx context.Context, op string, params any) (json.RawMessage, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("mls client closed")
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
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
		// Mark closed so subsequent Call returns fast. The read goroutine is
		// left running; it will exit when the subprocess eventually writes a
		// line or its stdout closes. See doc comment above.
		c.closed.Store(true)
		return nil, fmt.Errorf("mls call timeout/canceled: %w", ctx.Err())
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

// ─── Görev 2: Remove / Update / State ────────────────────────────────────────
//
// Bu üç işlem MLS Commit'in farklı varyantlarını sarar:
//   - RemoveMember: Remove proposal + Commit (post-compromise security)
//   - UpdateKey:    Self leaf key update Commit (forward secrecy rotasyonu)
//   - GetGroupState: Mevcut grup metadata (epoch, member count, ratchet root hash)
//
// Rust mls-cli karşılık gelen ops:
//   "remove_member"  params: {group_id, identity_id, target_leaf_index | target_did}
//   "update_key"     params: {group_id, identity_id}
//   "group_state"    params: {group_id}

// RemoveMemberResult — mls-cli'den dönen Commit blobu.
type RemoveMemberResult struct {
	CommitB64 string `json:"commit_b64"`
	Epoch     uint64 `json:"epoch"`
	// Çıkarılan üyenin leaf index'i (audit için).
	RemovedLeafIndex uint32 `json:"removed_leaf_index"`
}

// RemoveMember — Remove proposal + Commit üret.
// Caller bu commit'i sunucuya iletir; sunucu broadcast eder.
// Post-compromise security: bir sonraki epoch'tan itibaren target_did
// hiçbir mesajı çözemez (tree'den çıkar, ratchet anahtarları yenilenir).
func (c *Client) RemoveMember(ctx context.Context, groupID, identityID, targetDID string) (*RemoveMemberResult, error) {
	r, err := c.Call(ctx, "remove_member", map[string]string{
		"group_id":    groupID,
		"identity_id": identityID,
		"target_did":  targetDID,
	})
	if err != nil {
		return nil, err
	}
	var out RemoveMemberResult
	if err := json.Unmarshal(r, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateKeyResult — leaf key rotation Commit blobu.
type UpdateKeyResult struct {
	CommitB64 string `json:"commit_b64"`
	Epoch     uint64 `json:"epoch"`
}

// UpdateKey — kendi leaf'inin HPKE anahtarını rotate et + Commit üret.
// Forward secrecy: bu commit'ten önceki tüm mesaj anahtarları (eski leaf'le
// türetilmiş) artık geri-türetilemez. Caller commit'i sunucuya iletir.
func (c *Client) UpdateKey(ctx context.Context, groupID, identityID string) (*UpdateKeyResult, error) {
	r, err := c.Call(ctx, "update_key", map[string]string{
		"group_id":    groupID,
		"identity_id": identityID,
	})
	if err != nil {
		return nil, err
	}
	var out UpdateKeyResult
	if err := json.Unmarshal(r, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GroupState — bir grubun client-side gözlemlenebilir özet bilgisi.
// Server bu bilgiyi DB'den de türetebilir; ancak ratchet root hash
// yalnızca CLI üzerinden alınır (sunucu plaintext ağaca erişemez).
type GroupState struct {
	GroupID       string   `json:"group_id"`
	Epoch         uint64   `json:"epoch"`
	MemberCount   uint32   `json:"member_count"`
	Ciphersuite   string   `json:"ciphersuite"`
	TreeHashB64   string   `json:"tree_hash_b64"`
	MyLeafIndex   uint32   `json:"my_leaf_index"`
	MemberDIDs    []string `json:"member_dids,omitempty"`
}

// GetGroupState — caller'ın o gruptaki güncel durumunu döner.
// identityID opsiyonel: verilirse my_leaf_index dolu döner.
func (c *Client) GetGroupState(ctx context.Context, groupID string, identityID string) (*GroupState, error) {
	params := map[string]string{"group_id": groupID}
	if identityID != "" {
		params["identity_id"] = identityID
	}
	r, err := c.Call(ctx, "group_state", params)
	if err != nil {
		return nil, err
	}
	var out GroupState
	if err := json.Unmarshal(r, &out); err != nil {
		return nil, err
	}
	return &out, nil
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
