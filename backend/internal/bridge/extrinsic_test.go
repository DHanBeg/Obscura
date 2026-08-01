package bridge

import (
	"encoding/hex"
	"math/big"
	"strings"
	"testing"
)

// referenceGenesisHash / referenceSpecVersion / referenceTxVersion, Paseo'dan
// 2026-08-01'de gerçek RPC ile çekildi (state_getRuntimeVersion,
// chain_getBlockHash(0)) — bkz. konuşma geçmişi / dot_test.go canlı doğrulama.
const (
	referenceGenesisHashHex = "374057be67b355151f271ff70c3db98308c62c8adc48dc6724b6a009a1a014fd"
	referenceSpecVersion    = 2003001
	referenceTxVersion      = 26
)

// TestEraMortal_MatchesSubstrateInterface, EraMortal(64, 41)'in
// substrate-interface (bağımsız üçüncü taraf Python kütüphanesi) ile
// üretilen gerçek bir mortal era ("0x9502") ile birebir eşleştiğini kanıtlar.
func TestEraMortal_MatchesSubstrateInterface(t *testing.T) {
	got := EraMortal(64, 41)
	want := "9502"
	if hex.EncodeToString(got) != want {
		t.Errorf("EraMortal(64,41) = %x, want %s", got, want)
	}
}

// TestBuildSignedExtrinsic_MatchesSubstrateInterfaceStructure, bu paketin
// ürettiği imzalı extrinsic'in — imza baytları HARİÇ (sr25519 rastgele nonce
// kullandığı için deterministik değil) — substrate-interface'in //Alice ile
// ürettiği GERÇEK referans extrinsic'iyle (Immortal era, nonce=0, tip=0)
// birebir eşleştiğini kanıtlar: uzunluk öneki, version byte, adres, extra
// (era+nonce+tip+metadata-hash-mode), call bytes (pallet/call index + dest +
// amount) hepsi aynı.
//
// Referans (substrate-interface, 2026-08-01, Paseo canlı ağı):
//
//	0x41028400d43593c715fdd31c61141abd04a99fd6822c8558854ccde39a5684e7a56da27d
//	01f076f9794a094f3a3b34ccebcfdf58f1cd1e82ef80dff4a13ec8931ddd1b91137878e343
//	aca9ab65ffde15fbf46b52610dd2706497ecf8d3e1e428f0d18ef28600000000050300c6e4
//	d69f11267f6e8045b0b580fc77601fd94a864bedd5d115b50ae39b847f610700e8764817
func TestBuildSignedExtrinsic_MatchesSubstrateInterfaceStructure(t *testing.T) {
	md, err := DecodeMetadataV14(loadPaseoMetadataFixture(t))
	if err != nil {
		t.Fatalf("DecodeMetadataV14: %v", err)
	}

	alice, err := Sr25519DevKeypair("Alice")
	if err != nil {
		t.Fatalf("Sr25519DevKeypair(Alice): %v", err)
	}

	dest, _, err := DecodeSS58("5GZVKNQhXGfWWmAYy5psxjm3E2g9J9aYTbGK86qWYG9LnxSC")
	if err != nil {
		t.Fatalf("DecodeSS58: %v", err)
	}

	var genesis [32]byte
	gb, err := hex.DecodeString(referenceGenesisHashHex)
	if err != nil {
		t.Fatalf("genesis hex: %v", err)
	}
	copy(genesis[:], gb)

	params := SignedExtraParams{
		SpecVersion:   referenceSpecVersion,
		TxVersion:     referenceTxVersion,
		GenesisHash:   genesis,
		EraCheckpoint: genesis, // Immortal era: checkpoint == genesis
		Era:           EraImmortal(),
		Nonce:         0,
		Tip:           big.NewInt(0),
	}

	amount := big.NewInt(100_000_000_000)
	raw, err := BuildSignedExtrinsic(md, alice, dest, amount, params)
	if err != nil {
		t.Fatalf("BuildSignedExtrinsic: %v", err)
	}

	refHex := "41028400d43593c715fdd31c61141abd04a99fd6822c8558854ccde39a5684e7a56da27d" +
		"01f076f9794a094f3a3b34ccebcfdf58f1cd1e82ef80dff4a13ec8931ddd1b91137878e343" +
		"aca9ab65ffde15fbf46b52610dd2706497ecf8d3e1e428f0d18ef28600000000050300c6e4" +
		"d69f11267f6e8045b0b580fc77601fd94a864bedd5d115b50ae39b847f610700e8764817"
	ref, err := hex.DecodeString(refHex)
	if err != nil {
		t.Fatalf("ref hex: %v", err)
	}

	if len(raw) != len(ref) {
		t.Fatalf("uzunluk = %d, want %d", len(raw), len(ref))
	}

	// İmza (65..129 arası: 1 uzunluk-öneki-sonrası-offset hesapları) HARİÇ
	// her şey birebir eşleşmeli. Signature offset: len-prefix(2) + version(1)
	// + addr_tag(1) + pubkey(32) + sig_tag(1) = 37; imza 64 bayt.
	const sigStart = 2 + 1 + 1 + 32 + 1
	const sigEnd = sigStart + 64

	if got, want := raw[:sigStart], ref[:sigStart]; hex.EncodeToString(got) != hex.EncodeToString(want) {
		t.Errorf("imza ÖNCESİ kısım uyuşmuyor:\n got=%x\nwant=%x", got, want)
	}
	if got, want := raw[sigEnd:], ref[sigEnd:]; hex.EncodeToString(got) != hex.EncodeToString(want) {
		t.Errorf("imza SONRASI kısım (extra+call) uyuşmuyor:\n got=%x\nwant=%x", got, want)
	}

	// İmza kendi içinde tutarlı mı: gerçek imzalama payload'ı (call++extra++
	// additional) ile Sr25519Verify true dönüyor mu?
	var sig [64]byte
	copy(sig[:], raw[sigStart:sigEnd])
	call := raw[len(raw)-41:] // BuildBalancesTransferKeepAliveCall çıktısı hep 41 bayt (1+1+1+32+7)
	extra := raw[sigEnd : len(raw)-41]

	_, additional, err := buildSignedExtraAndAdditional(md, params)
	if err != nil {
		t.Fatalf("buildSignedExtraAndAdditional: %v", err)
	}
	payload := signingPayload(call, extra, additional)
	ok, err := Sr25519Verify(alice.Public, payload, sig)
	if err != nil {
		t.Fatalf("Sr25519Verify: %v", err)
	}
	if !ok {
		t.Fatal("Sr25519Verify(gerçek payload) = false, want true")
	}
}

// TestDecodeExtrinsicForReview_RoundTrip, BuildSignedExtrinsic'in ürettiği
// extrinsic'in DecodeExtrinsicForReview ile geri çözülüp dest/amount/nonce
// alanlarının BİREBİR eşleştiğini kanıtlar — kullanıcının "göndermeden önce
// insanca göster" şartının kod seviyesinde kanıtı.
func TestDecodeExtrinsicForReview_RoundTrip(t *testing.T) {
	md, err := DecodeMetadataV14(loadPaseoMetadataFixture(t))
	if err != nil {
		t.Fatalf("DecodeMetadataV14: %v", err)
	}

	signer, err := Sr25519DevKeypair("Bob")
	if err != nil {
		t.Fatalf("Sr25519DevKeypair(Bob): %v", err)
	}
	dest, err := deriveTestDestPubkey(t)
	if err != nil {
		t.Fatalf("dest: %v", err)
	}

	var genesis [32]byte
	gb, _ := hex.DecodeString(referenceGenesisHashHex)
	copy(genesis[:], gb)

	params := SignedExtraParams{
		SpecVersion:   referenceSpecVersion,
		TxVersion:     referenceTxVersion,
		GenesisHash:   genesis,
		EraCheckpoint: genesis,
		Era:           EraMortal(64, 12345),
		Nonce:         7,
		Tip:           big.NewInt(500),
	}
	amount := big.NewInt(1_234_567_890_123)

	raw, err := BuildSignedExtrinsic(md, signer, dest, amount, params)
	if err != nil {
		t.Fatalf("BuildSignedExtrinsic: %v", err)
	}

	summary, err := DecodeExtrinsicForReview(md, raw, 42)
	if err != nil {
		t.Fatalf("DecodeExtrinsicForReview: %v", err)
	}

	if summary.SignerPublicKey != signer.Public {
		t.Errorf("SignerPublicKey = %x, want %x", summary.SignerPublicKey, signer.Public)
	}
	if summary.DestPublicKey != dest {
		t.Errorf("DestPublicKey = %x, want %x", summary.DestPublicKey, dest)
	}
	if summary.AmountPlanck.Cmp(amount) != 0 {
		t.Errorf("Amount = %s, want %s", summary.AmountPlanck, amount)
	}
	if summary.Nonce != 7 {
		t.Errorf("Nonce = %d, want 7", summary.Nonce)
	}
	if summary.Tip.Cmp(big.NewInt(500)) != 0 {
		t.Errorf("Tip = %s, want 500", summary.Tip)
	}
	wantSS58, err := EncodeSS58(dest, 42)
	if err != nil {
		t.Fatalf("EncodeSS58: %v", err)
	}
	if summary.DestSS58 != wantSS58 {
		t.Errorf("DestSS58 = %s, want %s", summary.DestSS58, wantSS58)
	}
	t.Logf("dry-run özeti: signer=%s dest=%s amount=%s nonce=%d tip=%s era=%x",
		summary.SignerSS58, summary.DestSS58, summary.AmountPlanck, summary.Nonce, summary.Tip, summary.Era)
}

func deriveTestDestPubkey(t *testing.T) ([32]byte, error) {
	t.Helper()
	pub, _, err := DecodeSS58("5GZVKNQhXGfWWmAYy5psxjm3E2g9J9aYTbGK86qWYG9LnxSC")
	return pub, err
}

// TestBuildSignedExtrinsic_UnknownSignedExtension_Errors, metadata'da
// TANINMAYAN bir signed extension varsa BuildSignedExtrinsic'in hata
// döndürüp sessizce varsayılan kodlamaya düşmediğini kanıtlar.
func TestBuildSignedExtrinsic_UnknownSignedExtension_Errors(t *testing.T) {
	md := &Metadata{
		extrinsicVersion: 4,
		pallets: []palletMeta{
			{name: "Balances", index: 5, hasCalls: true, callsTy: 1},
		},
		types: map[uint32]portableType{
			1: {typeDef: typeDef{kind: typeDefVariant, variants: []variant{
				{name: "transfer_keep_alive", index: 3},
			}}},
		},
		signedExtensions: []signedExtensionMeta{
			{identifier: "BuNokTaninmiyorBirUzanti"},
		},
	}

	alice, err := Sr25519DevKeypair("Alice")
	if err != nil {
		t.Fatalf("Sr25519DevKeypair: %v", err)
	}
	var dest [32]byte
	_, err = BuildSignedExtrinsic(md, alice, dest, big.NewInt(1), SignedExtraParams{})
	if err == nil {
		t.Fatal("BuildSignedExtrinsic tanınmayan extension için hata dönmedi")
	}
	if !strings.Contains(err.Error(), "BuNokTaninmiyorBirUzanti") {
		t.Errorf("hata mesajı extension adını içermiyor: %v", err)
	}
}
