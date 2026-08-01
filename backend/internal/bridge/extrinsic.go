// Package bridge — imzalı extrinsic oluşturma (Bridge madde 4, PARÇA 2).
//
// balances.transfer_keep_alive extrinsic'ini uçtan uca kurar: call bytes
// (pallet/call index metadata'dan, HARDCODE YOK) + SignedExtra +
// AdditionalSigned (Paseo'nun gerçek signed_extensions listesine göre,
// sırayla) + sr25519 imza + son SCALE zarfı (compact-length prefix).
//
// Yapı, gerçek Paseo metadata'sına (state_getMetadata, V14) ve
// substrate-interface (py-polkascan, bağımsız üçüncü taraf Python
// kütüphanesi) ile üretilen referans extrinsic'e karşı byte-seviyesinde
// cross-check edildi (bkz. extrinsic_test.go).
package bridge

import (
	"fmt"
	"math/big"

	"golang.org/x/crypto/blake2b"
)

// multiAddressIDTag, MultiAddress::Id(AccountId32) varyantının SCALE
// etiketidir (sp_runtime::MultiAddress — Id=0, Index=1, Raw=2, Address32=3,
// Address20=4; bkz. Paseo metadata type 133, dot.go'nun PARÇA 1 notlarıyla
// tutarlı).
const multiAddressIDTag = 0x00

// multiSignatureSr25519Tag, MultiSignature::Sr25519 varyantının SCALE
// etiketidir (sp_runtime::MultiSignature — Ed25519=0, Sr25519=1, Ecdsa=2,
// Eth=3; kaynak: paritytech/polkadot-sdk substrate/primitives/runtime/src/lib.rs).
const multiSignatureSr25519Tag = 0x01

// signedExtrinsicBit, UncheckedExtrinsic'in version baytındaki "imzalı"
// bayrağıdır (0b1000_0000). version_byte = extrinsic_format_version | bu bit.
const signedExtrinsicBit = 0b1000_0000

// EraImmortal, extrinsic'in asla süresi dolmayan (mortality kontrolü
// olmayan) Era kodlamasını döndürür: tek bayt 0x00.
func EraImmortal() []byte { return []byte{0x00} }

// EraMortal, verilen period (kaç blok geçerli) ve current (o anki blok
// numarası) için 2 baytlık mortal Era kodlamasını üretir.
//
// Algoritma sp-runtime generic::Era::mortal ile birebir aynı (substrate-interface
// ile üretilen referans extrinsic'e karşı doğrulandı — bkz. extrinsic_test.go):
//  1. period, en yakın 2'nin kuvvetine yuvarlanır, [4, 65536] aralığına sıkıştırılır.
//  2. phase = current % period.
//  3. quantize_factor = max(1, period >> 12).
//  4. encoded (u16) = (period.trailing_zeros() - 1) | ((phase / quantize_factor) << 4).
//  5. 2 bayt little-endian.
func EraMortal(period, current uint64) []byte {
	p := period
	if p < 4 {
		p = 4
	}
	if p > 1<<16 {
		p = 1 << 16
	}
	// en yakın (>=) 2'nin kuvvetine yuvarla
	pow := uint64(4)
	for pow < p {
		pow <<= 1
	}
	p = pow

	phase := current % p
	quantizeFactor := p >> 12
	if quantizeFactor < 1 {
		quantizeFactor = 1
	}

	trailingZeros := trailingZeros64(p)
	encoded := uint16(trailingZeros-1) | uint16(phase/quantizeFactor)<<4

	return []byte{byte(encoded), byte(encoded >> 8)}
}

func trailingZeros64(v uint64) uint {
	if v == 0 {
		return 64
	}
	var n uint
	for v&1 == 0 {
		v >>= 1
		n++
	}
	return n
}

// BuildBalancesTransferKeepAliveCall, balances.transfer_keep_alive için call
// bytes üretir: palletIndex + callIndex (metadata.FindCall'dan, HARDCODE
// YOK) + MultiAddress::Id(dest) + Compact<u128>(amount).
func BuildBalancesTransferKeepAliveCall(md *Metadata, dest [32]byte, amount *big.Int) ([]byte, error) {
	palletIdx, callIdx, err := md.FindCall("Balances", "transfer_keep_alive")
	if err != nil {
		return nil, fmt.Errorf("balances.transfer_keep_alive call index çözülemedi: %w", err)
	}

	out := make([]byte, 0, 2+1+32+17)
	out = append(out, palletIdx, callIdx)
	out = append(out, multiAddressIDTag)
	out = append(out, dest[:]...)
	out = append(out, EncodeCompactBig(amount)...)
	return out, nil
}

// SignedExtraParams, imzalı extrinsic kurmak için gereken tüm zincir-özel
// veriyi taşır. Hepsi RPC'den çekilmeli (elle uydurma YOK) — bkz. dot.go
// (CheckDotConnection) ve bu paketin çağıranı.
type SignedExtraParams struct {
	SpecVersion   uint32   // state_getRuntimeVersion.specVersion
	TxVersion     uint32   // state_getRuntimeVersion.transactionVersion
	GenesisHash   [32]byte // chain_getBlockHash(0)
	EraCheckpoint [32]byte // Immortal ise genesis hash, Mortal ise era'nın başlangıç bloğunun hash'i
	Era           []byte   // EraImmortal() ya da EraMortal(...)
	Nonce         uint64   // system_accountNextIndex(SS58)
	Tip           *big.Int // ek ücret (0 = varsayılan fee yeterli)
}

// buildSignedExtraAndAdditional, metadata'nın signed_extensions listesini
// SIRAYLA gezip her extension için (extra, additional_signed) bayt çiftini
// üretir. Listede TANINMAYAN bir extension varsa HATA döner — asla varsayılan
// bir kodlamaya sessizce düşmez (yanlış encoding = yanlış imza payload'ı).
//
// Paseo'da doğrulanmış 11 extension destekleniyor (bkz. metadata_test.go).
func buildSignedExtraAndAdditional(md *Metadata, p SignedExtraParams) (extra, additional []byte, err error) {
	tip := p.Tip
	if tip == nil {
		tip = big.NewInt(0)
	}

	for _, id := range md.SignedExtensionIdentifiers() {
		switch id {
		case "AuthorizeCall", "CheckNonZeroSender", "CheckWeight", "PrevalidateAttests":
			// extra: unit struct (0 bayt), additional: unit/tuple (0 bayt)
		case "CheckSpecVersion":
			additional = append(additional, u32le(p.SpecVersion)...)
		case "CheckTxVersion":
			additional = append(additional, u32le(p.TxVersion)...)
		case "CheckGenesis":
			additional = append(additional, p.GenesisHash[:]...)
		case "CheckMortality":
			if len(p.Era) == 0 {
				return nil, nil, fmt.Errorf("CheckMortality: Era belirtilmedi")
			}
			extra = append(extra, p.Era...)
			additional = append(additional, p.EraCheckpoint[:]...)
		case "CheckNonce":
			extra = append(extra, EncodeCompactUint64(p.Nonce)...)
		case "ChargeTransactionPayment":
			extra = append(extra, EncodeCompactBig(tip)...)
		case "CheckMetadataHash":
			// Mode::Disabled (0x00) — RFC-78 merkleized metadata hash
			// doğrulaması implement edilmedi (ayrı, büyük bir merkleizasyon
			// algoritması gerektirir). Disabled, polkadot.js/subxt'in bu
			// özelliği desteklemeyen istemcilerde kullandığı TAM GEÇERLİ
			// bir moddur — extrinsic'i geçersiz kılmaz.
			extra = append(extra, 0x00)
			additional = append(additional, 0x00) // Option<[u8;32]>::None
		default:
			return nil, nil, fmt.Errorf(
				"bilinmeyen signed extension %q — metadata değişmiş olabilir, HARDCODE ile varsayılan kodlama uygulanmadı, DUR", id)
		}
	}
	return extra, additional, nil
}

func u32le(v uint32) []byte {
	return []byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)}
}

// signingPayload, sp_runtime'ın imza payload kuralını uygular: call++extra++
// additional birleştirilir; 256 baytı aşarsa blake2b-256 hash'i imzalanır,
// aşmazsa ham bayt dizisi imzalanır.
func signingPayload(call, extra, additional []byte) []byte {
	payload := make([]byte, 0, len(call)+len(extra)+len(additional))
	payload = append(payload, call...)
	payload = append(payload, extra...)
	payload = append(payload, additional...)
	if len(payload) > 256 {
		h := blake2b.Sum256(payload)
		return h[:]
	}
	return payload
}

// BuildSignedExtrinsic, imzalı bir balances.transfer_keep_alive extrinsic'i
// SCALE-encode eder: compact_len ++ (version|0x80) ++ MultiAddress::Id(signer)
// ++ MultiSignature::Sr25519(sig) ++ extra ++ call.
//
// GÖNDERMEZ — yalnızca bayt dizisini üretir. author_submitExtrinsic'e
// göndermeden önce DecodeExtrinsicForReview ile insanca gösterip kullanıcı
// onayı almak ZORUNLU (bkz. paket dokümantasyonu, kullanıcı talimatı).
func BuildSignedExtrinsic(md *Metadata, signer *Sr25519Keypair, dest [32]byte, amount *big.Int, p SignedExtraParams) ([]byte, error) {
	call, err := BuildBalancesTransferKeepAliveCall(md, dest, amount)
	if err != nil {
		return nil, err
	}
	extra, additional, err := buildSignedExtraAndAdditional(md, p)
	if err != nil {
		return nil, err
	}

	payload := signingPayload(call, extra, additional)
	sig, err := signer.Sign(payload)
	if err != nil {
		return nil, fmt.Errorf("imzalama: %w", err)
	}

	body := make([]byte, 0, 1+1+32+1+64+len(extra)+len(call))
	body = append(body, md.ExtrinsicVersion()|signedExtrinsicBit)
	body = append(body, multiAddressIDTag)
	body = append(body, signer.Public[:]...)
	body = append(body, multiSignatureSr25519Tag)
	body = append(body, sig[:]...)
	body = append(body, extra...)
	body = append(body, call...)

	out := make([]byte, 0, len(body)+5)
	out = append(out, EncodeCompactUint64(uint64(len(body)))...)
	out = append(out, body...)
	return out, nil
}

// ExtrinsicHash, bir extrinsic'in (length-prefix DAHİL, tam SCALE zarfı)
// zincirin author_submitExtrinsic'ten döneceği tx hash'ini döndürür:
// blake2b-256(raw) — sp_runtime::traits::Hash<BlakeTwo256> ile birebir aynı.
func ExtrinsicHash(raw []byte) [32]byte {
	return blake2b.Sum256(raw)
}

// ExtrinsicSummary, gönderilmeden önce insanca gözden geçirme için decode
// edilmiş extrinsic alanlarını taşır.
type ExtrinsicSummary struct {
	SignerPublicKey [32]byte
	SignerSS58      string
	DestPublicKey   [32]byte
	DestSS58        string
	AmountPlanck    *big.Int
	Nonce           uint64
	Tip             *big.Int
	Era             []byte
	PalletIndex     uint8
	CallIndex       uint8
}

// DecodeExtrinsicForReview, BuildSignedExtrinsic'in ürettiği bayt dizisini
// geri çözüp insanca gösterim için alanları çıkarır (dest/amount/nonce
// doğru mu diye GÖZLE onay için ZORUNLU adım — kullanıcı talimatı).
//
// Yalnızca bu paketin ürettiği (Balances.transfer_keep_alive, sr25519,
// MultiAddress::Id) şeklini destekler — genel amaçlı bir extrinsic decoder
// DEĞİLDİR.
func DecodeExtrinsicForReview(md *Metadata, raw []byte, ss58Prefix uint16) (*ExtrinsicSummary, error) {
	c := newScaleCursor(raw)

	bodyLen, err := c.readCompactUint32()
	if err != nil {
		return nil, fmt.Errorf("uzunluk öneki: %w", err)
	}
	if uint32(c.remaining()) != bodyLen {
		return nil, fmt.Errorf("uzunluk öneki (%d) gövde uzunluğuyla (%d) uyuşmuyor", bodyLen, c.remaining())
	}

	versionByte, err := c.readByte()
	if err != nil {
		return nil, fmt.Errorf("version byte: %w", err)
	}
	if versionByte&signedExtrinsicBit == 0 {
		return nil, fmt.Errorf("extrinsic imzasız (version byte=0x%x) — beklenen imzalı", versionByte)
	}
	if versionByte&^signedExtrinsicBit != md.ExtrinsicVersion() {
		return nil, fmt.Errorf("extrinsic version %d, metadata'nın beklediği %d ile uyuşmuyor", versionByte&^signedExtrinsicBit, md.ExtrinsicVersion())
	}

	addrTag, err := c.readByte()
	if err != nil {
		return nil, fmt.Errorf("signer address tag: %w", err)
	}
	if addrTag != multiAddressIDTag {
		return nil, fmt.Errorf("desteklenmeyen signer MultiAddress varyantı: %d (yalnızca Id destekleniyor)", addrTag)
	}
	signerPub, err := c.readBytes(32)
	if err != nil {
		return nil, fmt.Errorf("signer pubkey: %w", err)
	}

	sigTag, err := c.readByte()
	if err != nil {
		return nil, fmt.Errorf("signature tag: %w", err)
	}
	if sigTag != multiSignatureSr25519Tag {
		return nil, fmt.Errorf("desteklenmeyen imza şeması: %d (yalnızca sr25519 destekleniyor)", sigTag)
	}
	if _, err := c.readBytes(64); err != nil { // imza baytları (gösterimde kullanılmıyor)
		return nil, fmt.Errorf("signature bytes: %w", err)
	}

	var summary ExtrinsicSummary
	copy(summary.SignerPublicKey[:], signerPub)

	for _, id := range md.SignedExtensionIdentifiers() {
		switch id {
		case "AuthorizeCall", "CheckNonZeroSender", "CheckWeight", "PrevalidateAttests":
			// 0 bayt
		case "CheckSpecVersion", "CheckTxVersion":
			// extra'da 0 bayt (bu ikisi yalnızca additional_signed'da yer alır)
		case "CheckGenesis":
			// extra'da 0 bayt
		case "CheckMortality":
			era, err := readEra(c)
			if err != nil {
				return nil, fmt.Errorf("era: %w", err)
			}
			summary.Era = era
		case "CheckNonce":
			nonce, err := c.readCompact()
			if err != nil {
				return nil, fmt.Errorf("nonce: %w", err)
			}
			summary.Nonce = nonce.Uint64()
		case "ChargeTransactionPayment":
			tip, err := c.readCompact()
			if err != nil {
				return nil, fmt.Errorf("tip: %w", err)
			}
			summary.Tip = tip
		case "CheckMetadataHash":
			if _, err := c.readByte(); err != nil { // mode
				return nil, fmt.Errorf("metadata hash mode: %w", err)
			}
		default:
			return nil, fmt.Errorf("bilinmeyen signed extension %q — decode edilemiyor, DUR", id)
		}
	}

	palletIdx, err := c.readByte()
	if err != nil {
		return nil, fmt.Errorf("call pallet index: %w", err)
	}
	callIdx, err := c.readByte()
	if err != nil {
		return nil, fmt.Errorf("call index: %w", err)
	}
	wantPallet, wantCall, err := md.FindCall("Balances", "transfer_keep_alive")
	if err != nil {
		return nil, err
	}
	if palletIdx != wantPallet || callIdx != wantCall {
		return nil, fmt.Errorf("call (pallet=%d,call=%d) beklenen balances.transfer_keep_alive (pallet=%d,call=%d) değil", palletIdx, callIdx, wantPallet, wantCall)
	}
	summary.PalletIndex, summary.CallIndex = palletIdx, callIdx

	destTag, err := c.readByte()
	if err != nil {
		return nil, fmt.Errorf("dest tag: %w", err)
	}
	if destTag != multiAddressIDTag {
		return nil, fmt.Errorf("desteklenmeyen dest MultiAddress varyantı: %d", destTag)
	}
	destPub, err := c.readBytes(32)
	if err != nil {
		return nil, fmt.Errorf("dest pubkey: %w", err)
	}
	copy(summary.DestPublicKey[:], destPub)

	amount, err := c.readCompact()
	if err != nil {
		return nil, fmt.Errorf("amount: %w", err)
	}
	summary.AmountPlanck = amount

	if c.remaining() != 0 {
		return nil, fmt.Errorf("decode sonrası %d bayt artık kaldı", c.remaining())
	}

	signerSS58, err := EncodeSS58(summary.SignerPublicKey, ss58Prefix)
	if err != nil {
		return nil, fmt.Errorf("signer SS58: %w", err)
	}
	destSS58, err := EncodeSS58(summary.DestPublicKey, ss58Prefix)
	if err != nil {
		return nil, fmt.Errorf("dest SS58: %w", err)
	}
	summary.SignerSS58 = signerSS58
	summary.DestSS58 = destSS58

	return &summary, nil
}

// readEra, Era kodlamasını okur: ilk bayt 0x00 ise Immortal (1 bayt toplam),
// değilse Mortal (2 bayt toplam).
func readEra(c *scaleCursor) ([]byte, error) {
	b0, err := c.readByte()
	if err != nil {
		return nil, err
	}
	if b0 == 0x00 {
		return []byte{b0}, nil
	}
	b1, err := c.readByte()
	if err != nil {
		return nil, err
	}
	return []byte{b0, b1}, nil
}
