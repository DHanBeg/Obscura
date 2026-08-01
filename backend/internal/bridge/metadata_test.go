package bridge

import (
	"os"
	"strings"
	"testing"
)

// loadPaseoMetadataFixture, gerçek Paseo RPC'sinden çekilmiş (state_getMetadata,
// 2026-08-01) V14 metadata hex'ini test fixture'ından okur. Canlı ağa bağımlı
// olmayan, hermetic bir test için kullanılıyor.
func loadPaseoMetadataFixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/paseo_metadata_v14.hex")
	if err != nil {
		t.Fatalf("fixture okunamadı: %v", err)
	}
	return strings.TrimSpace(string(b))
}

// TestDecodeMetadataV14_RealPaseoMetadata, gerçek Paseo metadata'sının HİÇBİR
// bayt ARTMADAN (DecodeMetadataV14 içindeki "artık kalan bayt" kontrolü) tam
// olarak çözüldüğünü kanıtlar — bu, struct alan sıralamasının/tiplerinin
// scale-info/frame-metadata kaynağıyla birebir uyuştuğunun güçlü kanıtıdır
// (yanlış bir alan boyutu olsaydı cursor senkronizasyonu bozulur, ya decode
// hata verir ya da sonda artık bayt kalırdı).
func TestDecodeMetadataV14_RealPaseoMetadata(t *testing.T) {
	hexStr := loadPaseoMetadataFixture(t)
	md, err := DecodeMetadataV14(hexStr)
	if err != nil {
		t.Fatalf("DecodeMetadataV14: %v", err)
	}
	if md.ExtrinsicVersion() != 4 {
		t.Errorf("extrinsic version = %d, want 4", md.ExtrinsicVersion())
	}
}

// TestFindCall_BalancesTransferKeepAlive_MatchesLiveChain, Balances pallet
// index (5) ve transfer_keep_alive call index'inin (3) HARDCODE değil,
// metadata'dan çözüldüğünü kanıtlar. Bu iki değer önceden Python ile bağımsız
// olarak decode edilip (decode_meta.py) ve substrate-interface (py-polkascan,
// bağımsız üçüncü taraf kütüphane) ile compose_call() üzerinden CROSS-CHECK
// edilip doğrulandı: substrate-interface'in ürettiği call bytes'ın ilk 2
// baytı da 05 03 idi.
func TestFindCall_BalancesTransferKeepAlive_MatchesLiveChain(t *testing.T) {
	hexStr := loadPaseoMetadataFixture(t)
	md, err := DecodeMetadataV14(hexStr)
	if err != nil {
		t.Fatalf("DecodeMetadataV14: %v", err)
	}

	palletIdx, callIdx, err := md.FindCall("Balances", "transfer_keep_alive")
	if err != nil {
		t.Fatalf("FindCall: %v", err)
	}
	if palletIdx != 5 {
		t.Errorf("Balances pallet index = %d, want 5", palletIdx)
	}
	if callIdx != 3 {
		t.Errorf("transfer_keep_alive call index = %d, want 3", callIdx)
	}
}

// TestFindCall_UnknownPallet_ReturnsError, var olmayan bir pallet/call için
// HATA döndüğünü (asla varsayılan bir index'e sessizce düşmediğini) kanıtlar
// — kullanıcının "HARDCODE ETME, dur" talimatının kod seviyesinde garantisi.
func TestFindCall_UnknownPallet_ReturnsError(t *testing.T) {
	hexStr := loadPaseoMetadataFixture(t)
	md, err := DecodeMetadataV14(hexStr)
	if err != nil {
		t.Fatalf("DecodeMetadataV14: %v", err)
	}

	if _, _, err := md.FindCall("BuNokTaOlmayanPallet", "yok"); err == nil {
		t.Fatal("FindCall(olmayan pallet) hata dönmedi")
	}
	if _, _, err := md.FindCall("Balances", "boyle_bir_call_yok"); err == nil {
		t.Fatal("FindCall(olmayan call) hata dönmedi")
	}
}

// TestSignedExtensionIdentifiers_MatchesLiveChain, Paseo'nun signed extension
// listesini SIRASIYLA doğrular (2026-08-01 itibarıyla canlı zincirden
// decode_meta.py ile bağımsız doğrulandı). Sıra, extra/additional_signed
// kodlamasında bit-bit önemli — yanlış sıra yanlış imza payload'ı üretir.
func TestSignedExtensionIdentifiers_MatchesLiveChain(t *testing.T) {
	hexStr := loadPaseoMetadataFixture(t)
	md, err := DecodeMetadataV14(hexStr)
	if err != nil {
		t.Fatalf("DecodeMetadataV14: %v", err)
	}

	want := []string{
		"AuthorizeCall",
		"CheckNonZeroSender",
		"CheckSpecVersion",
		"CheckTxVersion",
		"CheckGenesis",
		"CheckMortality",
		"CheckNonce",
		"CheckWeight",
		"ChargeTransactionPayment",
		"PrevalidateAttests",
		"CheckMetadataHash",
	}
	got := md.SignedExtensionIdentifiers()
	if len(got) != len(want) {
		t.Fatalf("signed extension sayısı = %d, want %d (got=%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("signed extension[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
