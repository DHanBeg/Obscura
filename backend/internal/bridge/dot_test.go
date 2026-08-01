package bridge

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// TestScaleJunctionChainCode_Alice, scaleJunctionChainCode'un go-schnorrkel'in
// kendi derive_test.go'sundaki TestDeriveHard vektörüyle (sr25519-crust
// referans testinden alınan "Alice" -> chain code) birebir eşleştiğini
// kanıtlar. Eşleşmezse Sr25519DevKeypair'in ürettiği //Alice anahtarı
// Substrate'in beklediğinden FARKLI olur.
func TestScaleJunctionChainCode_Alice(t *testing.T) {
	want, err := hex.DecodeString("14416c6963650000000000000000000000000000000000000000000000000000")
	if err != nil {
		t.Fatalf("hex decode want: %v", err)
	}
	got, err := scaleJunctionChainCode("Alice")
	if err != nil {
		t.Fatalf("scaleJunctionChainCode: %v", err)
	}
	if !bytes.Equal(got[:], want) {
		t.Fatalf("chain code = %x, want %x", got, want)
	}
}

// TestSr25519DevKeypair_AliceMatchesSubstrate, //Alice dev hesabının
// bip39(dev mnemonic) -> ExpandEd25519 -> hard-derive("Alice") zincirinden
// Substrate'in HERKESÇE BİLİNEN kanonik Alice public key'ini ürettiğini
// kanıtlar (0xd43593c7...) ve bunun SS58 (prefix 42) karşılığının da
// kanonik 5GrwvaEF5zXb26Fz9rcQpDWS57CtERHpNehXCPcNoHGKutQY olduğunu
// doğrular. Bu iki sabit her Substrate/Polkadot.js/Talisman kurulumunda
// aynıdır — eşleşme, bip39+sr25519+SS58 zincirinin uçtan uca Substrate ile
// uyumlu olduğunun kanıtıdır.
func TestSr25519DevKeypair_AliceMatchesSubstrate(t *testing.T) {
	const wantPubHex = "d43593c715fdd31c61141abd04a99fd6822c8558854ccde39a5684e7a56da27d"
	const wantSS58 = "5GrwvaEF5zXb26Fz9rcQpDWS57CtERHpNehXCPcNoHGKutQY"

	kp, err := Sr25519DevKeypair("Alice")
	if err != nil {
		t.Fatalf("Sr25519DevKeypair(Alice): %v", err)
	}

	gotPubHex := hex.EncodeToString(kp.Public[:])
	if gotPubHex != wantPubHex {
		t.Fatalf("Alice public key = %s, want %s", gotPubHex, wantPubHex)
	}

	gotSS58, err := EncodeSS58(kp.Public, 42)
	if err != nil {
		t.Fatalf("EncodeSS58: %v", err)
	}
	if gotSS58 != wantSS58 {
		t.Fatalf("Alice SS58 = %s, want %s", gotSS58, wantSS58)
	}
}

// TestSr25519SignVerify_RoundTrip, imzalama+doğrulamanın kendi içinde
// tutarlı olduğunu kanıtlar: doğru mesaj/pubkey ile doğrulama true, yanlış
// mesaj veya yanlış pubkey ile false dönmeli.
func TestSr25519SignVerify_RoundTrip(t *testing.T) {
	kp, err := Sr25519DevKeypair("Alice")
	if err != nil {
		t.Fatalf("Sr25519DevKeypair(Alice): %v", err)
	}
	other, err := Sr25519DevKeypair("Bob")
	if err != nil {
		t.Fatalf("Sr25519DevKeypair(Bob): %v", err)
	}

	msg := []byte("obscura bridge dot foundation")
	sig, err := kp.Sign(msg)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	ok, err := Sr25519Verify(kp.Public, msg, sig)
	if err != nil {
		t.Fatalf("Verify (doğru): %v", err)
	}
	if !ok {
		t.Fatal("Verify (doğru mesaj+pubkey) = false, want true")
	}

	ok, err = Sr25519Verify(kp.Public, []byte("baska mesaj"), sig)
	if err != nil {
		t.Fatalf("Verify (yanlış mesaj): %v", err)
	}
	if ok {
		t.Fatal("Verify (yanlış mesaj) = true, want false")
	}

	ok, err = Sr25519Verify(other.Public, msg, sig)
	if err != nil {
		t.Fatalf("Verify (yanlış pubkey): %v", err)
	}
	if ok {
		t.Fatal("Verify (yanlış pubkey) = true, want false")
	}
}

// TestDecodeSS58_UserPaseoAddress, kullanıcının gerçek Paseo adresini
// decode edip checksum'ın geçerli olduğunu, prefix'in 42 olduğunu ve decode
// edilen public key'in TEKRAR encode edildiğinde AYNI adresi ürettiğini
// (round-trip) kanıtlar. blake2b tabanlı checksum gerçek bir adres üzerinde
// tutarsa (rastgele bir string üzerinde tesadüfen tutma ihtimali 2^-16),
// bu implementasyonun Substrate'in SS58 şemasıyla uyumlu olduğunun güçlü
// kanıtıdır.
func TestDecodeSS58_UserPaseoAddress(t *testing.T) {
	const userAddr = "5GZVKNQhXGfWWmAYy5psxjm3E2g9J9aYTbGK86qWYG9LnxSC"

	pub, prefix, err := DecodeSS58(userAddr)
	if err != nil {
		t.Fatalf("DecodeSS58(%s): %v", userAddr, err)
	}
	if prefix != 42 {
		t.Fatalf("prefix = %d, want 42", prefix)
	}

	roundTrip, err := EncodeSS58(pub, prefix)
	if err != nil {
		t.Fatalf("EncodeSS58 round-trip: %v", err)
	}
	if roundTrip != userAddr {
		t.Fatalf("round-trip SS58 = %s, want %s (pubkey=%x)", roundTrip, userAddr, pub)
	}
	t.Logf("kullanıcı adresi %s -> pubkey %x -> round-trip eşleşti", userAddr, pub)
}

// TestSS58_EncodeDecode_RoundTrip_RandomKey, rastgele bir public key için
// encode->decode simetrisini kontrol eder (bilinen sabitlerden bağımsız,
// genel doğruluk kontrolü).
func TestSS58_EncodeDecode_RoundTrip_RandomKey(t *testing.T) {
	kp, err := Sr25519DevKeypair("Bob")
	if err != nil {
		t.Fatalf("Sr25519DevKeypair(Bob): %v", err)
	}
	addr, err := EncodeSS58(kp.Public, 42)
	if err != nil {
		t.Fatalf("EncodeSS58: %v", err)
	}
	gotPub, gotPrefix, err := DecodeSS58(addr)
	if err != nil {
		t.Fatalf("DecodeSS58(%s): %v", addr, err)
	}
	if gotPrefix != 42 {
		t.Fatalf("prefix = %d, want 42", gotPrefix)
	}
	if gotPub != kp.Public {
		t.Fatalf("decoded pubkey = %x, want %x", gotPub, kp.Public)
	}
}

// TestDecodeSS58_BadChecksum, bozuk bir adresin checksum hatasıyla
// reddedildiğini kanıtlar (yanlış adrese token gitmemesi için kritik).
func TestDecodeSS58_BadChecksum(t *testing.T) {
	const userAddr = "5GZVKNQhXGfWWmAYy5psxjm3E2g9J9aYTbGK86qWYG9LnxSC"
	tampered := "5GZVKNQhXGfWWmAYy5psxjm3E2g9J9aYTbGK86qWYG9LnxSD" // son karakter değiştirildi

	if _, _, err := DecodeSS58(tampered); err == nil {
		t.Fatal("DecodeSS58(tampered) hata dönmedi, want checksum hatası")
	}
	// sağlam adresin hâlâ geçtiğini doğrula (kontrast için)
	if _, _, err := DecodeSS58(userAddr); err != nil {
		t.Fatalf("DecodeSS58(userAddr) beklenmedik hata: %v", err)
	}
}
