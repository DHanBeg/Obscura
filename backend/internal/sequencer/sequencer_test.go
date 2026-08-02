package sequencer

import (
	"encoding/binary"
	"encoding/hex"
	"path/filepath"
	"testing"
	"time"
)

func epochAlpha(epoch uint64) []byte {
	alpha := make([]byte, 0, 16)
	alpha = append(alpha, []byte("obscura-epoch")...)
	var eb [8]byte
	binary.BigEndian.PutUint64(eb[:], epoch)
	return append(alpha, eb[:]...)
}

// TestECVRFProveVerify happy path: üretilen proof aynı pubkey+alpha ile doğrulanır.
func TestECVRFProveVerify(t *testing.T) {
	key := loadOrCreateVRFKey("")
	alpha := epochAlpha(42)

	pi := ecvrfProve(key, alpha)
	if len(pi) != pointLen+challengeLen+32 {
		t.Fatalf("beklenmeyen pi uzunluğu: %d", len(pi))
	}
	if !ecvrfVerify(&key.PublicKey, alpha, pi) {
		t.Fatal("geçerli proof doğrulanamadı")
	}
}

// TestECVRFWrongAlpha: farklı alpha ile doğrulama başarısız olmalı (error case).
func TestECVRFWrongAlpha(t *testing.T) {
	key := loadOrCreateVRFKey("")
	pi := ecvrfProve(key, epochAlpha(1))
	if ecvrfVerify(&key.PublicKey, epochAlpha(2), pi) {
		t.Fatal("yanlış alpha ile doğrulama geçmemeliydi")
	}
}

// TestECVRFWrongKey: başka node'un anahtarıyla doğrulama başarısız olmalı.
func TestECVRFWrongKey(t *testing.T) {
	k1 := loadOrCreateVRFKey("")
	k2 := loadOrCreateVRFKey("")
	pi := ecvrfProve(k1, epochAlpha(7))
	if ecvrfVerify(&k2.PublicKey, epochAlpha(7), pi) {
		t.Fatal("yanlış anahtarla doğrulama geçmemeliydi")
	}
}

// TestECVRFTamperedProof: bozulmuş proof reddedilmeli.
func TestECVRFTamperedProof(t *testing.T) {
	key := loadOrCreateVRFKey("")
	pi := ecvrfProve(key, epochAlpha(5))
	pi[len(pi)-1] ^= 0xFF // s'i boz
	if ecvrfVerify(&key.PublicKey, epochAlpha(5), pi) {
		t.Fatal("bozuk proof doğrulanmamalıydı")
	}
}

// TestVRFDeterministic: aynı key+alpha aynı beta üretir.
func TestVRFDeterministic(t *testing.T) {
	key := loadOrCreateVRFKey("")
	alpha := epochAlpha(99)
	b1 := ecvrfProofToHash(key.Curve, ecvrfProve(key, alpha))
	b2 := ecvrfProofToHash(key.Curve, ecvrfProve(key, alpha))
	if hex.EncodeToString(b1) != hex.EncodeToString(b2) {
		t.Fatal("VRF beta deterministik değil")
	}
}

// TestLoadOrCreateVRFKey_EmptyPathIsEphemeralAndUnguessable: path boşsa her
// çağrı farklı (gerçekten rastgele) bir anahtar üretmeli — ESKİDEN
// (deriveVRFKey) aynı node_id HER ZAMAN aynı anahtarı veriyordu, yani
// node_id'yi bilen herkes "özel" anahtarı hesaplayabiliyordu (2026-08-02'de
// VRF adillik incelemesinde bulunan güvenlik açığı, bkz. loadOrCreateVRFKey
// yorumu). Artık node_id'den TAMAMEN bağımsız, gerçek entropiden üretiliyor.
func TestLoadOrCreateVRFKey_EmptyPathIsEphemeralAndUnguessable(t *testing.T) {
	a := loadOrCreateVRFKey("")
	b := loadOrCreateVRFKey("")
	if a.D.Cmp(b.D) == 0 {
		t.Fatal("path boşken iki ayrı çağrı aynı anahtarı üretti — rastgele değil")
	}
}

// TestLoadOrCreateVRFKey_PersistsAcrossReload: dosya yolu verildiğinde
// anahtar kalıcı olmalı — restart sonrası aynı anahtar (dolayısıyla aynı
// pubkey) yeniden yüklenmeli, node_id↔pubkey bağı sabit kalsın diye.
func TestLoadOrCreateVRFKey_PersistsAcrossReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vrf_identity.key")
	a := loadOrCreateVRFKey(path)
	b := loadOrCreateVRFKey(path)
	if a.D.Cmp(b.D) != 0 {
		t.Fatal("aynı dosya yolundan yüklenen anahtar farklı çıktı — kalıcılık bozuk")
	}
}

// TestVRFSelectHRW_StakeWeighted: seçilen node listede olmalı ve deterministik
// olmalı — proof'ları HAZIR (zaten "toplanmış") üç adaydan hrwWinner ile.
func TestVRFSelectHRW_StakeWeighted(t *testing.T) {
	s := NewSequencer("node-1", time.Second)
	nodes := []NodeInfo{
		{NodeID: "node-a", Stake: 10},
		{NodeID: "node-b", Stake: 20},
		{NodeID: "node-c", Stake: 70},
	}

	// Her aday için gerçek bir VRF proof üret (ayrı, throwaway anahtarlarla)
	// ve doğrulanmış proof toplama adımını simüle ederek s.vrfProofs'a ekle
	// (gerçek akışta bunlar handleIncomingVRFProof ile gelir).
	s.mu.Lock()
	s.vrfProofs = newVRFProofStore()
	for _, n := range nodes {
		cand := NewSequencer(n.NodeID, time.Second)
		proofHex, _ := cand.VRFProof(3)
		s.storeProofLocked(3, n.NodeID, proofHex)
	}
	s.mu.Unlock()

	sel1 := s.vrfSelectHRW(3, nodes)
	sel2 := s.vrfSelectHRW(3, nodes)
	if sel1 != sel2 {
		t.Fatal("vrfSelectHRW deterministik değil")
	}
	found := false
	for _, n := range nodes {
		if n.NodeID == sel1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("seçilen node listede yok: %s", sel1)
	}
}

// TestVRFSelectHRW_Empty: boş liste boş string döner.
func TestVRFSelectHRW_Empty(t *testing.T) {
	s := NewSequencer("node-1", time.Second)
	if got := s.vrfSelectHRW(1, nil); got != "" {
		t.Fatalf("boş liste için boş string beklendi, got=%q", got)
	}
}

// TestVRFSelectHRW_MissingProofsExcludeCandidate: proof'u toplanmamış aday
// yarışa katılamaz — sadece proof'u olanlar arasından seçim yapılmalı.
func TestVRFSelectHRW_MissingProofsExcludeCandidate(t *testing.T) {
	s := NewSequencer("node-1", time.Second)
	nodes := []NodeInfo{
		{NodeID: "node-a", Stake: 10},
		{NodeID: "node-b", Stake: 999999}, // çok yüksek stake ama proof'u YOK
	}

	s.mu.Lock()
	s.vrfProofs = newVRFProofStore()
	candA := NewSequencer("node-a", time.Second)
	proofA, _ := candA.VRFProof(5)
	s.storeProofLocked(5, "node-a", proofA)
	// node-b için KASITLI OLARAK proof eklenmiyor.
	s.mu.Unlock()

	got := s.vrfSelectHRW(5, nodes)
	if got != "node-a" {
		t.Fatalf("proof'u olmayan yüksek-stake'li aday yarışı kazanmamalıydı, got=%q", got)
	}
}

// TestSubmitBatchNotSequencer: sequencer olmayan node batch gönderemez.
func TestSubmitBatchNotSequencer(t *testing.T) {
	s := NewSequencer("node-not-selected", time.Second)
	s.AddNode("node-a", 100)
	s.AddNode("node-b", 100)
	_, err := s.SubmitBatch([]string{"tx1", "tx2"})
	if err == nil {
		t.Fatal("sequencer olmayan node hata vermeliydi")
	}
}

// TestVRFProofExternalVerify: VRFProof çıktısı dışarıdan doğrulanabilir olmalı.
func TestVRFProofExternalVerify(t *testing.T) {
	s := NewSequencer("node-1", time.Second)
	piHex, betaHex := s.VRFProof(11)
	pi, err := hex.DecodeString(piHex)
	if err != nil {
		t.Fatalf("pi decode: %v", err)
	}
	if !ecvrfVerify(&s.vrfKey.PublicKey, epochAlpha(11), pi) {
		t.Fatal("VRFProof çıktısı doğrulanamadı")
	}
	beta := ecvrfProofToHash(s.vrfKey.Curve, pi)
	if hex.EncodeToString(beta) != betaHex {
		t.Fatal("beta tutarsız")
	}
}
