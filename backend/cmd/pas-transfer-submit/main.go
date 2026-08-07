// pas-transfer-submit — ONAYLANMIŞ 0.1 PAS transferini //Alice'den hedef
// adrese GERÇEKTEN gönderir (author_submitAndWatchExtrinsic), inBlock/
// finalized durumunu izler, blok içeriğinden bağımsızca teyit eder, ve
// transfer öncesi/sonrası bakiyeleri karşılaştırır.
//
// DUR NOKTASI YOK — bu araç kullanıcının açık "gönder" onayından SONRA
// çalıştırılmak için yazıldı (bkz. pas-transfer-prep, aynı akışın
// gönderi-öncesi/DUR sürümü).
package main

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"obscura.network/core/internal/bridge"
)

const destSS58Address = "5GZVKNQhXGfWWmAYy5psxjm3E2g9J9aYTbGK86qWYG9LnxSC"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "HATA:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	cfg := bridge.ConfigFromEnv()
	if cfg.PolkadotRPC == "" {
		return fmt.Errorf("DOT_RPC_URL ortam değişkeni ayarlı değil")
	}
	c := bridge.New(cfg)

	fmt.Println("=== 1. Zincir verisi + extrinsic kurulumu (onaylanan akışla birebir) ===")

	md, err := c.FetchMetadata(ctx)
	if err != nil {
		return fmt.Errorf("metadata: %w", err)
	}
	rv, err := c.FetchRuntimeVersion(ctx)
	if err != nil {
		return fmt.Errorf("runtime version: %w", err)
	}
	genesis, err := c.FetchGenesisHash(ctx)
	if err != nil {
		return fmt.Errorf("genesis hash: %w", err)
	}
	decimals, err := c.FetchTokenDecimals(ctx)
	if err != nil {
		return fmt.Errorf("token decimals: %w", err)
	}
	if decimals < 1 {
		return fmt.Errorf("token decimals %d geçersiz", decimals)
	}

	alice, err := bridge.Sr25519DevKeypair("Alice")
	if err != nil {
		return fmt.Errorf("//Alice türetme: %w", err)
	}
	aliceSS58, err := bridge.EncodeSS58(alice.Public, 42)
	if err != nil {
		return fmt.Errorf("//Alice SS58: %w", err)
	}
	nonce, err := c.FetchAccountNonce(ctx, aliceSS58)
	if err != nil {
		return fmt.Errorf("nonce: %w", err)
	}
	destPub, destPrefix, err := bridge.DecodeSS58(destSS58Address)
	if err != nil {
		return fmt.Errorf("dest adres decode: %w", err)
	}
	if destPrefix != 42 {
		return fmt.Errorf("dest adres ss58 prefix %d, beklenen 42 DEĞİL — DUR", destPrefix)
	}
	amountPlanck := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals-1)), nil) // 0.1 birim

	params := bridge.SignedExtraParams{
		SpecVersion: rv.SpecVersion, TxVersion: rv.TransactionVersion,
		GenesisHash: genesis, EraCheckpoint: genesis, Era: bridge.EraImmortal(),
		Nonce: nonce, Tip: big.NewInt(0),
	}
	raw, err := bridge.BuildSignedExtrinsic(md, alice, destPub, amountPlanck, params)
	if err != nil {
		return fmt.Errorf("extrinsic kurma/imzalama: %w", err)
	}

	// Son GÜVENLİK kontrolü: göndermeden hemen önce yeniden decode edip
	// signer/dest/amount/nonce'un beklenenle eşleştiğini doğrula.
	summary, err := bridge.DecodeExtrinsicForReview(md, raw, 42)
	if err != nil {
		return fmt.Errorf("son decode kontrolü: %w", err)
	}
	if summary.SignerSS58 != aliceSS58 || summary.DestSS58 != destSS58Address ||
		summary.AmountPlanck.Cmp(amountPlanck) != 0 || summary.Nonce != nonce {
		return fmt.Errorf("son decode kontrolü BEKLENENLE UYUŞMUYOR — DUR, gönderilmedi")
	}
	txHash := bridge.ExtrinsicHash(raw)
	rawHex := fmt.Sprintf("0x%x", raw)
	fmt.Printf("  extrinsic hazır, nonce=%d, tx hash (blake2b-256) = 0x%x\n", nonce, txHash)

	fmt.Println("\n=== 2. Gönderim ÖNCESİ bakiyeler ===")
	aliceBefore, err := c.FetchFreeBalance(ctx, aliceSS58)
	if err != nil {
		return fmt.Errorf("//Alice bakiyesi (öncesi): %w", err)
	}
	destBefore, err := c.FetchFreeBalance(ctx, destSS58Address)
	if err != nil {
		return fmt.Errorf("dest bakiyesi (öncesi): %w", err)
	}
	fmt.Printf("  //Alice : %s\n", formatPAS(aliceBefore, decimals))
	fmt.Printf("  dest    : %s\n", formatPAS(destBefore, decimals))

	fmt.Println("\n=== 3. author_submitAndWatchExtrinsic — GÖNDERİLİYOR ===")

	var inBlockHash string
	finalStatus, watchErr := c.SubmitAndWatchExtrinsic(ctx, rawHex, func(status map[string]interface{}) {
		for k, v := range status {
			fmt.Printf("  [durum] %s = %v\n", k, v)
			if k == "inBlock" {
				if s, ok := v.(string); ok {
					inBlockHash = s
				}
			}
		}
	})
	if watchErr != nil && inBlockHash == "" {
		return fmt.Errorf("gönderim/izleme başarısız: %w", watchErr)
	}
	if watchErr != nil {
		fmt.Printf("  (not: finalized beklenirken zaman aşımı/hata: %v — ama inBlock görüldü, aşağıda devam)\n", watchErr)
	}

	fmt.Println("\n=== 4. Blok içeriğinden bağımsız teyit ===")
	if inBlockHash == "" {
		return fmt.Errorf("inBlock durumu hiç gözlenmedi — extrinsic bloğa girdiği doğrulanamadı")
	}
	blockExtrinsics, err := c.FetchBlockExtrinsics(ctx, inBlockHash)
	if err != nil {
		return fmt.Errorf("chain_getBlock(%s): %w", inBlockHash, err)
	}
	found := false
	for _, e := range blockExtrinsics {
		if strings.EqualFold(strings.TrimPrefix(e, "0x"), strings.TrimPrefix(rawHex, "0x")) {
			found = true
			break
		}
	}
	fmt.Printf("  blok hash    : %s\n", inBlockHash)
	fmt.Printf("  blokta %d extrinsic var, bizimki içinde mi: %v\n", len(blockExtrinsics), found)
	if !found {
		return fmt.Errorf("UYARI: extrinsic hex'i inBlock olarak bildirilen blokta BULUNAMADI")
	}
	if _, ok := finalStatus["finalized"]; ok {
		fmt.Printf("  finalized    : %v\n", finalStatus["finalized"])
	} else {
		fmt.Println("  finalized    : bu timeout içinde gözlenmedi (inBlock + blok içeriği teyidi yeterli kanıt)")
	}

	fmt.Println("\n=== 5. Gönderim SONRASI bakiyeler ===")
	aliceAfter, err := c.FetchFreeBalance(ctx, aliceSS58)
	if err != nil {
		return fmt.Errorf("//Alice bakiyesi (sonrası): %w", err)
	}
	destAfter, err := c.FetchFreeBalance(ctx, destSS58Address)
	if err != nil {
		return fmt.Errorf("dest bakiyesi (sonrası): %w", err)
	}
	aliceDelta := new(big.Int).Sub(aliceBefore, aliceAfter)
	destDelta := new(big.Int).Sub(destAfter, destBefore)
	fmt.Printf("  //Alice : %s  (değişim: -%s)\n", formatPAS(aliceAfter, decimals), formatPAS(aliceDelta, decimals))
	fmt.Printf("  dest    : %s  (değişim: +%s)\n", formatPAS(destAfter, decimals), formatPAS(destDelta, decimals))
	fmt.Printf("  dest tam olarak +0.1 PAS mi: %v\n", destDelta.Cmp(amountPlanck) == 0)

	fmt.Println("\n=== KANIT ÖZETİ ===")
	fmt.Printf("  tx hash   : 0x%x\n", txHash)
	fmt.Printf("  inBlock   : %s (blok içeriğinde doğrulandı: %v)\n", inBlockHash, found)
	fmt.Printf("  dest bakiyesi öncesi -> sonrası: %s -> %s\n", formatPAS(destBefore, decimals), formatPAS(destAfter, decimals))

	return nil
}

func formatPAS(planck *big.Int, decimals uint32) string {
	divisor := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil))
	f := new(big.Float).Quo(new(big.Float).SetInt(planck), divisor)
	return fmt.Sprintf("%s PAS (%s planck)", f.Text('f', int(decimals)), planck.String())
}
