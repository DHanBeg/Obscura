package bridge

import (
	"context"
	"database/sql"
	"encoding/hex"
	"math/big"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// abiEncodeLockedData builds the ABI-encoded tail of a mock
// Locked(address indexed user, uint256 amount, string destChain,
//
//	string recipient, uint256 nonce) event — i.e. what would land in
//
// ev.Data (the non-indexed fields: amount, destChain, recipient, nonce).
func abiEncodeLockedData(amount *big.Int, destChain, recipient string, nonce *big.Int) string {
	word := func(v *big.Int) []byte {
		w := make([]byte, 32)
		b := v.Bytes()
		copy(w[32-len(b):], b)
		return w
	}
	encodeString := func(s string) []byte {
		out := append([]byte{}, word(big.NewInt(int64(len(s))))...)
		out = append(out, []byte(s)...)
		if pad := (32 - len(s)%32) % 32; pad > 0 {
			out = append(out, make([]byte, pad)...)
		}
		return out
	}

	head := make([]byte, 128)
	copy(head[0:32], word(amount)) // slot0: amount
	// slot1/slot2 (destChain/recipient offsets) filled in below once tail is known
	copy(head[96:128], word(nonce)) // slot3: nonce

	destChainBytes := encodeString(destChain)
	destChainOffset := big.NewInt(int64(len(head)))
	recipientOffset := big.NewInt(int64(len(head) + len(destChainBytes)))
	recipientBytes := encodeString(recipient)

	copy(head[32:64], word(destChainOffset))
	copy(head[64:96], word(recipientOffset))

	full := append(head, destChainBytes...)
	full = append(full, recipientBytes...)
	return "0x" + hex.EncodeToString(full)
}

func TestDecodeLockedEventData(t *testing.T) {
	amount := big.NewInt(1_500_000_000_000_000_000) // 1.5 OBS (18 decimals)
	nonce := big.NewInt(42)
	destChain := "dot"
	recipient := "5FHneW46xGXgs5mUiveU4sbTyGBzmstUspZC92UhjJM694ty" // sr25519 SS58

	data := abiEncodeLockedData(amount, destChain, recipient, nonce)

	gotAmount, gotDestChain, gotRecipient, gotNonce, err := decodeLockedEventData(data)
	if err != nil {
		t.Fatalf("decodeLockedEventData: %v", err)
	}
	if gotAmount != amount.String() {
		t.Errorf("amount = %q, want %q", gotAmount, amount.String())
	}
	if gotDestChain != destChain {
		t.Errorf("destChain = %q, want %q", gotDestChain, destChain)
	}
	if gotRecipient != recipient {
		t.Errorf("recipient = %q, want %q", gotRecipient, recipient)
	}
	if gotNonce != nonce.String() {
		t.Errorf("nonce = %q, want %q", gotNonce, nonce.String())
	}
}

func TestDecodeLockedEventData_LongStrings(t *testing.T) {
	// recipient/destChain lengths that aren't multiples of 32 bytes exercise
	// the ABI tail padding math.
	amount := big.NewInt(999)
	nonce := big.NewInt(1)
	destChain := "polkadot-paseo-testnet"
	recipient := "5GrwvaEF5zXb26Fz9rcQpDWS57CtERHpNehXCPcNoHGKutQY"

	data := abiEncodeLockedData(amount, destChain, recipient, nonce)
	gotAmount, gotDestChain, gotRecipient, gotNonce, err := decodeLockedEventData(data)
	if err != nil {
		t.Fatalf("decodeLockedEventData: %v", err)
	}
	if gotAmount != amount.String() || gotDestChain != destChain ||
		gotRecipient != recipient || gotNonce != nonce.String() {
		t.Errorf("got (%q,%q,%q,%q), want (%q,%q,%q,%q)",
			gotAmount, gotDestChain, gotRecipient, gotNonce,
			amount.String(), destChain, recipient, nonce.String())
	}
}

func TestTopicToAddress(t *testing.T) {
	addr := "1234567890123456789012345678901234567890" // 40 hex chars = 20 bytes
	topic := "0x" + strings.Repeat("0", 24) + addr
	if got := topicToAddress(topic); got != "0x"+addr {
		t.Errorf("topicToAddress(%s) = %s, want 0x%s", topic, got, addr)
	}
}

// setupTestDB mirrors the bridge_transfers schema from migration
// 093_bridge_transfers in internal/db/database.go.
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "relayer_test.db")
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	_, err = sqlDB.Exec(`CREATE TABLE bridge_transfers (
		id                TEXT PRIMARY KEY,
		chain_from        TEXT NOT NULL,
		chain_to          TEXT NOT NULL,
		sender_address    TEXT NOT NULL,
		recipient_address TEXT NOT NULL,
		amount            TEXT NOT NULL,
		tx_hash           TEXT NOT NULL DEFAULT '',
		status            TEXT NOT NULL DEFAULT 'pending',
		created_at        DATETIME NOT NULL,
		confirmed_at      DATETIME,
		log_index         TEXT NOT NULL DEFAULT '',
		dot_tx_hash       TEXT NOT NULL DEFAULT '',
		retry_count       INTEGER NOT NULL DEFAULT 0,
		error_message     TEXT NOT NULL DEFAULT '',
		dot_extrinsic_hex TEXT NOT NULL DEFAULT '',
		dot_amount_planck TEXT NOT NULL DEFAULT ''
	)`)
	if err != nil {
		t.Fatalf("create bridge_transfers: %v", err)
	}
	if _, err := sqlDB.Exec(
		"CREATE UNIQUE INDEX idx_bridge_transfers_txhash_logindex ON bridge_transfers(tx_hash, log_index)",
	); err != nil {
		t.Fatalf("create unique index: %v", err)
	}
	return sqlDB
}

// TestProcessLockEvent_EndToEnd proves the fix against a full mock eth_getLogs
// entry: user in Topics[1], recipient/amount ABI-decoded from Data — not from
// Topics[2] (the pre-fix bug).
func TestProcessLockEvent_EndToEnd(t *testing.T) {
	sqlDB := setupTestDB(t)
	r := &Relayer{db: sqlDB}

	amount := big.NewInt(2_000_000_000_000_000_000)
	nonce := big.NewInt(7)
	recipient := "5GrwvaEF5zXb26Fz9rcQpDWS57CtERHpNehXCPcNoHGKutQY"
	data := abiEncodeLockedData(amount, "dot", recipient, nonce)

	senderAddr := "abcdef1234567890abcdef1234567890abcdef12" // 40 hex chars
	userTopic := "0x" + strings.Repeat("0", 24) + senderAddr

	ev := ethLog{
		Topics:          []string{lockEventTopic, userTopic},
		Data:            data,
		TransactionHash: "0xdeadbeef",
		LogIndex:        "0x0",
	}

	if err := r.processLockEvent(context.Background(), ev); err != nil {
		t.Fatalf("processLockEvent: %v", err)
	}

	var gotSender, gotRecipient, gotAmount, gotStatus string
	row := sqlDB.QueryRow(
		"SELECT sender_address, recipient_address, amount, status FROM bridge_transfers WHERE tx_hash = ?",
		"0xdeadbeef")
	if err := row.Scan(&gotSender, &gotRecipient, &gotAmount, &gotStatus); err != nil {
		t.Fatalf("query bridge_transfers: %v", err)
	}

	if gotSender != "0x"+senderAddr {
		t.Errorf("sender_address = %q, want %q", gotSender, "0x"+senderAddr)
	}
	if gotRecipient != recipient {
		t.Errorf("recipient_address = %q, want %q (this is the pre-fix bug: reading Topics[2] instead of ABI-decoding Data)", gotRecipient, recipient)
	}
	if gotAmount != amount.String() {
		t.Errorf("amount = %q, want %q", gotAmount, amount.String())
	}
	// r.dot yapılandırılmadı (DOT_RELAYER_SEED yok) -> DOT tarafı devre dışı,
	// satır 'pending' kalır (bkz. processDotSide, r.dot==nil dalı) — para
	// kaybolmaz, DOT tarafı ayağa kalkınca retry sweep işler.
	if gotStatus != "pending" {
		t.Errorf("status = %q, want %q (DOT tarafı yapılandırılmamışken 'pending' kalmalı)", gotStatus, "pending")
	}
}
