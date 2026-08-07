// Package bridge — ETH (OBS token) <-> DOT (PAS) atomic birim dönüşümü.
//
// Bridge madde 4, PARÇA 3: relayer'ın ETH tarafında gördüğü wei miktarını
// DOT tarafında göndereceği planck miktarına çevirmesi gerekiyor. Bu iki
// zincirin ondalık basamak sayısı FARKLI — yanlış çevrim doğrudan para
// kaybı/fazlası demek, bu yüzden hiçbir değer varsayılmaz:
//
//   - ETH tarafı: OBSBridge.sol'daki "amount OBS amount in atomic units
//     (18 decimals)" dokümantasyonuna göre SABİT 18 — bu proje kendi ERC20
//     token'ı (OBS), kontrat kaynağında açıkça belgelenmiş, zincirden ayrıca
//     sorgulanacak bir "decimals()" çağrısı yok (bridge kontratı bunu
//     varsaymıyor, koduna gömülü).
//   - DOT tarafı: Paseo'nun tokenDecimals'ı HER ZAMAN FetchTokenDecimals ile
//     zincirden çekilir (bkz. dot_submit.go) — bu dosyadaki fonksiyonlar
//     decimals'ı parametre olarak alır, hardcode ETMEZ.
package bridge

import (
	"fmt"
	"math/big"
)

// EthTokenDecimals, OBS token'ının atomic birim ondalık basamak sayısıdır
// (bkz. contracts/bridge/OBSBridge.sol, Lock() dokümantasyonu: "amount OBS
// amount in atomic units (18 decimals)"). ERC20 standardının olağan değeri
// ile aynı ama bu SABİT o dokümantasyondan geliyor, genel bir varsayım
// DEĞİL.
const EthTokenDecimals = 18

// EthWeiToDotPlanck, bir ETH tarafı miktarını (OBS, wei/atomic birim,
// EthTokenDecimals ondalık) DOT tarafı planck miktarına çevirir. dotDecimals
// ÇAĞIRAN TARAFINDAN zincirden (FetchTokenDecimals) çekilmiş olmalı —
// hardcode edilmiş bir değer bu fonksiyona geçirilirse hatalı çevrim riski
// çağıranın sorumluluğundadır.
//
// dotDecimals < EthTokenDecimals (ör. Paseo=10, ETH=18) olağan durumdur:
// bölme yapılır, kalan (dust) DOT tarafında temsil edilemeyecek kadar küçük
// birimdir ve kaybolur — dust ayrı döndürülür ki çağıran bunu loglayabilsin
// (sessizce yutulmaz).
//
// dotDecimals >= EthTokenDecimals durumunda çarpma yapılır, dust her zaman
// sıfırdır.
//
// Sonuç planck miktarı sıfırsa (tüm miktar dust'a düştü) HATA döner — sıfır
// miktarlı bir transfer extrinsic'i anlamsız/güvensizdir, sessizce
// gönderilmemelidir.
func EthWeiToDotPlanck(weiAmount *big.Int, dotDecimals uint32) (planck, dust *big.Int, err error) {
	if weiAmount == nil {
		return nil, nil, fmt.Errorf("weiAmount nil")
	}
	if weiAmount.Sign() < 0 {
		return nil, nil, fmt.Errorf("weiAmount negatif: %s", weiAmount.String())
	}

	if dotDecimals >= EthTokenDecimals {
		scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(dotDecimals-EthTokenDecimals)), nil)
		planck = new(big.Int).Mul(weiAmount, scale)
		return planck, big.NewInt(0), nil
	}

	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(EthTokenDecimals-dotDecimals)), nil)
	planck, dust = new(big.Int).QuoRem(weiAmount, divisor, new(big.Int))
	if planck.Sign() == 0 {
		return nil, nil, fmt.Errorf(
			"miktar (%s wei, eth decimals=%d) dot decimals=%d ile temsil edilemeyecek kadar küçük (tamamı dust)",
			weiAmount.String(), EthTokenDecimals, dotDecimals)
	}
	return planck, dust, nil
}
