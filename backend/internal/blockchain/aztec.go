// Package blockchain holds Go-side bridges to the external settlement layers
// Obscura uses. Today the only such layer is the planned Aztec zk-Rollup
// (ADR-0005); this file is the RPC client skeleton.
//
// STATUS: STUB. No real RPC integration. SubmitProof returns a sentinel error
// ("not-yet-implemented") and Healthcheck just sanity-checks configuration.
// Real wiring lands in FAZ 2 GA once Aztec mainnet date is firm; see ADR-0005
// "Phase 1" / "Phase 2".
//
// The interface is shaped so callers can write against it now and have a
// stable target when the live RPC arrives.
package blockchain

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ErrNotImplemented is returned by stub methods that will gain real behavior
// in FAZ 2 GA. Callers should treat it as "the bridge is configured but the
// remote layer is not yet live", not as a transient failure.
var ErrNotImplemented = errors.New("aztec bridge: not yet implemented (FAZ 2 GA)")

// AztecBridge is the RPC client for an Aztec node (sandbox, testnet, or
// mainnet depending on rpcURL). Construct with NewAztecBridge.
type AztecBridge struct {
	rpcURL  string
	http    *http.Client
	timeout time.Duration
}

// NewAztecBridge constructs a bridge pointed at rpcURL (e.g.
// "http://localhost:8080" for the local Aztec sandbox). rpcURL must be
// non-empty and an http(s) URL.
func NewAztecBridge(rpcURL string) (*AztecBridge, error) {
	if rpcURL == "" {
		return nil, errors.New("aztec bridge: empty rpcURL")
	}
	if !strings.HasPrefix(rpcURL, "http://") && !strings.HasPrefix(rpcURL, "https://") {
		return nil, fmt.Errorf("aztec bridge: rpcURL must be http(s), got %q", rpcURL)
	}
	return &AztecBridge{
		rpcURL:  strings.TrimRight(rpcURL, "/"),
		http:    &http.Client{Timeout: 10 * time.Second},
		timeout: 10 * time.Second,
	}, nil
}

// SubmitProof submits a PLONK proof plus its public inputs to the Aztec node
// for inclusion in the next block. Returns the resulting tx hash on success.
//
// STUB: returns ErrNotImplemented until FAZ 2 GA. The signature is locked so
// the OBS-token bridge (contracts/aztec/obs_token.nr) and downstream callers
// can be written against it today.
func (b *AztecBridge) SubmitProof(ctx context.Context, proof, publicInputs []byte) (txHash string, err error) {
	if b == nil {
		return "", errors.New("aztec bridge: nil receiver")
	}
	if len(proof) == 0 {
		return "", errors.New("aztec bridge: empty proof")
	}
	_ = ctx
	_ = publicInputs
	return "", ErrNotImplemented
}

// Healthcheck verifies the bridge can reach its configured Aztec node.
// Returns nil if the node responds within timeout, an error otherwise.
//
// Phase 0 implementation: HEAD request to rpcURL. Phase 1 will replace this
// with a JSON-RPC `node_getStatus` call.
func (b *AztecBridge) Healthcheck(ctx context.Context) error {
	if b == nil {
		return errors.New("aztec bridge: nil receiver")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, b.rpcURL, nil)
	if err != nil {
		return fmt.Errorf("aztec bridge: build request: %w", err)
	}
	resp, err := b.http.Do(req)
	if err != nil {
		return fmt.Errorf("aztec bridge: unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("aztec bridge: node 5xx: %d", resp.StatusCode)
	}
	return nil
}
