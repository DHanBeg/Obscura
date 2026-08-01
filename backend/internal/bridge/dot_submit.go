// Package bridge — extrinsic göndermek için gereken zincir verisini çeken
// RPC yardımcıları + author_submitExtrinsic (Bridge madde 4, PARÇA 2).
//
// ÖNEMLİ: SubmitExtrinsic bu dosyada TANIMLIDIR ama hiçbir yerden OTOMATİK
// ÇAĞRILMAZ. Gerçek zincire yazma geri alınamaz — çağıran kod, kullanıcıya
// DecodeExtrinsicForReview çıktısını gösterip AÇIK ONAY almadan bu
// fonksiyonu çağırmamalı.
package bridge

import (
	"context"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

// FetchMetadata, state_getMetadata'yı çağırıp V14 metadata'yı çözer.
func (c *Client) FetchMetadata(ctx context.Context) (*Metadata, error) {
	if c.cfg.PolkadotRPC == "" {
		return nil, fmt.Errorf("DOT_RPC_URL yapılandırılmadı")
	}
	result, err := c.rpcCallWithRetry(ctx, c.cfg.PolkadotRPC, map[string]interface{}{
		"jsonrpc": "2.0", "method": "state_getMetadata", "params": []interface{}{}, "id": 1,
	})
	if err != nil {
		return nil, fmt.Errorf("state_getMetadata: %w", err)
	}
	hexStr, _ := result["result"].(string)
	if hexStr == "" {
		return nil, fmt.Errorf("state_getMetadata boş sonuç döndü")
	}
	md, err := DecodeMetadataV14(hexStr)
	if err != nil {
		return nil, fmt.Errorf("metadata decode: %w", err)
	}
	return md, nil
}

// RuntimeVersion — state_getRuntimeVersion'dan bu paket için gereken alanlar.
type RuntimeVersion struct {
	SpecVersion        uint32
	TransactionVersion uint32
}

// FetchRuntimeVersion, state_getRuntimeVersion'ı çağırıp spec_version +
// transaction_version'ı döndürür (CheckSpecVersion/CheckTxVersion signed
// extension'ları için zorunlu — elle uydurma YOK).
func (c *Client) FetchRuntimeVersion(ctx context.Context) (*RuntimeVersion, error) {
	if c.cfg.PolkadotRPC == "" {
		return nil, fmt.Errorf("DOT_RPC_URL yapılandırılmadı")
	}
	result, err := c.rpcCallWithRetry(ctx, c.cfg.PolkadotRPC, map[string]interface{}{
		"jsonrpc": "2.0", "method": "state_getRuntimeVersion", "params": []interface{}{}, "id": 1,
	})
	if err != nil {
		return nil, fmt.Errorf("state_getRuntimeVersion: %w", err)
	}
	res, ok := result["result"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("state_getRuntimeVersion: beklenmeyen yanıt şekli")
	}
	specVersion, err := jsonNumberToUint32(res["specVersion"])
	if err != nil {
		return nil, fmt.Errorf("specVersion: %w", err)
	}
	txVersion, err := jsonNumberToUint32(res["transactionVersion"])
	if err != nil {
		return nil, fmt.Errorf("transactionVersion: %w", err)
	}
	return &RuntimeVersion{SpecVersion: specVersion, TransactionVersion: txVersion}, nil
}

// FetchGenesisHash, chain_getBlockHash(0)'ı çağırıp genesis hash'i 32 bayt
// olarak döndürür (CheckGenesis signed extension'ı için zorunlu).
func (c *Client) FetchGenesisHash(ctx context.Context) ([32]byte, error) {
	var out [32]byte
	if c.cfg.PolkadotRPC == "" {
		return out, fmt.Errorf("DOT_RPC_URL yapılandırılmadı")
	}
	result, err := c.rpcCallWithRetry(ctx, c.cfg.PolkadotRPC, map[string]interface{}{
		"jsonrpc": "2.0", "method": "chain_getBlockHash", "params": []interface{}{0}, "id": 1,
	})
	if err != nil {
		return out, fmt.Errorf("chain_getBlockHash(0): %w", err)
	}
	hexStr, _ := result["result"].(string)
	b, err := hex.DecodeString(strings.TrimPrefix(hexStr, "0x"))
	if err != nil {
		return out, fmt.Errorf("genesis hash hex decode: %w", err)
	}
	if len(b) != 32 {
		return out, fmt.Errorf("genesis hash %d bayt, beklenen 32", len(b))
	}
	copy(out[:], b)
	return out, nil
}

// FetchBlockNumber, chain_getHeader(blockHash)'ı çağırıp bloğun numarasını
// döndürür (finality kontrolü için — inBlock hash'inin finalized zincirde
// olup olmadığını numaraya göre karşılaştırmak amacıyla).
func (c *Client) FetchBlockNumber(ctx context.Context, blockHash string) (uint64, error) {
	if c.cfg.PolkadotRPC == "" {
		return 0, fmt.Errorf("DOT_RPC_URL yapılandırılmadı")
	}
	result, err := c.rpcCallWithRetry(ctx, c.cfg.PolkadotRPC, map[string]interface{}{
		"jsonrpc": "2.0", "method": "chain_getHeader", "params": []interface{}{blockHash}, "id": 1,
	})
	if err != nil {
		return 0, fmt.Errorf("chain_getHeader: %w", err)
	}
	res, ok := result["result"].(map[string]interface{})
	if !ok {
		return 0, fmt.Errorf("chain_getHeader: beklenmeyen yanıt şekli")
	}
	numStr, _ := res["number"].(string)
	if numStr == "" {
		return 0, fmt.Errorf("chain_getHeader: number alanı yok")
	}
	return jsonNumberToUint64(numStr)
}

// FetchFinalizedHead, chain_getFinalizedHead'i çağırıp zincirin şu anki
// finalized blok hash'ini döndürür.
func (c *Client) FetchFinalizedHead(ctx context.Context) (string, error) {
	if c.cfg.PolkadotRPC == "" {
		return "", fmt.Errorf("DOT_RPC_URL yapılandırılmadı")
	}
	result, err := c.rpcCallWithRetry(ctx, c.cfg.PolkadotRPC, map[string]interface{}{
		"jsonrpc": "2.0", "method": "chain_getFinalizedHead", "params": []interface{}{}, "id": 1,
	})
	if err != nil {
		return "", fmt.Errorf("chain_getFinalizedHead: %w", err)
	}
	hash, _ := result["result"].(string)
	if hash == "" {
		return "", fmt.Errorf("chain_getFinalizedHead boş sonuç döndü")
	}
	return hash, nil
}

// FetchBlockHashAt, chain_getBlockHash(number)'ı çağırıp o numaradaki
// CANONICAL (kanonik) blok hash'ini döndürür — finalized zincirde
// inBlock hash'inin numarasında hangi blok olduğunu bağımsızca teyit etmek
// için (numara eşleşmesi yetmez, hash de eşleşmeli — reorg ihtimaline karşı).
func (c *Client) FetchBlockHashAt(ctx context.Context, number uint64) (string, error) {
	if c.cfg.PolkadotRPC == "" {
		return "", fmt.Errorf("DOT_RPC_URL yapılandırılmadı")
	}
	result, err := c.rpcCallWithRetry(ctx, c.cfg.PolkadotRPC, map[string]interface{}{
		"jsonrpc": "2.0", "method": "chain_getBlockHash", "params": []interface{}{number}, "id": 1,
	})
	if err != nil {
		return "", fmt.Errorf("chain_getBlockHash(%d): %w", number, err)
	}
	hash, _ := result["result"].(string)
	if hash == "" {
		return "", fmt.Errorf("chain_getBlockHash(%d) boş sonuç döndü", number)
	}
	return hash, nil
}

// FetchAccountNonce, system_accountNextIndex'i çağırıp verilen SS58 adresin
// bir sonraki nonce'unu döndürür (CheckNonce signed extension'ı için
// zorunlu — elle uydurma YOK, transaction pool'daki bekleyenleri de
// hesaba katar).
func (c *Client) FetchAccountNonce(ctx context.Context, ss58Address string) (uint64, error) {
	if c.cfg.PolkadotRPC == "" {
		return 0, fmt.Errorf("DOT_RPC_URL yapılandırılmadı")
	}
	result, err := c.rpcCallWithRetry(ctx, c.cfg.PolkadotRPC, map[string]interface{}{
		"jsonrpc": "2.0", "method": "system_accountNextIndex", "params": []interface{}{ss58Address}, "id": 1,
	})
	if err != nil {
		return 0, fmt.Errorf("system_accountNextIndex: %w", err)
	}
	return jsonNumberToUint64(result["result"])
}

// FetchTokenDecimals, system_properties'i çağırıp zincirin native token
// ondalık basamak sayısını döndürür (planck<->PAS çevrimi için ZORUNLU —
// elle uydurma YOK, zincirden değişebilir).
func (c *Client) FetchTokenDecimals(ctx context.Context) (uint32, error) {
	if c.cfg.PolkadotRPC == "" {
		return 0, fmt.Errorf("DOT_RPC_URL yapılandırılmadı")
	}
	result, err := c.rpcCallWithRetry(ctx, c.cfg.PolkadotRPC, map[string]interface{}{
		"jsonrpc": "2.0", "method": "system_properties", "params": []interface{}{}, "id": 1,
	})
	if err != nil {
		return 0, fmt.Errorf("system_properties: %w", err)
	}
	res, ok := result["result"].(map[string]interface{})
	if !ok {
		return 0, fmt.Errorf("system_properties: beklenmeyen yanıt şekli")
	}
	raw, ok := res["tokenDecimals"]
	if !ok {
		return 0, fmt.Errorf("system_properties yanıtında tokenDecimals yok")
	}
	// tokenDecimals genelde [12] gibi bir dizi (çoklu-asset zincirler için) ama
	// bazı düğümler tek sayı döndürebilir — ikisini de destekle.
	switch v := raw.(type) {
	case []interface{}:
		if len(v) == 0 {
			return 0, fmt.Errorf("system_properties tokenDecimals boş dizi")
		}
		return jsonNumberToUint32(v[0])
	default:
		return jsonNumberToUint32(v)
	}
}

// SubmitExtrinsic, imzalı bir extrinsic'i (hex, "0x" önekli ya da öneksiz)
// author_submitExtrinsic ile Paseo'ya gönderir ve tx hash'i döndürür.
//
// DUR: bu fonksiyon GERİ ALINAMAZ bir zincir yazma işlemi tetikler. Çağıran
// kod, DecodeExtrinsicForReview ile üretilen özeti kullanıcıya gösterip
// AÇIK ONAY almadan bunu çağırmamalıdır. Bu paketin kendi testleri bu
// fonksiyonu hiçbir zaman gerçek ağa karşı çalıştırmaz.
func (c *Client) SubmitExtrinsic(ctx context.Context, extrinsicHex string) (txHash string, err error) {
	if c.cfg.PolkadotRPC == "" {
		return "", fmt.Errorf("DOT_RPC_URL yapılandırılmadı")
	}
	if !strings.HasPrefix(extrinsicHex, "0x") {
		extrinsicHex = "0x" + extrinsicHex
	}
	result, err := c.rpcCallWithRetry(ctx, c.cfg.PolkadotRPC, map[string]interface{}{
		"jsonrpc": "2.0", "method": "author_submitExtrinsic", "params": []interface{}{extrinsicHex}, "id": 1,
	})
	if err != nil {
		return "", fmt.Errorf("author_submitExtrinsic: %w", err)
	}
	hash, _ := result["result"].(string)
	if hash == "" {
		return "", fmt.Errorf("author_submitExtrinsic boş tx hash döndü")
	}
	return hash, nil
}

// FetchBlockExtrinsics, chain_getBlock(blockHash)'ı çağırıp bloktaki
// extrinsic'lerin hex listesini döndürür (gönderilen extrinsic'in gerçekten
// o blokta yer aldığını hex-eşleşmesiyle teyit etmek için — inBlock durumu
// tek başına yetmez, bloğun içeriği bağımsızca doğrulanır).
func (c *Client) FetchBlockExtrinsics(ctx context.Context, blockHash string) ([]string, error) {
	if c.cfg.PolkadotRPC == "" {
		return nil, fmt.Errorf("DOT_RPC_URL yapılandırılmadı")
	}
	result, err := c.rpcCallWithRetry(ctx, c.cfg.PolkadotRPC, map[string]interface{}{
		"jsonrpc": "2.0", "method": "chain_getBlock", "params": []interface{}{blockHash}, "id": 1,
	})
	if err != nil {
		return nil, fmt.Errorf("chain_getBlock: %w", err)
	}
	res, ok := result["result"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("chain_getBlock: beklenmeyen yanıt şekli")
	}
	block, ok := res["block"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("chain_getBlock: block alanı yok")
	}
	extList, ok := block["extrinsics"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("chain_getBlock: extrinsics alanı yok")
	}
	out := make([]string, 0, len(extList))
	for _, e := range extList {
		s, _ := e.(string)
		out = append(out, s)
	}
	return out, nil
}

// jsonNumberToUint32/64 — encoding/json'ın sayıları float64 olarak decode
// etmesiyle (büyük değerlerde hassasiyet kaybı riski) ilgilenir: RPC
// yanıtları bazen sayısal (float64) bazen string olarak gelebilir, ikisini
// de destekler.
func jsonNumberToUint32(v interface{}) (uint32, error) {
	u, err := jsonNumberToUint64(v)
	if err != nil {
		return 0, err
	}
	if u > 1<<32-1 {
		return 0, fmt.Errorf("değer u32 sınırını aşıyor: %d", u)
	}
	return uint32(u), nil
}

func jsonNumberToUint64(v interface{}) (uint64, error) {
	switch x := v.(type) {
	case float64:
		if x < 0 {
			return 0, fmt.Errorf("negatif sayı: %v", x)
		}
		return uint64(x), nil
	case string:
		s := strings.TrimPrefix(x, "0x")
		if s != x {
			u, err := strconv.ParseUint(s, 16, 64)
			if err != nil {
				return 0, fmt.Errorf("hex sayı parse: %w", err)
			}
			return u, nil
		}
		u, err := strconv.ParseUint(x, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("ondalık sayı parse: %w", err)
		}
		return u, nil
	default:
		return 0, fmt.Errorf("beklenmeyen sayı tipi: %T", v)
	}
}
