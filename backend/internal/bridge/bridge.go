// Package bridge — Cross-chain bridge (FAZ 3, spec 12.3)
//
// Ethereum ve Polkadot köprüsü için interface + HTTP client implementasyonu.
// FAZ 3: RPC tabanlı stub + lock/unlock mekanizması.
// FAZ 4: Tam on-chain relayer + ZK proof gönderimi.
//
// ETH transaction signing durumu:
//   - Mevcut: eth_sendTransaction (hosted/managed node — node private key'i kullanır)
//   - TODO: ETH_PRIVATE_KEY env ile eth_sendRawTransaction (self-custody relayer)
//     Bunun için secp256k1 ECDSA + RLP encoding gerekir. CGO_ENABLED=0 kısıtı
//     nedeniyle go-ethereum/crypto kullanılamaz. Alternatif: pure-Go secp256k1
//     (github.com/decred/dcrd/dcrec/secp256k1 — CGO-free, zaten transitif bağımlılık
//     olarak mevcut) + manuel RLP encoding.
//     Bkz: ADR-BRIDGE-001 (taslak).
package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

// Chain — desteklenen zincirler
type Chain string

const (
	ChainEthereum Chain = "ethereum"
	ChainPolkadot Chain = "polkadot"
)

// BridgeRequest — OBS token köprüleme talebi
type BridgeRequest struct {
	FromChain     Chain  `json:"from_chain"`
	ToChain       Chain  `json:"to_chain"`
	Amount        string `json:"amount"`        // OBS miktarı (token birim)
	SenderAddr    string `json:"sender_addr"`   // kaynak zincir adresi
	RecipientAddr string `json:"recipient_addr"` // hedef zincir adresi
	ObsDID        string `json:"obs_did"`       // Obscura DID (kanıt için)
}

// BridgeResult — köprüleme sonucu
type BridgeResult struct {
	TxID      string    `json:"tx_id"`
	Status    string    `json:"status"`    // "pending" | "confirmed" | "failed"
	Chain     Chain     `json:"chain"`
	CreatedAt time.Time `json:"created_at"`
}

// Config — bridge yapılandırması (env'den okunur)
type Config struct {
	EthereumRPC    string // ETH_RPC_URL env
	PolkadotRPC    string // DOT_RPC_URL env
	LockContract   string // ETH_LOCK_CONTRACT env (Ethereum lock contract adresi)
	RelayerSecret  string // BRIDGE_RELAYER_SECRET env
	EthPrivateKey  string // ETH_PRIVATE_KEY env (hex, 0x prefix opsiyonel)
	                      // TODO(ADR-BRIDGE-001): eth_sendRawTransaction için kullanılacak.
	                      // Şu an sadece varlığı kontrol edilir; gerçek signing FAZ 4'te.
	RPCTimeout     time.Duration // varsayılan 15s
	RPCMaxRetries  int           // varsayılan 3
}

// ConfigFromEnv — ortam değişkenlerinden Config oluştur
func ConfigFromEnv() Config {
	timeout := 15 * time.Second
	return Config{
		EthereumRPC:   os.Getenv("ETH_RPC_URL"),
		PolkadotRPC:   os.Getenv("DOT_RPC_URL"),
		LockContract:  os.Getenv("ETH_LOCK_CONTRACT"),
		RelayerSecret: os.Getenv("BRIDGE_RELAYER_SECRET"),
		EthPrivateKey: os.Getenv("ETH_PRIVATE_KEY"), // saklamak için SecretManager önerilir
		RPCTimeout:    timeout,
		RPCMaxRetries: 3,
	}
}

// Client — bridge istemcisi
type Client struct {
	cfg  Config
	http *http.Client
}

// New — yeni bridge istemcisi
func New(cfg Config) *Client {
	timeout := cfg.RPCTimeout
	if timeout == 0 {
		timeout = 15 * time.Second
	}
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: timeout},
	}
}

// Lock — kaynak zincirde OBS kilitle (Ethereum ERC-20 lock)
func (c *Client) Lock(req BridgeRequest) (*BridgeResult, error) {
	if req.FromChain == ChainEthereum {
		return c.ethLock(req)
	}
	if req.FromChain == ChainPolkadot {
		return c.dotLock(req)
	}
	return nil, fmt.Errorf("desteklenmeyen kaynak zincir: %s", req.FromChain)
}

// Unlock — hedef zincirde OBS kilidi aç (mint veya release)
func (c *Client) Unlock(txID string, toChain Chain, recipientAddr, amount string) (*BridgeResult, error) {
	if toChain == ChainEthereum {
		return c.ethUnlock(txID, recipientAddr, amount)
	}
	if toChain == ChainPolkadot {
		return c.dotUnlock(txID, recipientAddr, amount)
	}
	return nil, fmt.Errorf("desteklenmeyen hedef zincir: %s", toChain)
}

// Status — köprüleme işleminin durumunu sorgula
func (c *Client) Status(txID string, chain Chain) (*BridgeResult, error) {
	switch chain {
	case ChainEthereum:
		return c.ethTxStatus(txID)
	case ChainPolkadot:
		return c.dotTxStatus(txID)
	}
	return nil, fmt.Errorf("bilinmeyen zincir: %s", chain)
}

// ─── Ethereum ─────────────────────────────────────────────────────────────────

func (c *Client) ethLock(req BridgeRequest) (*BridgeResult, error) {
	if c.cfg.EthereumRPC == "" {
		return nil, fmt.Errorf("ETH_RPC_URL yapılandırılmadı")
	}
	if c.cfg.LockContract == "" {
		return nil, fmt.Errorf("ETH_LOCK_CONTRACT yapılandırılmadı")
	}

	// Signing stratejisi seç:
	//   - ETH_PRIVATE_KEY varsa → eth_sendRawTransaction (self-custody relayer) [STUB]
	//   - Yoksa → eth_sendTransaction (managed/hosted node — node'un kendi key'i)
	if c.cfg.EthPrivateKey != "" {
		return c.ethLockSigned(req)
	}
	return c.ethLockManaged(req)
}

// ethLockSigned — ETH_PRIVATE_KEY ile imzalı ham transaction gönder.
//
// TODO(ADR-BRIDGE-001): Gerçek secp256k1 ECDSA + RLP encoding burada yapılacak.
// Gerekli bileşenler (hepsi CGO-free):
//   - github.com/decred/dcrd/dcrec/secp256k1/v4 — ECDSA imzası (zaten transitif bağımlılık)
//   - Manuel EIP-155 RLP encoding (go std library yeterli)
//   - eth_getTransactionCount (nonce sorgusu)
//   - eth_gasPrice / eth_estimateGas
//
// Şu an: "sign_stub" transaction bytes döndürür, gerçek broadcast yapmaz.
func (c *Client) ethLockSigned(req BridgeRequest) (*BridgeResult, error) {
	// sign_stub: gerçek signing implementasyonu pending (bkz ADR-BRIDGE-001)
	log.Printf("[bridge] ETH lock sign_stub: pending real key signing (from=%s, amount=%s, contract=%s)",
		req.SenderAddr, req.Amount, c.cfg.LockContract)

	// Nonce sorgula (gerçek implementasyon için altyapı)
	nonceBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "eth_getTransactionCount",
		"params":  []interface{}{req.SenderAddr, "latest"},
		"id":      1,
	}
	nonceResult, err := c.rpcCallWithRetry(context.Background(), c.cfg.EthereumRPC, nonceBody)
	if err != nil {
		log.Printf("[bridge] ETH nonce sorgusu başarısız: %v", err)
		// Stub: nonce hatası lock işlemini durdurmaz
	} else {
		log.Printf("[bridge] ETH nonce: %v", nonceResult["result"])
	}

	// TODO: Gerçek implementasyonda burada yapılacaklar:
	// 1. nonce := parseHexUint64(nonceResult["result"])
	// 2. gasPrice := eth_gasPrice RPC çağrısı
	// 3. calldata := buildLockCalldata(...)
	// 4. tx := buildEIP155Tx(nonce, gasPrice, gasLimit, lockContract, 0, calldata, chainID)
	// 5. sig := secp256k1.Sign(keccak256(tx), privateKey)
	// 6. rawTx := rlpEncode(tx + sig)
	// 7. eth_sendRawTransaction(rawTx)

	// Stub sonuç — production'da gerçek tx hash olacak
	stubTxID := fmt.Sprintf("stub_signed_%s_%d", req.SenderAddr[:min(8, len(req.SenderAddr))], time.Now().UnixNano())
	return &BridgeResult{
		TxID:      stubTxID,
		Status:    "pending_sign_stub",
		Chain:     ChainEthereum,
		CreatedAt: time.Now().UTC(),
	}, nil
}

// ethLockManaged — managed/hosted node üzerinden eth_sendTransaction.
// Node'un kendi private key'ini kullanır (ör. Infura, Alchemy yönetimli hesap).
func (c *Client) ethLockManaged(req BridgeRequest) (*BridgeResult, error) {
	body := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "eth_sendTransaction",
		"params": []interface{}{
			map[string]string{
				"from": req.SenderAddr,
				"to":   c.cfg.LockContract,
				"data": buildLockCalldata(req.RecipientAddr, req.Amount, string(req.ToChain), req.ObsDID),
			},
		},
		"id": 1,
	}

	result, err := c.rpcCallWithRetry(context.Background(), c.cfg.EthereumRPC, body)
	if err != nil {
		return nil, fmt.Errorf("eth lock (managed): %w", err)
	}

	txHash, _ := result["result"].(string)
	if txHash == "" {
		return nil, fmt.Errorf("eth lock: boş tx hash döndü")
	}
	log.Printf("[bridge] ETH lock gönderildi: txHash=%s, amount=%s", txHash, req.Amount)
	return &BridgeResult{
		TxID:      txHash,
		Status:    "pending",
		Chain:     ChainEthereum,
		CreatedAt: time.Now().UTC(),
	}, nil
}

// min — Go 1.21 öncesi uyumluluk için (generic min yoksa)
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (c *Client) ethUnlock(txID, recipient, amount string) (*BridgeResult, error) {
	// Production'da: relayer servis veya on-chain relay proof doğrular
	// FAZ 3 stub: başarılı kabul et
	return &BridgeResult{
		TxID:      "eth_unlock_" + txID,
		Status:    "confirmed",
		Chain:     ChainEthereum,
		CreatedAt: time.Now().UTC(),
	}, nil
}

func (c *Client) ethTxStatus(txID string) (*BridgeResult, error) {
	body := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "eth_getTransactionReceipt",
		"params":  []interface{}{txID},
		"id":      1,
	}
	result, err := c.rpcCallWithRetry(context.Background(), c.cfg.EthereumRPC, body)
	if err != nil {
		return nil, err
	}
	status := "pending"
	if rec, ok := result["result"].(map[string]interface{}); ok && rec != nil {
		if rec["status"] == "0x1" {
			status = "confirmed"
		} else if rec["status"] == "0x0" {
			status = "failed"
		}
	}
	return &BridgeResult{TxID: txID, Status: status, Chain: ChainEthereum, CreatedAt: time.Now().UTC()}, nil
}

// ─── Polkadot ─────────────────────────────────────────────────────────────────

func (c *Client) dotLock(req BridgeRequest) (*BridgeResult, error) {
	if c.cfg.PolkadotRPC == "" {
		return nil, fmt.Errorf("DOT_RPC_URL yapılandırılmadı")
	}
	// author_submitExtrinsic — XCM veya pallet_bridge çağrısı
	body := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "author_submitExtrinsic",
		"params":  []interface{}{buildDotLockExtrinsic(req)},
		"id":      1,
	}
	result, err := c.rpcCallWithRetry(context.Background(), c.cfg.PolkadotRPC, body)
	if err != nil {
		return nil, fmt.Errorf("dot lock: %w", err)
	}
	txHash, _ := result["result"].(string)
	return &BridgeResult{
		TxID:      txHash,
		Status:    "pending",
		Chain:     ChainPolkadot,
		CreatedAt: time.Now().UTC(),
	}, nil
}

func (c *Client) dotUnlock(txID, recipient, amount string) (*BridgeResult, error) {
	return &BridgeResult{
		TxID:      "dot_unlock_" + txID,
		Status:    "confirmed",
		Chain:     ChainPolkadot,
		CreatedAt: time.Now().UTC(),
	}, nil
}

func (c *Client) dotTxStatus(txID string) (*BridgeResult, error) {
	body := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "chain_getBlock",
		"params":  []interface{}{txID},
		"id":      1,
	}
	result, err := c.rpcCallWithRetry(context.Background(), c.cfg.PolkadotRPC, body)
	if err != nil {
		return nil, err
	}
	status := "pending"
	if result["result"] != nil {
		status = "confirmed"
	}
	return &BridgeResult{TxID: txID, Status: status, Chain: ChainPolkadot, CreatedAt: time.Now().UTC()}, nil
}

// ─── Yardımcılar ──────────────────────────────────────────────────────────────

// rpcCall — tek seferlik RPC çağrısı (geriye dönük uyumluluk için korunuyor)
func (c *Client) rpcCall(rpcURL string, body interface{}) (map[string]interface{}, error) {
	return c.rpcCallWithRetry(context.Background(), rpcURL, body)
}

// rpcCallWithRetry — exponential backoff ile yeniden dene.
// Geçici ağ hataları (bağlantı zaman aşımı, 5xx) için retry yapar;
// kalıcı hatalar (RPC error payload) için hemen döner.
func (c *Client) rpcCallWithRetry(ctx context.Context, rpcURL string, body interface{}) (map[string]interface{}, error) {
	maxRetries := c.cfg.RPCMaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("RPC istek serileştirme: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 500ms, 1s, 2s
			backoff := time.Duration(500*(1<<uint(attempt-1))) * time.Millisecond
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("RPC iptal (ctx): %w", ctx.Err())
			case <-time.After(backoff):
			}
			log.Printf("[bridge] RPC yeniden deneme %d/%d: %s", attempt+1, maxRetries, rpcURL)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, rpcURL,
			bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("RPC istek oluşturma: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if c.cfg.RelayerSecret != "" {
			req.Header.Set("X-Relayer-Secret", c.cfg.RelayerSecret)
		}

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("RPC bağlantısı (deneme %d): %w", attempt+1, err)
			log.Printf("[bridge] RPC hata: %v", lastErr)
			continue // retry
		}

		var result map[string]interface{}
		decodeErr := json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()

		if decodeErr != nil {
			lastErr = fmt.Errorf("RPC yanıt decode (deneme %d, HTTP %d): %w",
				attempt+1, resp.StatusCode, decodeErr)
			if resp.StatusCode >= 500 {
				log.Printf("[bridge] RPC sunucu hatası %d, yeniden deneniyor: %v", resp.StatusCode, lastErr)
				continue // retry on 5xx
			}
			return nil, lastErr // 4xx veya parse hatası — kalıcı
		}

		// JSON-RPC hata payload → kalıcı hata, retry yok
		if errObj, ok := result["error"]; ok && errObj != nil {
			return nil, fmt.Errorf("RPC protokol hatası: %v", errObj)
		}

		return result, nil
	}

	return nil, fmt.Errorf("RPC %d denemede başarısız: %w", maxRetries, lastErr)
}

func buildLockCalldata(recipient, amount, targetChain, obsDID string) string {
	// ABI encoding stub: lock(address recipient, uint256 amount, string targetChain, string obsDID)
	// Production'da: go-ethereum ABI encoder kullanılır
	return fmt.Sprintf("0xlock_%s_%s_%s_%s", recipient, amount, targetChain, obsDID)
}

func buildDotLockExtrinsic(req BridgeRequest) string {
	// XCM extrinsic encoding stub
	// Production'da: scale encoding + polkadot-api kullanılır
	return fmt.Sprintf("0x%x", []byte(fmt.Sprintf("lock_%s_%s", req.RecipientAddr, req.Amount)))
}
