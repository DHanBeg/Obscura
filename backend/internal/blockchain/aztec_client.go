// aztec_client.go — Higher-level OBS-token client over an Aztec PXE/sandbox.
//
// Where AztecBridge (aztec.go) is a low-level proof-submission skeleton, this
// AztecClient speaks the OBSToken contract API (transfer / balance_of /
// total_supply / mint) by talking JSON-RPC to a PXE / sandbox endpoint.
//
// Design notes:
//   - Configuration comes from env (NewAztecClient). A zero-value endpoint is
//     a configuration error, not a panic.
//   - If the sandbox is unreachable, calls degrade gracefully: read-style calls
//     return a typed error the caller can detect; transfer returns a synthetic
//     tx hash in stub mode so local dev / tests proceed without a live node.
//   - We NEVER log the admin private key. It is read from env and forwarded to
//     the PXE only.
package blockchain

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ErrSandboxUnreachable means the configured PXE/sandbox did not respond.
// Callers that want a hard failure should check for it; callers that want
// best-effort behavior in local dev can fall back to stub mode.
var ErrSandboxUnreachable = errors.New("aztec client: sandbox unreachable")

// AztecClient wraps Aztec sandbox / node PXE RPC calls for the OBS token.
type AztecClient struct {
	SandboxURL   string
	ContractAddr string
	AdminPrivKey string // never logged

	// StubMode, when true, lets state-changing calls return synthetic results
	// instead of failing when the sandbox is unreachable. Enabled by default
	// for local dev; set OBSCURA_AZTEC_STRICT=1 to force real RPC.
	StubMode bool

	http *http.Client
}

// NewAztecClient builds a client from environment variables:
//
//	AZTEC_SANDBOX_URL    PXE RPC endpoint (e.g. http://localhost:8080)
//	OBS_TOKEN_ADDRESS    deployed OBSToken contract address
//	AZTEC_ADMIN_PRIVKEY  admin (owner/minter) key for signing — optional
//	OBSCURA_AZTEC_STRICT when "1", disables stub fallback
func NewAztecClient() *AztecClient {
	return &AztecClient{
		SandboxURL:   os.Getenv("AZTEC_SANDBOX_URL"),
		ContractAddr: os.Getenv("OBS_TOKEN_ADDRESS"),
		AdminPrivKey: os.Getenv("AZTEC_ADMIN_PRIVKEY"),
		StubMode:     os.Getenv("OBSCURA_AZTEC_STRICT") != "1",
		http:         &http.Client{Timeout: 15 * time.Second},
	}
}

// callTimeout returns the per-call deadline, defaulting to 15s when the
// underlying http client has no timeout configured (e.g. in tests).
func (c *AztecClient) callTimeout() time.Duration {
	if c.http != nil && c.http.Timeout > 0 {
		return c.http.Timeout
	}
	return 15 * time.Second
}

// jsonrpcReq is a minimal JSON-RPC 2.0 request envelope.
type jsonrpcReq struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      int           `json:"id"`
}

// jsonrpcResp is a minimal JSON-RPC 2.0 response envelope.
type jsonrpcResp struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	ID int `json:"id"`
}

// call performs a single JSON-RPC POST to the PXE endpoint.
func (c *AztecClient) call(ctx context.Context, method string, params []interface{}) (json.RawMessage, error) {
	if c == nil {
		return nil, errors.New("aztec client: nil receiver")
	}
	if c.SandboxURL == "" {
		return nil, errors.New("aztec client: AZTEC_SANDBOX_URL not set")
	}

	body, err := json.Marshal(jsonrpcReq{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      1,
	})
	if err != nil {
		return nil, fmt.Errorf("aztec client: marshal request: %w", err)
	}

	url := strings.TrimRight(c.SandboxURL, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("aztec client: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSandboxUnreachable, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("aztec client: read body: %w", err)
	}
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("%w: HTTP %d", ErrSandboxUnreachable, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("aztec client: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var out jsonrpcResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("aztec client: decode response: %w", err)
	}
	if out.Error != nil {
		return nil, fmt.Errorf("aztec client: rpc error %d: %s", out.Error.Code, out.Error.Message)
	}
	return out.Result, nil
}

// Transfer executes a private OBS transfer from fromDID to toDID.
//
// It simulates then sends the transaction via the PXE. If the sandbox is
// unreachable and StubMode is enabled, it returns a synthetic tx hash so local
// development can proceed; in strict mode it returns ErrSandboxUnreachable.
func (c *AztecClient) Transfer(fromDID, toDID string, amount uint64) (txHash string, err error) {
	if c == nil {
		return "", errors.New("aztec client: nil receiver")
	}
	if c.SandboxURL == "" {
		return "", errors.New("aztec client: AZTEC_SANDBOX_URL not set")
	}
	if c.ContractAddr == "" {
		return "", errors.New("aztec client: OBS_TOKEN_ADDRESS not set")
	}
	if fromDID == "" || toDID == "" {
		return "", errors.New("aztec client: empty from/to")
	}
	if amount == 0 {
		return "", errors.New("aztec client: amount must be > 0")
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.callTimeout())
	defer cancel()

	// 1) Simulate to surface contract-side asserts (balance, pause) early.
	simParams := []interface{}{
		c.ContractAddr,
		"transfer",
		[]interface{}{fromDID, toDID, amount},
	}
	if _, err := c.call(ctx, "pxe_simulateTx", simParams); err != nil {
		if errors.Is(err, ErrSandboxUnreachable) && c.StubMode {
			return stubTxHash(), nil
		}
		return "", fmt.Errorf("aztec client: simulate transfer: %w", err)
	}

	// 2) Send the proven tx.
	sendParams := []interface{}{
		c.ContractAddr,
		"transfer",
		[]interface{}{fromDID, toDID, amount},
	}
	res, err := c.call(ctx, "pxe_sendTx", sendParams)
	if err != nil {
		if errors.Is(err, ErrSandboxUnreachable) && c.StubMode {
			return stubTxHash(), nil
		}
		return "", fmt.Errorf("aztec client: send transfer: %w", err)
	}

	var hash string
	if err := json.Unmarshal(res, &hash); err != nil || hash == "" {
		// Some PXE builds return an object; fall back to the raw payload.
		hash = strings.Trim(string(res), `"`)
	}
	if hash == "" {
		return "", errors.New("aztec client: empty tx hash from PXE")
	}
	return hash, nil
}

// TotalSupply reads the public total_supply view. Returns the value as a
// decimal string (Field values can exceed uint64).
func (c *AztecClient) TotalSupply() (string, error) {
	if c == nil {
		return "", errors.New("aztec client: nil receiver")
	}
	if c.ContractAddr == "" {
		return "", errors.New("aztec client: OBS_TOKEN_ADDRESS not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.callTimeout())
	defer cancel()

	res, err := c.call(ctx, "pxe_viewTx", []interface{}{
		c.ContractAddr, "total_supply", []interface{}{},
	})
	if err != nil {
		return "", fmt.Errorf("aztec client: total_supply: %w", err)
	}
	return strings.Trim(string(res), `"`), nil
}

// Healthcheck verifies the PXE responds to a node-info probe.
func (c *AztecClient) Healthcheck() error {
	if c == nil {
		return errors.New("aztec client: nil receiver")
	}
	if c.SandboxURL == "" {
		return errors.New("aztec client: AZTEC_SANDBOX_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.call(ctx, "pxe_getNodeInfo", []interface{}{}); err != nil {
		return err
	}
	return nil
}

// stubTxHash produces a random 0x-prefixed 32-byte hash for stub mode.
func stubTxHash() string {
	var b [32]byte
	_, _ = rand.Read(b[:])
	return "0x" + hex.EncodeToString(b[:])
}
