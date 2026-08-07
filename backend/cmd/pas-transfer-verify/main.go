// pas-transfer-verify — SALT OKUNUR doğrulama aracı. Hiçbir extrinsic
// GÖNDERMEZ (author_submitExtrinsic/submitAndWatch ÇAĞRILMAZ). pas-transfer-
// submit'in zaten gönderdiği (nonce=7) transferin blok içeriğinden bağımsız
// teyidini, finality durumunu ve önce/sonra bakiyeleri gösterir.
package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"obscura.network/core/internal/bridge"
)

const (
	destSS58Address = "5GZVKNQhXGfWWmAYy5psxjm3E2g9J9aYTbGK86qWYG9LnxSC"
	aliceSS58       = "5GrwvaEF5zXb26Fz9rcQpDWS57CtERHpNehXCPcNoHGKutQY"
	knownInBlock    = "0x2c5007a382e73927906b82e99625c16d1fe38fab5f62f4b2181197c2a47972c5"
	knownTxHash     = "0xf323a18c5ea198b3ccbcbd2a880768a1c456a4384cd5ee11ca3511e4f8f9a8d3"
	// pas-transfer-submit'in "gönderim öncesi" adımında bastığı, gönderimden
	// hemen önce okunmuş gerçek değerler (planck) — bağımsız before/after
	// karşılaştırması için.
	aliceBeforePlanck = "44971083099923"
	destBeforePlanck  = "49999976538538"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "HATA:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := bridge.ConfigFromEnv()
	if cfg.PolkadotRPC == "" {
		return fmt.Errorf("DOT_RPC_URL ortam değişkeni ayarlı değil")
	}
	c := bridge.New(cfg)

	decimals, err := c.FetchTokenDecimals(ctx)
	if err != nil {
		return fmt.Errorf("token decimals: %w", err)
	}

	fmt.Println("=== 1. Blok içeriği (bağımsız teyit) ===")
	exts, err := c.FetchBlockExtrinsics(ctx, knownInBlock)
	if err != nil {
		return fmt.Errorf("chain_getBlock(%s): %w", knownInBlock, err)
	}
	fmt.Printf("  blok %s içinde %d extrinsic var\n", knownInBlock, len(exts))

	md, err := c.FetchMetadata(ctx)
	if err != nil {
		return fmt.Errorf("metadata: %w", err)
	}
	found := false
	for i, e := range exts {
		raw, decErr := hexDecode(e)
		if decErr != nil {
			continue
		}
		summary, decErr := bridge.DecodeExtrinsicForReview(md, raw, 42)
		if decErr != nil {
			continue // bu extrinsic balances.transfer_keep_alive şeklinde değil, atla
		}
		if summary.SignerSS58 == aliceSS58 && summary.DestSS58 == destSS58Address {
			fmt.Printf("  [%d] EŞLEŞTİ: signer=%s dest=%s amount=%s planck nonce=%d\n",
				i, summary.SignerSS58, summary.DestSS58, summary.AmountPlanck.String(), summary.Nonce)
			computedHash := bridge.ExtrinsicHash(raw)
			fmt.Printf("       bu extrinsic'in hash'i: 0x%x (beklenen tx hash ile eşleşiyor mu: %v)\n",
				computedHash, fmt.Sprintf("0x%x", computedHash) == knownTxHash)
			found = true
		}
	}
	if !found {
		return fmt.Errorf("blokta //Alice -> dest transferi BULUNAMADI")
	}

	fmt.Println("\n=== 2. Finality durumu ===")
	inBlockNum, err := c.FetchBlockNumber(ctx, knownInBlock)
	if err != nil {
		return fmt.Errorf("inBlock numarası: %w", err)
	}
	finalizedHash, err := c.FetchFinalizedHead(ctx)
	if err != nil {
		return fmt.Errorf("finalized head: %w", err)
	}
	finalizedNum, err := c.FetchBlockNumber(ctx, finalizedHash)
	if err != nil {
		return fmt.Errorf("finalized numarası: %w", err)
	}
	fmt.Printf("  inBlock  #%d (%s)\n", inBlockNum, knownInBlock)
	fmt.Printf("  finalized head #%d (%s)\n", finalizedNum, finalizedHash)
	if finalizedNum >= inBlockNum {
		canonicalHash, err := c.FetchBlockHashAt(ctx, inBlockNum)
		if err != nil {
			return fmt.Errorf("canonical hash #%d: %w", inBlockNum, err)
		}
		isFinalized := strings.EqualFold(canonicalHash, knownInBlock)
		fmt.Printf("  finalized zincirde #%d hash'i: %s (inBlock ile eşleşiyor mu: %v)\n", inBlockNum, canonicalHash, isFinalized)
		if isFinalized {
			fmt.Println("  SONUÇ: FINALIZED — blok geri alınamaz şekilde zincire yazıldı.")
		} else {
			fmt.Println("  UYARI: inBlock hash finalized zincirdeki hash ile UYUŞMUYOR (reorg olmuş olabilir)")
		}
	} else {
		fmt.Println("  henüz finalize olmamış (finalized head inBlock'u geçmedi) — birkaç saniye sonra tekrar kontrol edilebilir")
	}

	fmt.Println("\n=== 3. Bakiye önce/sonra ===")
	aliceBefore, _ := new(big.Int).SetString(aliceBeforePlanck, 10)
	destBefore, _ := new(big.Int).SetString(destBeforePlanck, 10)
	aliceAfter, err := c.FetchFreeBalance(ctx, aliceSS58)
	if err != nil {
		return fmt.Errorf("//Alice bakiyesi: %w", err)
	}
	destAfter, err := c.FetchFreeBalance(ctx, destSS58Address)
	if err != nil {
		return fmt.Errorf("dest bakiyesi: %w", err)
	}
	aliceDelta := new(big.Int).Sub(aliceBefore, aliceAfter)
	destDelta := new(big.Int).Sub(destAfter, destBefore)
	fmt.Printf("  //Alice : %s -> %s  (değişim: -%s)\n",
		formatPAS(aliceBefore, decimals), formatPAS(aliceAfter, decimals), formatPAS(aliceDelta, decimals))
	fmt.Printf("  dest    : %s -> %s  (değişim: +%s)\n",
		formatPAS(destBefore, decimals), formatPAS(destAfter, decimals), formatPAS(destDelta, decimals))
	expectedAmount := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals-1)), nil)
	fmt.Printf("  dest tam +0.1 PAS mi: %v\n", destDelta.Cmp(expectedAmount) == 0)
	fmt.Printf("  Alice en az 0.1 PAS azaldı mı (fee dahil daha fazla olmalı): %v\n", aliceDelta.Cmp(expectedAmount) >= 0)

	return nil
}

func hexDecode(s string) ([]byte, error) {
	return hex.DecodeString(strings.TrimPrefix(s, "0x"))
}

func formatPAS(planck *big.Int, decimals uint32) string {
	divisor := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil))
	f := new(big.Float).Quo(new(big.Float).SetInt(planck), divisor)
	return fmt.Sprintf("%s PAS", f.Text('f', int(decimals)))
}
