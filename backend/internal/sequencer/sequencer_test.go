package sequencer

import (
	"encoding/binary"
	"encoding/hex"
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
	key := deriveVRFKey("node-1")
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
	key := deriveVRFKey("node-1")
	pi := ecvrfProve(key, epochAlpha(1))
	if ecvrfVerify(&key.PublicKey, epochAlpha(2), pi) {
		t.Fatal("yanlış alpha ile doğrulama geçmemeliydi")
	}
}

// TestECVRFWrongKey: başka node'un anahtarıyla doğrulama başarısız olmalı.
func TestECVRFWrongKey(t *testing.T) {
	k1 := deriveVRFKey("node-1")
	k2 := deriveVRFKey("node-2")
	pi := ecvrfProve(k1, epochAlpha(7))
	if ecvrfVerify(&k2.PublicKey, epochAlpha(7), pi) {
		t.Fatal("yanlış anahtarla doğrulama geçmemeliydi")
	}
}

// TestECVRFTamperedProof: bozulmuş proof reddedilmeli.
func TestECVRFTamperedProof(t *testing.T) {
	key := deriveVRFKey("node-1")
	pi := ecvrfProve(key, epochAlpha(5))
	pi[len(pi)-1] ^= 0xFF // s'i boz
	if ecvrfVerify(&key.PublicKey, epochAlpha(5), pi) {
		t.Fatal("bozuk proof doğrulanmamalıydı")
	}
}

// TestVRFDeterministic: aynı key+alpha aynı beta üretir.
func TestVRFDeterministic(t *testing.T) {
	key := deriveVRFKey("node-1")
	alpha := epochAlpha(99)
	b1 := ecvrfProofToHash(key.Curve, ecvrfProve(key, alpha))
	b2 := ecvrfProofToHash(key.Curve, ecvrfProve(key, alpha))
	if hex.EncodeToString(b1) != hex.EncodeToString(b2) {
		t.Fatal("VRF beta deterministik değil")
	}
}

// TestDeriveVRFKeyStable: aynı NODE_ID aynı anahtarı verir.
func TestDeriveVRFKeyStable(t *testing.T) {
	a := deriveVRFKey("node-x")
	b := deriveVRFKey("node-x")
	if a.D.Cmp(b.D) != 0 {
		t.Fatal("deriveVRFKey deterministik değil")
	}
	c := deriveVRFKey("node-y")
	if a.D.Cmp(c.D) == 0 {
		t.Fatal("farklı NODE_ID aynı anahtarı verdi")
	}
}

// TestVRFSelectStakeWeighted: seçilen node listede olmalı ve deterministik olmalı.
func TestVRFSelectStakeWeighted(t *testing.T) {
	s := NewSequencer("node-1", time.Second)
	nodes := []NodeInfo{
		{NodeID: "node-a", Stake: 10},
		{NodeID: "node-b", Stake: 20},
		{NodeID: "node-c", Stake: 70},
	}
	sel1 := s.vrfSelect(3, nodes)
	sel2 := s.vrfSelect(3, nodes)
	if sel1 != sel2 {
		t.Fatal("vrfSelect deterministik değil")
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

// TestVRFSelectEmpty: boş liste boş string döner.
func TestVRFSelectEmpty(t *testing.T) {
	s := NewSequencer("node-1", time.Second)
	if got := s.vrfSelect(1, nil); got != "" {
		t.Fatalf("boş liste için boş string beklendi, got=%q", got)
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
