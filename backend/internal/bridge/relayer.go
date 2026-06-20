// Package bridge — ETH→DOT event relayer (spec Bölüm 12.3)
//
// Her 30s'de Ethereum'u eth_getLogs ile polling yapar (CGO=0, raw HTTP JSON-RPC).
// Yeni Lock event'i bulunca bridge_transfers tablosuna pending kaydı yazar,
// ardından DOT mint'i simüle ederek confirmed'a alır.
// Gerçek DOT extrinsic gönderimi FAZ 4'te.
package bridge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Lock(address,address,uint256) event topic (keccak256 precomputed)
const lockEventTopic = "0x8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925"

// Relayer polls Ethereum for OBSBridge Lock events.
type Relayer struct {
	EthRPCURL    string
	DotRPCURL    string
	ContractAddr string
	PollInterval time.Duration
	db           *sql.DB
	httpClient   *http.Client
	lastBlock    uint64
}

// NewRelayer reads config from env and returns a Relayer.
func NewRelayer(database *sql.DB) *Relayer {
	return &Relayer{
		EthRPCURL:    os.Getenv("ETH_RPC_URL"),
		DotRPCURL:    os.Getenv("DOT_RPC_URL"),
		ContractAddr: os.Getenv("ETH_BRIDGE_CONTRACT"),
		PollInterval: 30 * time.Second,
		db:           database,
		httpClient:   &http.Client{Timeout: 15 * time.Second},
	}
}

// StartRelayer spawns background polling goroutine.
func StartRelayer(ctx context.Context, database *sql.DB) {
	r := NewRelayer(database)
	go r.run(ctx)
}

func (r *Relayer) run(ctx context.Context) {
	if r.EthRPCURL == "" || r.ContractAddr == "" {
		log.Println("[relayer] bridge not configured — set ETH_RPC_URL + ETH_BRIDGE_CONTRACT")
		return
	}
	log.Printf("[relayer] started — contract=%s interval=%s", r.ContractAddr, r.PollInterval)

	if head, err := r.ethBlockNumber(ctx); err == nil {
		r.lastBlock = head
		log.Printf("[relayer] initialised at block %d", head)
	}

	ticker := time.NewTicker(r.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Println("[relayer] stopped")
			return
		case <-ticker.C:
			if err := r.poll(ctx); err != nil {
				log.Printf("[relayer] poll error: %v", err)
			}
		}
	}
}

func (r *Relayer) poll(ctx context.Context) error {
	head, err := r.ethBlockNumber(ctx)
	if err != nil {
		return fmt.Errorf("eth_blockNumber: %w", err)
	}
	if head <= r.lastBlock {
		return nil
	}

	from := fmt.Sprintf("0x%x", r.lastBlock+1)
	to := fmt.Sprintf("0x%x", head)
	events, err := r.ethGetLogs(ctx, from, to)
	if err != nil {
		return fmt.Errorf("eth_getLogs: %w", err)
	}
	log.Printf("[relayer] blocks %d→%d: %d event(s)", r.lastBlock+1, head, len(events))
	for _, ev := range events {
		if err := r.processLockEvent(ctx, ev); err != nil {
			log.Printf("[relayer] processLockEvent tx=%s: %v", ev.TransactionHash, err)
		}
	}
	r.lastBlock = head
	return nil
}

type ethLog struct {
	Topics          []string `json:"topics"`
	Data            string   `json:"data"`
	TransactionHash string   `json:"transactionHash"`
	LogIndex        string   `json:"logIndex"`
}

func (r *Relayer) ethGetLogs(ctx context.Context, from, to string) ([]ethLog, error) {
	body := map[string]interface{}{
		"jsonrpc": "2.0", "method": "eth_getLogs", "id": 1,
		"params": []interface{}{map[string]interface{}{
			"fromBlock": from, "toBlock": to,
			"address": r.ContractAddr,
			"topics":  []string{lockEventTopic},
		}},
	}
	raw, err := r.rpcCall(ctx, r.EthRPCURL, body)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(raw["result"])
	if err != nil {
		return nil, fmt.Errorf("marshal logs: %w", err)
	}
	var logs []ethLog
	if err := json.Unmarshal(b, &logs); err != nil {
		return nil, fmt.Errorf("unmarshal logs: %w", err)
	}
	return logs, nil
}

func (r *Relayer) ethBlockNumber(ctx context.Context) (uint64, error) {
	raw, err := r.rpcCall(ctx, r.EthRPCURL, map[string]interface{}{
		"jsonrpc": "2.0", "method": "eth_blockNumber", "params": []interface{}{}, "id": 1,
	})
	if err != nil {
		return 0, err
	}
	s := strings.TrimPrefix(raw["result"].(string), "0x")
	return strconv.ParseUint(s, 16, 64)
}

func (r *Relayer) processLockEvent(ctx context.Context, ev ethLog) error {
	sender := topicToAddress(safeIdx(ev.Topics, 1))
	recipient := topicToAddress(safeIdx(ev.Topics, 2))
	amount := hexUint256ToDecimal(ev.Data)
	txHash := ev.TransactionHash

	var existing string
	if err := r.db.QueryRowContext(ctx,
		"SELECT id FROM bridge_transfers WHERE tx_hash = ?", txHash,
	).Scan(&existing); err == nil {
		return nil // duplicate
	}

	id := syntheticID(txHash, ev.LogIndex)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO bridge_transfers
			(id,chain_from,chain_to,sender_address,recipient_address,amount,tx_hash,status,created_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		id, "eth", "dot", sender, recipient, amount, txHash, "pending", now,
	); err != nil {
		return fmt.Errorf("insert: %w", err)
	}
	log.Printf("[relayer] lock id=%s %s→%s %s wei", id, sender, recipient, amount)

	// Simulate DOT mint (FAZ 4: replace with real substrate extrinsic)
	confirmed := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.ExecContext(ctx,
		"UPDATE bridge_transfers SET status='confirmed', confirmed_at=? WHERE id=?",
		confirmed, id)
	return err
}

func (r *Relayer) rpcCall(ctx context.Context, url string, body interface{}) (map[string]interface{}, error) {
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(b, &result); err != nil {
		return nil, err
	}
	if errVal, ok := result["error"]; ok && errVal != nil {
		return nil, fmt.Errorf("rpc error: %v", errVal)
	}
	return result, nil
}

func topicToAddress(topic string) string {
	topic = strings.TrimPrefix(topic, "0x")
	if len(topic) < 64 {
		return "0x" + topic
	}
	return "0x" + topic[24:]
}

func hexUint256ToDecimal(data string) string {
	data = strings.TrimPrefix(data, "0x")
	if data == "" {
		return "0"
	}
	b, err := hex.DecodeString(data)
	if err != nil {
		return "0"
	}
	const base = uint64(1_000_000_000)
	limbs := make([]uint64, 0, 12)
	for _, byt := range b {
		carry := uint64(byt)
		for i := range limbs {
			v := limbs[i]*256 + carry
			limbs[i] = v % base
			carry = v / base
		}
		for carry > 0 {
			limbs = append(limbs, carry%base)
			carry /= base
		}
	}
	if len(limbs) == 0 {
		return "0"
	}
	sb := strings.Builder{}
	sb.WriteString(strconv.FormatUint(limbs[len(limbs)-1], 10))
	for i := len(limbs) - 2; i >= 0; i-- {
		sb.WriteString(fmt.Sprintf("%09d", limbs[i]))
	}
	return sb.String()
}

func syntheticID(txHash, logIndex string) string {
	h := sha256.Sum256([]byte(txHash + ":" + logIndex))
	return hex.EncodeToString(h[:16])
}

func safeIdx(s []string, i int) string {
	if i < len(s) {
		return s[i]
	}
	return ""
}
