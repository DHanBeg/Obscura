// Package bridge — Substrate runtime metadata (V14) decoder.
//
// Bridge madde 4, PARÇA 2: pallet/call index'lerini HARDCODE ETMEK YERİNE
// zincirin kendi state_getMetadata çıktısından çözer. Paseo runtime'ı
// güncellendiğinde (spec_version değiştiğinde) index'ler değişebilir —
// bu dosya her seferinde canlı metadata'ya karşı çalışır.
//
// Yalnızca V14 desteklenir (Paseo'nun döndürdüğü sürüm, doğrulandı: bkz.
// metadata_test.go). V15 farklı bir ExtrinsicMetadata şekli kullanır
// (address_ty/call_ty/signature_ty/extra_ty ayrı ayrı) — Paseo V14
// döndürdüğü sürece kapsam dışı.
//
// Kaynak referansı (yapı birebir buradan alındı):
// github.com/paritytech/frame-metadata (v14.rs) +
// github.com/paritytech/scale-info (ty/*.rs, portable.rs, form.rs).
package bridge

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// scaleTypeDefKind, TypeDef enum'ının SCALE tag değerleridir (scale-info
// ty/mod.rs — sıra backwards-compat için sabit, değiştirilemez).
const (
	typeDefComposite   = 0
	typeDefVariant     = 1
	typeDefSequence    = 2
	typeDefArray       = 3
	typeDefTuple       = 4
	typeDefPrimitive   = 5
	typeDefCompact     = 6
	typeDefBitSequence = 7
)

// Field — scale-info Field (isim + tip id + type_name + docs, decode
// sırasında yalnızca ty kullanılıyor).
type field struct {
	ty uint32
}

// variant — scale-info Variant: enum'ın bir kolu.
type variant struct {
	name  string
	index uint8
}

// typeDef — bu paketin ihtiyaç duyduğu kadarıyla scale-info TypeDef.
// Yalnızca Variant kolu (pallet call enum'ları için) tam saklanıyor; diğer
// kollar yalnızca decode sırasında doğru bayt sayısını atlamak için
// kullanılıyor ve saklanmıyor.
type typeDef struct {
	kind     int
	variants []variant // yalnızca kind==typeDefVariant için dolu
}

// portableType — PortableRegistry'deki tek bir kayıt.
type portableType struct {
	id      uint32
	path    []string
	typeDef typeDef
}

// palletMeta — bir pallet'in bu paket için gerekli metadata'sı.
type palletMeta struct {
	name     string
	index    uint8
	callsTy  uint32
	hasCalls bool
}

// signedExtensionMeta — bir signed extension'ın kimliği + extra/additional
// tip id'leri.
type signedExtensionMeta struct {
	identifier       string
	ty               uint32
	additionalSigned uint32
}

// Metadata — bu paketin ihtiyaç duyduğu kadarıyla çözülmüş RuntimeMetadataV14.
type Metadata struct {
	types            map[uint32]portableType
	pallets          []palletMeta
	extrinsicVersion uint8
	signedExtensions []signedExtensionMeta
}

// DecodeMetadataV14, state_getMetadata'nın döndürdüğü "0x6d657461..." hex
// string'ini çözer. Yalnızca V14 destekleniyor (bkz. paket dokümantasyonu).
func DecodeMetadataV14(hexStr string) (*Metadata, error) {
	raw := strings.TrimPrefix(hexStr, "0x")
	data, err := hex.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("metadata hex decode: %w", err)
	}

	c := newScaleCursor(data)
	magic, err := c.readBytes(4)
	if err != nil {
		return nil, fmt.Errorf("magic: %w", err)
	}
	if string(magic) != "meta" {
		return nil, fmt.Errorf("geçersiz metadata magic: %x (beklenen 'meta')", magic)
	}
	version, err := c.readByte()
	if err != nil {
		return nil, fmt.Errorf("version: %w", err)
	}
	if version != 14 {
		return nil, fmt.Errorf("desteklenmeyen metadata sürümü: %d (yalnızca V14 destekleniyor, HARDCODE ile V15/başka sürüm varsayılmadı)", version)
	}

	types, err := decodePortableRegistry(c)
	if err != nil {
		return nil, fmt.Errorf("type registry: %w", err)
	}

	pallets, err := decodePallets(c)
	if err != nil {
		return nil, fmt.Errorf("pallets: %w", err)
	}

	extrinsicTy, err := c.readCompactUint32()
	if err != nil {
		return nil, fmt.Errorf("extrinsic.ty: %w", err)
	}
	_ = extrinsicTy
	extVersion, err := c.readByte()
	if err != nil {
		return nil, fmt.Errorf("extrinsic.version: %w", err)
	}
	signedExts, err := decodeSignedExtensions(c)
	if err != nil {
		return nil, fmt.Errorf("signed_extensions: %w", err)
	}

	// runtime.ty (Compact<u32>) — son alan, kullanılmıyor ama tüketilmeli.
	if _, err := c.readCompactUint32(); err != nil {
		return nil, fmt.Errorf("runtime.ty: %w", err)
	}

	if c.remaining() != 0 {
		return nil, fmt.Errorf("metadata decode sonrası %d bayt artık kaldı (yapı yanlış çözülmüş olabilir)", c.remaining())
	}

	typesByID := make(map[uint32]portableType, len(types))
	for _, t := range types {
		typesByID[t.id] = t
	}

	return &Metadata{
		types:            typesByID,
		pallets:          pallets,
		extrinsicVersion: extVersion,
		signedExtensions: signedExts,
	}, nil
}

// ExtrinsicVersion, zincirin kullandığı UncheckedExtrinsic format sürümünü
// döndürür (Paseo'da doğrulandı: 4).
func (m *Metadata) ExtrinsicVersion() uint8 { return m.extrinsicVersion }

// SignedExtensionIdentifiers, zincirin signed extension kimliklerini SIRAYLA
// döndürür (bu sıra hem extra hem additional_signed kodlamasında zorunlu).
func (m *Metadata) SignedExtensionIdentifiers() []string {
	out := make([]string, len(m.signedExtensions))
	for i, se := range m.signedExtensions {
		out[i] = se.identifier
	}
	return out
}

// FindCall, verilen pallet+call adının pallet index + call variant index'ini
// metadata'dan çözer. Bulunamazsa (pallet yok, calls yok, variant yok) HATA
// döner — hiçbir zaman varsayılan/hardcode bir index'e düşmez.
func (m *Metadata) FindCall(palletName, callName string) (palletIndex, callIndex uint8, err error) {
	var pallet *palletMeta
	for i := range m.pallets {
		if m.pallets[i].name == palletName {
			pallet = &m.pallets[i]
			break
		}
	}
	if pallet == nil {
		return 0, 0, fmt.Errorf("pallet %q metadata'da bulunamadı", palletName)
	}
	if !pallet.hasCalls {
		return 0, 0, fmt.Errorf("pallet %q call tanımlamıyor", palletName)
	}

	callsType, ok := m.types[pallet.callsTy]
	if !ok {
		return 0, 0, fmt.Errorf("pallet %q calls tipi (id=%d) registry'de yok", palletName, pallet.callsTy)
	}
	if callsType.typeDef.kind != typeDefVariant {
		return 0, 0, fmt.Errorf("pallet %q calls tipi variant/enum değil (kind=%d)", palletName, callsType.typeDef.kind)
	}

	for _, v := range callsType.typeDef.variants {
		if v.name == callName {
			return pallet.index, v.index, nil
		}
	}
	return 0, 0, fmt.Errorf("call %q pallet %q içinde bulunamadı", callName, palletName)
}

// ─── decode helpers ────────────────────────────────────────────────────────

func decodePortableRegistry(c *scaleCursor) ([]portableType, error) {
	n, err := c.readCompactUint32()
	if err != nil {
		return nil, fmt.Errorf("types vec uzunluk: %w", err)
	}
	out := make([]portableType, 0, n)
	for i := uint32(0); i < n; i++ {
		id, err := c.readCompactUint32()
		if err != nil {
			return nil, fmt.Errorf("type[%d].id: %w", i, err)
		}
		path, err := decodePath(c)
		if err != nil {
			return nil, fmt.Errorf("type[%d].path: %w", i, err)
		}
		if err := skipTypeParams(c); err != nil {
			return nil, fmt.Errorf("type[%d].type_params: %w", i, err)
		}
		td, err := decodeTypeDef(c)
		if err != nil {
			return nil, fmt.Errorf("type[%d].type_def: %w", i, err)
		}
		if err := skipStringVec(c); err != nil { // docs
			return nil, fmt.Errorf("type[%d].docs: %w", i, err)
		}
		out = append(out, portableType{id: id, path: path, typeDef: td})
	}
	return out, nil
}

func decodePath(c *scaleCursor) ([]string, error) {
	n, err := c.readCompactUint32()
	if err != nil {
		return nil, err
	}
	segs := make([]string, n)
	for i := range segs {
		s, err := c.readString()
		if err != nil {
			return nil, err
		}
		segs[i] = s
	}
	return segs, nil
}

// skipTypeParams, Vec<TypeParameter{name:String, ty:Option<Compact<u32>>}>'ı
// atlar (bu paket type_param içeriğini kullanmıyor, yalnızca doğru offset'e
// ilerlemek için tüketiliyor).
func skipTypeParams(c *scaleCursor) error {
	n, err := c.readCompactUint32()
	if err != nil {
		return err
	}
	for i := uint32(0); i < n; i++ {
		if _, err := c.readString(); err != nil {
			return err
		}
		hasTy, err := c.readOptionTag()
		if err != nil {
			return err
		}
		if hasTy {
			if _, err := c.readCompactUint32(); err != nil {
				return err
			}
		}
	}
	return nil
}

func skipStringVec(c *scaleCursor) error {
	n, err := c.readCompactUint32()
	if err != nil {
		return err
	}
	for i := uint32(0); i < n; i++ {
		if _, err := c.readString(); err != nil {
			return err
		}
	}
	return nil
}

func decodeField(c *scaleCursor) (field, error) {
	hasName, err := c.readOptionTag()
	if err != nil {
		return field{}, fmt.Errorf("field.name tag: %w", err)
	}
	if hasName {
		if _, err := c.readString(); err != nil {
			return field{}, fmt.Errorf("field.name: %w", err)
		}
	}
	ty, err := c.readCompactUint32()
	if err != nil {
		return field{}, fmt.Errorf("field.ty: %w", err)
	}
	hasTypeName, err := c.readOptionTag()
	if err != nil {
		return field{}, fmt.Errorf("field.type_name tag: %w", err)
	}
	if hasTypeName {
		if _, err := c.readString(); err != nil {
			return field{}, fmt.Errorf("field.type_name: %w", err)
		}
	}
	if err := skipStringVec(c); err != nil { // docs
		return field{}, fmt.Errorf("field.docs: %w", err)
	}
	return field{ty: ty}, nil
}

func decodeFields(c *scaleCursor) ([]field, error) {
	n, err := c.readCompactUint32()
	if err != nil {
		return nil, err
	}
	out := make([]field, n)
	for i := range out {
		f, err := decodeField(c)
		if err != nil {
			return nil, fmt.Errorf("field[%d]: %w", i, err)
		}
		out[i] = f
	}
	return out, nil
}

func decodeVariant(c *scaleCursor) (variant, error) {
	name, err := c.readString()
	if err != nil {
		return variant{}, fmt.Errorf("variant.name: %w", err)
	}
	if _, err := decodeFields(c); err != nil {
		return variant{}, fmt.Errorf("variant.fields: %w", err)
	}
	index, err := c.readByte()
	if err != nil {
		return variant{}, fmt.Errorf("variant.index: %w", err)
	}
	if err := skipStringVec(c); err != nil { // docs
		return variant{}, fmt.Errorf("variant.docs: %w", err)
	}
	return variant{name: name, index: index}, nil
}

func decodeTypeDef(c *scaleCursor) (typeDef, error) {
	tag, err := c.readByte()
	if err != nil {
		return typeDef{}, err
	}
	switch tag {
	case typeDefComposite:
		if _, err := decodeFields(c); err != nil {
			return typeDef{}, fmt.Errorf("composite.fields: %w", err)
		}
		return typeDef{kind: typeDefComposite}, nil
	case typeDefVariant:
		n, err := c.readCompactUint32()
		if err != nil {
			return typeDef{}, fmt.Errorf("variant.count: %w", err)
		}
		variants := make([]variant, n)
		for i := range variants {
			v, err := decodeVariant(c)
			if err != nil {
				return typeDef{}, fmt.Errorf("variant[%d]: %w", i, err)
			}
			variants[i] = v
		}
		return typeDef{kind: typeDefVariant, variants: variants}, nil
	case typeDefSequence:
		if _, err := c.readCompactUint32(); err != nil { // type_param
			return typeDef{}, fmt.Errorf("sequence.type_param: %w", err)
		}
		return typeDef{kind: typeDefSequence}, nil
	case typeDefArray:
		if _, err := c.readU32LE(); err != nil { // len (fixed u32, compact değil)
			return typeDef{}, fmt.Errorf("array.len: %w", err)
		}
		if _, err := c.readCompactUint32(); err != nil { // type_param
			return typeDef{}, fmt.Errorf("array.type_param: %w", err)
		}
		return typeDef{kind: typeDefArray}, nil
	case typeDefTuple:
		n, err := c.readCompactUint32()
		if err != nil {
			return typeDef{}, fmt.Errorf("tuple.count: %w", err)
		}
		for i := uint32(0); i < n; i++ {
			if _, err := c.readCompactUint32(); err != nil {
				return typeDef{}, fmt.Errorf("tuple.field[%d]: %w", i, err)
			}
		}
		return typeDef{kind: typeDefTuple}, nil
	case typeDefPrimitive:
		if _, err := c.readByte(); err != nil { // primitive id (0..14)
			return typeDef{}, fmt.Errorf("primitive.id: %w", err)
		}
		return typeDef{kind: typeDefPrimitive}, nil
	case typeDefCompact:
		if _, err := c.readCompactUint32(); err != nil { // type_param
			return typeDef{}, fmt.Errorf("compact.type_param: %w", err)
		}
		return typeDef{kind: typeDefCompact}, nil
	case typeDefBitSequence:
		if _, err := c.readCompactUint32(); err != nil { // bit_store_type
			return typeDef{}, fmt.Errorf("bitsequence.store: %w", err)
		}
		if _, err := c.readCompactUint32(); err != nil { // bit_order_type
			return typeDef{}, fmt.Errorf("bitsequence.order: %w", err)
		}
		return typeDef{kind: typeDefBitSequence}, nil
	default:
		return typeDef{}, fmt.Errorf("bilinmeyen TypeDef tag: %d (scale-info sürümü uyuşmuyor olabilir)", tag)
	}
}

func decodePallets(c *scaleCursor) ([]palletMeta, error) {
	n, err := c.readCompactUint32()
	if err != nil {
		return nil, fmt.Errorf("pallets vec uzunluk: %w", err)
	}
	out := make([]palletMeta, n)
	for i := uint32(0); i < n; i++ {
		name, err := c.readString()
		if err != nil {
			return nil, fmt.Errorf("pallet[%d].name: %w", i, err)
		}

		if err := skipOptionPalletStorage(c); err != nil {
			return nil, fmt.Errorf("pallet[%d].storage: %w", i, err)
		}

		hasCalls, err := c.readOptionTag()
		if err != nil {
			return nil, fmt.Errorf("pallet[%d].calls tag: %w", i, err)
		}
		var callsTy uint32
		if hasCalls {
			callsTy, err = c.readCompactUint32()
			if err != nil {
				return nil, fmt.Errorf("pallet[%d].calls.ty: %w", i, err)
			}
		}

		if err := skipOptionSingleTyStruct(c); err != nil { // event
			return nil, fmt.Errorf("pallet[%d].event: %w", i, err)
		}

		if err := skipConstants(c); err != nil {
			return nil, fmt.Errorf("pallet[%d].constants: %w", i, err)
		}

		if err := skipOptionSingleTyStruct(c); err != nil { // error
			return nil, fmt.Errorf("pallet[%d].error: %w", i, err)
		}

		index, err := c.readByte()
		if err != nil {
			return nil, fmt.Errorf("pallet[%d].index: %w", i, err)
		}

		out[i] = palletMeta{name: name, index: index, callsTy: callsTy, hasCalls: hasCalls}
	}
	return out, nil
}

// skipOptionSingleTyStruct, Option<{ ty: Compact<u32> }> şeklindeki
// PalletEventMetadata/PalletErrorMetadata alanlarını atlar.
func skipOptionSingleTyStruct(c *scaleCursor) error {
	has, err := c.readOptionTag()
	if err != nil {
		return err
	}
	if has {
		if _, err := c.readCompactUint32(); err != nil {
			return err
		}
	}
	return nil
}

func skipOptionPalletStorage(c *scaleCursor) error {
	has, err := c.readOptionTag()
	if err != nil {
		return err
	}
	if !has {
		return nil
	}
	if _, err := c.readString(); err != nil { // prefix
		return fmt.Errorf("storage.prefix: %w", err)
	}
	n, err := c.readCompactUint32()
	if err != nil {
		return fmt.Errorf("storage.entries count: %w", err)
	}
	for i := uint32(0); i < n; i++ {
		if err := skipStorageEntry(c); err != nil {
			return fmt.Errorf("storage.entries[%d]: %w", i, err)
		}
	}
	return nil
}

func skipStorageEntry(c *scaleCursor) error {
	if _, err := c.readString(); err != nil { // name
		return fmt.Errorf("name: %w", err)
	}
	if _, err := c.readByte(); err != nil { // modifier (Optional=0/Default=1)
		return fmt.Errorf("modifier: %w", err)
	}
	tag, err := c.readByte()
	if err != nil {
		return fmt.Errorf("storage entry type tag: %w", err)
	}
	switch tag {
	case 0: // Plain(Type)
		if _, err := c.readCompactUint32(); err != nil {
			return fmt.Errorf("plain.ty: %w", err)
		}
	case 1: // Map{hashers, key, value}
		nh, err := c.readCompactUint32()
		if err != nil {
			return fmt.Errorf("map.hashers count: %w", err)
		}
		for i := uint32(0); i < nh; i++ {
			if _, err := c.readByte(); err != nil { // StorageHasher tag
				return fmt.Errorf("map.hashers[%d]: %w", i, err)
			}
		}
		if _, err := c.readCompactUint32(); err != nil { // key
			return fmt.Errorf("map.key: %w", err)
		}
		if _, err := c.readCompactUint32(); err != nil { // value
			return fmt.Errorf("map.value: %w", err)
		}
	default:
		return fmt.Errorf("bilinmeyen StorageEntryType tag: %d", tag)
	}
	// default: Vec<u8>
	dn, err := c.readCompactUint32()
	if err != nil {
		return fmt.Errorf("default vec len: %w", err)
	}
	if _, err := c.readBytes(int(dn)); err != nil {
		return fmt.Errorf("default bytes: %w", err)
	}
	return skipStringVec(c) // docs
}

func skipConstants(c *scaleCursor) error {
	n, err := c.readCompactUint32()
	if err != nil {
		return fmt.Errorf("constants count: %w", err)
	}
	for i := uint32(0); i < n; i++ {
		if _, err := c.readString(); err != nil { // name
			return fmt.Errorf("constants[%d].name: %w", i, err)
		}
		if _, err := c.readCompactUint32(); err != nil { // ty
			return fmt.Errorf("constants[%d].ty: %w", i, err)
		}
		dn, err := c.readCompactUint32() // value: Vec<u8>
		if err != nil {
			return fmt.Errorf("constants[%d].value len: %w", i, err)
		}
		if _, err := c.readBytes(int(dn)); err != nil {
			return fmt.Errorf("constants[%d].value: %w", i, err)
		}
		if err := skipStringVec(c); err != nil { // docs
			return fmt.Errorf("constants[%d].docs: %w", i, err)
		}
	}
	return nil
}

func decodeSignedExtensions(c *scaleCursor) ([]signedExtensionMeta, error) {
	n, err := c.readCompactUint32()
	if err != nil {
		return nil, fmt.Errorf("count: %w", err)
	}
	out := make([]signedExtensionMeta, n)
	for i := uint32(0); i < n; i++ {
		identifier, err := c.readString()
		if err != nil {
			return nil, fmt.Errorf("[%d].identifier: %w", i, err)
		}
		ty, err := c.readCompactUint32()
		if err != nil {
			return nil, fmt.Errorf("[%d].ty: %w", i, err)
		}
		additional, err := c.readCompactUint32()
		if err != nil {
			return nil, fmt.Errorf("[%d].additional_signed: %w", i, err)
		}
		out[i] = signedExtensionMeta{identifier: identifier, ty: ty, additionalSigned: additional}
	}
	return out, nil
}
