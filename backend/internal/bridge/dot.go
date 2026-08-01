// Package bridge — Polkadot (Paseo) tarafı temel: RPC bağlantısı, sr25519
// imzalama, SS58 adres kodlama.
//
// FAZ 4 madde 4, PARÇA 1: bağlantı + imzalama temeli (bu dosya).
// PARÇA 2 (metadata.go/scale.go/extrinsic.go/dot_submit.go): extrinsic
// oluşturma + SCALE encoding + author_submitExtrinsic.
//
// CGO_ENABLED=0 kısıtı korunuyor: go-schnorrkel (sr25519), golang.org/x/crypto
// (blake2b) ve mr-tron/base58 hepsi pure Go.
package bridge

import (
	"bytes"
	"context"
	"fmt"

	"github.com/ChainSafe/go-schnorrkel"
	"github.com/mr-tron/base58"
	"golang.org/x/crypto/blake2b"
)

// ─── RPC bağlantısı (Paseo) ───────────────────────────────────────────────

// DotHealth — Paseo düğümünden alınan temel canlılık/kimlik bilgisi.
type DotHealth struct {
	Chain       string                 // system_chain sonucu (ör. "Paseo")
	Health      map[string]interface{} // system_health sonucu (peers/isSyncing/shouldHavePeers)
	GenesisHash string                 // chain_getBlockHash(0) — zincir genesis hash'i
}

// CheckDotConnection, DOT_RPC_URL'e system_chain + system_health +
// chain_getBlockHash(0) çağırarak bağlantıyı doğrular. Mevcut Client'ın
// rpcCallWithRetry'ını (retry/backoff dahil) kullanır — ayrı bir HTTP
// istemcisi açılmaz.
func (c *Client) CheckDotConnection(ctx context.Context) (*DotHealth, error) {
	if c.cfg.PolkadotRPC == "" {
		return nil, fmt.Errorf("DOT_RPC_URL yapılandırılmadı")
	}

	chainResult, err := c.rpcCallWithRetry(ctx, c.cfg.PolkadotRPC, map[string]interface{}{
		"jsonrpc": "2.0", "method": "system_chain", "params": []interface{}{}, "id": 1,
	})
	if err != nil {
		return nil, fmt.Errorf("system_chain: %w", err)
	}
	chain, _ := chainResult["result"].(string)

	healthResult, err := c.rpcCallWithRetry(ctx, c.cfg.PolkadotRPC, map[string]interface{}{
		"jsonrpc": "2.0", "method": "system_health", "params": []interface{}{}, "id": 2,
	})
	if err != nil {
		return nil, fmt.Errorf("system_health: %w", err)
	}
	health, _ := healthResult["result"].(map[string]interface{})

	blockHashResult, err := c.rpcCallWithRetry(ctx, c.cfg.PolkadotRPC, map[string]interface{}{
		"jsonrpc": "2.0", "method": "chain_getBlockHash", "params": []interface{}{0}, "id": 3,
	})
	if err != nil {
		return nil, fmt.Errorf("chain_getBlockHash: %w", err)
	}
	genesisHash, _ := blockHashResult["result"].(string)

	return &DotHealth{Chain: chain, Health: health, GenesisHash: genesisHash}, nil
}

// ─── sr25519 imzalama ─────────────────────────────────────────────────────

// substrateSigningContext, Substrate'in sr25519 modülünün her imzalanan
// mesaja eklediği domain-separation tag'idir (bkz. sp-core sr25519::Pair —
// schnorrkel::signing_context(b"substrate")). Bu olmadan üretilen imza
// Substrate düğümünde doğrulanmaz.
var substrateSigningContext = []byte("substrate")

// substrateDevMnemonic, Substrate'in standart geliştirme hesaplarının
// ("//Alice", "//Bob", ...) türetildiği sabit mnemonic'tir.
const substrateDevMnemonic = "bottom drive obey lake curtain smoke basket hold race lonely fit walk"

// Sr25519Keypair — sr25519 gizli/açık anahtar çifti.
type Sr25519Keypair struct {
	secret *schnorrkel.SecretKey
	Public [32]byte
}

// Sr25519KeypairFromSeed, 32 baytlık ham bir seed'den (mini secret key)
// sr25519 anahtar çifti türetir (ed25519-tarzı bit-clamping genişletmesiyle,
// Substrate'in kullandığı yöntem). Gerçek anahtar entegrasyonu (Talisman'daki
// hesaptan seed çıkarma) ayrı bir adımdır — bu fonksiyon deterministik test
// seed'leri için kullanılır.
func Sr25519KeypairFromSeed(seed [32]byte) (*Sr25519Keypair, error) {
	msk, err := schnorrkel.NewMiniSecretKeyFromRaw(seed)
	if err != nil {
		return nil, fmt.Errorf("mini secret key: %w", err)
	}
	sk := msk.ExpandEd25519()
	pub := msk.Public()
	if pub == nil {
		return nil, fmt.Errorf("public key türetilemedi")
	}
	return &Sr25519Keypair{secret: sk, Public: pub.Encode()}, nil
}

// Sr25519DevKeypair, Substrate'in standart dev mnemonic'inden "//<name>"
// hard-derivation ile bilinen bir geliştirme hesabı türetir (ör. "Alice",
// "Bob") — yalnızca test/kanıt amaçlıdır.
func Sr25519DevKeypair(name string) (*Sr25519Keypair, error) {
	rootMsk, err := schnorrkel.MiniSecretKeyFromMnemonic(substrateDevMnemonic, "")
	if err != nil {
		return nil, fmt.Errorf("mnemonic -> mini secret: %w", err)
	}
	rootSk := rootMsk.ExpandEd25519()

	cc, err := scaleJunctionChainCode(name)
	if err != nil {
		return nil, fmt.Errorf("junction chain code: %w", err)
	}
	derived, err := schnorrkel.DeriveKeyHard(rootSk, []byte{}, cc)
	if err != nil {
		return nil, fmt.Errorf("hard derive //%s: %w", name, err)
	}
	sk, err := derived.Secret()
	if err != nil {
		return nil, fmt.Errorf("derived secret: %w", err)
	}
	pub, err := sk.Public()
	if err != nil {
		return nil, fmt.Errorf("derived public: %w", err)
	}
	return &Sr25519Keypair{secret: sk, Public: pub.Encode()}, nil
}

// scaleJunctionChainCode, sp-core'un DeriveJunction string kodlamasını
// üretir: SCALE compact-length-prefix + UTF-8 baytlar, 32 bayta sıfırla
// doldurulur (junction adı 32 baytı aşarsa blake2b-256 ile hash'lenir —
// pratikte "Alice"/"Bob" gibi kısa isimler bu dala hiç girmez).
//
// Doğrulama: go-schnorrkel'in kendi TestDeriveHard vektörü ("Alice" ->
// "14416c6963650000...0000") bu fonksiyonun çıktısıyla dot_test.go'da
// karşılaştırılıyor.
func scaleJunctionChainCode(s string) ([32]byte, error) {
	var cc [32]byte
	encoded := scaleCompactEncodeString(s)
	if len(encoded) <= 32 {
		copy(cc[:], encoded)
		return cc, nil
	}
	h, err := blake2b.New256(nil)
	if err != nil {
		return cc, err
	}
	h.Write(encoded)
	copy(cc[:], h.Sum(nil))
	return cc, nil
}

// scaleCompactEncodeString, SCALE compact-int kodlamalı uzunluk öneki +
// UTF-8 baytları üretir. Junction isimleri (ör. "Alice") her zaman <64 bayt
// olduğundan yalnızca tek-baytlık compact-int dalı implement edildi.
func scaleCompactEncodeString(s string) []byte {
	b := []byte(s)
	n := len(b)
	if n >= 64 {
		// Substrate junction isimleri pratikte bu sınırı aşmaz; kapsam dışı.
		h := blake2b.Sum256(b)
		return h[:]
	}
	return append([]byte{byte(n << 2)}, b...)
}

// Sign, Substrate'in sr25519 imza şemasıyla ("substrate" domain-separation
// context'i ile) bir mesaj imzalar.
func (kp *Sr25519Keypair) Sign(msg []byte) ([64]byte, error) {
	t := schnorrkel.NewSigningContext(substrateSigningContext, msg)
	sig, err := kp.secret.Sign(t)
	if err != nil {
		return [64]byte{}, fmt.Errorf("sr25519 sign: %w", err)
	}
	return sig.Encode(), nil
}

// Sr25519Verify, verilen 32-baytlık public key ve 64-baytlık imzayla
// Substrate uyumlu doğrulama yapar.
func Sr25519Verify(pub [32]byte, msg []byte, sig [64]byte) (bool, error) {
	pk, err := schnorrkel.NewPublicKey(pub)
	if err != nil {
		return false, fmt.Errorf("public key decode: %w", err)
	}
	var s schnorrkel.Signature
	if err := s.Decode(sig); err != nil {
		return false, fmt.Errorf("signature decode: %w", err)
	}
	t := schnorrkel.NewSigningContext(substrateSigningContext, msg)
	return pk.Verify(&s, t)
}

// ─── SS58 adres kodlama ───────────────────────────────────────────────────

// ss58ChecksumLen, 32-baytlık AccountId32 (sr25519/ed25519 public key)
// payload'ı için blake2b-512 checksum'ından kullanılan bayt sayısıdır
// (bkz. sp-core Ss58Codec — payload uzunluğuna göre değişir, 32 baytlık
// hesap public key'leri için sabittir).
const ss58ChecksumLen = 2

var ss58Prefix = []byte("SS58PRE")

// EncodeSS58, bir sr25519/ed25519 32-baytlık public key'i verilen ağ
// prefix'iyle (Paseo/generic Substrate: 42) SS58 adresine kodlar.
//
// Yalnızca tek-baytlık prefix aralığı (<64) desteklenir — Paseo'nun prefix'i
// (42) bu aralıkta. 2-baytlık prefix kodlaması (64-16383 arası, nadir
// kullanılan zincirler) bu görevin kapsamı dışında bırakıldı (YAGNI);
// doğrulanmamış kod göndermektense açıkça hata dönmek tercih edildi.
func EncodeSS58(pub [32]byte, networkPrefix uint16) (string, error) {
	if networkPrefix >= 64 {
		return "", fmt.Errorf("desteklenmeyen ss58 prefix: %d (yalnızca <64 desteklenir)", networkPrefix)
	}

	body := make([]byte, 0, 1+32+ss58ChecksumLen)
	body = append(body, byte(networkPrefix))
	body = append(body, pub[:]...)

	checksum, err := ss58Checksum(body)
	if err != nil {
		return "", err
	}
	body = append(body, checksum[:ss58ChecksumLen]...)

	return base58.Encode(body), nil
}

// DecodeSS58, bir SS58 adresini network prefix + 32-baytlık public key'e
// çözer; checksum doğrulaması başarısız olursa (yanlış adres/bozuk veri)
// hata döner. Aynı şekilde yalnızca tek-baytlık prefix aralığını (<64)
// destekler.
func DecodeSS58(addr string) (pub [32]byte, networkPrefix uint16, err error) {
	data, decErr := base58.Decode(addr)
	if decErr != nil {
		return pub, 0, fmt.Errorf("base58 decode: %w", decErr)
	}

	const prefixLen = 1
	const bodyLen = prefixLen + 32
	if len(data) != bodyLen+ss58ChecksumLen {
		return pub, 0, fmt.Errorf("beklenmeyen ss58 uzunluk: %d bayt (beklenen %d)", len(data), bodyLen+ss58ChecksumLen)
	}
	if data[0] >= 64 {
		return pub, 0, fmt.Errorf("desteklenmeyen ss58 prefix baytı: %d (yalnızca <64 desteklenir)", data[0])
	}
	networkPrefix = uint16(data[0])

	checksum, err := ss58Checksum(data[:bodyLen])
	if err != nil {
		return pub, 0, err
	}
	if !bytes.Equal(checksum[:ss58ChecksumLen], data[bodyLen:]) {
		return pub, 0, fmt.Errorf("ss58 checksum uyuşmazlığı — geçersiz adres")
	}

	copy(pub[:], data[prefixLen:bodyLen])
	return pub, networkPrefix, nil
}

// ss58Checksum, blake2b-512("SS58PRE" || body) döndürür (sp-core'un ss58hash
// fonksiyonuyla birebir aynı).
func ss58Checksum(body []byte) ([]byte, error) {
	h, err := blake2b.New512(nil)
	if err != nil {
		return nil, err
	}
	h.Write(ss58Prefix)
	h.Write(body)
	return h.Sum(nil), nil
}
