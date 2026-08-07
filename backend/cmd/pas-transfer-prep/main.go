// pas-transfer-prep — Paseo balances.transfer_keep_alive extrinsic'ini
// kurar, //Alice ile imzalar, DECODE EDİP GÖSTERİR. author_submitExtrinsic
// ÇAĞIRMAZ (kullanıcı talimatı: gönderme, sadece hazırla ve göster).
package main

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"time"

	"obscura.network/core/internal/bridge"
)

const (
	destSS58AddressWant = "5GZVKNQhXGfWWmAYy5psxjm3E2g9J9aYTbGK86qWYG9LnxSC"
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

	fmt.Println("=== 1. Canlı zincir verisi çekiliyor (Paseo) ===")

	md, err := c.FetchMetadata(ctx)
	if err != nil {
		return fmt.Errorf("metadata: %w", err)
	}
	fmt.Printf("  metadata: OK (extrinsic version=%d)\n", md.ExtrinsicVersion())

	rv, err := c.FetchRuntimeVersion(ctx)
	if err != nil {
		return fmt.Errorf("runtime version: %w", err)
	}
	fmt.Printf("  runtime version: spec=%d tx=%d\n", rv.SpecVersion, rv.TransactionVersion)

	genesis, err := c.FetchGenesisHash(ctx)
	if err != nil {
		return fmt.Errorf("genesis hash: %w", err)
	}
	fmt.Printf("  genesis hash: 0x%x\n", genesis)

	decimals, err := c.FetchTokenDecimals(ctx)
	if err != nil {
		return fmt.Errorf("token decimals: %w", err)
	}
	fmt.Printf("  token decimals: %d (zincirden — VARSAYILMADI)\n", decimals)

	palletIdx, callIdx, err := md.FindCall("Balances", "transfer_keep_alive")
	if err != nil {
		return fmt.Errorf("call index: %w", err)
	}
	fmt.Printf("  balances.transfer_keep_alive index: pallet=%d call=%d (metadata'dan)\n", palletIdx, callIdx)

	alice, err := bridge.Sr25519DevKeypair("Alice")
	if err != nil {
		return fmt.Errorf("//Alice türetme: %w", err)
	}
	aliceSS58, err := bridge.EncodeSS58(alice.Public, 42)
	if err != nil {
		return fmt.Errorf("//Alice SS58: %w", err)
	}
	fmt.Printf("  //Alice adresi: %s\n", aliceSS58)

	nonce, err := c.FetchAccountNonce(ctx, aliceSS58)
	if err != nil {
		return fmt.Errorf("nonce: %w", err)
	}
	fmt.Printf("  //Alice nonce: %d\n", nonce)

	destPub, destPrefix, err := bridge.DecodeSS58(destSS58AddressWant)
	if err != nil {
		return fmt.Errorf("dest adres decode: %w", err)
	}
	if destPrefix != 42 {
		return fmt.Errorf("dest adres ss58 prefix %d, beklenen 42 (Paseo/generic Substrate) DEĞİL — DUR", destPrefix)
	}

	// 0.1 PAS -> planck: 0.1 = 10^-1, planck = 10^(decimals-1). Ondalık
	// aritmetiği (float) KULLANILMAZ — tam sayı üssü ile hesaplanır.
	if decimals < 1 {
		return fmt.Errorf("token decimals %d, 0.1 birim hesaplamak için yetersiz", decimals)
	}
	amountPlanck := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals-1)), nil)

	fmt.Println("\n=== 2. Extrinsic kuruluyor, //Alice ile imzalanıyor ===")

	params := bridge.SignedExtraParams{
		SpecVersion:   rv.SpecVersion,
		TxVersion:     rv.TransactionVersion,
		GenesisHash:   genesis,
		EraCheckpoint: genesis, // Immortal -> checkpoint = genesis
		Era:           bridge.EraImmortal(),
		Nonce:         nonce,
		Tip:           big.NewInt(0),
	}

	raw, err := bridge.BuildSignedExtrinsic(md, alice, destPub, amountPlanck, params)
	if err != nil {
		return fmt.Errorf("extrinsic kurma/imzalama: %w", err)
	}
	fmt.Printf("  imzalı extrinsic (SCALE hex, %d bayt): 0x%x\n", len(raw), raw)

	fmt.Println("\n=== 3. DecodeExtrinsicForReview — GÖZLE ONAY İÇİN ===")

	summary, err := bridge.DecodeExtrinsicForReview(md, raw, 42)
	if err != nil {
		return fmt.Errorf("decode-for-review: %w", err)
	}

	amountPASFloat := new(big.Float).Quo(
		new(big.Float).SetInt(summary.AmountPlanck),
		new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)),
	)

	fmt.Printf("  signer (kimden) : %s\n", summary.SignerSS58)
	fmt.Printf("    -> //Alice ile eşleşiyor mu: %v\n", summary.SignerSS58 == aliceSS58)
	fmt.Printf("  dest (kime)     : %s\n", summary.DestSS58)
	fmt.Printf("    -> istenen adresle eşleşiyor mu: %v\n", summary.DestSS58 == destSS58AddressWant)
	fmt.Printf("  amount (planck) : %s\n", summary.AmountPlanck.String())
	fmt.Printf("  amount (PAS)    : %s (decimals=%d, zincirden)\n", amountPASFloat.Text('f', int(decimals)), decimals)
	fmt.Printf("  nonce           : %d\n", summary.Nonce)
	fmt.Printf("  tip (planck)    : %s\n", summary.Tip.String())
	fmt.Printf("  era             : %x (0x00 = Immortal)\n", summary.Era)
	fmt.Printf("  pallet/call idx : %d / %d\n", summary.PalletIndex, summary.CallIndex)

	fmt.Println("\n=== 4. Bakiye/fee kontrolü ===")

	freeBalance, err := c.FetchFreeBalance(ctx, aliceSS58)
	if err != nil {
		return fmt.Errorf("bakiye sorgusu: %w", err)
	}
	fee, err := c.FetchPartialFee(ctx, "0x"+fmt.Sprintf("%x", raw))
	if err != nil {
		return fmt.Errorf("fee sorgusu: %w", err)
	}
	divisor := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil))
	balancePAS := new(big.Float).Quo(new(big.Float).SetInt(freeBalance), divisor)
	feePAS := new(big.Float).Quo(new(big.Float).SetInt(fee), divisor)
	total := new(big.Int).Add(summary.AmountPlanck, fee)
	totalPAS := new(big.Float).Quo(new(big.Float).SetInt(total), divisor)
	covers := freeBalance.Cmp(total) >= 0

	fmt.Printf("  //Alice bakiyesi     : %s planck = %s PAS\n", freeBalance.String(), balancePAS.Text('f', int(decimals)))
	fmt.Printf("  tahmini fee (zincir) : %s planck = %s PAS\n", fee.String(), feePAS.Text('f', int(decimals)))
	fmt.Printf("  amount + fee toplam  : %s planck = %s PAS\n", total.String(), totalPAS.Text('f', int(decimals)))
	fmt.Printf("  bakiye yeterli mi    : %v\n", covers)
	if !covers {
		return fmt.Errorf("//Alice bakiyesi yetersiz — DUR, gönderilmeyecek")
	}

	fmt.Println("\n=== DUR — İşte gönderilecek işlem. author_submitExtrinsic ÇAĞRILMADI. ===")
	fmt.Println("Onay verirsen ikinci adımda bu hex ile SubmitExtrinsic çağrılacak.")

	return nil
}
