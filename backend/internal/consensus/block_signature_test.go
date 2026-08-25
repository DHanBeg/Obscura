package consensus

// A3.2 — Block.Sig (proposer imzası) gerçek Ed25519 doğrulama kanıtı.
// Aynı 4-test disiplini (bkz. vote_signature_test.go): pozitif, sahte,
// tampering, yanlış-anahtar. Proposer alanı artık beyan-etiketi DEĞİL —
// sadece registry'deki gerçek özel anahtarın sahibi geçerli bir Sig
// üretebilir.

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"testing"

	"obscura.network/core/internal/sequencer"
)

func injectBlockProposal(e *Engine, b Block) {
	raw, _ := json.Marshal(ConsensusMsg{Type: "block_proposal", Block: &b})
	e.handleMsg(raw)
}

// validTestBlock — height/TxRoot tutarlı, boş-Ops'lu bir blok (Sig hariç
// tüm alanlar dolu). Sig, ayrı ayrı her testte senaryoya göre eklenir.
func validTestBlock(height uint64, proposer, hash string) Block {
	return Block{
		Height: height, Round: 0, ParentHash: GenesisParentHash,
		TxRoot: sequencer.ComputeMerkleRoot(nil), Proposer: proposer,
		Timestamp: 1700000000000, Hash: hash, Ops: nil,
	}
}

// TestBlockSignature_Valid_Accepted — POZİTİF: peer-1'in gerçek özel
// anahtarıyla Hash üzerinde imzaladığı blok kabul edilir (currentBlock set
// edilir, phase PREVOTE'a geçer).
func TestBlockSignature_Valid_Accepted(t *testing.T) {
	pub1, priv1 := genKeypair(t)
	reg := fakeRegistry{"peer-1": pub1}
	e := newTestEngineWithRegistry(reg, 2)

	b := validTestBlock(e.height, "peer-1", "hash-A")
	b.Sig = hex.EncodeToString(ed25519.Sign(priv1, []byte(b.Hash)))

	injectBlockProposal(e, b)

	if e.currentBlock == nil || e.currentBlock.Hash != "hash-A" {
		t.Fatalf("geçerli imzalı blok REDDEDİLDİ — kabul edilmeliydi, currentBlock=%v", e.currentBlock)
	}
	if e.phase != PhasePrevote {
		t.Fatalf("kabul sonrası phase PREVOTE olmalıydı, got=%s", e.phase)
	}
}

// TestBlockSignature_Forged_Rejected — NEGATİF: rastgele/geçersiz imzalı
// blok (imza hiç üretilmemiş, uydurma) reddedilir — currentBlock set
// edilmez.
func TestBlockSignature_Forged_Rejected(t *testing.T) {
	pub1, _ := genKeypair(t)
	reg := fakeRegistry{"peer-1": pub1}
	e := newTestEngineWithRegistry(reg, 2)

	b := validTestBlock(e.height, "peer-1", "hash-A")
	b.Sig = hex.EncodeToString(make([]byte, ed25519.SignatureSize)) // format doğru, içerik sahte

	injectBlockProposal(e, b)

	if e.currentBlock != nil {
		t.Fatal("sahte imzalı blok KABUL EDİLDİ — reddedilmeliydi (sahte-yeşil tuzağı)")
	}
	if e.phase != PhasePropose {
		t.Fatalf("reddedilen blok sonrası phase değişmemeliydi, got=%s", e.phase)
	}
}

// TestBlockSignature_Tampered_Rejected — peer-1 GERÇEKTEN "hash-A"yı
// imzaladı, ama mesaj gönderilmeden önce Hash "hash-EVIL"e değiştirildi
// (Sig aynı kaldı). Sig, Hash string'i üzerinden doğrulandığı için bu
// reddedilmeli — imza sadece "var mı" değil, GERÇEKTEN o Hash'i mi
// kapsıyor diye kontrol ediliyor.
func TestBlockSignature_Tampered_Rejected(t *testing.T) {
	pub1, priv1 := genKeypair(t)
	reg := fakeRegistry{"peer-1": pub1}
	e := newTestEngineWithRegistry(reg, 2)

	b := validTestBlock(e.height, "peer-1", "hash-A")
	b.Sig = hex.EncodeToString(ed25519.Sign(priv1, []byte(b.Hash))) // "hash-A" üzerinden imzalandı

	b.Hash = "hash-EVIL" // imzalandıktan SONRA tahrif edildi

	injectBlockProposal(e, b)

	if e.currentBlock != nil {
		t.Fatal("tahrif edilmiş (imzalandıktan sonra Hash değiştirilmiş) blok KABUL EDİLDİ")
	}
}

// TestBlockSignature_WrongKey_Rejected — blok "peer-1" olduğunu iddia
// ediyor, format olarak geçerli bir Ed25519 imza taşıyor, ama imza
// peer-1'in DEĞİL peer-2'nin özel anahtarıyla üretilmiş —
// impersonation/sahte-proposer senaryosu.
func TestBlockSignature_WrongKey_Rejected(t *testing.T) {
	pub1, _ := genKeypair(t)
	_, priv2 := genKeypair(t) // peer-2'nin özel anahtarı — registry'de peer-1 diye YOK
	reg := fakeRegistry{"peer-1": pub1}
	e := newTestEngineWithRegistry(reg, 2)

	b := validTestBlock(e.height, "peer-1", "hash-A")
	b.Sig = hex.EncodeToString(ed25519.Sign(priv2, []byte(b.Hash))) // peer-1 iddiası, peer-2'nin anahtarı

	injectBlockProposal(e, b)

	if e.currentBlock != nil {
		t.Fatal("başka node'un anahtarıyla imzalanmış blok 'peer-1' kimliğiyle KABUL EDİLDİ — impersonation")
	}
}
