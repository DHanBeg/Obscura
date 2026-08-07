// Package bridge — LayerZero-style cross-chain bridge API (spec Bölüm 12.2/3).
//
// This file layers a simple "lock-and-bridge / poll status" surface on top of
// the lower-level Client RPC helpers. The shape mirrors LayerZero's messaging
// pattern: a sender locks tokens on the source chain, an off-chain relayer
// observes the Locked event, and on the destination chain a paired contract
// mints/releases an equivalent amount to the recipient.
//
// What's real here:
//   - JSON-RPC calls to ETHEREUM_RPC_URL / POLKADOT_RPC_URL (eth_sendTransaction,
//     eth_getTransactionReceipt, author_submitExtrinsic, chain_getBlock).
//   - Calldata stub for OBSBridge.lock(address,uint256,string,string) (see
//     contracts/bridge/OBSBridge.sol).
//   - In-memory bookkeeping so the API layer can return a stable BridgeStatus
//     even before the receipt is mined.
//
// What's NOT real (deferred):
//   - secp256k1 raw-tx signing — covered by ADR-BRIDGE-001. ethLockSigned still
//     falls through to managed mode while the signing plumbing is built out.
//   - Polkadot scale encoding — buildDotLockExtrinsic remains a hex stub.
//   - Cross-chain merkle proofs for Unlock. Production wants the relayer to
//     publish a proof of the source-chain Locked event; today we trust the
//     relayer and rely on multi-sig + replay protection at the contract.

package bridge

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// BridgeStatusInfo is the API-facing view of a single bridge transaction.
// LayerZero's `MessagingReceipt` is the inspiration: a unique id, a source
// chain commitment (txHash), the destination chain, and a coarse status.
type BridgeStatusInfo struct {
	TxHash        string    `json:"tx_hash"`
	SourceChain   Chain     `json:"source_chain"`
	DestChain     Chain     `json:"dest_chain"`
	Recipient     string    `json:"recipient"`
	Amount        int64     `json:"amount"`        // OBS atomic units fits int64 for FAZ 2
	Status        string    `json:"status"`        // "pending" | "confirmed" | "failed"
	Confirmations int       `json:"confirmations"` // best-effort, 0 until first poll
	SubmittedAt   time.Time `json:"submitted_at"`
	LastCheckedAt time.Time `json:"last_checked_at,omitempty"`
}

// inflightBridges keeps a process-local map of submitted bridge txns so the
// status endpoint can report metadata (dest chain, recipient, amount) that
// the source-chain receipt alone doesn't surface. Persisted state lives in
// the bridge_txns DB table — see migration in db/database.go (future PR).
//
// This map is intentionally simple: it's a soft cache, not a source of truth.
// On node restart the map is empty and Status() falls back to the on-chain
// receipt for status only. Acceptable trade-off for FAZ 2 — the bridge UI
// re-queries periodically.
var (
	inflightMu sync.RWMutex
	inflight   = make(map[string]*BridgeStatusInfo)
)

// LockAndBridge locks `amount` OBS on the source chain (Ethereum or Polkadot)
// and emits a Locked event consumed by the off-chain relayer.
//
// Returns the source-chain transaction hash on success; the caller polls
// BridgeStatus to watch the cross-chain delivery.
//
// `amount` is in OBS atomic units (uint256 on-chain, int64 here — FAZ 2 caps
// individual bridge amounts at 9.2e18, well below the 1e9 * 1e18 supply cap;
// larger transfers split into multiple bridge ops).
//
// `destChain` is one of the Chain constants (lowercase string).
//
// `recipient` is the destination-chain address (hex-encoded for ETH, SS58 for
// DOT). The format is opaque to this layer — the destination contract decodes.
func (c *Client) LockAndBridge(
	ctx context.Context,
	amount int64,
	destChain Chain,
	recipient string,
) (string, error) {
	if c == nil {
		return "", fmt.Errorf("bridge: nil client")
	}
	if amount <= 0 {
		return "", fmt.Errorf("bridge: amount must be positive")
	}
	if recipient == "" {
		return "", fmt.Errorf("bridge: recipient required")
	}

	// Source chain selection. We always lock on Ethereum today (the canonical
	// OBSBridge contract lives there). FAZ 4 will let users pick the source.
	source := ChainEthereum
	if destChain != ChainEthereum && destChain != ChainPolkadot {
		return "", fmt.Errorf("bridge: unsupported dest chain %q", destChain)
	}
	if source == destChain {
		return "", fmt.Errorf("bridge: source and dest must differ")
	}

	// Build the bridge request shape the underlying Lock helper expects.
	// SenderAddr is intentionally empty here — for managed-RPC mode the node
	// supplies its own funding address (eth_accounts[0]). Self-custody mode
	// (ethLockSigned) is still a stub; when it lands ADR-BRIDGE-001 will
	// thread the configured relayer address.
	req := BridgeRequest{
		FromChain:     source,
		ToChain:       destChain,
		Amount:        fmt.Sprintf("%d", amount),
		SenderAddr:    deriveRelayerAddr(c.cfg.EthPrivateKey),
		RecipientAddr: recipient,
		ObsDID:        "", // optional metadata; filled by API layer if available
	}

	res, err := c.Lock(req)
	if err != nil {
		// Wrap so callers can distinguish "config missing" from "RPC down" by
		// substring; we don't currently expose typed errors from rpcCallWithRetry.
		return "", fmt.Errorf("bridge: lock failed: %w", err)
	}

	txHash := res.TxID
	if txHash == "" {
		txHash = "stub_" + randHex(8) // sign-stub path returns a synthetic id
	}

	// Record in-flight for the status endpoint.
	inflightMu.Lock()
	inflight[txHash] = &BridgeStatusInfo{
		TxHash:        txHash,
		SourceChain:   source,
		DestChain:     destChain,
		Recipient:     recipient,
		Amount:        amount,
		Status:        "pending",
		Confirmations: 0,
		SubmittedAt:   time.Now().UTC(),
	}
	inflightMu.Unlock()

	log.Printf("[bridge] LockAndBridge OK: tx=%s amount=%d %s→%s recipient=%s",
		txHash, amount, source, destChain, redactAddr(recipient))
	return txHash, nil
}

// BridgeStatus polls the source-chain receipt for `txHash` and returns the
// freshest BridgeStatusInfo. If the tx is unknown to the in-flight cache,
// the call still works but DestChain/Recipient/Amount come back zero-valued
// (only Status is populated from the receipt). This lets external relayers
// look up tx state without having gone through LockAndBridge.
func (c *Client) BridgeStatus(ctx context.Context, txHash string) (*BridgeStatusInfo, error) {
	if c == nil {
		return nil, fmt.Errorf("bridge: nil client")
	}
	if txHash == "" {
		return nil, fmt.Errorf("bridge: txHash required")
	}

	inflightMu.RLock()
	info, known := inflight[txHash]
	if known {
		// Copy so the caller can mutate without racing other readers.
		c := *info
		info = &c
	} else {
		info = &BridgeStatusInfo{
			TxHash:      txHash,
			SourceChain: ChainEthereum, // default; corrected below if poll succeeds
			Status:      "pending",
			SubmittedAt: time.Now().UTC(),
		}
	}
	inflightMu.RUnlock()

	// Pick the right chain to poll. If we recorded the txn we know the source.
	chain := info.SourceChain
	if chain == "" {
		chain = ChainEthereum
	}

	res, err := c.Status(txHash, chain)
	if err != nil {
		// Soft-fail: return the cached info so the UI can still render.
		info.LastCheckedAt = time.Now().UTC()
		return info, nil
	}

	info.Status = res.Status
	info.LastCheckedAt = time.Now().UTC()
	if info.Status == "confirmed" {
		info.Confirmations = 1 // we don't track depth yet; "confirmed at least once"
	}

	// Update the cache so subsequent polls see the latest status without
	// hitting RPC. Only do this if the entry was already in flight (don't
	// poison the cache with unknown txns).
	if known {
		inflightMu.Lock()
		if cur, ok := inflight[txHash]; ok {
			cur.Status = info.Status
			cur.LastCheckedAt = info.LastCheckedAt
			cur.Confirmations = info.Confirmations
		}
		inflightMu.Unlock()
	}

	return info, nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// randHex returns 2n hex characters from crypto/rand. Used to synthesize a
// transaction id when the underlying RPC path returns nothing (stub mode).
func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failure on Linux/Windows means the OS is broken; panic
		// is appropriate (mirrors stdlib uuid behavior).
		panic("bridge: crypto/rand: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// deriveRelayerAddr is a placeholder for "given an ETH private key in hex,
// derive the corresponding 0x-prefixed address". Real implementation needs
// secp256k1 pubkey derivation + keccak256(pub)[12:] — see ADR-BRIDGE-001.
//
// Today: if ETH_PRIVATE_KEY isn't set we return "" (managed-RPC mode picks
// its own from address). If it IS set we return a synthetic placeholder so
// the Lock call has a non-empty `from` field — production-incorrect but
// keeps the orchestration testable.
func deriveRelayerAddr(privKeyHex string) string {
	if privKeyHex == "" {
		return ""
	}
	// Truncate to a deterministic 8-char tag so logs don't leak the key.
	trimmed := strings.TrimPrefix(strings.TrimPrefix(privKeyHex, "0x"), "0X")
	if len(trimmed) < 8 {
		return "0xrelayer_unknown"
	}
	return "0xrelayer_" + trimmed[:8]
}

// redactAddr trims a hex/SS58 address for log output so we don't dump the
// recipient in full on every bridge call.
func redactAddr(addr string) string {
	if len(addr) <= 12 {
		return addr
	}
	return addr[:6] + "..." + addr[len(addr)-4:]
}

// ResetInflightForTest clears the inflight cache. ONLY for tests.
func ResetInflightForTest() {
	inflightMu.Lock()
	inflight = make(map[string]*BridgeStatusInfo)
	inflightMu.Unlock()
}

// _ = context.Background  // ctx threading reserved for future polling tasks.
var _ = context.Background
