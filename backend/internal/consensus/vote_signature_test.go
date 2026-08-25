package consensus

// A3.1 — Vote.Sig gerçek Ed25519 imza/doğrulama kanıtı.
//
// Sahte-yeşil tuzağı: "imza kontrol ediliyor gibi ama aslında geçiyor" bug'ı
// sadece pozitif testle YAKALANMAZ. Bu yüzden 4 test ZORUNLU:
//  1. pozitif  — geçerli imzalı oy kabul edilir (prevotes map'ine girer)
//  2. sahte    — rastgele/geçersiz imza reddedilir
//  3. tampering — imzalanan içerik SONRADAN değiştirilirse imza geçersiz olur
//     (imza gerçekten gövdeyi mi kapsıyor, yoksa sadece "var mı" mı bakılıyor)
//  4. yanlış-anahtar — doğru formatlı ama BAŞKA node'un anahtarıyla imzalanmış
//     oy reddedilir (kimlik gerçekten doğrulanıyor mu, yoksa sadece format mı)

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
)

// fakeRegistry — federation.Get'in test-double'ı: nodeID → gerçek Ed25519
// genel anahtarı. Üretimde bunun yerini main.go'daki federation.Get(nodeID)
// tabanlı bftVerifyFn alır (bkz. cmd/node/main.go).
type fakeRegistry map[string]ed25519.PublicKey

func (r fakeRegistry) verify(nodeID string, payload []byte, sigHex string) error {
	pub, ok := r[nodeID]
	if !ok {
		return errors.New("nodeID bilinmiyor")
	}
	sig, err := hex.DecodeString(sigHex)
	if err != nil {
		return err
	}
	if len(sig) != ed25519.SignatureSize || !ed25519.Verify(pub, payload, sig) {
		return errors.New("imza geçersiz")
	}
	return nil
}

func genKeypair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("anahtar üretilemedi: %v", err)
	}
	return pub, priv
}

// injectVote — Vote'u bir ConsensusMsg içinde JSON'a çevirip doğrudan
// e.handleMsg'e verir (gerçek P2P mesaj alma yolunu birebir taklit eder).
func injectVote(e *Engine, v Vote) {
	raw, _ := json.Marshal(ConsensusMsg{Type: "vote", Vote: &v})
	e.handleMsg(raw)
}

func newTestEngineWithRegistry(reg fakeRegistry, quorum int) *Engine {
	return NewEngine("self-node", quorum, nil, noopPublish, noopSubscribe, nil, nil, nil, reg.verify)
}

// TestVoteSignature_Valid_Accepted — POZİTİF: peer-1'in kendi gerçek özel
// anahtarıyla imzaladığı bir oy, registry'deki genel anahtarıyla doğrulanır
// ve prevotes map'ine kabul edilir.
func TestVoteSignature_Valid_Accepted(t *testing.T) {
	pub1, priv1 := genKeypair(t)
	reg := fakeRegistry{"peer-1": pub1}
	e := newTestEngineWithRegistry(reg, 2) // quorum=2: tek oy kendi başına faz değiştirmesin, sadece kabul/red'e bakıyoruz

	v := Vote{Phase: PhasePrevote, Height: e.height, Round: 0, BlockHash: "block-abc", NodeID: "peer-1"}
	sig := ed25519.Sign(priv1, voteSigningPayload(v))
	v.Sig = hex.EncodeToString(sig)

	injectVote(e, v)

	got, ok := e.prevotes["peer-1"]
	if !ok {
		t.Fatal("geçerli imzalı oy REDDEDİLDİ — kabul edilmeliydi")
	}
	if got.BlockHash != "block-abc" {
		t.Fatalf("kabul edilen oyun içeriği bozuk: %+v", got)
	}
}

// TestVoteSignature_Forged_Rejected — NEGATİF: rastgele/geçersiz imzalı oy
// (imza hiç üretilmemiş, elle uydurulmuş hex) prevotes map'ine GİRMEMELİ.
// Bu test olmadan "imza doğrulanıyor" iddia edilemez — sadece pozitif test
// sahte-yeşildir (verifyFn hiç çağrılmasa ya da her zaman nil dönse bile
// pozitif test geçerdi).
func TestVoteSignature_Forged_Rejected(t *testing.T) {
	pub1, _ := genKeypair(t)
	reg := fakeRegistry{"peer-1": pub1}
	e := newTestEngineWithRegistry(reg, 2)

	v := Vote{
		Phase: PhasePrevote, Height: e.height, Round: 0, BlockHash: "block-abc", NodeID: "peer-1",
		Sig: hex.EncodeToString(make([]byte, ed25519.SignatureSize)), // format doğru (64 byte), içerik sahte
	}
	injectVote(e, v)

	if _, ok := e.prevotes["peer-1"]; ok {
		t.Fatal("sahte imzalı oy KABUL EDİLDİ — reddedilmeliydi (sahte-yeşil tuzağı)")
	}
}

// TestVoteSignature_Tampered_Rejected — imza GEÇERLİ bir imza (peer-1
// gerçekten imzaladı) ama İÇERİK imzalandıktan SONRA değiştirildi
// (BlockHash "block-abc" → "block-EVIL"). İmza gövdeyi gerçekten kapsıyorsa
// bu reddedilmeli; sadece "imza var mı" kontrolü yapan bir stub bunu
// YAKALAYAMAZ.
func TestVoteSignature_Tampered_Rejected(t *testing.T) {
	pub1, priv1 := genKeypair(t)
	reg := fakeRegistry{"peer-1": pub1}
	e := newTestEngineWithRegistry(reg, 2)

	v := Vote{Phase: PhasePrevote, Height: e.height, Round: 0, BlockHash: "block-abc", NodeID: "peer-1"}
	sig := ed25519.Sign(priv1, voteSigningPayload(v)) // "block-abc" üzerinden imzalandı
	v.Sig = hex.EncodeToString(sig)

	v.BlockHash = "block-EVIL" // imzalandıktan SONRA tahrif edildi

	injectVote(e, v)

	if _, ok := e.prevotes["peer-1"]; ok {
		t.Fatal("tahrif edilmiş (imzalandıktan sonra değiştirilmiş) oy KABUL EDİLDİ")
	}
}

// TestVoteSignature_WrongKey_Rejected — YANLIŞ-ANAHTAR: mesaj "peer-1"
// olduğunu iddia ediyor ve format olarak geçerli bir Ed25519 imza taşıyor,
// ama imza peer-1'in DEĞİL başka bir node'un (peer-2) özel anahtarıyla
// üretilmiş — kimlik hırsızlığı/impersonation senaryosu. Registry'deki
// peer-1 genel anahtarıyla doğrulama BAŞARISIZ olmalı.
func TestVoteSignature_WrongKey_Rejected(t *testing.T) {
	pub1, _ := genKeypair(t)
	_, priv2 := genKeypair(t) // peer-2'nin özel anahtarı — registry'de YOK
	reg := fakeRegistry{"peer-1": pub1}
	e := newTestEngineWithRegistry(reg, 2)

	v := Vote{Phase: PhasePrevote, Height: e.height, Round: 0, BlockHash: "block-abc", NodeID: "peer-1"}
	sig := ed25519.Sign(priv2, voteSigningPayload(v)) // peer-1 iddiası, ama peer-2'nin anahtarıyla imzalı
	v.Sig = hex.EncodeToString(sig)

	injectVote(e, v)

	if _, ok := e.prevotes["peer-1"]; ok {
		t.Fatal("başka node'un anahtarıyla imzalanmış oy 'peer-1' kimliğiyle KABUL EDİLDİ — impersonation")
	}
}

// TestVoteSignature_QuorumReachedOnlyWithValidSignatures — uçtan uca:
// quorum=2, biri geçerli biri sahte imzalı 2 oy gelirse quorum'a
// ULAŞILMAMALI (sahte sayılmadığı için sadece 1 geçerli oy var).
func TestVoteSignature_QuorumReachedOnlyWithValidSignatures(t *testing.T) {
	pub1, priv1 := genKeypair(t)
	pub2, _ := genKeypair(t)
	reg := fakeRegistry{"peer-1": pub1, "peer-2": pub2}
	e := newTestEngineWithRegistry(reg, 2)

	valid := Vote{Phase: PhasePrevote, Height: e.height, Round: 0, BlockHash: "block-abc", NodeID: "peer-1"}
	valid.Sig = hex.EncodeToString(ed25519.Sign(priv1, voteSigningPayload(valid)))
	injectVote(e, valid)

	forged := Vote{
		Phase: PhasePrevote, Height: e.height, Round: 0, BlockHash: "block-abc", NodeID: "peer-2",
		Sig: hex.EncodeToString(make([]byte, ed25519.SignatureSize)),
	}
	injectVote(e, forged)

	if len(e.prevotes) != 1 {
		t.Fatalf("sadece 1 geçerli oy kabul edilmeliydi, prevotes=%v", e.prevotes)
	}
	if e.phase == PhasePrecommit {
		t.Fatal("quorum=2 iken 1 geçerli oyla PRECOMMIT'e geçildi — sahte oy sayılmış olmalı")
	}
}
