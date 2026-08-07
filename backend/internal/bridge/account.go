// Package bridge — hesap bakiyesi + tahmini işlem ücreti sorgulama
// (Bridge madde 4, PARÇA 2 eki: gönderim öncesi "bakiye fee'yi karşılıyor
// mu" kontrolü için).
package bridge

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/cespare/xxhash/v2"
	"golang.org/x/crypto/blake2b"
)

// twox128, Substrate storage key'lerinde pallet/storage adı için kullanılan
// twox-128 hash'idir: iki adet seed'li (0, 1) XXH64 çıktısının little-endian
// baytlarının ardışık birleşimi (bkz. paritytech/substrate frame_support::
// storage::storage_prefix — parity-scale-codec/twox-hash ile birebir aynı).
func twox128(data []byte) []byte {
	out := make([]byte, 0, 16)
	for seed := uint64(0); seed < 2; seed++ {
		h := xxhash.NewWithSeed(seed)
		h.Write(data) //nolint:errcheck // xxhash.Digest.Write asla hata döndürmez
		sum := h.Sum64()
		out = append(out,
			byte(sum), byte(sum>>8), byte(sum>>16), byte(sum>>24),
			byte(sum>>32), byte(sum>>40), byte(sum>>48), byte(sum>>56))
	}
	return out
}

// blake2b128Concat, Blake2_128Concat storage hasher'ıdır: blake2b-128(key) ++
// key (map anahtarını hash sonuna EKLER — decode/iterate için gerekli, bu
// paket yalnızca key üretimi için kullanıyor).
func blake2b128Concat(key []byte) ([]byte, error) {
	h, err := blake2b.New(16, nil)
	if err != nil {
		return nil, err
	}
	h.Write(key)
	out := h.Sum(nil)
	return append(out, key...), nil
}

// systemAccountStorageKey, System.Account storage map'inin bir AccountId32
// için tam storage key'ini üretir: twox128("System") ++ twox128("Account")
// ++ Blake2_128Concat(accountId).
func systemAccountStorageKey(accountID [32]byte) (string, error) {
	suffix, err := blake2b128Concat(accountID[:])
	if err != nil {
		return "", fmt.Errorf("blake2b128concat: %w", err)
	}
	key := append(twox128([]byte("System")), twox128([]byte("Account"))...)
	key = append(key, suffix...)
	return "0x" + hex.EncodeToString(key), nil
}

// FetchFreeBalance, System.Account storage'ından verilen SS58 adresin
// serbest (free) bakiyesini planck cinsinden döndürür. AccountInfo SCALE
// yerleşimi: nonce(u32) ++ consumers(u32) ++ providers(u32) ++
// sufficients(u32) ++ data.free(u128) ++ ... — yalnızca free alanı okunur,
// diğer alanlar (reserved/frozen) bu kontrol için gerekmiyor.
//
// Hesabın hiç zincir tarihi yoksa (storage boş/null döner) bakiye 0 kabul
// edilir — bu bir hata DEĞİLDİR (yeni/boş hesap).
func (c *Client) FetchFreeBalance(ctx context.Context, ss58Address string) (*big.Int, error) {
	if c.cfg.PolkadotRPC == "" {
		return nil, fmt.Errorf("DOT_RPC_URL yapılandırılmadı")
	}
	pub, _, err := DecodeSS58(ss58Address)
	if err != nil {
		return nil, fmt.Errorf("adres decode: %w", err)
	}
	key, err := systemAccountStorageKey(pub)
	if err != nil {
		return nil, fmt.Errorf("storage key: %w", err)
	}
	result, err := c.rpcCallWithRetry(ctx, c.cfg.PolkadotRPC, map[string]interface{}{
		"jsonrpc": "2.0", "method": "state_getStorage", "params": []interface{}{key}, "id": 1,
	})
	if err != nil {
		return nil, fmt.Errorf("state_getStorage(System.Account): %w", err)
	}
	hexStr, _ := result["result"].(string)
	if hexStr == "" {
		return big.NewInt(0), nil // hesap zincirde hiç yok -> bakiye 0
	}
	raw, err := hex.DecodeString(strings.TrimPrefix(hexStr, "0x"))
	if err != nil {
		return nil, fmt.Errorf("storage value hex decode: %w", err)
	}
	c2 := newScaleCursor(raw)
	// header: nonce(u32) ++ consumers(u32) ++ providers(u32) ++ sufficients(u32) = 16 bayt
	if _, err := c2.readBytes(16); err != nil {
		return nil, fmt.Errorf("AccountInfo header: %w", err)
	}
	freeBytes, err := c2.readBytes(16)
	if err != nil {
		return nil, fmt.Errorf("AccountData.free: %w", err)
	}
	return leBytesToBigInt(freeBytes), nil
}

// leBytesToBigInt, little-endian bayt dizisini *big.Int'e çevirir
// (big.Int.SetBytes big-endian beklediği için baytlar önce ters çevrilir).
func leBytesToBigInt(b []byte) *big.Int {
	rev := make([]byte, len(b))
	for i, v := range b {
		rev[len(b)-1-i] = v
	}
	return new(big.Int).SetBytes(rev)
}

// FetchPartialFee, payment_queryInfo çağırıp verilen (imzalı ya da imzasız)
// extrinsic hex'i için zincirin hesapladığı tahmini işlem ücretini (planck)
// döndürür — elle ücret tahmini/uydurma YOK.
func (c *Client) FetchPartialFee(ctx context.Context, extrinsicHex string) (*big.Int, error) {
	if c.cfg.PolkadotRPC == "" {
		return nil, fmt.Errorf("DOT_RPC_URL yapılandırılmadı")
	}
	if !strings.HasPrefix(extrinsicHex, "0x") {
		extrinsicHex = "0x" + extrinsicHex
	}
	result, err := c.rpcCallWithRetry(ctx, c.cfg.PolkadotRPC, map[string]interface{}{
		"jsonrpc": "2.0", "method": "payment_queryInfo", "params": []interface{}{extrinsicHex}, "id": 1,
	})
	if err != nil {
		return nil, fmt.Errorf("payment_queryInfo: %w", err)
	}
	res, ok := result["result"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("payment_queryInfo: beklenmeyen yanıt şekli")
	}
	feeStr, ok := res["partialFee"].(string)
	if !ok {
		return nil, fmt.Errorf("payment_queryInfo yanıtında partialFee yok")
	}
	fee, ok := new(big.Int).SetString(feeStr, 10)
	if !ok {
		return nil, fmt.Errorf("partialFee parse edilemedi: %q", feeStr)
	}
	return fee, nil
}
